// Package service: platform skill library (000098).
//
// A skill is registered ONCE at platform level (bundle + provenance source)
// and assigned to 0..N workspaces. Assignment MATERIALIZES an ordinary
// tenant_skill_catalog row instead of sharing the platform row: skill installs
// stamp a bundle snapshot into each sandbox config row's own payload, and the
// catalog row is fully owned by the workspace (it may be deleted or
// re-registered locally), so a shared row would leak one workspace's
// definition state into another. Materialization copies the platform zip into
// the workspace's own object storage through the existing register-from-
// archive path, so the replace-pin machinery protecting running installs is
// reused verbatim.
//
// Unlike sandbox connections (000097), where the materialized row IS the
// assignment, skills keep a separate assignment row: a workspace deleting its
// catalog row locally must not block its own delete flow, so the assignment
// survives as an empty shell that push heals by re-materializing:
//
//	assignment list = platform_skill_assignments WHERE skill_id = ?
//	drift           = platform_skills.updated_at > assignments.pushed_at
//
// Skill edits propagate only through the explicit Push action. Installing onto
// sandbox configs stays a per-workspace decision in the existing flow — push
// never triggers an install, and a sandbox running an older version simply
// keeps showing the workspace's own "updatable" chip.
//
// The platform zip lives in the PLATFORM storage namespace (sentinel tenant
// id), so it follows process-wide storage and never a workspace-configured
// backend — the correct scope for bytes no workspace owns.
package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// platformSkillStorageTenantID is the reserved tenant the platform skill
// library stores its zips under. resourceCatalog.Register refuses tenant 0,
// and a real tenant row would drag platform bytes into a workspace-configured
// backend; 1<<62 is unreachable by the small autoincrement tenant sequence and
// resolves to the environment backend (storagebackend.go finds no row).
const platformSkillStorageTenantID uint64 = 1 << 62

// ErrPlatformSkillNotFound is the sentinel behind the platform surface's 404s.
var ErrPlatformSkillNotFound = stderrors.New("platform skill not found")

// ErrPlatformSkillNameConflict marks a create whose name collides with another
// live platform skill (partial unique index, migration 000098).
var ErrPlatformSkillNameConflict = stderrors.New(
	"a live platform skill with this name already exists")

// ErrPlatformSkillNameImmutable refuses an update whose bundle carries a
// different name: skill names are runtime directories and agent-visible
// identities, and renaming would orphan the (tenant, name) mapping every
// materialized row is addressed by. Renaming is a new skill.
var ErrPlatformSkillNameImmutable = stderrors.New(
	"a skill's name cannot change; register the new name as a new skill")

// errPlatformSkillCatalogTaken marks a materialization the workspace blocked
// itself: a live self-built catalog row already holds the skill's name. It is
// an outcome (blocked name_conflict), never an HTTP error — partial success
// is the norm for replace-all assignments and pushes.
var errPlatformSkillCatalogTaken = stderrors.New(
	"workspace already has a self-built skill with this name")

// ErrPlatformSkillCategoryNotFound marks a non-empty category that has no
// registry row (000105). Categories are created FIRST in the category manager
// and the register drawer is no longer creatable, so a miss is API/UI
// staleness — never a silent new category. SKILL.md frontmatter categories
// are the one lenient path: honored only when registered, dropped otherwise.
var ErrPlatformSkillCategoryNotFound = stderrors.New(
	"category is not registered; create it in the category manager first")

// ErrPlatformSkillCategoryExists refuses creating a category (or renaming one)
// onto a name another live category already holds.
var ErrPlatformSkillCategoryExists = stderrors.New(
	"a category with this name already exists")

// PlatformSkillAssignmentsExistError refuses to delete a platform skill that
// still has live assignments. The admin unassignes (guarded) first; there is
// no purge-on-delete because materialized rows are real workspace catalog
// rows that may own installs.
type PlatformSkillAssignmentsExistError struct {
	Assignments []PlatformSkillAssignment
}

func (e *PlatformSkillAssignmentsExistError) Error() string {
	return fmt.Sprintf(
		"platform skill still has %d assigned workspace(s)", len(e.Assignments))
}

// PlatformSkillAssignment is one assignment row as the platform console sees
// it: workspace name, the materialized catalog row's name, how many sandbox
// installs ride on it, and the drift marker.
type PlatformSkillAssignment struct {
	TenantID     uint64     `json:"tenant_id"`
	TenantName   string     `json:"tenant_name"`
	CatalogID    string     `json:"catalog_id,omitempty"`
	CatalogName  string     `json:"catalog_name,omitempty"`
	InstallCount int        `json:"install_count"`
	PushedAt     *time.Time `json:"pushed_at,omitempty"`
	Drift        bool       `json:"drift"`
}

// PlatformSkillAssignmentOutcome is the per-workspace result of a replace-all
// assignment PUT. Partial success is the norm: one blocked workspace must not
// fail the others.
type PlatformSkillAssignmentOutcome struct {
	TenantID   uint64   `json:"tenant_id"`
	TenantName string   `json:"tenant_name"`
	Status     string   `json:"status"` // assigned | unassigned | blocked | error
	Code       string   `json:"code,omitempty"`
	Message    string   `json:"message,omitempty"`
	CatalogID  string   `json:"catalog_id,omitempty"`
	SkillNames []string `json:"skill_names,omitempty"`
}

// PlatformSkillPushOutcome is the per-workspace result of a push.
type PlatformSkillPushOutcome struct {
	TenantID   uint64   `json:"tenant_id"`
	TenantName string   `json:"tenant_name"`
	Status     string   `json:"status"` // updated | healed | blocked | error
	Code       string   `json:"code,omitempty"`
	Message    string   `json:"message,omitempty"`
	CatalogID  string   `json:"catalog_id,omitempty"`
	SkillNames []string `json:"skill_names,omitempty"`
}

// Push outcome statuses beyond the shared vocabulary (sandbox_connection.go).
const (
	pushStatusHealed = "healed"

	assignmentCodeNameConflict = "name_conflict"
)

// PlatformSkillService owns the platform skill lifecycle and the
// materialization into workspaces.
type PlatformSkillService struct {
	skills      repository.PlatformSkillRepository
	assignments repository.PlatformSkillAssignmentRepository

	// tenantSkills reads the materialized catalog rows (by name for the
	// materialize guard, by id for the unassign guard) and stamps provenance.
	tenantSkills repository.TenantSkillRepository

	// catalogSvc supplies RegisterCatalogFromArchive (store + pin machinery)
	// and DeleteCatalog (installs guard + zip drop) that materialize, push and
	// unassign ride on.
	catalogSvc *TenantSkillService

	tenants SandboxConnectionTenantReader // workspace names (000097 narrow interface)

	// resolver addresses the platform storage namespace through the sentinel
	// tenant — never a workspace-configured backend.
	resolver interfaces.StorageBackendResolver

	// sourceHTTP pulls remote skill archives. Nil means the package SSRF-safe
	// default; tests inject httptest clients.
	sourceHTTP *http.Client

	now func() time.Time
}

// NewPlatformSkillService wires the platform skill library service.
func NewPlatformSkillService(
	skills repository.PlatformSkillRepository,
	assignments repository.PlatformSkillAssignmentRepository,
	tenantSkills repository.TenantSkillRepository,
	catalogSvc *TenantSkillService,
	tenants SandboxConnectionTenantReader,
	resolver interfaces.StorageBackendResolver,
) *PlatformSkillService {
	return &PlatformSkillService{
		skills:       skills,
		assignments:  assignments,
		tenantSkills: tenantSkills,
		catalogSvc:   catalogSvc,
		tenants:      tenants,
		resolver:     resolver,
		sourceHTTP:   nil, // skillSourceHTTPClient supplies the SSRF-safe default
		now:          time.Now,
	}
}

// platformFileService resolves storage for the platform namespace. The
// sentinel tenant intentionally has no tenant row, so hydration falls through
// to the environment backend — platform bytes follow process-wide storage.
func (s *PlatformSkillService) platformFileService(
	ctx context.Context,
) (interfaces.FileService, error) {
	if s.resolver == nil {
		return nil, stderrors.New("storage resolver is not configured")
	}
	fs, _, err := s.resolver.ResolveFileService(
		ctx, &types.Tenant{ID: platformSkillStorageTenantID}, "", "", "")
	if err != nil {
		return nil, err
	}
	if fs == nil {
		return nil, stderrors.New("platform file service is not configured")
	}
	return fs, nil
}

// platformSkillObjectKey is the one storage key of a skill's zip. Re-register
// overwrites it in place — same convention as the tenant catalog's
// tenant-skills/catalog/<id>.zip key.
func platformSkillObjectKey(id string) string {
	return fmt.Sprintf("platform-skills/%s.zip", id)
}

// archive reads the skill's zip back from the platform namespace, bounded and
// digest-checked like downloadSkillBundle: a corrupted or torn object must
// fail the push, not poison N workspaces.
func (s *PlatformSkillService) archive(
	ctx context.Context, skill *types.PlatformSkillEntity,
) ([]byte, error) {
	if skill == nil || strings.TrimSpace(skill.BundleRef) == "" {
		return nil, apperrors.NewBadRequestError(
			"the archive of this skill is no longer stored; register it again")
	}
	fs, err := s.platformFileService(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := fs.GetFile(ctx, skill.BundleRef)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	archive, err := io.ReadAll(io.LimitReader(reader, maxSkillBundleTotalBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(archive)) > maxSkillBundleTotalBytes {
		return nil, apperrors.NewBadRequestError("skill bundle exceeds the size limit")
	}
	if want := strings.TrimSpace(skill.BundleSHA256); want != "" &&
		!archiveMatchesSHA(archive, want) {
		return nil, apperrors.NewBadRequestError(
			"the stored archive of this skill does not match its recorded digest")
	}
	return archive, nil
}

// Get returns one platform skill, or ErrPlatformSkillNotFound.
func (s *PlatformSkillService) Get(
	ctx context.Context, id string,
) (*types.PlatformSkillEntity, error) {
	skill, err := s.skills.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, ErrPlatformSkillNotFound
	}
	return skill, nil
}

// List returns the whole platform catalog.
func (s *PlatformSkillService) List(
	ctx context.Context,
) ([]*types.PlatformSkillEntity, error) {
	return s.skills.List(ctx)
}

// PlatformSkillMeta is optional registration metadata threaded into creates.
// Variadic so existing callers stay unchanged; an empty meta registers with
// the SKILL.md frontmatter values alone. ZhName/ZhDescription (000106) have no
// frontmatter counterpart: empty means "no Chinese copy yet", and the console
// falls back to the SKILL.md name/description until one is set.
type PlatformSkillMeta struct {
	Category      string
	Author        string
	ZhName        string
	ZhDescription string
}

// CreateFromArchive registers a new platform skill from an uploaded zip.
// source is provenance only ("upload" or a pasted locator) and is never
// re-fetched.
func (s *PlatformSkillService) CreateFromArchive(
	ctx context.Context, archive []byte, source string, meta ...PlatformSkillMeta,
) (*types.PlatformSkillEntity, error) {
	bundle, err := ParseSkillBundle(archive)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, bundle, archive, source, meta...)
}

// CreateFromSource registers a new platform skill by fetching a public source
// (ClawHub / GitHub / git host / direct zip) through the same normalized
// fetch the workspace install flow uses.
func (s *PlatformSkillService) CreateFromSource(
	ctx context.Context, source string, meta ...PlatformSkillMeta,
) (*types.PlatformSkillEntity, error) {
	bundle, archive, err := fetchNormalizedSkillBundle(ctx, source, s.sourceHTTP)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, bundle, archive, source, meta...)
}

func (s *PlatformSkillService) create(
	ctx context.Context, bundle *SkillBundle, archive []byte, source string, meta ...PlatformSkillMeta,
) (*types.PlatformSkillEntity, error) {
	existing, err := s.skills.GetByName(ctx, bundle.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPlatformSkillNameConflict
	}
	skill := &types.PlatformSkillEntity{
		ID:           uuid.NewString(),
		Name:         bundle.Name,
		Version:      bundle.Version,
		Description:  bundle.Description,
		Instructions: bundle.Instructions,
		BundleSHA256: bundle.SHA256,
		Source:       strings.TrimSpace(source),
	}
	// Definition metadata (000105): a form-provided category must ALREADY be
	// registered — the drawer is not creatable, so a miss is staleness, not a
	// new category. SKILL.md frontmatter only seeds what the form left empty,
	// and then only when that category is registered too: otherwise the skill
	// lands uncategorized rather than quietly minting a registry name nobody
	// created.
	for _, m := range meta {
		if m.Category != "" {
			if err := s.requireRegisteredCategory(ctx, m.Category); err != nil {
				return nil, err
			}
			skill.Category = m.Category
		}
		if m.Author != "" {
			skill.Author = m.Author
		}
		// 000106: Chinese display copy rides the same create; empty means "not
		// provided yet" (SKILL.md has no zh_name/zh_description to fall back to).
		if m.ZhName != "" {
			skill.ZhName = m.ZhName
		}
		if m.ZhDescription != "" {
			skill.ZhDescription = m.ZhDescription
		}
	}
	if skill.Category == "" && bundle.Category != "" {
		if err := s.requireRegisteredCategory(ctx, bundle.Category); err == nil {
			skill.Category = bundle.Category
		} else if !stderrors.Is(err, ErrPlatformSkillCategoryNotFound) {
			return nil, err
		}
	}
	if skill.Author == "" {
		skill.Author = bundle.Author
	}
	if err := s.storeArchive(ctx, skill, archive); err != nil {
		return nil, err
	}
	if err := s.skills.Create(ctx, skill); err != nil {
		// The row never landed, so nothing will ever name this object again.
		s.deletePlatformObjectBestEffort(ctx, skill.BundleRef)
		return nil, err
	}
	logger.Infof(ctx, "[platform-skill] registered %q (%s) from %q",
		skill.Name, skill.ID, skill.Source)
	return skill, nil
}

// storeArchive writes the zip into the platform namespace and stamps the
// entity's BundleRef/BundleSHA256.
func (s *PlatformSkillService) storeArchive(
	ctx context.Context, skill *types.PlatformSkillEntity, archive []byte,
) error {
	fs, err := s.platformFileService(ctx)
	if err != nil {
		return err
	}
	ref, err := fs.SaveBytes(
		ctx, archive, platformSkillStorageTenantID, platformSkillObjectKey(skill.ID), false)
	if err != nil {
		return fmt.Errorf("store platform skill bundle: %w", err)
	}
	skill.BundleRef = ref
	if digest := skillArchiveSHA256(archive); skill.BundleSHA256 == "" {
		skill.BundleSHA256 = digest
	}
	return nil
}

// UpdateFromArchive re-registers a platform skill from an uploaded zip. The
// bundle's name must equal the stored name (names are identities); everything
// else is replaced. The repository's updated_at bump lights the drift markers
// on every assignment; Push clears them.
func (s *PlatformSkillService) UpdateFromArchive(
	ctx context.Context, id string, archive []byte, source string,
) (*types.PlatformSkillEntity, error) {
	bundle, err := ParseSkillBundle(archive)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, id, bundle, archive, source)
}

// UpdateFromSource re-registers a platform skill by fetching a public source.
func (s *PlatformSkillService) UpdateFromSource(
	ctx context.Context, id, source string,
) (*types.PlatformSkillEntity, error) {
	bundle, archive, err := fetchNormalizedSkillBundle(ctx, source, s.sourceHTTP)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, id, bundle, archive, source)
}

func (s *PlatformSkillService) update(
	ctx context.Context, id string, bundle *SkillBundle, archive []byte, source string,
) (*types.PlatformSkillEntity, error) {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if bundle.Name != skill.Name {
		return nil, ErrPlatformSkillNameImmutable
	}
	skill.Version = bundle.Version
	skill.Description = bundle.Description
	skill.Instructions = bundle.Instructions
	skill.BundleSHA256 = bundle.SHA256
	// Category/author stay untouched by a bundle re-register: they are
	// admin-managed metadata (the repository's column list keeps them out)
	// and are edited only through UpdateMeta.
	if trimmed := strings.TrimSpace(source); trimmed != "" {
		skill.Source = trimmed
	}
	if err := s.storeArchive(ctx, skill, archive); err != nil {
		return nil, err
	}
	if err := s.skills.Update(ctx, skill); err != nil {
		return nil, err
	}
	// Re-read so the response carries the DB-side updated_at the drift
	// markers are computed from.
	return s.skills.GetByID(ctx, id)
}

// UpdateMeta rewrites the admin-managed definition metadata: category/author
// plus the 000106 Chinese display fields. A nil pointer keeps the current
// value; a present string (including "") replaces it. The repository's
// updated_at stamp lights drift on every assignment so a push carries the new
// metadata into the materialized workspace rows — unlike update() above, which
// never touches these columns.
func (s *PlatformSkillService) UpdateMeta(
	ctx context.Context, id string, category, author, zhName, zhDescription *string,
) (*types.PlatformSkillEntity, error) {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	nextCategory, nextAuthor := skill.Category, skill.Author
	nextZhName, nextZhDescription := skill.ZhName, skill.ZhDescription
	if category != nil {
		nextCategory = strings.TrimSpace(*category)
	}
	if author != nil {
		nextAuthor = strings.TrimSpace(*author)
	}
	if zhName != nil {
		nextZhName = strings.TrimSpace(*zhName)
	}
	if zhDescription != nil {
		nextZhDescription = strings.TrimSpace(*zhDescription)
	}
	// A non-empty target category must be registered (000105); an explicit ""
	// clears back to uncategorized and always stays allowed.
	if nextCategory != "" {
		if err := s.requireRegisteredCategory(ctx, nextCategory); err != nil {
			return nil, err
		}
	}
	if nextCategory == skill.Category && nextAuthor == skill.Author &&
		nextZhName == skill.ZhName && nextZhDescription == skill.ZhDescription {
		return skill, nil
	}
	if err := s.skills.UpdateMeta(
		ctx, id, nextCategory, nextAuthor, nextZhName, nextZhDescription); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "[platform-skill] updated meta of %s (%q)", id, skill.Name)
	return s.skills.GetByID(ctx, id)
}

// ListCategories returns the category registry with per-name usage counts —
// the console's category-manager directory.
func (s *PlatformSkillService) ListCategories(
	ctx context.Context,
) ([]repository.SkillCategoryCount, error) {
	return s.skills.ListCategories(ctx)
}

// CreateCategory registers one category in the console's category manager.
// Categories are created here — never by registering a skill — and become the
// vocabulary skills reference. A duplicate live name is a conflict.
func (s *PlatformSkillService) CreateCategory(
	ctx context.Context, name string,
) (*types.PlatformSkillCategoryEntity, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperrors.NewBadRequestError("category name is required")
	}
	existing, err := s.skills.GetCategoryByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPlatformSkillCategoryExists
	}
	e := &types.PlatformSkillCategoryEntity{ID: uuid.NewString(), Name: name}
	if err := s.skills.CreateCategory(ctx, e); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "[platform-skill] registered category %q", name)
	return e, nil
}

// requireRegisteredCategory verifies a non-empty category names a live registry
// row. Skill writes (register form, meta edit) call it so no write can mint an
// unregistered category; only SKILL.md frontmatter is allowed to silently drop
// one.
func (s *PlatformSkillService) requireRegisteredCategory(
	ctx context.Context, name string,
) error {
	row, err := s.skills.GetCategoryByName(ctx, name)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrPlatformSkillCategoryNotFound
	}
	return nil
}

// RenameCategory renames one registry category and moves every referencing
// skill onto the new name. The target must be a fresh name — renaming onto an
// existing category would merge two registry rows under one name. The count is
// how many skills (and therefore assignments) went drifting.
func (s *PlatformSkillService) RenameCategory(
	ctx context.Context, from, to string,
) (int64, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" || from == to {
		return 0, apperrors.NewBadRequestError("invalid category rename")
	}
	src, err := s.skills.GetCategoryByName(ctx, from)
	if err != nil {
		return 0, err
	}
	if src == nil {
		return 0, ErrPlatformSkillCategoryNotFound
	}
	dst, err := s.skills.GetCategoryByName(ctx, to)
	if err != nil {
		return 0, err
	}
	if dst != nil {
		return 0, ErrPlatformSkillCategoryExists
	}
	return s.skills.RenameCategory(ctx, from, to)
}

// RemoveCategory deletes one registry category and sends its skills back to
// uncategorized. Lenient about an already-deleted name: clearing orphaned
// strings is still worth doing.
func (s *PlatformSkillService) RemoveCategory(
	ctx context.Context, name string,
) (int64, error) {
	return s.skills.ClearCategory(ctx, strings.TrimSpace(name))
}

// Delete removes a platform skill, but only with no live assignments: each
// one is a real workspace catalog row that may own installs, and unassigning
// is a guarded per-workspace decision (see unassign), not a delete-side
// cascade.
func (s *PlatformSkillService) Delete(ctx context.Context, id string) error {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	rows, err := s.assignments.ListBySkill(ctx, id)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return &PlatformSkillAssignmentsExistError{
			Assignments: s.toAssignments(ctx, skill, rows),
		}
	}
	if err := s.skills.SoftDelete(ctx, id); err != nil {
		return err
	}
	s.deletePlatformObjectBestEffort(ctx, skill.BundleRef)
	return nil
}

func (s *PlatformSkillService) deletePlatformObjectBestEffort(
	ctx context.Context, ref string,
) {
	if ref == "" {
		return
	}
	fs, err := s.platformFileService(ctx)
	if err != nil || fs == nil {
		logger.Warnf(ctx, "[platform-skill] resolve file service to delete %s failed: %v",
			ref, err)
		return
	}
	if err := fs.DeleteFile(ctx, ref); err != nil {
		logger.Warnf(ctx, "[platform-skill] delete object %s failed: %v", ref, err)
	}
}

// ListAssignments returns the skill's assignments as the console sees them:
// workspace names, materialized row names, per-row install counts, drift.
func (s *PlatformSkillService) ListAssignments(
	ctx context.Context, skillID string,
) ([]PlatformSkillAssignment, error) {
	skill, err := s.Get(ctx, skillID)
	if err != nil {
		return nil, err
	}
	rows, err := s.assignments.ListBySkill(ctx, skillID)
	if err != nil {
		return nil, err
	}
	return s.toAssignments(ctx, skill, rows), nil
}

// materializedCatalog resolves the live catalog row an assignment points at.
// The recorded id can be stale (workspace deleted the row); the name lookup
// recovers the current row when it is still ours, and reports nil when the
// workspace deleted it or a self-built skill took the name over.
func (s *PlatformSkillService) materializedCatalog(
	ctx context.Context, skill *types.PlatformSkillEntity, row *types.PlatformSkillAssignmentEntity,
) *types.TenantSkillCatalogEntity {
	if row == nil {
		return nil
	}
	if id := strings.TrimSpace(row.MaterializedCatalogID); id != "" {
		if catalog, err := s.tenantSkills.GetCatalog(ctx, row.TenantID, id); err != nil {
			logger.Warnf(ctx, "[platform-skill] cannot read catalog %s of tenant %d: %v",
				id, row.TenantID, err)
		} else if catalog != nil && catalog.SourcePlatformSkillID == skill.ID {
			return catalog
		}
	}
	if catalog, err := s.tenantSkills.GetCatalogByName(ctx, row.TenantID, skill.Name); err != nil {
		logger.Warnf(ctx, "[platform-skill] cannot resolve catalog by name %q for tenant %d: %v",
			skill.Name, row.TenantID, err)
	} else if catalog != nil && catalog.SourcePlatformSkillID == skill.ID {
		return catalog
	}
	return nil
}

func (s *PlatformSkillService) toAssignments(
	ctx context.Context,
	skill *types.PlatformSkillEntity,
	rows []*types.PlatformSkillAssignmentEntity,
) []PlatformSkillAssignment {
	out := make([]PlatformSkillAssignment, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		assignment := PlatformSkillAssignment{
			TenantID: row.TenantID,
			PushedAt: &row.PushedAt,
			// A fresh assignment seeds PushedAt from the skill's updated_at,
			// so "assigned" never reads as "drifting".
			Drift: skill.UpdatedAt.After(row.PushedAt),
		}
		if name := s.tenantName(ctx, row.TenantID); name != "" {
			assignment.TenantName = name
		}
		if catalog := s.materializedCatalog(ctx, skill, row); catalog != nil {
			assignment.CatalogID = catalog.ID
			assignment.CatalogName = catalog.Name
			if installs, err := s.tenantSkills.ListSkillsByCatalog(
				ctx, row.TenantID, catalog.ID); err != nil {
				logger.Warnf(ctx,
					"[platform-skill] cannot count installs of catalog %s: %v", catalog.ID, err)
			} else {
				assignment.InstallCount = len(installs)
			}
		}
		out = append(out, assignment)
	}
	return out
}

func (s *PlatformSkillService) tenantName(ctx context.Context, tenantID uint64) string {
	if s.tenants == nil {
		return ""
	}
	tenant, err := s.tenants.GetTenantByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return ""
	}
	return tenant.Name
}

// ReplaceAssignments diffs the current assignments against the target
// workspace set: added tenants get a materialized catalog row, removed ones go
// through the guarded unassign. Partial success is the norm — every outcome is
// reported, failures never roll back the other workspaces.
func (s *PlatformSkillService) ReplaceAssignments(
	ctx context.Context, skillID string, tenantIDs []uint64,
) ([]PlatformSkillAssignmentOutcome, []PlatformSkillAssignment, error) {
	skill, err := s.Get(ctx, skillID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.assignments.ListBySkill(ctx, skillID)
	if err != nil {
		return nil, nil, err
	}
	current := make(map[uint64]*types.PlatformSkillAssignmentEntity, len(rows))
	for _, row := range rows {
		if row != nil {
			current[row.TenantID] = row
		}
	}
	target := make(map[uint64]struct{}, len(tenantIDs))
	for _, id := range tenantIDs {
		target[id] = struct{}{}
	}

	results := make([]PlatformSkillAssignmentOutcome, 0, len(tenantIDs)+len(current))
	// Deterministic order: additions then removals, each by tenant id.
	adds := make([]uint64, 0, len(tenantIDs))
	for id := range target {
		if _, exists := current[id]; !exists {
			adds = append(adds, id)
		}
	}
	sort.Slice(adds, func(i, j int) bool { return adds[i] < adds[j] })
	for _, tenantID := range adds {
		results = append(results, s.assignOutcome(ctx, skill, tenantID))
	}

	removals := make([]uint64, 0, len(current))
	for id := range current {
		if _, kept := target[id]; !kept {
			removals = append(removals, id)
		}
	}
	sort.Slice(removals, func(i, j int) bool { return removals[i] < removals[j] })
	for _, tenantID := range removals {
		results = append(results, s.unassignOutcome(ctx, skill, current[tenantID]))
	}

	updated, err := s.ListAssignments(ctx, skillID)
	if err != nil {
		return results, nil, err
	}
	return results, updated, nil
}

func (s *PlatformSkillService) assignOutcome(
	ctx context.Context, skill *types.PlatformSkillEntity, tenantID uint64,
) PlatformSkillAssignmentOutcome {
	outcome := PlatformSkillAssignmentOutcome{TenantID: tenantID, Status: assignmentStatusError}
	if name := s.tenantName(ctx, tenantID); name != "" {
		outcome.TenantName = name
	}
	catalog, err := s.materialize(ctx, skill, tenantID)
	if err != nil {
		outcome.Status, outcome.Code = assignmentOutcomeError(err)
		outcome.Message = err.Error()
		return outcome
	}
	outcome.Status = assignmentStatusAssigned
	outcome.CatalogID = catalog.ID
	return outcome
}

// materialize pushes the skill's bundle into one workspace as an ordinary
// catalog row. A live self-built row holding the name blocks the assignment —
// NO auto-suffixing: the skill name is the runtime directory
// /opt/weknora/tenant/skills/<name> and the agent-visible identity, so
// "name (2)" would change what the skill IS (unlike a sandbox config, whose
// name is only a label).
func (s *PlatformSkillService) materialize(
	ctx context.Context, skill *types.PlatformSkillEntity, tenantID uint64,
) (*types.TenantSkillCatalogEntity, error) {
	existing, err := s.tenantSkills.GetCatalogByName(ctx, tenantID, skill.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.SourcePlatformSkillID != skill.ID {
		return nil, errPlatformSkillCatalogTaken
	}
	archive, err := s.archive(ctx, skill)
	if err != nil {
		return nil, err
	}
	// The platform definition's category/author ride along: they are part of
	// what "assigned this skill" means to a workspace's browser page.
	catalog, err := s.catalogSvc.RegisterCatalogFromArchive(
		ctx, tenantID, archive, CatalogMeta{
			Category: skill.Category,
			Author:   skill.Author,
		})
	if err != nil {
		return nil, err
	}
	// Provenance stamp. The register path's map-updates never touch the
	// column, so this survives every later re-register; a fresh row starts
	// empty and is stamped here.
	if catalog.SourcePlatformSkillID != skill.ID {
		if err := s.tenantSkills.SetCatalogSourcePlatformSkill(
			ctx, tenantID, catalog.ID, skill.ID); err != nil {
			return nil, err
		}
	}
	// The assignment row (idempotent on heal/re-assign) records the link and
	// seeds PushedAt from the skill's updated_at so nothing drifts on arrival.
	if _, err := s.upsertAssignment(ctx, skill, tenantID, catalog.ID); err != nil {
		return nil, err
	}
	logger.Infof(ctx,
		"[platform-skill] materialized catalog %s (%q) for tenant %d from skill %s",
		catalog.ID, catalog.Name, tenantID, skill.ID)
	return catalog, nil
}

func (s *PlatformSkillService) upsertAssignment(
	ctx context.Context, skill *types.PlatformSkillEntity, tenantID uint64, catalogID string,
) (*types.PlatformSkillAssignmentEntity, error) {
	row, err := s.assignments.GetBySkillAndTenant(ctx, skill.ID, tenantID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &types.PlatformSkillAssignmentEntity{
			ID:                    uuid.NewString(),
			TenantID:              tenantID,
			SkillID:               skill.ID,
			MaterializedCatalogID: catalogID,
			PushedAt:              skill.UpdatedAt,
		}
		if err := s.assignments.Create(ctx, row); err != nil {
			return nil, err
		}
		return row, nil
	}
	row.MaterializedCatalogID = catalogID
	row.PushedAt = skill.UpdatedAt
	if err := s.assignments.Update(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *PlatformSkillService) unassignOutcome(
	ctx context.Context, skill *types.PlatformSkillEntity, row *types.PlatformSkillAssignmentEntity,
) PlatformSkillAssignmentOutcome {
	outcome := PlatformSkillAssignmentOutcome{
		TenantID: row.TenantID,
		Status:   assignmentStatusError,
	}
	if name := s.tenantName(ctx, row.TenantID); name != "" {
		outcome.TenantName = name
	}
	if err := s.unassign(ctx, skill, row); err != nil {
		outcome.Status, outcome.Code = assignmentOutcomeError(err)
		outcome.Message = err.Error()
		var skillsErr *SkillsInstalledError
		if stderrors.As(err, &skillsErr) && skillsErr != nil {
			outcome.SkillNames = skillsErr.SkillNames
		}
		return outcome
	}
	outcome.Status = assignmentStatusUnassigned
	return outcome
}

// assignmentOutcomeError maps a materialize/unassign failure onto the outcome
// vocabulary: the two guarded refusals become blocked, everything else error.
func assignmentOutcomeError(err error) (status, code string) {
	switch {
	case isSkillsInstalledError(err):
		return assignmentStatusBlocked, assignmentCodeSkills
	case stderrors.Is(err, errPlatformSkillCatalogTaken):
		return assignmentStatusBlocked, assignmentCodeNameConflict
	default:
		return assignmentStatusError, ""
	}
}

// unassign removes one assignment. The materialized row is deleted through
// the workspace catalog delete flow, but only while it is still ours: a row
// the workspace already deleted needs no cleanup, and a self-built row that
// took the name over is the workspace's to keep. The installed-skills guard
// runs FIRST — the platform surface deliberately has no force cascade; the
// workspace admin retains theirs in the managed panel.
func (s *PlatformSkillService) unassign(
	ctx context.Context, skill *types.PlatformSkillEntity, row *types.PlatformSkillAssignmentEntity,
) error {
	if catalog := s.materializedCatalog(ctx, skill, row); catalog != nil {
		installs, err := s.tenantSkills.ListSkillsByCatalog(ctx, row.TenantID, catalog.ID)
		if err != nil {
			return err
		}
		if len(installs) > 0 {
			names := make([]string, 0, len(installs))
			for _, install := range installs {
				if install != nil {
					names = append(names, install.Name)
				}
			}
			return &SkillsInstalledError{SkillNames: names}
		}
		// DeleteCatalog re-runs the installs guard (race second opinion) and
		// drops the workspace's copy of the zip.
		if err := s.catalogSvc.DeleteCatalog(ctx, row.TenantID, catalog.ID); err != nil {
			return err
		}
	}
	return s.assignments.SoftDelete(ctx, row.ID)
}

// Push propagates the skill's current bundle to every assignment through the
// workspace register path — which pins the replaced archive onto installs
// still serving it. A workspace whose catalog row is gone (locally deleted)
// is HEALED by re-materializing; a self-built takeover of the name blocks.
// pushed_at is bumped only after a row's re-register succeeded, so a failed
// push keeps the drift marker on. Push never installs onto any sandbox: an
// install is a per-workspace decision and an already-installed older version
// keeps showing the workspace's own "updatable" chip.
func (s *PlatformSkillService) Push(
	ctx context.Context, skillID string,
) ([]PlatformSkillPushOutcome, error) {
	skill, err := s.Get(ctx, skillID)
	if err != nil {
		return nil, err
	}
	rows, err := s.assignments.ListBySkill(ctx, skillID)
	if err != nil {
		return nil, err
	}
	results := make([]PlatformSkillPushOutcome, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		results = append(results, s.pushRow(ctx, skill, row))
	}
	return results, nil
}

func (s *PlatformSkillService) pushRow(
	ctx context.Context, skill *types.PlatformSkillEntity, row *types.PlatformSkillAssignmentEntity,
) PlatformSkillPushOutcome {
	outcome := PlatformSkillPushOutcome{
		TenantID: row.TenantID,
		Status:   pushStatusError,
	}
	if name := s.tenantName(ctx, row.TenantID); name != "" {
		outcome.TenantName = name
	}
	catalog := s.materializedCatalog(ctx, skill, row)
	if catalog == nil {
		// Nothing of ours is live under the name. Either the workspace deleted
		// its row (heal by re-materializing) or a self-built skill took the
		// name over (blocked — the same guard materialize applies).
		existing, err := s.tenantSkills.GetCatalogByName(ctx, row.TenantID, skill.Name)
		if err != nil {
			outcome.Message = err.Error()
			return outcome
		}
		if existing != nil {
			outcome.Status = assignmentStatusBlocked
			outcome.Code = assignmentCodeNameConflict
			outcome.Message = errPlatformSkillCatalogTaken.Error()
			return outcome
		}
		healed, err := s.materialize(ctx, skill, row.TenantID)
		if err != nil {
			outcome.Status, outcome.Code = assignmentOutcomeError(err)
			outcome.Message = err.Error()
			return outcome
		}
		outcome.Status = pushStatusHealed
		outcome.CatalogID = healed.ID
		return outcome
	}
	// Row still ours: re-register the current platform bytes through the
	// workspace path (store + pin replaced bundles + same-digest short circuit).
	// Meta rides along like on first materialize — a push is exactly how
	// category/author edits reach the workspace rows.
	archive, err := s.archive(ctx, skill)
	if err != nil {
		outcome.Message = err.Error()
		return outcome
	}
	if _, err := s.catalogSvc.RegisterCatalogFromArchive(
		ctx, row.TenantID, archive, CatalogMeta{
			Category: skill.Category,
			Author:   skill.Author,
		}); err != nil {
		outcome.Status, outcome.Code = assignmentOutcomeError(err)
		outcome.Message = err.Error()
		return outcome
	}
	if _, err := s.upsertAssignment(ctx, skill, row.TenantID, catalog.ID); err != nil {
		// The workspace got the new bundle but the bookkeeping failed; report
		// it so the operator knows the drift marker stays on.
		outcome.Status = pushStatusError
		outcome.Message = "pushed but failed to record push state: " + err.Error()
		return outcome
	}
	outcome.Status = pushStatusUpdated
	outcome.CatalogID = catalog.ID
	return outcome
}

// ListFiles lists the skill's bundle tree for the platform file browser.
func (s *PlatformSkillService) ListFiles(
	ctx context.Context, id string,
) ([]SkillFileEntry, error) {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	archive, err := s.archive(ctx, skill)
	if err != nil {
		return nil, err
	}
	return listSkillZipFiles(archive)
}

// ReadFile reads one file out of the skill's bundle for the platform file
// browser.
func (s *PlatformSkillService) ReadFile(
	ctx context.Context, id, relativePath string,
) (*SkillFileContent, error) {
	clean, err := safeSkillFilePath(relativePath)
	if err != nil {
		return nil, apperrors.NewBadRequestError(err.Error())
	}
	skill, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	archive, err := s.archive(ctx, skill)
	if err != nil {
		return nil, err
	}
	body, err := readSkillZipFile(archive, clean)
	if err != nil {
		if stderrors.Is(err, errSkillFileMissing) {
			return nil, apperrors.NewNotFoundError("skill file not found")
		}
		return nil, err
	}
	return projectSkillFileContent(clean, body), nil
}
