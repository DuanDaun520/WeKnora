// Package service: platform sandbox connections (000097).
//
// A sandbox connection is configured ONCE at platform level (provider
// endpoints, credentials, runtime defaults) and assigned to 0..N workspaces.
// Assignment MATERIALIZES an ordinary tenant_sandbox_configs row instead of
// sharing the platform row: skill installs stamp a snapshot into each config
// row's own payload (switchImagePointer) and volume names are per-tenant, so
// one shared row would leak a workspace's skills and volumes into another.
// The materialized row IS the assignment:
//
//	assignment list = tenant_sandbox_configs WHERE source_connection_id = ?
//	drift           = connection.updated_at > row.source_pushed_at
//
// Connection edits propagate only through the explicit Push action, which
// reuses the workspace Update cordon flow per row so live sandboxes are never
// stranded by a credential rotation. Workspace admins keep full control of
// their materialized rows — they are ordinary configs and may diverge.
package service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// ErrSandboxConnectionNotFound is the sentinel behind the platform surface's
// 404s.
var ErrSandboxConnectionNotFound = stderrors.New("sandbox connection not found")

// ErrSandboxConnectionNameConflict marks a create/update whose name collides
// with another live connection (partial unique index, migration 000097).
var ErrSandboxConnectionNameConflict = stderrors.New(
	"a live sandbox connection with this name already exists")

// SkillsInstalledError refuses an unassign while the materialized config has
// installed skills: unassigning deletes the row, and the platform surface has
// no force path — the workspace admin retains theirs in the managed panel.
type SkillsInstalledError struct {
	SkillNames []string
}

func (e *SkillsInstalledError) Error() string {
	return fmt.Sprintf("sandbox config has installed skills: %s", strings.Join(e.SkillNames, ", "))
}

// AssignmentsExistError refuses to delete a connection that still has live
// materialized rows. Unlike MCP there is no purge-on-delete: these
// "assignments" are real workspace configs that may own skills and sandboxes.
// The admin unassigns (guarded) first.
type AssignmentsExistError struct {
	Assignments []SandboxConnectionAssignment
}

func (e *AssignmentsExistError) Error() string {
	return fmt.Sprintf("sandbox connection still has %d assigned workspace(s)", len(e.Assignments))
}

// sandboxConnectionSkillStore is the skill slice the platform service needs:
// the unassign guard and the per-row skill counts. Satisfied by
// repository.TenantSkillRepository.
type sandboxConnectionSkillStore interface {
	ListSkillsByConfig(
		ctx context.Context, tenantID uint64, configID string,
	) ([]*types.TenantSkillEntity, error)
}

// SandboxConnectionTenantReader hydrates workspace names for assignment
// listings and outcomes. Satisfied by interfaces.TenantService.
type SandboxConnectionTenantReader interface {
	GetTenantByID(ctx context.Context, tenantID uint64) (*types.Tenant, error)
}

// SandboxConnectionAssignment is one materialized row as the platform console
// sees it.
type SandboxConnectionAssignment struct {
	TenantID   uint64     `json:"tenant_id"`
	TenantName string     `json:"tenant_name"`
	ConfigID   string     `json:"config_id"`
	ConfigName string     `json:"config_name"`
	PushedAt   *time.Time `json:"pushed_at,omitempty"`
	Drift      bool       `json:"drift"`
	SkillCount int        `json:"skill_count"`
}

// AssignmentOutcome is the per-workspace result of a replace-all assignment
// PUT. Partial success is the norm: one blocked workspace must not fail the
// others.
type AssignmentOutcome struct {
	TenantID         uint64            `json:"tenant_id"`
	TenantName       string            `json:"tenant_name"`
	Status           string            `json:"status"` // assigned | unassigned | blocked | error
	Code             string            `json:"code,omitempty"`
	Message          string            `json:"message,omitempty"`
	ConfigID         string            `json:"config_id,omitempty"`
	MaterializedName string            `json:"materialized_name,omitempty"`
	SkillNames       []string          `json:"skill_names,omitempty"`
	Inventory        *SandboxInventory `json:"inventory,omitempty"`
}

// PushOutcome is the per-workspace result of a push.
type PushOutcome struct {
	TenantID   uint64            `json:"tenant_id"`
	TenantName string            `json:"tenant_name"`
	ConfigID   string            `json:"config_id"`
	Status     string            `json:"status"` // updated | blocked_live_sandboxes | blocked_skill_snapshot | skipped_cordoned | error
	Code       string            `json:"code,omitempty"`
	Message    string            `json:"message,omitempty"`
	Inventory  *SandboxInventory `json:"inventory,omitempty"`
}

// CreateSandboxConnectionInput is the platform create payload. Config carries
// the same masked-secret placeholders the workspace drawer uses.
type CreateSandboxConnectionInput struct {
	Name        string
	Description string
	Config      *types.TenantSandboxConfig
}

// UpdateSandboxConnectionInput is the platform update payload.
type UpdateSandboxConnectionInput struct {
	Name        string
	Description string
	Config      *types.TenantSandboxConfig
}

// PlatformTemplateQueryInput describes the platform connection drawer's
// template query. ConnectionID is optional and lets masked credentials resolve
// against the saved connection while editing; replace_standard requires it.
type PlatformTemplateQueryInput struct {
	Config          *types.TenantSandboxConfig
	ConnectionID    string
	EnsureStandard  bool
	ReplaceStandard bool
}

// Assignment / push outcome status values (handler and tests match on these).
const (
	assignmentStatusAssigned   = "assigned"
	assignmentStatusUnassigned = "unassigned"
	assignmentStatusBlocked    = "blocked"
	assignmentStatusError      = "error"

	pushStatusUpdated          = "updated"
	pushStatusBlockedLive      = "blocked_live_sandboxes"
	pushStatusBlockedSkillSnap = "blocked_skill_snapshot"
	pushStatusSkippedCordoned  = "skipped_cordoned"
	pushStatusError            = "error"

	assignmentCodeSkills = "skills_installed"
)

// SandboxConnectionService owns the platform connection lifecycle and the
// materialization into workspaces.
type SandboxConnectionService struct {
	connections repository.SandboxConnectionRepository
	configs     repository.TenantSandboxConfigRepository

	// configSvc supplies the workspace Update (cordon flow) and Delete
	// (inventory guards) that materialize/push/unassign ride on, plus the
	// shared template-query core.
	configSvc *TenantSandboxConfigService

	skills  sandboxConnectionSkillStore
	tenants SandboxConnectionTenantReader
	now     func() time.Time
}

// NewSandboxConnectionService wires the platform connection service.
func NewSandboxConnectionService(
	connections repository.SandboxConnectionRepository,
	configs repository.TenantSandboxConfigRepository,
	configSvc *TenantSandboxConfigService,
	skills sandboxConnectionSkillStore,
	tenants SandboxConnectionTenantReader,
) *SandboxConnectionService {
	return &SandboxConnectionService{
		connections: connections,
		configs:     configs,
		configSvc:   configSvc,
		skills:      skills,
		tenants:     tenants,
		now:         time.Now,
	}
}

// sanitizeConnectionPayload runs the shared sandbox-config sanitize path and
// then strips the row-local fields a shared connection must never carry:
// SkillImage (stamped per materialized config by the skill installer) and
// VolumeMount (per-tenant volume names).
func sanitizeConnectionPayload(
	incoming, existing *types.TenantSandboxConfig,
) (*types.TenantSandboxConfig, error) {
	merged, err := SanitizeSandboxConfig(incoming, existing)
	if err != nil {
		return nil, err
	}
	if err := validateNamedSandboxBackend(merged); err != nil {
		return nil, err
	}
	if merged != nil {
		merged.SkillImage = nil
		merged.VolumeMount = nil
	}
	return merged, nil
}

// deepCopySandboxConfig isolates one row's payload from another's: the
// connection and every materialized config must never share a Config pointer,
// or an in-memory mutation during one write would bleed into all of them.
// TenantSandboxConfig is a plain JSON-marshalable struct, so a round trip is
// the copy.
func deepCopySandboxConfig(cfg *types.TenantSandboxConfig) *types.TenantSandboxConfig {
	if cfg == nil {
		return nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		// Unreachable for this struct; fail loudly rather than share silently.
		panic(fmt.Sprintf("sandbox: cannot deep-copy config payload: %v", err))
	}
	var out types.TenantSandboxConfig
	if err := json.Unmarshal(data, &out); err != nil {
		panic(fmt.Sprintf("sandbox: cannot deep-copy config payload: %v", err))
	}
	return &out
}

func connectionNameValid(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrSandboxConfigNameRequired
	}
	if strings.TrimSpace(name) == types.SandboxWorkspacePolicyConfigName {
		return apperrors.NewBadRequestError("this name is reserved")
	}
	return nil
}

func (s *SandboxConnectionService) rejectDuplicateName(
	ctx context.Context, name, excludeID string,
) error {
	list, err := s.connections.List(ctx)
	if err != nil {
		return err
	}
	for _, e := range list {
		if e == nil || e.ID == excludeID {
			continue
		}
		if e.Name == name {
			return ErrSandboxConnectionNameConflict
		}
	}
	return nil
}

// Create stores a new platform connection. It starts unassigned; assigning is
// a separate replace-all action.
func (s *SandboxConnectionService) Create(
	ctx context.Context, in CreateSandboxConnectionInput,
) (*types.SandboxConnectionEntity, error) {
	if err := connectionNameValid(in.Name); err != nil {
		return nil, err
	}
	merged, err := sanitizeConnectionPayload(in.Config, nil)
	if err != nil {
		return nil, err
	}
	if err := s.rejectDuplicateName(ctx, strings.TrimSpace(in.Name), ""); err != nil {
		return nil, err
	}
	entity := &types.SandboxConnectionEntity{
		ID:          uuid.New().String(),
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
		Config:      merged,
	}
	if merged != nil {
		entity.SandboxType = merged.SandboxType
	}
	if err := s.connections.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get returns one connection, or ErrSandboxConnectionNotFound.
func (s *SandboxConnectionService) Get(
	ctx context.Context, id string,
) (*types.SandboxConnectionEntity, error) {
	entity, err := s.connections.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, ErrSandboxConnectionNotFound
	}
	return entity, nil
}

// List returns the whole platform catalog.
func (s *SandboxConnectionService) List(
	ctx context.Context,
) ([]*types.SandboxConnectionEntity, error) {
	return s.connections.List(ctx)
}

// Update edits a connection. The repository bumps updated_at, which lights
// the drift markers on stale materialized rows; Push clears them.
func (s *SandboxConnectionService) Update(
	ctx context.Context, id string, in UpdateSandboxConnectionInput,
) (*types.SandboxConnectionEntity, error) {
	entity, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := connectionNameValid(in.Name); err != nil {
		return nil, err
	}
	merged, err := sanitizeConnectionPayload(in.Config, entity.Config)
	if err != nil {
		return nil, err
	}
	if err := s.rejectDuplicateName(ctx, strings.TrimSpace(in.Name), id); err != nil {
		return nil, err
	}
	entity.Name = strings.TrimSpace(in.Name)
	entity.Description = in.Description
	entity.Config = merged
	if merged != nil {
		entity.SandboxType = merged.SandboxType
	}
	if err := s.connections.Update(ctx, entity); err != nil {
		return nil, err
	}
	// Re-read so the response carries the DB-side updated_at the drift
	// markers are computed from.
	return s.connections.GetByID(ctx, id)
}

// Delete removes a connection, but only with no live materialized rows: they
// may own skills and sandboxes, and unassigning them is a guarded per-workspace
// decision (see unassign), not something a platform delete should cascade.
func (s *SandboxConnectionService) Delete(ctx context.Context, id string) error {
	conn, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	rows, err := s.configs.ListBySourceConnection(ctx, id)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return &AssignmentsExistError{Assignments: s.toAssignments(ctx, conn, rows)}
	}
	return s.connections.SoftDelete(ctx, id)
}

// ListAssignments returns the connection's materialized rows as the console
// sees them: workspace names, per-row skill counts, drift markers.
func (s *SandboxConnectionService) ListAssignments(
	ctx context.Context, connectionID string,
) ([]SandboxConnectionAssignment, error) {
	conn, err := s.Get(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.configs.ListBySourceConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	return s.toAssignments(ctx, conn, rows), nil
}

func (s *SandboxConnectionService) toAssignments(
	ctx context.Context,
	conn *types.SandboxConnectionEntity,
	rows []*types.TenantSandboxConfigEntity,
) []SandboxConnectionAssignment {
	out := make([]SandboxConnectionAssignment, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		assignment := SandboxConnectionAssignment{
			TenantID: row.TenantID,
			// A fresh assignment seeds SourcePushedAt from the connection's
			// updated_at, so "assigned" never reads as "drifting".
			PushedAt:   row.SourcePushedAt,
			ConfigID:   row.ID,
			ConfigName: row.Name,
		}
		if row.SourcePushedAt == nil || conn.UpdatedAt.After(*row.SourcePushedAt) {
			assignment.Drift = true
		}
		if tenant := s.tenantName(ctx, row.TenantID); tenant != "" {
			assignment.TenantName = tenant
		}
		if skills, err := s.skills.ListSkillsByConfig(ctx, row.TenantID, row.ID); err != nil {
			logger.Warnf(ctx, "[sandbox-connection] cannot count skills of config %s: %v",
				row.ID, err)
		} else {
			assignment.SkillCount = len(skills)
		}
		out = append(out, assignment)
	}
	return out
}

func (s *SandboxConnectionService) tenantName(ctx context.Context, tenantID uint64) string {
	if s.tenants == nil {
		return ""
	}
	tenant, err := s.tenants.GetTenantByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return ""
	}
	return tenant.Name
}

// ReplaceAssignments diffs the current materialized rows against the target
// workspace set: added tenants get a materialized config, removed ones go
// through the guarded unassign. Partial success is the norm — every outcome is
// reported, failures never roll back the other workspaces.
func (s *SandboxConnectionService) ReplaceAssignments(
	ctx context.Context, connectionID string, tenantIDs []uint64,
) ([]AssignmentOutcome, []SandboxConnectionAssignment, error) {
	conn, err := s.Get(ctx, connectionID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.configs.ListBySourceConnection(ctx, connectionID)
	if err != nil {
		return nil, nil, err
	}
	current := make(map[uint64]*types.TenantSandboxConfigEntity, len(rows))
	for _, row := range rows {
		if row != nil {
			current[row.TenantID] = row
		}
	}
	target := make(map[uint64]struct{}, len(tenantIDs))
	for _, id := range tenantIDs {
		target[id] = struct{}{}
	}

	results := make([]AssignmentOutcome, 0, len(tenantIDs)+len(current))
	// Deterministic order: additions then removals, each by tenant id.
	adds := make([]uint64, 0, len(tenantIDs))
	for id := range target {
		if _, exists := current[id]; !exists {
			adds = append(adds, id)
		}
	}
	sort.Slice(adds, func(i, j int) bool { return adds[i] < adds[j] })
	for _, tenantID := range adds {
		results = append(results, s.assign(ctx, conn, tenantID))
	}

	removals := make([]uint64, 0, len(current))
	for id := range current {
		if _, kept := target[id]; !kept {
			removals = append(removals, id)
		}
	}
	sort.Slice(removals, func(i, j int) bool { return removals[i] < removals[j] })
	for _, tenantID := range removals {
		results = append(results, s.unassignOutcome(ctx, current[tenantID]))
	}

	updated, err := s.ListAssignments(ctx, connectionID)
	if err != nil {
		return results, nil, err
	}
	return results, updated, nil
}

func (s *SandboxConnectionService) assign(
	ctx context.Context, conn *types.SandboxConnectionEntity, tenantID uint64,
) AssignmentOutcome {
	outcome := AssignmentOutcome{TenantID: tenantID, Status: assignmentStatusError}
	if name := s.tenantName(ctx, tenantID); name != "" {
		outcome.TenantName = name
	}
	entity, err := s.materialize(ctx, conn, tenantID)
	if err != nil {
		outcome.Message = err.Error()
		return outcome
	}
	outcome.Status = assignmentStatusAssigned
	outcome.ConfigID = entity.ID
	outcome.MaterializedName = entity.Name
	return outcome
}

// materializedName picks the config name for a materialized row: the
// connection's name, auto-suffixed " (2)", " (3)"… when the workspace already
// has a live config with that name (partial unique index on (tenant_id, name),
// migration 000082). A replace-all assignment must not fail wholesale because
// one workspace happens to have a same-named self-built config.
func materializedName(taken map[string]struct{}, base string) string {
	name := strings.TrimSpace(base)
	if _, exists := taken[name]; !exists {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)", name, i)
		if _, exists := taken[candidate]; !exists {
			return candidate
		}
	}
}

func (s *SandboxConnectionService) materialize(
	ctx context.Context, conn *types.SandboxConnectionEntity, tenantID uint64,
) (*types.TenantSandboxConfigEntity, error) {
	list, err := s.configs.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]struct{}, len(list)+1)
	for _, e := range list {
		if e != nil {
			taken[e.Name] = struct{}{}
		}
	}
	// Never share the connection's payload pointer with a workspace row.
	payload := deepCopySandboxConfig(conn.Config)
	if payload != nil {
		// Defense in depth with sanitizeConnectionPayload: row-local state
		// must not ride into a workspace.
		payload.SkillImage = nil
		payload.VolumeMount = nil
	}
	pushedAt := conn.UpdatedAt
	entity := &types.TenantSandboxConfigEntity{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		Name:               materializedName(taken, conn.Name),
		Description:        conn.Description,
		SandboxType:        conn.SandboxType,
		Config:             payload,
		SourceConnectionID: conn.ID,
		SourcePushedAt:     &pushedAt,
	}
	if err := s.configs.Create(ctx, entity); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "[sandbox-connection] materialized config %s (%q) for tenant %d from connection %s",
		entity.ID, entity.Name, tenantID, conn.ID)
	return entity, nil
}

func (s *SandboxConnectionService) unassignOutcome(
	ctx context.Context, row *types.TenantSandboxConfigEntity,
) AssignmentOutcome {
	outcome := AssignmentOutcome{
		TenantID: row.TenantID,
		ConfigID: row.ID,
		Status:   assignmentStatusError,
	}
	if name := s.tenantName(ctx, row.TenantID); name != "" {
		outcome.TenantName = name
	}
	if err := s.unassign(ctx, row); err != nil {
		switch {
		case isSkillsInstalledError(err):
			var skillsErr *SkillsInstalledError
			stderrors.As(err, &skillsErr)
			outcome.Status = assignmentStatusBlocked
			outcome.Code = assignmentCodeSkills
			outcome.Message = err.Error()
			if skillsErr != nil {
				outcome.SkillNames = skillsErr.SkillNames
			}
		default:
			status, code, inv, message := sandboxRefusalOutcome(err)
			outcome.Status = status
			outcome.Code = code
			outcome.Message = message
			outcome.Inventory = inv
		}
		return outcome
	}
	outcome.Status = assignmentStatusUnassigned
	return outcome
}

func isSkillsInstalledError(err error) bool {
	var skillsErr *SkillsInstalledError
	return stderrors.As(err, &skillsErr)
}

// unassign removes one materialized row through the workspace delete flow.
// The installed-skills guard runs FIRST: a config with skills must be cleaned
// up in the workspace (or its skills removed) before the platform stops
// tracking it — the platform surface deliberately has no force cascade.
func (s *SandboxConnectionService) unassign(
	ctx context.Context, row *types.TenantSandboxConfigEntity,
) error {
	skills, err := s.skills.ListSkillsByConfig(ctx, row.TenantID, row.ID)
	if err != nil {
		return err
	}
	if len(skills) > 0 {
		names := make([]string, 0, len(skills))
		for _, skill := range skills {
			if skill != nil {
				names = append(names, skill.Name)
			}
		}
		return &SkillsInstalledError{SkillNames: names}
	}
	return s.configSvc.Delete(ctx, row.TenantID, row.ID, false)
}

// sandboxRefusalOutcome maps a workspace Update/Delete refusal onto the
// push/unassign outcome vocabulary. Statuses reuse the tenant surface's
// conflict codes so the console can render one shared set of reasons.
func sandboxRefusalOutcome(err error) (status, code string, inventory *SandboxInventory, message string) {
	var live *SandboxesStillLiveError
	switch {
	case stderrors.As(err, &live):
		return assignmentStatusBlocked, "sandboxes_still_live", &live.Inventory, err.Error()
	case stderrors.Is(err, ErrSandboxInventoryUnverifiable):
		return assignmentStatusBlocked, "sandbox_inventory_unverifiable", nil, err.Error()
	case stderrors.Is(err, ErrSkillSnapshotReleaseFailed):
		return assignmentStatusBlocked, "skill_snapshot_release_failed", nil, err.Error()
	case stderrors.Is(err, ErrSkillSnapshotBlocksTemplateChange):
		return pushStatusBlockedSkillSnap, "", nil, err.Error()
	case stderrors.Is(err, repository.ErrSandboxConfigCordoned):
		return pushStatusSkippedCordoned, "", nil, err.Error()
	default:
		return assignmentStatusError, "", nil, err.Error()
	}
}

// Push propagates the connection's current payload to every materialized row
// through the workspace Update flow — cordon, live-sandbox refusal, sweep; all
// of it per row. The row's own name is kept: materialization may have
// auto-suffixed it against a same-named self-built config, and re-imposing the
// connection name would collide with that config on every push, forever.
// source_pushed_at is bumped only after a row's Update succeeded, so a failed
// push keeps the drift marker on.
func (s *SandboxConnectionService) Push(
	ctx context.Context, connectionID string,
) ([]PushOutcome, error) {
	conn, err := s.Get(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.configs.ListBySourceConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	results := make([]PushOutcome, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		results = append(results, s.pushRow(ctx, conn, row))
	}
	return results, nil
}

func (s *SandboxConnectionService) pushRow(
	ctx context.Context, conn *types.SandboxConnectionEntity, row *types.TenantSandboxConfigEntity,
) PushOutcome {
	outcome := PushOutcome{
		TenantID: row.TenantID,
		ConfigID: row.ID,
		Status:   pushStatusError,
	}
	if name := s.tenantName(ctx, row.TenantID); name != "" {
		outcome.TenantName = name
	}
	// A fresh cordon means another request is mid-flight on this row; pushing
	// under it would race its inventory sweep. Skip, stay drifting, retry.
	if row.IsCordoned(s.now(), types.SandboxCordonLease) {
		outcome.Status = pushStatusSkippedCordoned
		outcome.Message = "config is being modified by another request"
		return outcome
	}
	payload := deepCopySandboxConfig(conn.Config)
	if payload != nil {
		payload.SkillImage = nil
		payload.VolumeMount = nil
	}
	// Name comes from the row (see Push). The merge inside the workspace
	// Update keeps the row's own SkillImage/VolumeMount untouched.
	if _, err := s.configSvc.Update(ctx, row.TenantID, row.ID, UpdateSandboxConfigInput{
		Name:        row.Name,
		Description: conn.Description,
		Config:      payload,
	}); err != nil {
		status, code, inv, message := sandboxRefusalOutcome(err)
		// Push's vocabulary folds every hard refusal (in practice only live
		// sandboxes reach here through Update) into blocked_live_sandboxes;
		// the code keeps the specific reason.
		if status == assignmentStatusBlocked {
			status = pushStatusBlockedLive
		}
		outcome.Status = status
		outcome.Code = code
		outcome.Message = message
		outcome.Inventory = inv
		return outcome
	}
	if err := s.configs.MarkSourcePushed(ctx, row.TenantID, row.ID, conn.ID, s.now()); err != nil {
		// The row got the new payload but the bookkeeping failed; report it
		// so the operator knows the drift marker stays on.
		outcome.Status = pushStatusError
		outcome.Message = "pushed but failed to record push state: " + err.Error()
		return outcome
	}
	outcome.Status = pushStatusUpdated
	return outcome
}

// QueryTemplates runs the connection drawer's template query: the provider
// catalog for the (possibly unsaved) payload, with the same ensure/replace
// standard-template actions the workspace drawer has. Rebuilding the cluster's
// standard template is refused while any materialized row has a skill snapshot
// or an in-flight install — sessions boot those snapshots and would miss the
// rebuild.
func (s *SandboxConnectionService) QueryTemplates(
	ctx context.Context, in PlatformTemplateQueryInput,
) (*SandboxTemplateCatalog, error) {
	if in.ReplaceStandard && strings.TrimSpace(in.ConnectionID) == "" {
		return nil, apperrors.NewBadRequestError(
			"connection_id is required to rebuild the standard template")
	}
	var existing *types.TenantSandboxConfig
	var conn *types.SandboxConnectionEntity
	if id := strings.TrimSpace(in.ConnectionID); id != "" {
		loaded, err := s.connections.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if loaded == nil {
			return nil, ErrSandboxConnectionNotFound
		}
		conn = loaded
		existing = loaded.Config
	}
	merged := types.MergeSandboxConfigForUpdate(in.Config, existing)
	if merged == nil {
		merged = types.MergeSandboxConfigForUpdate(existing, nil)
	}
	if merged == nil {
		return nil, apperrors.NewBadRequestError("sandbox connection config is required")
	}
	if in.ReplaceStandard {
		if err := s.refuseReplaceWithMaterializedSkills(ctx, in.ConnectionID); err != nil {
			return nil, err
		}
	}
	return s.configSvc.runTemplateQueryCore(
		ctx, "connection:"+in.ConnectionID, merged,
		in.EnsureStandard, in.ReplaceStandard,
		func(ctx context.Context, newID string, oldIDs []string) error {
			if conn == nil || conn.Config == nil {
				return nil
			}
			setSpawnTemplateID(conn.Config, newID)
			if err := s.connections.Update(ctx, conn); err != nil {
				return err
			}
			logger.Infof(ctx, "[sandbox-connection] connection %s spawn template is now %s after rebuild",
				conn.ID, newID)
			return nil
		},
	)
}

func (s *SandboxConnectionService) refuseReplaceWithMaterializedSkills(
	ctx context.Context, connectionID string,
) error {
	rows, err := s.configs.ListBySourceConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if configHasSkillSnapshot(row.Config) || s.hasInFlightSkill(ctx, row) {
			return ErrSkillSnapshotBlocksTemplateChange
		}
	}
	return nil
}

// hasInFlightSkill mirrors TenantSandboxConfigService.configHasInFlightSkill
// but reads the skill store THIS service owns — the config service inside the
// platform surface may be wired without one. Fail-closed on read errors, same
// as the workspace path.
func (s *SandboxConnectionService) hasInFlightSkill(
	ctx context.Context, row *types.TenantSandboxConfigEntity,
) bool {
	if s.skills == nil {
		return false
	}
	skills, err := s.skills.ListSkillsByConfig(ctx, row.TenantID, row.ID)
	if err != nil {
		logger.Warnf(ctx,
			"[sandbox-connection] cannot read skills of config %s while judging a template rebuild: %v",
			row.ID, err)
		return true
	}
	for _, skill := range skills {
		if skill == nil {
			continue
		}
		switch skill.Status {
		case types.SkillStatusInstalling, types.SkillStatusRemoving:
			return true
		}
	}
	return false
}
