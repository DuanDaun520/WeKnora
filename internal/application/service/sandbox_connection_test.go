// Tests for the platform sandbox-connection service (000097). The fixture
// rides the REAL TenantSandboxConfigService over an in-memory config repo, so
// materialize/push/unassign exercise the production cordon and delete guards
// rather than reimplementing them.
package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
)

// --- fakes ---

type fakeSandboxConnectionsRepo struct {
	rows      []*types.SandboxConnectionEntity
	now       func() time.Time
	updateErr error
}

func (f *fakeSandboxConnectionsRepo) Create(
	_ context.Context, e *types.SandboxConnectionEntity,
) error {
	f.rows = append(f.rows, e)
	return nil
}

func (f *fakeSandboxConnectionsRepo) GetByID(
	_ context.Context, id string,
) (*types.SandboxConnectionEntity, error) {
	for _, r := range f.rows {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, nil
}

func (f *fakeSandboxConnectionsRepo) List(
	context.Context,
) ([]*types.SandboxConnectionEntity, error) {
	return f.rows, nil
}

func (f *fakeSandboxConnectionsRepo) Update(
	_ context.Context, e *types.SandboxConnectionEntity,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	for i, r := range f.rows {
		if r.ID == e.ID {
			e.UpdatedAt = f.now()
			f.rows[i] = e
			return nil
		}
	}
	return nil
}

func (f *fakeSandboxConnectionsRepo) SoftDelete(_ context.Context, id string) error {
	kept := f.rows[:0]
	for _, r := range f.rows {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	f.rows = kept
	return nil
}

// fakeConnConfigRepo is a multi-workspace config store: unlike fakeConfigRepo
// (one entity) it holds every tenant's rows so replace-all assignment diffs
// can be exercised.
type fakeConnConfigRepo struct {
	rows      []*types.TenantSandboxConfigEntity
	markErr   error
	updateErr error
}

func (f *fakeConnConfigRepo) Create(
	_ context.Context, e *types.TenantSandboxConfigEntity,
) error {
	f.rows = append(f.rows, e)
	return nil
}

func (f *fakeConnConfigRepo) GetByID(
	_ context.Context, tenantID uint64, id string,
) (*types.TenantSandboxConfigEntity, error) {
	for _, r := range f.rows {
		if r.TenantID == tenantID && r.ID == id {
			return r, nil
		}
	}
	return nil, nil
}

func (f *fakeConnConfigRepo) ListByTenant(
	_ context.Context, tenantID uint64,
) ([]*types.TenantSandboxConfigEntity, error) {
	var out []*types.TenantSandboxConfigEntity
	for _, r := range f.rows {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeConnConfigRepo) ListAll(context.Context) ([]*types.TenantSandboxConfigEntity, error) {
	return f.rows, nil
}

func (f *fakeConnConfigRepo) Update(
	_ context.Context, e *types.TenantSandboxConfigEntity,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	for i, r := range f.rows {
		if r.TenantID == e.TenantID && r.ID == e.ID {
			e.UpdatedAt = time.Now()
			f.rows[i] = e
			return nil
		}
	}
	return nil
}

func (f *fakeConnConfigRepo) SoftDelete(_ context.Context, tenantID uint64, id string) error {
	kept := f.rows[:0]
	for _, r := range f.rows {
		if r.TenantID != tenantID || r.ID != id {
			kept = append(kept, r)
		}
	}
	f.rows = kept
	return nil
}

func (f *fakeConnConfigRepo) SetCordon(
	_ context.Context, tenantID uint64, id string, at time.Time,
) error {
	row, _ := f.GetByID(context.Background(), tenantID, id)
	if row == nil {
		return nil
	}
	if row.CordonedAt != nil && row.CordonedAt.After(at.Add(-types.SandboxCordonLease)) {
		return repository.ErrSandboxConfigCordoned
	}
	row.CordonedAt = &at
	return nil
}

func (f *fakeConnConfigRepo) ClearCordon(
	_ context.Context, tenantID uint64, id string,
) error {
	row, _ := f.GetByID(context.Background(), tenantID, id)
	if row != nil {
		row.CordonedAt = nil
	}
	return nil
}

func (f *fakeConnConfigRepo) ListBySourceConnection(
	_ context.Context, connectionID string,
) ([]*types.TenantSandboxConfigEntity, error) {
	var out []*types.TenantSandboxConfigEntity
	for _, r := range f.rows {
		if r.SourceConnectionID == connectionID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeConnConfigRepo) MarkSourcePushed(
	_ context.Context, tenantID uint64, id, connectionID string, at time.Time,
) error {
	if f.markErr != nil {
		return f.markErr
	}
	row, _ := f.GetByID(context.Background(), tenantID, id)
	if row != nil {
		row.SourceConnectionID = connectionID
		row.SourcePushedAt = &at
	}
	return nil
}

type fakeConnSkills struct {
	byConfig map[string][]*types.TenantSkillEntity
}

func (f *fakeConnSkills) ListSkillsByConfig(
	_ context.Context, _ uint64, configID string,
) ([]*types.TenantSkillEntity, error) {
	return f.byConfig[configID], nil
}

type fakeConnTenants struct {
	names map[uint64]string
}

func (f *fakeConnTenants) GetTenantByID(
	_ context.Context, tenantID uint64,
) (*types.Tenant, error) {
	if name, ok := f.names[tenantID]; ok {
		return &types.Tenant{ID: tenantID, Name: name}, nil
	}
	return nil, nil
}

// --- fixture ---

type sandboxConnectionFixture struct {
	svc       *SandboxConnectionService
	connRepo  *fakeSandboxConnectionsRepo
	cfgRepo   *fakeConnConfigRepo
	configSvc *TenantSandboxConfigService
	client    *stubProviderClient
	skills    *fakeConnSkills
	tenants   *fakeConnTenants
}

func newSandboxConnectionFixture(t *testing.T) *sandboxConnectionFixture {
	t.Helper()
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	clock := struct{ t time.Time }{t: time.Now()}
	now := func() time.Time { return clock.t }
	fx := &sandboxConnectionFixture{
		connRepo: &fakeSandboxConnectionsRepo{now: now},
		cfgRepo:  &fakeConnConfigRepo{},
		client:   &stubProviderClient{},
		skills:   &fakeConnSkills{byConfig: map[string][]*types.TenantSkillEntity{}},
		tenants:  &fakeConnTenants{names: map[uint64]string{7: "alpha", 8: "beta"}},
	}
	fx.configSvc = NewTenantSandboxConfigService(
		fx.cfgRepo, stubAgentRepo{}, testGlobalSandboxConfig(), nil, nil)
	fx.configSvc.newClient = func(*sandbox.Config) (sandbox.ConfigSandboxClient, error) {
		return fx.client, nil
	}
	fx.svc = NewSandboxConnectionService(
		fx.connRepo, fx.cfgRepo, fx.configSvc, fx.skills, fx.tenants)
	fx.svc.now = now
	return fx
}

func (fx *sandboxConnectionFixture) createConnection(
	t *testing.T, name string, cfg *types.TenantSandboxConfig,
) *types.SandboxConnectionEntity {
	t.Helper()
	e, err := fx.svc.Create(context.Background(), CreateSandboxConnectionInput{
		Name: name, Description: "conn desc", Config: cfg,
	})
	require.NoError(t, err)
	return e
}

func (fx *sandboxConnectionFixture) assign(
	t *testing.T, connectionID string, tenantIDs ...uint64,
) []AssignmentOutcome {
	t.Helper()
	results, _, err := fx.svc.ReplaceAssignments(context.Background(), connectionID, tenantIDs)
	require.NoError(t, err)
	return results
}

func (fx *sandboxConnectionFixture) materializedRow(
	t *testing.T, connectionID string, tenantID uint64,
) *types.TenantSandboxConfigEntity {
	t.Helper()
	rows, err := fx.cfgRepo.ListBySourceConnection(context.Background(), connectionID)
	require.NoError(t, err)
	for _, r := range rows {
		if r.TenantID == tenantID {
			return r
		}
	}
	t.Fatalf("no materialized row for connection %s tenant %d", connectionID, tenantID)
	return nil
}

// --- create / update / delete ---

func TestSandboxConnectionCreateValidatesName(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	ctx := context.Background()

	_, err := fx.svc.Create(ctx, CreateSandboxConnectionInput{
		Name: "   ", Config: e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300),
	})
	require.ErrorIs(t, err, ErrSandboxConfigNameRequired)

	_, err = fx.svc.Create(ctx, CreateSandboxConnectionInput{
		Name: types.SandboxWorkspacePolicyConfigName,
		Config: e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300),
	})
	var badRequest *apperrors.AppError
	require.ErrorAs(t, err, &badRequest)
}

func TestSandboxConnectionCreateRejectsDuplicateLiveName(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))

	_, err := fx.svc.Create(context.Background(), CreateSandboxConnectionInput{
		Name: "team-e2b", Config: e2bCfg("key-b", "https://api.e2b.app", "e2b.app", "t2", 300),
	})
	require.ErrorIs(t, err, ErrSandboxConnectionNameConflict)
}

// The shared payload must never carry row-local state: a skill snapshot on the
// connection would leak one workspace's skills into every assignment.
func TestSandboxConnectionCreateStripsRowLocalState(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	cfg := e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300)
	cfg.SkillImage = &types.SkillImageConfig{SnapshotID: "snap-forged"}
	cfg.VolumeMount = &types.VolumeMountConfig{}

	conn := fx.createConnection(t, "team-e2b", cfg)

	require.Nil(t, conn.Config.SkillImage)
	require.Nil(t, conn.Config.VolumeMount)
}

func TestSandboxConnectionUpdateRefreshesUpdatedAt(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	before := conn.UpdatedAt

	time.Sleep(2 * time.Millisecond)
	updated, err := fx.svc.Update(context.Background(), conn.ID, UpdateSandboxConnectionInput{
		Name:        "team-e2b",
		Description: "new desc",
		Config:      e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 600),
	})
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(before), "repo-side bump drives the drift markers")
	require.Equal(t, 600, updated.Config.E2B.E2BSandboxTTLSeconds)
}

func TestDeleteConnectionBlockedWhileAssigned(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)

	err := fx.svc.Delete(context.Background(), conn.ID)
	var assigned *AssignmentsExistError
	require.ErrorAs(t, err, &assigned)
	require.Len(t, assigned.Assignments, 1)
	require.Equal(t, uint64(7), assigned.Assignments[0].TenantID)

	// Unassign first, then delete succeeds.
	fx.assign(t, conn.ID)
	require.NoError(t, fx.svc.Delete(context.Background(), conn.ID))
}

// --- assignment / materialization ---

func TestMaterializeAutoSuffixesConfigName(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))

	// Workspace 7 already has a self-built config with the same name, and a
	// sibling of the suffix the collision will produce next.
	fx.cfgRepo.rows = append(fx.cfgRepo.rows,
		&types.TenantSandboxConfigEntity{ID: "cfg-self", TenantID: 7, Name: "team-e2b"},
		&types.TenantSandboxConfigEntity{ID: "cfg-sibling", TenantID: 7, Name: "team-e2b (2)"},
	)

	fx.assign(t, conn.ID, 7)

	row := fx.materializedRow(t, conn.ID, 7)
	require.Equal(t, "team-e2b (3)", row.Name)
}

// A fresh assignment reads as in-sync: seeded source_pushed_at suppresses the
// drift marker until the connection actually changes.
func TestAssignSeedsPushedAtSoFreshAssignmentDoesNotDrift(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))

	fx.assign(t, conn.ID, 7, 8)

	assignments, err := fx.svc.ListAssignments(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Len(t, assignments, 2)
	for _, a := range assignments {
		require.False(t, a.Drift, "fresh assignment must not read as drifting")
		require.NotNil(t, a.PushedAt)
		require.Equal(t, "team-e2b", a.ConfigName)
	}
}

func TestReplaceAssignmentsReportsPerTenantOutcomes(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))

	results := fx.assign(t, conn.ID, 7, 8)
	require.Len(t, results, 2)
	for _, r := range results {
		require.Equal(t, "assigned", r.Status)
		require.NotEmpty(t, r.ConfigID)
		require.Equal(t, "team-e2b", r.MaterializedName)
		require.Equal(t, "beta", resultsByTenant(results, 8).TenantName)
	}

	results = fx.assign(t, conn.ID, 7)
	require.Len(t, results, 1)
	removed := resultsByTenant(results, 8)
	require.Equal(t, "unassigned", removed.Status)

	assignments, err := fx.svc.ListAssignments(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Len(t, assignments, 1)
	require.Equal(t, uint64(7), assignments[0].TenantID)
}

func resultsByTenant(results []AssignmentOutcome, tenantID uint64) AssignmentOutcome {
	for _, r := range results {
		if r.TenantID == tenantID {
			return r
		}
	}
	return AssignmentOutcome{}
}

// --- unassign guards ---

func TestUnassignBlockedWhileSkillsInstalled(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	row := fx.materializedRow(t, conn.ID, 7)
	fx.skills.byConfig[row.ID] = []*types.TenantSkillEntity{
		{Name: "pdf-tools"}, {Name: "web-scraper"},
	}

	results := fx.assign(t, conn.ID)

	blocked := resultsByTenant(results, 7)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "skills_installed", blocked.Code)
	require.Equal(t, []string{"pdf-tools", "web-scraper"}, blocked.SkillNames)

	// The row survives: the platform surface has no force cascade.
	require.NotNil(t, fx.materializedRow(t, conn.ID, 7))
}

func TestUnassignBlockedByLiveSandboxes(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	fx.client.inventories = [][]sandbox.RemoteSandboxSummary{{{ID: "sbx-1"}}}

	results := fx.assign(t, conn.ID)

	blocked := resultsByTenant(results, 7)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "sandboxes_still_live", blocked.Code)
	require.NotNil(t, blocked.Inventory)
	require.Equal(t, 1, blocked.Inventory.SandboxCount)
	require.NotNil(t, fx.materializedRow(t, conn.ID, 7))
}

// An unreachable provider is never a reason to force: the platform surface
// passes force=false unconditionally.
func TestUnassignBlockedWhenInventoryUnverifiable(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	fx.client.listErr = context.DeadlineExceeded

	results := fx.assign(t, conn.ID)

	blocked := resultsByTenant(results, 7)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "sandbox_inventory_unverifiable", blocked.Code)
	require.NotNil(t, fx.materializedRow(t, conn.ID, 7))
}

// --- push ---

// The row's own name (possibly auto-suffixed against a self-built config) and
// its installer-stamped state are the workspace's; a push only carries the
// shared payload.
func TestPushKeepsRowNameAndRowLocalState(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.cfgRepo.rows = append(fx.cfgRepo.rows,
		&types.TenantSandboxConfigEntity{ID: "cfg-self", TenantID: 7, Name: "team-e2b"})
	fx.assign(t, conn.ID, 7)
	row := fx.materializedRow(t, conn.ID, 7)
	require.Equal(t, "team-e2b (2)", row.Name)
	// What a skill install + volume wiring would have stamped onto the row.
	row.Config.SkillImage = &types.SkillImageConfig{SnapshotID: "snap-1"}
	row.Config.VolumeMount = &types.VolumeMountConfig{VolumeName: "weknora-tenant-7-skills"}

	// Non-identity edit on the connection, then push.
	_, err := fx.svc.Update(context.Background(), conn.ID, UpdateSandboxConnectionInput{
		Name:        "team-e2b",
		Description: "rotated desc",
		Config:      e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 600),
	})
	require.NoError(t, err)

	outcomes, err := fx.svc.Push(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.Equal(t, "updated", outcomes[0].Status)

	updated := fx.materializedRow(t, conn.ID, 7)
	require.Equal(t, "team-e2b (2)", updated.Name, "push must not rename a suffixed row")
	require.NotNil(t, updated.Config.SkillImage)
	require.Equal(t, "snap-1", updated.Config.SkillImage.SnapshotID)
	require.NotNil(t, updated.Config.VolumeMount)
	require.Equal(t, 600, updated.Config.E2B.E2BSandboxTTLSeconds,
		"non-identity fields do propagate")
	require.NotNil(t, updated.SourcePushedAt)
	require.False(t, fx.assignmentsOf(t, conn.ID)[0].Drift)
}

func TestPushBlockedBySkillSnapshotOnIdentityRotation(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	row := fx.materializedRow(t, conn.ID, 7)
	row.Config.SkillImage = &types.SkillImageConfig{SnapshotID: "snap-1"}
	pushedBefore := row.SourcePushedAt

	// Rotate the endpoint: sessions would boot the snapshot against the old
	// environment, so the workspace update flow refuses.
	_, err := fx.svc.Update(context.Background(), conn.ID, UpdateSandboxConnectionInput{
		Name:        "team-e2b",
		Description: "conn desc",
		Config:      e2bCfg("key-a", "https://api-2.e2b.app", "e2b.app", "t1", 300),
	})
	require.NoError(t, err)

	outcomes, err := fx.svc.Push(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Equal(t, "blocked_skill_snapshot", outcomes[0].Status)
	require.Equal(t, "snap-1", row.Config.SkillImage.SnapshotID, "refused push must not touch the row")
	require.Equal(t, pushedBefore, row.SourcePushedAt, "drift marker stays on")
	require.True(t, fx.assignmentsOf(t, conn.ID)[0].Drift)
}

func TestPushBlockedByLiveSandboxes(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)

	_, err := fx.svc.Update(context.Background(), conn.ID, UpdateSandboxConnectionInput{
		Name:        "team-e2b",
		Description: "conn desc",
		Config:      e2bCfg("key-b", "https://api.e2b.app", "e2b.app", "t1", 300),
	})
	require.NoError(t, err)
	fx.client.inventories = [][]sandbox.RemoteSandboxSummary{{{ID: "sbx-1"}}}

	outcomes, err := fx.svc.Push(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Equal(t, "blocked_live_sandboxes", outcomes[0].Status)
	require.Equal(t, "sandboxes_still_live", outcomes[0].Code)
	require.Equal(t, 1, outcomes[0].Inventory.SandboxCount)
}

// A fresh cordon means another request is mid-flight on the row; pushing under
// it would race the sweep.
func TestPushSkipsFreshlyCordonedRow(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	row := fx.materializedRow(t, conn.ID, 7)
	now := fx.svc.now()
	row.CordonedAt = &now

	_, err := fx.svc.Update(context.Background(), conn.ID, UpdateSandboxConnectionInput{
		Name:        "team-e2b",
		Description: "conn desc",
		Config:      e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 600),
	})
	require.NoError(t, err)

	outcomes, err := fx.svc.Push(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Equal(t, "skipped_cordoned", outcomes[0].Status)
	require.Equal(t, 300, row.Config.E2B.E2BSandboxTTLSeconds, "the row must be untouched")
	require.True(t, fx.assignmentsOf(t, conn.ID)[0].Drift)
}

// If the payload lands but the bookkeeping cannot be recorded, the drift
// marker must stay on — a stale "in sync" would hide the next edit too.
func TestPushReportsBookkeepingFailure(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	seeded := fx.materializedRow(t, conn.ID, 7).SourcePushedAt
	fx.cfgRepo.markErr = context.DeadlineExceeded

	outcomes, err := fx.svc.Push(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Equal(t, "error", outcomes[0].Status)
	require.Contains(t, outcomes[0].Message, "push state")
	require.Equal(t, seeded, fx.materializedRow(t, conn.ID, 7).SourcePushedAt,
		"the stamp must not advance, so the drift marker stays on")
}

func (fx *sandboxConnectionFixture) assignmentsOf(
	t *testing.T, connectionID string,
) []SandboxConnectionAssignment {
	t.Helper()
	assignments, err := fx.svc.ListAssignments(context.Background(), connectionID)
	require.NoError(t, err)
	return assignments
}

// --- drift lifecycle ---

func TestDriftLifecycleAroundEditAndPush(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 300))
	fx.assign(t, conn.ID, 7)
	require.False(t, fx.assignmentsOf(t, conn.ID)[0].Drift)

	time.Sleep(2 * time.Millisecond)
	_, err := fx.svc.Update(context.Background(), conn.ID, UpdateSandboxConnectionInput{
		Name:        "team-e2b",
		Description: "conn desc",
		Config:      e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "t1", 600),
	})
	require.NoError(t, err)
	require.True(t, fx.assignmentsOf(t, conn.ID)[0].Drift, "connection edit lights the marker")

	outcomes, err := fx.svc.Push(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Equal(t, "updated", outcomes[0].Status)
	require.False(t, fx.assignmentsOf(t, conn.ID)[0].Drift, "successful push clears it")
}

// --- platform template query ---

func TestPlatformQueryTemplatesReplacePersistsToConnection(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "tpl-old", 300))
	fx.client.templates = []sandbox.RemoteTemplate{
		{ID: "tpl-old", Name: "weknora", Status: "ready", Standard: true},
	}
	fx.client.replaced = &sandbox.RemoteTemplate{ID: "tpl-new", Name: "weknora", Status: "ready", Standard: true}

	result, err := fx.svc.QueryTemplates(context.Background(), PlatformTemplateQueryInput{
		ConnectionID:    conn.ID,
		ReplaceStandard: true,
	})
	require.NoError(t, err)
	require.Equal(t, "tpl-new", result.StandardTemplateID)

	stored, err := fx.connRepo.GetByID(context.Background(), conn.ID)
	require.NoError(t, err)
	require.Equal(t, "tpl-new", stored.Config.E2B.TemplateID,
		"the rebuilt template id must be persisted on the connection")
}

func TestPlatformQueryTemplatesReplaceRefusedWithMaterializedSkills(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	conn := fx.createConnection(t, "team-e2b", e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "tpl-old", 300))
	fx.assign(t, conn.ID, 7)
	row := fx.materializedRow(t, conn.ID, 7)
	row.Config.SkillImage = &types.SkillImageConfig{SnapshotID: "snap-1"}
	fx.client.replaced = &sandbox.RemoteTemplate{ID: "tpl-new", Status: "ready", Standard: true}

	_, err := fx.svc.QueryTemplates(context.Background(), PlatformTemplateQueryInput{
		ConnectionID:    conn.ID,
		ReplaceStandard: true,
	})
	require.ErrorIs(t, err, ErrSkillSnapshotBlocksTemplateChange)
	require.Equal(t, int32(0), fx.client.replaceCalls.Load(),
		"the refusal must happen before any provider rebuild")

	// An in-flight install blocks the rebuild even without a snapshot yet.
	row.Config.SkillImage = nil
	fx.skills.byConfig[row.ID] = []*types.TenantSkillEntity{
		{Name: "pdf-tools", Status: types.SkillStatusInstalling},
	}
	_, err = fx.svc.QueryTemplates(context.Background(), PlatformTemplateQueryInput{
		ConnectionID:    conn.ID,
		ReplaceStandard: true,
	})
	require.ErrorIs(t, err, ErrSkillSnapshotBlocksTemplateChange)
}

func TestPlatformQueryTemplatesReplaceRequiresConnectionID(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	_, err := fx.svc.QueryTemplates(context.Background(), PlatformTemplateQueryInput{
		ReplaceStandard: true,
	})
	var badRequest *apperrors.AppError
	require.ErrorAs(t, err, &badRequest)
}

func TestPlatformQueryTemplatesUnknownConnection(t *testing.T) {
	fx := newSandboxConnectionFixture(t)
	_, err := fx.svc.QueryTemplates(context.Background(), PlatformTemplateQueryInput{
		ConnectionID: "nope",
		Config:       e2bCfg("key-a", "https://api.e2b.app", "e2b.app", "", 300),
	})
	require.ErrorIs(t, err, ErrSandboxConnectionNotFound)
}
