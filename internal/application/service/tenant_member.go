package service

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// isDuplicateMembership recognises the unique-constraint violation that
// the tenant_members partial unique index throws when two concurrent
// AddMember / EnsureMember calls race past the in-service Get() check.
// We map this to ErrMembershipAlreadyExists so handlers can return 409
// instead of an opaque 500; the underlying DB still rejects the second
// insert, so this is purely about error-translation, not weakening any
// invariant.
//
// gorm.ErrDuplicatedKey covers the dialect-translated case (gorm ≥1.25
// with TranslateError enabled). The string match on "duplicate" /
// "unique" is the fallback for raw drivers that don't surface the
// sentinel — Postgres "duplicate key value violates unique constraint",
// SQLite "UNIQUE constraint failed", MySQL "Duplicate entry" all
// contain at least one of those tokens.
func isDuplicateMembership(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint")
}

// Sentinel errors returned by tenantMemberService. Callers compare with
// errors.Is to render appropriate HTTP responses (404 / 409 / 403).
var (
	// ErrMembershipNotFound is returned when no active membership row
	// matches the (user, tenant) pair the caller requested.
	ErrMembershipNotFound = errors.New("tenant membership not found")

	// ErrMembershipAlreadyExists is returned by AddMember when the
	// (user, tenant) pair already has an active membership.
	ErrMembershipAlreadyExists = errors.New("tenant membership already exists")

	// ErrInvalidTenantRole is returned when the caller passes a role
	// value that is not one of the four defined TenantRole constants.
	ErrInvalidTenantRole = errors.New("invalid tenant role")

	// ErrAPIKeyCannotAssignOwner is returned when an API-key principal
	// attempts to persist the Owner role through member or invitation
	// management. manage_members deliberately excludes ownership transfer:
	// a machine principal may manage lower roles, but must never mint a
	// durable human Owner who could subsequently manage API keys or delete
	// the tenant.
	ErrAPIKeyCannotAssignOwner = errors.New("API keys cannot assign the owner role")

	// ErrLastOwner is returned when an operation would leave the tenant
	// without an active Owner. Demoting the last Owner or removing them
	// is forbidden; an explicit ownership transfer must happen first.
	ErrLastOwner = errors.New("cannot demote or remove the last active owner of the tenant")
)

const (
	listMembersDefaultPageSize = 20
	listMembersMaxPageSize     = 100
)

// tenantMemberService implements interfaces.TenantMemberService.
type tenantMemberService struct {
	repo      interfaces.TenantMemberRepository
	audit     interfaces.AuditLogService     // optional; nil ⇒ no audit, business ops still succeed
	userRepo  interfaces.UserRepository      // optional; used to clear stale home-tenant pointers
	tokenRepo interfaces.AuthTokenRepository // optional; used to revoke sessions after removal
}

// NewTenantMemberService constructs the service. Wired up via the DI
// container alongside the other application services. The auditService
// is optional — passing nil disables durable audit but keeps the
// underlying mutations working, so a container reshuffle that
// constructs tenant_member before audit_log won't crash and tests
// don't need to stub the dependency unless they care about audit
// behaviour.
//
// userRepo / tokenRepo are also optional and only used by RemoveMember
// cleanup: after a membership is soft-deleted we best-effort clear a
// dangling users.tenant_id / LastActiveTenantID and revoke outstanding
// sessions so the removed user cannot keep a JWT scoped to a workspace
// they no longer belong to. Passing nil keeps unit tests that only
// exercise membership invariants free of extra stubs.
func NewTenantMemberService(
	repo interfaces.TenantMemberRepository,
	audit interfaces.AuditLogService,
	userRepo interfaces.UserRepository,
	tokenRepo interfaces.AuthTokenRepository,
) interfaces.TenantMemberService {
	return &tenantMemberService{
		repo:      repo,
		audit:     audit,
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
	}
}

// emitAudit is the per-mutation audit hook. Best-effort: a nil audit
// service or a write failure is logged inside the audit service itself
// and never bubbles up to the caller. RBAC mutations succeed even if
// audit is unavailable; the alternative (failing the business op when
// the audit table is down) is far worse.
func (s *tenantMemberService) emitAudit(ctx context.Context, entry *types.AuditLog) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Log(ctx, entry)
}

// auditActorRole picks up the caller's role at write-time. Empty if
// auth middleware didn't set it (e.g. service-internal flows like
// EnsureMember during workspace bootstrap, where there is no "caller").
func auditActorRole(ctx context.Context) string {
	return string(types.TenantRoleFromContext(ctx))
}

// auditActor returns the calling user id from context, "" when no
// authenticated caller is present (service-internal paths).
func auditActor(ctx context.Context) string {
	uid, _ := types.UserIDFromContext(ctx)
	return uid
}

// rejectAPIKeyOwnerAssignment is the service-layer boundary shared by
// direct membership writes and both invitation creation paths. Route RBAC
// intentionally defers API-key authorization to APIKeyGate, so checking the
// authenticated principal in the service is required to preserve the
// manage_members "no ownership transfer" contract across every caller.
func rejectAPIKeyOwnerAssignment(ctx context.Context, role types.TenantRole) error {
	if role != types.TenantRoleOwner {
		return nil
	}
	if _, ok := types.TenantAPIKeyScopeFromContext(ctx); ok {
		return ErrAPIKeyCannotAssignOwner
	}
	return nil
}

// AddMember inserts a new active membership row. Returns
// ErrMembershipAlreadyExists if the user is already an active member of
// the tenant, and ErrInvalidTenantRole for unknown roles.
func (s *tenantMemberService) AddMember(
	ctx context.Context,
	userID string,
	tenantID uint64,
	role types.TenantRole,
	invitedBy *string,
) (*types.TenantMember, error) {
	if !role.IsValid() {
		return nil, ErrInvalidTenantRole
	}
	if err := rejectAPIKeyOwnerAssignment(ctx, role); err != nil {
		return nil, err
	}
	existing, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrMembershipAlreadyExists
	}
	member := &types.TenantMember{
		UserID:    userID,
		TenantID:  tenantID,
		Role:      role,
		Status:    types.TenantMemberStatusActive,
		InvitedBy: invitedBy,
		JoinedAt:  time.Now(),
	}
	if err := s.repo.Create(ctx, member); err != nil {
		// TOCTOU race: a concurrent AddMember / EnsureMember slipped past
		// the Get above. The DB's partial unique index on
		// (user_id, tenant_id) WHERE deleted_at IS NULL caught it; map
		// to the same sentinel the in-service check would have returned
		// so callers get a clean 409 instead of an opaque 500.
		if isDuplicateMembership(err) {
			return nil, ErrMembershipAlreadyExists
		}
		return nil, err
	}
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       types.AuditActionMemberAdded,
		TargetType:   "tenant_member",
		TargetUserID: userID,
		Outcome:      types.AuditOutcomeSuccess,
	})
	return member, nil
}

// EnsureMember is idempotent: if the user already has an active membership
// in the tenant it is returned unchanged; otherwise a new admin row is
// created. Roles were flattened by the enterprise user-system rework, so
// "initial member" is always admin (not owner — owner carries the
// last-owner invariant that would deadlock admin-driven unbinds once
// everyone is the same rank). Used by workspace creation and the auth
// middleware's orphan-tenant self-heal so re-running on an existing row
// (e.g. after a partial failure) does not double-insert.
func (s *tenantMemberService) EnsureMember(
	ctx context.Context,
	userID string,
	tenantID uint64,
) (*types.TenantMember, error) {
	existing, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	member := &types.TenantMember{
		UserID:   userID,
		TenantID: tenantID,
		Role:     types.TenantRoleAdmin,
		Status:   types.TenantMemberStatusActive,
		JoinedAt: time.Now(),
	}
	if err := s.repo.Create(ctx, member); err != nil {
		// Idempotent contract: if a concurrent Ensure/AddMember beat us
		// (two simultaneous self-heal paths firing on parallel JWTs), the
		// partial unique index rejects the second insert. Re-read and
		// return the winning row so EnsureMember stays observably
		// idempotent.
		if isDuplicateMembership(err) {
			if winner, getErr := s.repo.Get(ctx, userID, tenantID); getErr == nil && winner != nil {
				logger.Infof(ctx,
					"EnsureMember lost race for user=%s tenant=%d, returning winning row (role=%s)",
					userID, tenantID, winner.Role)
				return winner, nil
			}
		}
		return nil, err
	}
	logger.Infof(ctx, "Bootstrapped admin membership for user=%s tenant=%d", userID, tenantID)
	return member, nil
}

// GetMembership returns the active membership or (nil, nil) when absent.
func (s *tenantMemberService) GetMembership(
	ctx context.Context,
	userID string,
	tenantID uint64,
) (*types.TenantMember, error) {
	return s.repo.Get(ctx, userID, tenantID)
}

// ListByUser proxies to the repository; included on the service so HTTP
// handlers depend only on the service interface.
func (s *tenantMemberService) ListByUser(ctx context.Context, userID string) ([]*types.TenantMember, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListByTenant proxies to the repository.
func (s *tenantMemberService) ListByTenant(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

// ListMembersPage returns a slice plus total matching query (handlers parse
// page/page_size; defensive clamps here mirror list handler limits).
func (s *tenantMemberService) ListMembersPage(
	ctx context.Context,
	tenantID uint64,
	query string,
	page, pageSize int,
) ([]*types.TenantMember, int64, error) {
	query = strings.TrimSpace(query)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = listMembersDefaultPageSize
	}
	if pageSize > listMembersMaxPageSize {
		pageSize = listMembersMaxPageSize
	}
	total, err := s.repo.CountFilteredByTenant(ctx, tenantID, query)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	members, err := s.repo.ListPagedByTenant(ctx, tenantID, query, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return members, total, nil
}

// HasAnyMembers proxies to the repository.
func (s *tenantMemberService) HasAnyMembers(ctx context.Context, tenantID uint64) (bool, error) {
	return s.repo.HasAnyMembers(ctx, tenantID)
}

// UpdateRole enforces the "cannot demote the last Owner" invariant before
// delegating to the repository. Re-promoting an existing Owner is a no-op
// from the invariant's perspective.
func (s *tenantMemberService) UpdateRole(
	ctx context.Context,
	userID string,
	tenantID uint64,
	newRole types.TenantRole,
) error {
	if !newRole.IsValid() {
		return ErrInvalidTenantRole
	}
	if err := rejectAPIKeyOwnerAssignment(ctx, newRole); err != nil {
		return err
	}
	current, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrMembershipNotFound
	}
	if current.Role == newRole {
		return nil
	}
	oldRole := current.Role
	// Owner demotion is the dangerous path: two concurrent demotions of
	// two different Owners with the old "Get → Count → Update" sequence
	// could each observe count=2 and both commit, leaving the tenant
	// ownerless. Route through the repo's atomic helper instead, which
	// takes a row-level UPDATE lock on every other active Owner before
	// committing the role change.
	if current.Role == types.TenantRoleOwner && newRole != types.TenantRoleOwner {
		err := s.repo.DemoteOwnerAtomically(ctx, userID, tenantID, newRole)
		switch {
		case errors.Is(err, apprepo.ErrLastOwner):
			return ErrLastOwner
		case err != nil:
			return err
		}
		s.emitRoleChangeAudit(ctx, tenantID, userID, oldRole, newRole)
		return nil
	}
	if err := s.repo.UpdateRole(ctx, userID, tenantID, newRole); err != nil {
		return err
	}
	s.emitRoleChangeAudit(ctx, tenantID, userID, oldRole, newRole)
	return nil
}

// emitRoleChangeAudit packs the old/new role into Details so the
// audit-log UI can render "promoted Alice from contributor to admin"
// without a separate column per role transition.
func (s *tenantMemberService) emitRoleChangeAudit(
	ctx context.Context,
	tenantID uint64,
	targetUserID string,
	oldRole, newRole types.TenantRole,
) {
	details, _ := json.Marshal(map[string]string{
		"old_role": string(oldRole),
		"new_role": string(newRole),
	})
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       types.AuditActionMemberRoleChanged,
		TargetType:   "tenant_member",
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(details),
	})
}

// RemoveMember enforces the "cannot remove the last Owner" invariant
// before soft-deleting the membership. For Owner removals it routes
// through the repo's transactional helper so the count + delete commit
// atomically (no TOCTOU between checking owner count and deleting).
//
// The audit row distinguishes "voluntary leave" (caller == target,
// driven by POST /leave) from "kicked" (caller != target, driven by
// DELETE /tenants/:id/members/:user_id). Both go through this same
// service method but the recorded action differs so an audit reader
// can tell the two apart.
//
// After a successful soft-delete, best-effort cleanup clears any
// dangling users.tenant_id / LastActiveTenantID that still points at
// the removed workspace and revokes outstanding auth tokens. Without
// this, a tenantless→invited→removed user keeps a stale home pointer
// and the login path synthesises the removed workspace back into the
// space switcher (see issue #2586). Cleanup failures are logged but
// never fail the removal itself: the membership row is already gone
// and the login-path membership checks act as a second line of defence.
func (s *tenantMemberService) RemoveMember(ctx context.Context, userID string, tenantID uint64) error {
	current, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrMembershipNotFound
	}
	if current.Role == types.TenantRoleOwner {
		err := s.repo.RemoveOwnerAtomically(ctx, userID, tenantID)
		switch {
		case errors.Is(err, apprepo.ErrLastOwner):
			return ErrLastOwner
		case err != nil:
			return err
		}
		s.emitRemovalAudit(ctx, tenantID, userID)
		s.cleanupRemovedMemberState(ctx, userID, tenantID)
		return nil
	}
	if err := s.repo.SoftDelete(ctx, userID, tenantID); err != nil {
		return err
	}
	s.emitRemovalAudit(ctx, tenantID, userID)
	s.cleanupRemovedMemberState(ctx, userID, tenantID)
	return nil
}

// cleanupRemovedMemberState drops stale home/preference pointers that
// reference the removed tenant and revokes the user's sessions so any
// JWT still scoped to that tenant cannot keep serving 403-only UI.
// All steps are best-effort and nil-safe for partial DI graphs in tests.
func (s *tenantMemberService) cleanupRemovedMemberState(ctx context.Context, userID string, tenantID uint64) {
	if s.userRepo != nil {
		user, err := s.userRepo.GetUserByID(ctx, userID)
		if err != nil {
			logger.Warnf(ctx,
				"RemoveMember cleanup: failed to load user %s after removing tenant %d: %v",
				userID, tenantID, err)
		} else if user != nil {
			changed := false
			if user.TenantID == tenantID {
				user.TenantID = 0
				changed = true
			}
			if user.Preferences.LastActiveTenantID != nil && *user.Preferences.LastActiveTenantID == tenantID {
				user.Preferences.LastActiveTenantID = nil
				changed = true
			}
			if changed {
				user.UpdatedAt = time.Now()
				if err := s.userRepo.UpdateUser(ctx, user); err != nil {
					logger.Warnf(ctx,
						"RemoveMember cleanup: failed to clear stale tenant pointers for user %s tenant %d: %v",
						userID, tenantID, err)
				}
			}
		}
	}

	if s.tokenRepo == nil {
		return
	}
	if err := s.tokenRepo.RevokeTokensByUserID(ctx, userID); err != nil {
		logger.Warnf(ctx,
			"RemoveMember cleanup: failed to revoke tokens for user %s after removing tenant %d: %v",
			userID, tenantID, err)
	}
}

// emitRemovalAudit picks AuditActionMemberLeft when the caller is
// removing themselves, AuditActionMemberRemoved otherwise. Caller
// detection uses the user-id from the request context — the same
// source the LeaveTenant handler uses to derive its `userID` arg.
func (s *tenantMemberService) emitRemovalAudit(
	ctx context.Context,
	tenantID uint64,
	targetUserID string,
) {
	action := types.AuditActionMemberRemoved
	if actor := auditActor(ctx); actor != "" && actor == targetUserID {
		action = types.AuditActionMemberLeft
	}
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       action,
		TargetType:   "tenant_member",
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
	})
}

// memberPasswordResetDigits is the digit count of tenant-admin generated
// reset passwords (000101: "重置为8位数字").
const memberPasswordResetDigits = 8

// ErrMemberUserMissing is returned by ResetMemberPassword when the
// membership points at a user row that cannot be loaded (deleted user,
// partial DI graph in tests). Handlers surface it as 404/500.
var ErrMemberUserMissing = errors.New("member user record not found")

// ResetMemberPassword replaces the target member's password with a random
// 8-digit numeric password and returns it exactly once. The 8-digit code
// satisfies the length-only ValidatePasswordPolicy; MustChangePassword=true
// still forces a rotation at the next login, exactly like the system-admin
// reset path (product requirement: admins hand out a short spoken-friendly
// code). Sessions are revoked so a reset doubles as "log the member out
// everywhere".
func (s *tenantMemberService) ResetMemberPassword(
	ctx context.Context,
	tenantID uint64,
	targetUserID string,
) (string, error) {
	if s.userRepo == nil {
		return "", ErrMemberUserMissing
	}
	user, err := s.userRepo.GetUserByID(ctx, targetUserID)
	if err != nil {
		// Join keeps the underlying cause (typically gorm.ErrRecordNotFound)
		// while still matching the sentinel the handler/test asserts on.
		return "", errors.Join(ErrMemberUserMissing, err)
	}
	if user == nil {
		return "", ErrMemberUserMissing
	}

	plain, err := generateNumericPassword(memberPasswordResetDigits)
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	user.PasswordHash = string(hash)
	user.MustChangePassword = true
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return "", err
	}

	// Best-effort session revocation — mirrors AdminResetPassword: a user
	// whose password was just rolled must not keep riding old tokens.
	if s.tokenRepo != nil {
		if err := s.tokenRepo.RevokeTokensByUserID(ctx, targetUserID); err != nil {
			logger.Warnf(ctx,
				"ResetMemberPassword: failed to revoke tokens for user %s tenant %d: %v",
				targetUserID, tenantID, err)
		}
	}

	details, _ := json.Marshal(map[string]any{
		"generated_digits": memberPasswordResetDigits,
	})
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       types.AuditActionMemberPasswordReset,
		TargetType:   "tenant_member",
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(details),
	})
	return plain, nil
}

// generateNumericPassword returns a cryptographically random string of
// exactly n digits, biased to never start with 0 so the admin can read it
// aloud without a leading-zero ambiguity (range 1000…-9999…).
func generateNumericPassword(n int) (string, error) {
	if n < 4 {
		n = 4
	}
	buf := make([]byte, n)
	// First digit: 1-9.
	v, err := cryptoRandInt(9)
	if err != nil {
		return "", err
	}
	buf[0] = byte('1' + v)
	for i := 1; i < n; i++ {
		v, err := cryptoRandInt(10)
		if err != nil {
			return "", err
		}
		buf[i] = byte('0' + v)
	}
	return string(buf), nil
}

// cryptoRandInt returns a crypto/rand non-negative int below max.
func cryptoRandInt(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("cryptoRandInt: max must be positive")
	}
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

// GetMemberStats proxies the repository so handlers stay service-only.
func (s *tenantMemberService) GetMemberStats(
	ctx context.Context,
	tenantID uint64,
	userID string,
) (*types.MemberStats, error) {
	return s.repo.MemberWorkspaceStats(ctx, tenantID, userID)
}
