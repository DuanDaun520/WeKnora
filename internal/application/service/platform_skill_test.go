// Tests for the platform skill library service (000098). The fixture rides
// the REAL TenantSkillService register/delete paths over the shared in-memory
// skill repo, so materialize/push/unassign exercise the production store and
// pin machinery rather than reimplementing it.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- fakes ---

type platformTestClock struct{ t time.Time }

func (c *platformTestClock) now() time.Time { return c.t }
func (c *platformTestClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

type fakePlatformSkillsRepo struct {
	rows       []*types.PlatformSkillEntity
	categories []*types.PlatformSkillCategoryEntity
	now        func() time.Time
	createErr  error
	updateErr  error
}

func (f *fakePlatformSkillsRepo) Create(
	_ context.Context, e *types.PlatformSkillEntity,
) error {
	if f.createErr != nil {
		return f.createErr
	}
	cp := *e
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = f.now()
	}
	cp.UpdatedAt = f.now()
	f.rows = append(f.rows, &cp)
	return nil
}

func (f *fakePlatformSkillsRepo) GetByID(
	_ context.Context, id string,
) (*types.PlatformSkillEntity, error) {
	for _, r := range f.rows {
		if r.ID == id {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakePlatformSkillsRepo) GetByName(
	_ context.Context, name string,
) (*types.PlatformSkillEntity, error) {
	for _, r := range f.rows {
		if r.Name == name {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakePlatformSkillsRepo) List(
	context.Context,
) ([]*types.PlatformSkillEntity, error) {
	return f.rows, nil
}

// Update stamps updated_at on the store side, mirroring the real repository's
// DB-time bump that drives the assignment drift markers.
func (f *fakePlatformSkillsRepo) Update(
	_ context.Context, e *types.PlatformSkillEntity,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	for i, r := range f.rows {
		if r.ID == e.ID {
			cp := *e
			cp.UpdatedAt = f.now()
			f.rows[i] = &cp
			return nil
		}
	}
	return nil
}

// UpdateMeta stamps updated_at like Update does — the meta edit is a drift
// driver too. It only rewrites the metadata columns, never the bundle facts.
func (f *fakePlatformSkillsRepo) UpdateMeta(
	_ context.Context, id, category, author, zhName, zhDescription, helpURL string,
) error {
	for i, r := range f.rows {
		if r.ID == id {
			cp := *r
			cp.Category, cp.Author = category, author
			cp.ZhName, cp.ZhDescription = zhName, zhDescription
			cp.HelpURL = helpURL
			cp.UpdatedAt = f.now()
			f.rows[i] = &cp
			return nil
		}
	}
	return nil
}

func (f *fakePlatformSkillsRepo) CreateCategory(
	_ context.Context, e *types.PlatformSkillCategoryEntity,
) error {
	cp := *e
	f.categories = append(f.categories, &cp)
	return nil
}

func (f *fakePlatformSkillsRepo) GetCategoryByName(
	_ context.Context, name string,
) (*types.PlatformSkillCategoryEntity, error) {
	for _, c := range f.categories {
		if c.Name == name {
			cp := *c
			return &cp, nil
		}
	}
	return nil, nil
}

// ListCategories mirrors the real registry LEFT JOIN: every registered category
// (fresh, unused ones included) with the count of live skills referencing it.
func (f *fakePlatformSkillsRepo) ListCategories(
	context.Context,
) ([]repository.SkillCategoryCount, error) {
	counts := map[string]int64{}
	for _, r := range f.rows {
		if r.Category != "" {
			counts[r.Category]++
		}
	}
	names := make([]string, 0, len(f.categories))
	for _, c := range f.categories {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	out := make([]repository.SkillCategoryCount, 0, len(names))
	for _, name := range names {
		out = append(out, repository.SkillCategoryCount{Name: name, Count: counts[name]})
	}
	return out, nil
}

func (f *fakePlatformSkillsRepo) RenameCategory(
	_ context.Context, from, to string,
) (int64, error) {
	var moved int64
	for _, c := range f.categories {
		if c.Name == from {
			c.Name = to
		}
	}
	for i, r := range f.rows {
		if r.Category != from {
			continue
		}
		cp := *r
		cp.Category = to
		cp.UpdatedAt = f.now()
		f.rows[i] = &cp
		moved++
	}
	return moved, nil
}

func (f *fakePlatformSkillsRepo) ClearCategory(
	_ context.Context, name string,
) (int64, error) {
	var cleared int64
	kept := f.categories[:0]
	for _, c := range f.categories {
		if c.Name != name {
			kept = append(kept, c)
		}
	}
	f.categories = kept
	for i, r := range f.rows {
		if r.Category != name {
			continue
		}
		cp := *r
		cp.Category = ""
		cp.UpdatedAt = f.now()
		f.rows[i] = &cp
		cleared++
	}
	return cleared, nil
}

func (f *fakePlatformSkillsRepo) SoftDelete(_ context.Context, id string) error {
	kept := f.rows[:0]
	for _, r := range f.rows {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	f.rows = kept
	return nil
}

type fakePlatformSkillAssignmentsRepo struct {
	rows      []*types.PlatformSkillAssignmentEntity
	createErr error
	updateErr error
}

func (f *fakePlatformSkillAssignmentsRepo) Create(
	_ context.Context, e *types.PlatformSkillAssignmentEntity,
) error {
	if f.createErr != nil {
		return f.createErr
	}
	cp := *e
	f.rows = append(f.rows, &cp)
	return nil
}

func (f *fakePlatformSkillAssignmentsRepo) GetBySkillAndTenant(
	_ context.Context, skillID string, tenantID uint64,
) (*types.PlatformSkillAssignmentEntity, error) {
	for _, r := range f.rows {
		if r.SkillID == skillID && r.TenantID == tenantID {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakePlatformSkillAssignmentsRepo) ListBySkill(
	_ context.Context, skillID string,
) ([]*types.PlatformSkillAssignmentEntity, error) {
	var out []*types.PlatformSkillAssignmentEntity
	for _, r := range f.rows {
		if r.SkillID == skillID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakePlatformSkillAssignmentsRepo) Update(
	_ context.Context, e *types.PlatformSkillAssignmentEntity,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	for i, r := range f.rows {
		if r.ID == e.ID {
			cp := *e
			f.rows[i] = &cp
			return nil
		}
	}
	return nil
}

func (f *fakePlatformSkillAssignmentsRepo) SoftDelete(_ context.Context, id string) error {
	kept := f.rows[:0]
	for _, r := range f.rows {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	f.rows = kept
	return nil
}

// platformFileSave records one SaveBytes so tests can assert WHICH namespace
// an object landed in.
type platformFileSave struct {
	TenantID uint64
	FileName string
	Ref      string
}

// platformFileStore serves both the platform namespace (sentinel tenant) and
// the workspaces' own copies out of one map, like a real backend would.
type platformFileStore struct {
	mu      sync.Mutex
	next    int
	stored  map[string][]byte
	saves   []platformFileSave
	deleted []string
	saveErr error
}

func (s *platformFileStore) CheckConnectivity(context.Context) error { return nil }
func (s *platformFileStore) SaveFile(
	context.Context, *multipart.FileHeader, uint64, string,
) (string, error) {
	return "", nil
}

func (s *platformFileStore) SaveBytes(
	_ context.Context, data []byte, tenantID uint64, fileName string, _ bool,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return "", s.saveErr
	}
	if s.stored == nil {
		s.stored = map[string][]byte{}
	}
	s.next++
	ref := fmt.Sprintf("file://bundle-%d.zip", s.next)
	copied := make([]byte, len(data))
	copy(copied, data)
	s.stored[ref] = copied
	s.saves = append(s.saves, platformFileSave{TenantID: tenantID, FileName: fileName, Ref: ref})
	return ref, nil
}

func (s *platformFileStore) GetFile(_ context.Context, ref string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if data, ok := s.stored[ref]; ok {
		return newPlatformNopCloser(data), nil
	}
	return nil, errors.New("bundle not found")
}

func (s *platformFileStore) GetFileURL(context.Context, string) (string, error) {
	return "", nil
}

func (s *platformFileStore) CopyFile(
	_ context.Context, srcPath string, _ uint64, _ string,
) (string, error) {
	return srcPath, nil
}

func (s *platformFileStore) DeleteFile(_ context.Context, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stored, ref)
	s.deleted = append(s.deleted, ref)
	return nil
}

func (s *platformFileStore) deletedRefs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deleted...)
}

func (s *platformFileStore) saveCalls() []platformFileSave {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]platformFileSave(nil), s.saves...)
}

type platformNopCloser struct {
	r *platformBytesReader
}

func newPlatformNopCloser(data []byte) io.ReadCloser {
	return &platformNopCloser{r: &platformBytesReader{data: data}}
}
func (c *platformNopCloser) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *platformNopCloser) Close() error               { return nil }

type platformBytesReader struct {
	data []byte
	off  int
}

func (r *platformBytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

type platformStorageResolver struct{ store *platformFileStore }

func (r *platformStorageResolver) ResolveFileService(
	context.Context, *types.Tenant, string, string, string,
) (interfaces.FileService, string, error) {
	return r.store, "", nil
}

func (r *platformStorageResolver) ResolveBackend(
	context.Context, *types.Tenant, string, string,
) (*types.StorageBackend, error) {
	return nil, nil
}

// --- fixture ---

type platformSkillFixture struct {
	svc        *PlatformSkillService
	skillRepo  *fakePlatformSkillsRepo
	assignRepo *fakePlatformSkillAssignmentsRepo
	tenantRepo *installSkillRepo
	catalogSvc *TenantSkillService
	files      *platformFileStore
	tenants    *fakeConnTenants
	clock      *platformTestClock
}

func newPlatformSkillFixture(t *testing.T) *platformSkillFixture {
	t.Helper()
	clock := &platformTestClock{t: time.Now()}
	fx := &platformSkillFixture{
		skillRepo:  &fakePlatformSkillsRepo{now: clock.now},
		assignRepo: &fakePlatformSkillAssignmentsRepo{},
		tenantRepo: newInstallSkillRepo(),
		files:      &platformFileStore{},
		tenants:    &fakeConnTenants{names: map[uint64]string{7: "alpha", 8: "beta"}},
		clock:      clock,
	}
	resolver := &platformStorageResolver{store: fx.files}
	fx.catalogSvc = NewTenantSkillService(
		fx.tenantRepo, nil, resolver, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	fx.catalogSvc.now = clock.now
	fx.svc = NewPlatformSkillService(
		fx.skillRepo, fx.assignRepo, fx.tenantRepo, fx.catalogSvc, fx.tenants, resolver)
	fx.svc.now = clock.now
	return fx
}

func platformSkillMD(name, body string) string {
	return "---\nname: " + name + "\ndescription: platform test skill\n---\n\n" + body + "\n"
}

func platformSkillZip(t *testing.T, name, body string) []byte {
	t.Helper()
	return zipBundle(t, map[string]string{
		"SKILL.md":       platformSkillMD(name, body),
		"scripts/run.py": "print('run')\n",
	})
}

func (fx *platformSkillFixture) registerSkill(
	t *testing.T, name, body string,
) *types.PlatformSkillEntity {
	t.Helper()
	skill, err := fx.svc.CreateFromArchive(
		context.Background(), platformSkillZip(t, name, body), "upload")
	require.NoError(t, err)
	return skill
}

// registerCategory pre-registers a category (000105): skills may only reference
// categories that exist in the registry, so tests that assert a category lands
// on a skill must create it first.
func (fx *platformSkillFixture) registerCategory(t *testing.T, name string) {
	t.Helper()
	_, err := fx.svc.CreateCategory(context.Background(), name)
	require.NoError(t, err)
}

func (fx *platformSkillFixture) assign(
	t *testing.T, skillID string, tenantIDs ...uint64,
) []PlatformSkillAssignmentOutcome {
	t.Helper()
	results, _, err := fx.svc.ReplaceAssignments(context.Background(), skillID, tenantIDs)
	require.NoError(t, err)
	return results
}

func (fx *platformSkillFixture) assignmentsOf(
	t *testing.T, skillID string,
) []PlatformSkillAssignment {
	t.Helper()
	assignments, err := fx.svc.ListAssignments(context.Background(), skillID)
	require.NoError(t, err)
	return assignments
}

func (fx *platformSkillFixture) catalogOf(
	t *testing.T, tenantID uint64, name string,
) *types.TenantSkillCatalogEntity {
	t.Helper()
	cat, err := fx.tenantRepo.GetCatalogByName(context.Background(), tenantID, name)
	require.NoError(t, err)
	return cat
}

func platformOutcomeByTenant(
	results []PlatformSkillAssignmentOutcome, tenantID uint64,
) PlatformSkillAssignmentOutcome {
	for _, r := range results {
		if r.TenantID == tenantID {
			return r
		}
	}
	return PlatformSkillAssignmentOutcome{}
}

func platformPushByTenant(
	results []PlatformSkillPushOutcome, tenantID uint64,
) PlatformSkillPushOutcome {
	for _, r := range results {
		if r.TenantID == tenantID {
			return r
		}
	}
	return PlatformSkillPushOutcome{}
}

// --- create / update / delete ---

func TestPlatformSkillCreateRejectsDuplicateLiveName(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	fx.registerSkill(t, "pdf-tools", "body v1")

	_, err := fx.svc.CreateFromArchive(
		context.Background(), platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.ErrorIs(t, err, ErrPlatformSkillNameConflict)
}

// The platform zip lives in the platform namespace, never in a workspace's
// storage: the sentinel tenant id keeps it off every workspace-configured
// backend.
func TestPlatformSkillCreateStoresZipInPlatformNamespace(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")

	saves := fx.files.saveCalls()
	require.Len(t, saves, 1)
	require.Equal(t, platformSkillStorageTenantID, saves[0].TenantID)
	require.Equal(t, platformSkillObjectKey(skill.ID), saves[0].FileName)

	files, err := fx.svc.ListFiles(context.Background(), skill.ID)
	require.NoError(t, err)
	require.Len(t, files, 2)
	content, err := fx.svc.ReadFile(context.Background(), skill.ID, "SKILL.md")
	require.NoError(t, err)
	require.Contains(t, content.Content, "body v1")
}

func TestPlatformSkillUpdateRefusesRenameAndKeepsRow(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	savesBefore := len(fx.files.saveCalls())

	_, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "other-name", "body v2"), "upload")
	require.ErrorIs(t, err, ErrPlatformSkillNameImmutable)

	stored, err := fx.skillRepo.GetByID(context.Background(), skill.ID)
	require.NoError(t, err)
	require.Equal(t, skill.BundleRef, stored.BundleRef)
	require.Equal(t, skill.BundleSHA256, stored.BundleSHA256)
	require.Len(t, fx.files.saveCalls(), savesBefore,
		"a refused rename must not have stored a new zip")
}

func TestPlatformSkillUpdateReplacesBundleAndLightsDrift(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")

	fx.clock.advance(2 * time.Millisecond)
	updated, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(skill.UpdatedAt),
		"the repo-side bump is what the drift markers are computed from")
	require.NotEqual(t, skill.BundleSHA256, updated.BundleSHA256)
}

func TestPlatformSkillDeleteBlockedWhileAssigned(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)

	err := fx.svc.Delete(context.Background(), skill.ID)
	var assigned *PlatformSkillAssignmentsExistError
	require.ErrorAs(t, err, &assigned)
	require.Len(t, assigned.Assignments, 1)
	require.Equal(t, uint64(7), assigned.Assignments[0].TenantID)

	fx.assign(t, skill.ID)
	require.NoError(t, fx.svc.Delete(context.Background(), skill.ID))
	require.Contains(t, fx.files.deletedRefs(), skill.BundleRef,
		"deleting the last live copy of the platform skill drops its zip")
}

// --- assignment / materialization ---

func TestAssignMaterializesCatalogWithProvenanceAndNoDrift(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")

	fx.assign(t, skill.ID, 7, 8)

	for _, tenantID := range []uint64{7, 8} {
		cat := fx.catalogOf(t, tenantID, "pdf-tools")
		require.NotNil(t, cat)
		require.Equal(t, skill.ID, cat.SourcePlatformSkillID)
		require.Equal(t, skill.BundleSHA256, cat.BundleSHA256)
	}
	assignments := fx.assignmentsOf(t, skill.ID)
	require.Len(t, assignments, 2)
	for _, a := range assignments {
		require.False(t, a.Drift, "a fresh assignment must not read as drifting")
		require.Equal(t, "pdf-tools", a.CatalogName)
	}

	// One platform zip + one stored copy per workspace's own object storage.
	saves := fx.files.saveCalls()
	require.Len(t, saves, 3)
	for _, save := range saves[1:] {
		require.Equal(t, uint64(0), save.TenantID&platformSkillStorageTenantID,
			"workspace copies must never land in the platform namespace")
	}
}

func TestAssignBlockedBySelfBuiltName(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	// Workspace 7 already registered its own pdf-tools.
	require.NoError(t, fx.tenantRepo.CreateCatalog(context.Background(),
		&types.TenantSkillCatalogEntity{ID: "cat-self", TenantID: 7, Name: "pdf-tools"}))

	results := fx.assign(t, skill.ID, 7, 8)

	blocked := platformOutcomeByTenant(results, 7)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "name_conflict", blocked.Code)
	require.Equal(t, "assigned", platformOutcomeByTenant(results, 8).Status,
		"one blocked workspace must not fail the other")
	require.Len(t, fx.assignmentsOf(t, skill.ID), 1,
		"the blocked workspace gets no assignment row")
	self := fx.catalogOf(t, 7, "pdf-tools")
	require.Equal(t, "cat-self", self.ID, "the self-built row is untouched")
}

func TestPlatformSkillReplaceAssignmentsReportsPerTenantOutcomes(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")

	results := fx.assign(t, skill.ID, 7, 8)
	require.Len(t, results, 2)
	for _, r := range results {
		require.Equal(t, "assigned", r.Status)
		require.NotEmpty(t, r.CatalogID)
	}
	require.Equal(t, "beta", platformOutcomeByTenant(results, 8).TenantName)

	results = fx.assign(t, skill.ID, 7)
	require.Len(t, results, 1)
	require.Equal(t, "unassigned", platformOutcomeByTenant(results, 8).Status)
	require.Len(t, fx.assignmentsOf(t, skill.ID), 1)
}

// --- unassign guards ---

func TestPlatformSkillUnassignBlockedWhileSkillsInstalled(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	cat := fx.catalogOf(t, 7, "pdf-tools")
	require.NoError(t, fx.tenantRepo.CreateSkill(context.Background(),
		&types.TenantSkillEntity{
			ID: "sk-1", TenantID: 7, SandboxConfigID: "cfg-1", CatalogID: cat.ID,
			Name: "pdf-tools", Status: types.SkillStatusReady, Enabled: true,
		}))

	results := fx.assign(t, skill.ID)

	blocked := platformOutcomeByTenant(results, 7)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "skills_installed", blocked.Code)
	require.Equal(t, []string{"pdf-tools"}, blocked.SkillNames)
	require.NotNil(t, fx.catalogOf(t, 7, "pdf-tools"),
		"the platform surface has no force cascade")
}

func TestUnassignDropsCatalogRowAndTenantZip(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	cat := fx.catalogOf(t, 7, "pdf-tools")
	tenantRef := cat.BundleRef

	results := fx.assign(t, skill.ID)

	require.Equal(t, "unassigned", platformOutcomeByTenant(results, 7).Status)
	gone, err := fx.tenantRepo.GetCatalogByName(context.Background(), 7, "pdf-tools")
	require.NoError(t, err)
	require.Nil(t, gone)
	require.Contains(t, fx.files.deletedRefs(), tenantRef,
		"unassigning drops the workspace's copy of the zip")
	require.NotContains(t, fx.files.deletedRefs(), skill.BundleRef,
		"the platform zip belongs to the skill, not the workspace")
}

// A workspace that deleted its catalog row locally needs no cleanup: the
// assignment was an empty shell by the time the unassign arrived.
func TestUnassignAfterLocalDeleteIsClean(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	cat := fx.catalogOf(t, 7, "pdf-tools")
	require.NoError(t, fx.tenantRepo.DeleteCatalog(context.Background(), 7, cat.ID))

	results := fx.assign(t, skill.ID)

	require.Equal(t, "unassigned", platformOutcomeByTenant(results, 7).Status)
	require.Empty(t, fx.assignmentsOf(t, skill.ID))
}

// A self-built row that took the freed name over is the workspace's to keep.
func TestUnassignSkipsSelfBuiltTakeover(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	cat := fx.catalogOf(t, 7, "pdf-tools")
	require.NoError(t, fx.tenantRepo.DeleteCatalog(context.Background(), 7, cat.ID))
	require.NoError(t, fx.tenantRepo.CreateCatalog(context.Background(),
		&types.TenantSkillCatalogEntity{ID: "cat-new", TenantID: 7, Name: "pdf-tools"}))

	results := fx.assign(t, skill.ID)

	require.Equal(t, "unassigned", platformOutcomeByTenant(results, 7).Status)
	require.NotNil(t, fx.catalogOf(t, 7, "pdf-tools"))
}

// --- push ---

func TestPushUpdatesCatalogClearsDriftAndNeverInstalls(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	require.False(t, fx.assignmentsOf(t, skill.ID)[0].Drift)

	fx.clock.advance(2 * time.Millisecond)
	updated, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)
	require.True(t, fx.assignmentsOf(t, skill.ID)[0].Drift, "the skill edit lights the marker")

	outcomes, err := fx.svc.Push(context.Background(), skill.ID)
	require.NoError(t, err)
	require.Equal(t, "updated", platformPushByTenant(outcomes, 7).Status)
	require.False(t, fx.assignmentsOf(t, skill.ID)[0].Drift, "a successful push clears it")

	cat := fx.catalogOf(t, 7, "pdf-tools")
	require.Equal(t, updated.BundleSHA256, cat.BundleSHA256)
	require.Equal(t, skill.ID, cat.SourcePlatformSkillID,
		"a push keeps the provenance column intact")
	skills, err := fx.tenantRepo.ListSkillsByTenant(context.Background(), 7)
	require.NoError(t, err)
	require.Empty(t, skills, "push must never install onto a sandbox")
}

// If the bundle lands but the bookkeeping cannot be recorded, the drift marker
// must stay on — a stale "in sync" would hide the next edit too.
func TestPushReportsBookkeepingFailureKeepsDrift(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)

	fx.clock.advance(2 * time.Millisecond)
	_, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)
	fx.assignRepo.updateErr = errors.New("database unavailable")

	outcomes, err := fx.svc.Push(context.Background(), skill.ID)
	require.NoError(t, err)
	blocked := platformPushByTenant(outcomes, 7)
	require.Equal(t, "error", blocked.Status)
	require.Contains(t, blocked.Message, "push state")
	require.True(t, fx.assignmentsOf(t, skill.ID)[0].Drift,
		"the stamp must not advance, so the drift marker stays on")
}

func TestPushHealsLocallyDeletedRow(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	firstCat := fx.catalogOf(t, 7, "pdf-tools")
	require.NoError(t, fx.tenantRepo.DeleteCatalog(context.Background(), 7, firstCat.ID))

	fx.clock.advance(2 * time.Millisecond)
	updated, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)

	outcomes, err := fx.svc.Push(context.Background(), skill.ID)
	require.NoError(t, err)
	require.Equal(t, "healed", platformPushByTenant(outcomes, 7).Status)

	healed := fx.catalogOf(t, 7, "pdf-tools")
	require.Equal(t, skill.ID, healed.SourcePlatformSkillID)
	require.Equal(t, updated.BundleSHA256, healed.BundleSHA256)
	require.False(t, fx.assignmentsOf(t, skill.ID)[0].Drift)
}

func TestPushBlockedBySelfBuiltTakeoverKeepsDrift(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	cat := fx.catalogOf(t, 7, "pdf-tools")
	require.NoError(t, fx.tenantRepo.DeleteCatalog(context.Background(), 7, cat.ID))
	require.NoError(t, fx.tenantRepo.CreateCatalog(context.Background(),
		&types.TenantSkillCatalogEntity{ID: "cat-self", TenantID: 7, Name: "pdf-tools"}))

	fx.clock.advance(2 * time.Millisecond)
	_, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)

	outcomes, err := fx.svc.Push(context.Background(), skill.ID)
	require.NoError(t, err)
	blocked := platformPushByTenant(outcomes, 7)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "name_conflict", blocked.Code)
	require.NotNil(t, fx.catalogOf(t, 7, "pdf-tools"))
	require.True(t, fx.assignmentsOf(t, skill.ID)[0].Drift,
		"a blocked push keeps the drift marker on")
}

// The sandbox that installed the pushed-out version goes on running it, so the
// archive it was built from has to outlive the push that supersedes it — the
// workspace register path's pin machinery, reached through the platform push.
func TestPushPinsInstallsToTheReplacedBundle(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	cat := fx.catalogOf(t, 7, "pdf-tools")
	oldRef := cat.BundleRef
	oldSHA := cat.BundleSHA256
	// What a workspace install of v1 looks like: no ref of its own, the
	// definition's digest.
	require.NoError(t, fx.tenantRepo.CreateSkill(context.Background(),
		&types.TenantSkillEntity{
			ID: "sk-1", TenantID: 7, SandboxConfigID: "cfg-1", CatalogID: cat.ID,
			Name: "pdf-tools", BundleSHA256: oldSHA,
			Status: types.SkillStatusReady, Enabled: true,
		}))

	fx.clock.advance(2 * time.Millisecond)
	_, err := fx.svc.UpdateFromArchive(
		context.Background(), skill.ID, platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)

	outcomes, err := fx.svc.Push(context.Background(), skill.ID)
	require.NoError(t, err)
	require.Equal(t, "updated", platformPushByTenant(outcomes, 7).Status)

	pinned, err := fx.tenantRepo.GetSkill(context.Background(), 7, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, oldRef, pinned.BundleRef,
		"the install now names the archive it was built from")
	require.Equal(t, oldSHA, pinned.BundleSHA256)
	require.NotContains(t, fx.files.deletedRefs(), oldRef,
		"an archive a live install was built from must survive the push")
}

// --- definition metadata (000104): category / author ---

// platformSkillMDWithMeta builds a manifest whose frontmatter carries
// author/category, so tests can pin the form-vs-frontmatter precedence.
func platformSkillMDWithMeta(name string) string {
	return "---\nname: " + name + "\ndescription: platform test skill\n" +
		"author: FM Author\ncategory: fm-cat\n---\n\nbody v1\n"
}

// platformSkillZipWithMeta zips a skill whose frontmatter names the given
// category (no author), for frontmatter-category handling tests.
func platformSkillZipWithMeta(t *testing.T, name, category string) []byte {
	t.Helper()
	manifest := "---\nname: " + name + "\ndescription: platform test skill\n" +
		"category: " + category + "\n---\n\nbody v1\n"
	return zipBundle(t, map[string]string{
		"SKILL.md":       manifest,
		"scripts/run.py": "print('run')\n",
	})
}

// Explicit registration metadata wins; whatever it leaves empty falls back to
// the SKILL.md frontmatter values.
func TestPlatformSkillCreateMetaPrecedence(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")
	fx.registerCategory(t, "fm-cat")
	zipWithMeta := func(name string) []byte {
		return zipBundle(t, map[string]string{
			"SKILL.md":       platformSkillMDWithMeta(name),
			"scripts/run.py": "print('run')\n",
		})
	}

	overridden, err := fx.svc.CreateFromArchive(ctx, zipWithMeta("pdf-tools"),
		"upload", PlatformSkillMeta{Category: "docs", Author: "Alice"})
	require.NoError(t, err)
	require.Equal(t, "docs", overridden.Category)
	require.Equal(t, "Alice", overridden.Author)

	fallback, err := fx.svc.CreateFromArchive(ctx, zipWithMeta("doc-tools"), "upload")
	require.NoError(t, err)
	require.Equal(t, "fm-cat", fallback.Category)
	require.Equal(t, "FM Author", fallback.Author)
}

// A form category that is not registered is refused outright (the drawer is not
// creatable), while a SKILL.md frontmatter category nobody registered is
// silently dropped — the skill lands uncategorized instead of minting one.
func TestPlatformSkillUnregisteredCategoryHandling(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")

	_, err := fx.svc.CreateFromArchive(ctx, platformSkillZip(t, "pdf-tools", "v1"),
		"upload", PlatformSkillMeta{Category: "ghost"})
	require.ErrorIs(t, err, ErrPlatformSkillCategoryNotFound)

	_, err = fx.svc.CreateFromArchive(ctx, platformSkillZip(t, "pdf-tools", "v1"),
		"upload", PlatformSkillMeta{Category: "docs"})
	require.NoError(t, err)

	frontmatterOnly, err := fx.svc.CreateFromArchive(ctx,
		platformSkillZipWithMeta(t, "doc-tools", "fm-ghost"), "upload")
	require.NoError(t, err)
	require.Empty(t, frontmatterOnly.Category,
		"an unregistered frontmatter category must not be stored")
}

// A meta edit alone is a drift driver exactly like a bundle edit, and the push
// carries the new category/author into the materialized workspace row — even
// though the bundle bytes never changed.
func TestPlatformSkillMetaEditDrivesDriftAndPushPropagates(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	fx.assign(t, skill.ID, 7)
	require.False(t, fx.assignmentsOf(t, skill.ID)[0].Drift)

	fx.clock.advance(2 * time.Millisecond)
	category, author := "docs", "Alice"
	updated, err := fx.svc.UpdateMeta(ctx, skill.ID, &category, &author, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(skill.UpdatedAt),
		"the repo-side bump is what the drift markers are computed from")
	require.True(t, fx.assignmentsOf(t, skill.ID)[0].Drift,
		"a meta edit lights the marker without any bundle change")

	outcomes, err := fx.svc.Push(ctx, skill.ID)
	require.NoError(t, err)
	require.Equal(t, "updated", platformPushByTenant(outcomes, 7).Status)
	require.False(t, fx.assignmentsOf(t, skill.ID)[0].Drift)

	row := fx.catalogOf(t, 7, "pdf-tools")
	require.Equal(t, "docs", row.Category)
	require.Equal(t, "Alice", row.Author)
	require.Equal(t, skill.BundleSHA256, row.BundleSHA256,
		"a meta push re-registers the same bundle, not a new one")
}

// A meta edit that changes nothing must not fake a drift: the no-op
// short-circuit keeps updated_at where it was.
func TestPlatformSkillMetaNoopKeepsUpdatedAt(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	skill := fx.registerSkill(t, "pdf-tools", "body v1")
	stored, err := fx.skillRepo.GetByID(ctx, skill.ID)
	require.NoError(t, err)

	fx.clock.advance(2 * time.Millisecond)
	sameCategory, sameAuthor := stored.Category, stored.Author
	updated, err := fx.svc.UpdateMeta(ctx, skill.ID, &sameCategory, &sameAuthor, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, stored.UpdatedAt, updated.UpdatedAt,
		"a no-op meta edit must not stamp updated_at")
}

// A bundle re-register never touches the admin-managed metadata, even when the
// new archive ships different frontmatter values.
func TestPlatformSkillBundleReregisterKeepsMeta(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")
	skill, err := fx.svc.CreateFromArchive(ctx,
		platformSkillZip(t, "pdf-tools", "body v1"), "upload",
		PlatformSkillMeta{Category: "docs", Author: "Alice"})
	require.NoError(t, err)

	fx.clock.advance(2 * time.Millisecond)
	updated, err := fx.svc.UpdateFromArchive(ctx, skill.ID,
		platformSkillZip(t, "pdf-tools", "body v2"), "upload")
	require.NoError(t, err)
	require.Equal(t, "docs", updated.Category)
	require.Equal(t, "Alice", updated.Author)
	require.NotEqual(t, skill.BundleSHA256, updated.BundleSHA256,
		"the bundle itself did change")
}

func TestPlatformSkillCategoryDirectoryBulkOps(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")
	fx.registerCategory(t, "office")
	registerCategorized := func(name, category string) {
		t.Helper()
		meta := PlatformSkillMeta{Category: category}
		if category == "" {
			meta = PlatformSkillMeta{}
		}
		_, err := fx.svc.CreateFromArchive(ctx,
			platformSkillZip(t, name, "body v1"), "upload", meta)
		require.NoError(t, err)
	}
	requireCategories := func(want []repository.SkillCategoryCount) {
		t.Helper()
		rows, err := fx.svc.ListCategories(ctx)
		require.NoError(t, err)
		require.Equal(t, want, rows)
	}

	registerCategorized("pdf-tools", "docs")
	registerCategorized("doc-tools", "docs")
	registerCategorized("sheet-tools", "office")
	registerCategorized("misc-tools", "")
	requireCategories([]repository.SkillCategoryCount{
		{Name: "docs", Count: 2}, {Name: "office", Count: 1},
	})

	moved, err := fx.svc.RenameCategory(ctx, "docs", "文档")
	require.NoError(t, err)
	require.EqualValues(t, 2, moved)
	requireCategories([]repository.SkillCategoryCount{
		{Name: "office", Count: 1}, {Name: "文档", Count: 2},
	})

	cleared, err := fx.svc.RemoveCategory(ctx, "office")
	require.NoError(t, err)
	require.EqualValues(t, 1, cleared)
	requireCategories([]repository.SkillCategoryCount{
		{Name: "文档", Count: 2},
	})
}

// The registry owns the vocabulary: creation refuses a duplicate live name,
// rename refuses onto an existing one, and renaming an unknown category is a
// not-found. A freshly created (unused) category still lists with count 0.
func TestPlatformSkillCategoryRegistryValidation(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")
	fx.registerCategory(t, "office")

	_, err := fx.svc.CreateCategory(ctx, "docs")
	require.ErrorIs(t, err, ErrPlatformSkillCategoryExists)

	created, err := fx.svc.CreateCategory(ctx, "未用分类")
	require.NoError(t, err)
	require.Equal(t, "未用分类", created.Name)
	rows, err := fx.svc.ListCategories(ctx)
	require.NoError(t, err)
	require.Equal(t, []repository.SkillCategoryCount{
		{Name: "docs", Count: 0}, {Name: "office", Count: 0}, {Name: "未用分类", Count: 0},
	}, rows)

	_, err = fx.svc.RenameCategory(ctx, "docs", "office")
	require.ErrorIs(t, err, ErrPlatformSkillCategoryExists)

	_, err = fx.svc.RenameCategory(ctx, "ghost", "文档")
	require.ErrorIs(t, err, ErrPlatformSkillCategoryNotFound)
}

// An UpdateMeta that sets an unregistered category is refused too: only an
// explicit "" (clearing to uncategorized) or a registered name passes.
func TestPlatformSkillMetaRejectsUnregisteredCategory(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	fx.registerCategory(t, "docs")
	skill := fx.registerSkill(t, "pdf-tools", "body v1")

	category := "ghost"
	_, err := fx.svc.UpdateMeta(ctx, skill.ID, &category, nil, nil, nil, nil)
	require.ErrorIs(t, err, ErrPlatformSkillCategoryNotFound)

	category = "docs"
	updated, err := fx.svc.UpdateMeta(ctx, skill.ID, &category, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "docs", updated.Category)

	empty := ""
	updated, err = fx.svc.UpdateMeta(ctx, skill.ID, &empty, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, updated.Category)
}

// The 000106 Chinese display fields ride the create meta and the meta endpoint
// like category/author: set on create, editable (and clearable) through
// UpdateMeta, untouched by a bundle re-register.
func TestPlatformSkillZhMetaRoundtrip(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()

	created, err := fx.svc.CreateFromArchive(ctx,
		platformSkillZip(t, "pdf-tools", "v1"), "upload",
		PlatformSkillMeta{ZhName: "PDF 工具", ZhDescription: "处理 PDF 的技能"})
	require.NoError(t, err)
	require.Equal(t, "PDF 工具", created.ZhName)
	require.Equal(t, "处理 PDF 的技能", created.ZhDescription)

	zhName, zhDesc := "中文名已改", "更长的中文描述"
	updated, err := fx.svc.UpdateMeta(ctx, created.ID, nil, nil, &zhName, &zhDesc, nil)
	require.NoError(t, err)
	require.Equal(t, zhName, updated.ZhName)
	require.Equal(t, zhDesc, updated.ZhDescription)

	empty := ""
	cleared, err := fx.svc.UpdateMeta(ctx, created.ID, nil, nil, &empty, &empty, nil)
	require.NoError(t, err)
	require.Empty(t, cleared.ZhName)
	require.Empty(t, cleared.ZhDescription)

	// A bundle re-register never clobbers the Chinese copy.
	zhName = "保留"
	noop, err := fx.svc.UpdateMeta(ctx, created.ID, nil, nil, &zhName, nil, nil)
	require.NoError(t, err)
	rebundled, err := fx.svc.UpdateFromArchive(ctx, created.ID,
		platformSkillZip(t, "pdf-tools", "v2"), "upload")
	require.NoError(t, err)
	require.Equal(t, "保留", rebundled.ZhName)
	require.NotEqual(t, noop.BundleSHA256, rebundled.BundleSHA256)
}

// 000109：介绍与帮助网址 —— create 可带，meta PUT 可改/可清/校验 http(s)，
// 且与 zh 元数据一样不会被 bundle 重注册覆盖。
func TestPlatformSkillHelpURLMeta(t *testing.T) {
	fx := newPlatformSkillFixture(t)
	ctx := context.Background()
	created, err := fx.svc.CreateFromArchive(ctx,
		platformSkillZip(t, "helped-skill", "v1"), "upload",
		PlatformSkillMeta{HelpURL: "https://docs.example.com/skills"})
	require.NoError(t, err)
	require.Equal(t, "https://docs.example.com/skills", created.HelpURL)

	next := "https://wiki.example.org/helped"
	updated, err := fx.svc.UpdateMeta(ctx, created.ID, nil, nil, nil, nil, &next)
	require.NoError(t, err)
	require.Equal(t, next, updated.HelpURL)

	// 非 http(s)（含 javascript:/data:）一律拒绝。
	bad := "javascript:alert(1)"
	_, err = fx.svc.UpdateMeta(ctx, created.ID, nil, nil, nil, nil, &bad)
	require.Error(t, err)
	noHost := "notaurl"
	_, err = fx.svc.UpdateMeta(ctx, created.ID, nil, nil, nil, nil, &noHost)
	require.Error(t, err)

	// 显式空串清除。
	empty := ""
	cleared, err := fx.svc.UpdateMeta(ctx, created.ID, nil, nil, nil, nil, &empty)
	require.NoError(t, err)
	require.Empty(t, cleared.HelpURL)

	// bundle 重注册不触碰 help_url。
	set := "https://docs.example.com/keep"
	_, err = fx.svc.UpdateMeta(ctx, created.ID, nil, nil, nil, nil, &set)
	require.NoError(t, err)
	rebundled, err := fx.svc.UpdateFromArchive(ctx, created.ID,
		platformSkillZip(t, "helped-skill", "v2"), "upload")
	require.NoError(t, err)
	require.Equal(t, set, rebundled.HelpURL)
}
