package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

var (
	jwtSecretOnce sync.Once
	jwtSecret     string

	// ErrUserEmailExists is returned by Register when the target entity's
	// email already exists.
	ErrUserEmailExists = errors.New("user with this email already exists")

	// ErrUserUsernameExists is returned by Register when the target entity's
	// username already exists.
	ErrUserUsernameExists = errors.New("user with this username already exists")

	// ErrUserIdentityConflict is returned by AdminCreateUser when only part
	// of the requested identity (email or username) collides with an existing
	// user, so a blind idempotent retry would return the wrong account.
	ErrUserIdentityConflict = errors.New("email and username refer to conflicting existing identities")

	// ErrUserEmployeeIDExists is returned by AdminCreateUser when the
	// requested employee ID (工号) is already taken by a live account.
	// Employee ID is the sole identity key in enterprise mode, so a
	// collision is always "the same account", never a conflicting identity.
	ErrUserEmployeeIDExists = errors.New("user with this employee ID already exists")

	// ErrPasswordPolicy is returned when a newly chosen password does not
	// meet the product's public minimum-length contract (≥6 characters).
	// It is exported so HTTP handlers can translate the failure to a 400
	// without exposing bcrypt or persistence errors.
	ErrPasswordPolicy = errors.New("password must be at least 6 characters")

	// ErrInvalidOldPassword is returned by ChangePassword when the supplied
	// current password does not match the stored hash. Handlers map this to
	// a 400 so callers can prompt the user without treating it as a 500.
	ErrInvalidOldPassword = errors.New("invalid old password")

	// ErrSamePassword is returned when the new password equals the current
	// one so callers can reject no-op rotations that would still revoke
	// every session.
	ErrSamePassword = errors.New("new password must differ from current password")
)

// Machine-readable change-password failure reasons for HTTP details fields.
const (
	DetailInvalidOldPassword = "invalid_old_password"
	DetailPasswordPolicy     = "password_policy"
	DetailSamePassword       = "same_password"
)

// getJwtSecret retrieves the JWT secret from the environment, falling back to a securely generated random secret.
func getJwtSecret() string {
	jwtSecretOnce.Do(func() {
		if envSecret := strings.TrimSpace(os.Getenv("JWT_SECRET")); envSecret != "" {
			jwtSecret = envSecret
			return
		}

		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			panic(fmt.Sprintf("failed to generate JWT secret: %v", err))
		}
		jwtSecret = base64.StdEncoding.EncodeToString(randomBytes)
	})

	return jwtSecret
}

// userService implements the UserService interface
type userService struct {
	userRepo         interfaces.UserRepository
	tokenRepo        interfaces.AuthTokenRepository
	tenantService    interfaces.TenantService
	memberService    interfaces.TenantMemberService
	config           *config.Config
	systemSettingSvc interfaces.SystemSettingService
	// fileService/resourceCatalog back the avatar surface (user_avatar.go);
	// nil-safe for partial DI graphs in tests.
	fileService      interfaces.FileService
	resourceCatalog  interfaces.ResourceCatalog
}

// NewUserService creates a new user service instance
func NewUserService(
	configInfo *config.Config,
	userRepo interfaces.UserRepository,
	tokenRepo interfaces.AuthTokenRepository,
	tenantService interfaces.TenantService,
	memberService interfaces.TenantMemberService,
	systemSettingSvc interfaces.SystemSettingService,
	fileService interfaces.FileService,
	resourceCatalog interfaces.ResourceCatalog,
) interfaces.UserService {
	return &userService{
		userRepo:         userRepo,
		tokenRepo:        tokenRepo,
		tenantService:    tenantService,
		memberService:    memberService,
		config:           configInfo,
		systemSettingSvc: systemSettingSvc,
		fileService:      fileService,
		resourceCatalog:  resourceCatalog,
	}
}

// Login authenticates a user and returns tokens
func (s *userService) Login(ctx context.Context, req *types.LoginRequest) (*types.LoginResponse, error) {
	logger.Info(ctx, "Start user login")
	// Look up by employee ID (工号) — the sole login identifier in the
	// enterprise mode. The generic error message deliberately mirrors the
	// identifier-agnostic wording so probes cannot distinguish "unknown
	// employee id" from "wrong password".
	user, err := s.userRepo.GetUserByEmployeeID(ctx, req.EmployeeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get user by employee id: %v", err)
		return &types.LoginResponse{
			Success: false,
			Message: "Invalid employee ID or password",
		}, nil
	}
	if user == nil {
		logger.Warn(ctx, "User not found for employee id")
		return &types.LoginResponse{
			Success: false,
			Message: "Invalid employee ID or password",
		}, nil
	}

	// Check if user is active
	if !user.IsActive {
		logger.Warn(ctx, "User account is disabled")
		return &types.LoginResponse{
			Success: false,
			Message: "Account is disabled",
		}, nil
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		logger.Warn(ctx, "Password verification failed")
		return &types.LoginResponse{
			Success: false,
			Message: "Invalid employee ID or password",
		}, nil
	}
	logger.Info(ctx, "Password verification successful")

	// Stamp last_login_at (000101) best-effort: the member-management UI
	// renders 最后登录时间 from it. A write failure must never fail the
	// login itself — the next successful login retries.
	if err := s.userRepo.UpdateLastLoginAt(ctx, user.ID, time.Now()); err != nil {
		logger.Warnf(ctx, "Failed to update last_login_at for user %s: %v", user.ID, err)
	}

	// Generate tokens. Resolve the target tenant once so the JWT claim
	// and the tenant we return below agree — otherwise an honoured
	// "last active tenant" preference would mint a token for tenant N
	// but tell the client they're in their home tenant.
	logger.Info(ctx, "Generating tokens")
	resolvedTenantID := s.resolveLoginTenantID(ctx, user)
	accessToken, refreshToken, err := s.generateTokensForTenant(ctx, user, resolvedTenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate tokens: %v", err)
		return &types.LoginResponse{
			Success: false,
			Message: "Login failed",
		}, nil
	}
	logger.Info(ctx, "Tokens generated successfully")

	// Get tenant information. A zero resolved ID is a valid tenantless
	// identity, not a failed tenant lookup.
	var tenant *types.Tenant
	if resolvedTenantID > 0 {
		tenant, err = s.tenantService.GetTenantByID(ctx, resolvedTenantID)
		if err != nil {
			logger.Warn(ctx, "Failed to get tenant info")
		} else {
			logger.Info(ctx, "Tenant information retrieved successfully")
		}
	}

	memberships := s.buildMembershipsForUser(ctx, user, tenant)

	logger.Info(ctx, "User logged in successfully")
	return &types.LoginResponse{
		Success:            true,
		Message:            "Login successful",
		User:               user,
		ActiveTenant:       tenant,
		Memberships:        memberships,
		Token:              accessToken,
		RefreshToken:       refreshToken,
		MustChangePassword: user.MustChangePassword,
	}, nil
}

// buildMembershipsForUser returns the user's tenant memberships projected
// into the login-response shape. activeTenant (if non-nil and matching one
// of the rows) is used to reuse its already-fetched name without a second
// DB lookup; other tenants are looked up individually. Errors are logged
// but never propagated — a missing memberships array degrades gracefully
// to length 0 rather than failing the whole login.
//
// When the membership service is unavailable (e.g. in tests that wire only
// part of the dependency graph), this falls back to a single synthesized
// row built from User.TenantID + the active tenant so callers always get
// at least one entry.
func (s *userService) BuildLoginMemberships(
	ctx context.Context,
	user *types.User,
	activeTenant *types.Tenant,
) []types.Membership {
	return s.buildMembershipsForUser(ctx, user, activeTenant)
}

func (s *userService) buildMembershipsForUser(
	ctx context.Context,
	user *types.User,
	activeTenant *types.Tenant,
) []types.Membership {
	if user == nil {
		return []types.Membership{}
	}
	// Only synthesise a membership from User.TenantID when the membership
	// service is entirely unavailable (partial DI graphs / legacy tests).
	// Once ListByUser is reachable, an empty or fully-filtered result is
	// authoritative: inventing a row from a stale users.tenant_id is what
	// kept removed workspaces visible in the space switcher (#2586).
	if s.memberService == nil {
		return synthFallbackMembership(user, activeTenant)
	}
	rows, err := s.memberService.ListByUser(ctx, user.ID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list memberships for user %s: %v", user.ID, err)
		return []types.Membership{}
	}
	if len(rows) == 0 {
		return []types.Membership{}
	}
	// 收集需要批量查询名称的 tenant id（跳过 activeTenant 因为它已经在手）。
	needsLookup := make([]uint64, 0, len(rows))
	for _, m := range rows {
		if m == nil || m.Status != types.TenantMemberStatusActive {
			continue
		}
		if activeTenant != nil && m.TenantID == activeTenant.ID {
			continue
		}
		needsLookup = append(needsLookup, m.TenantID)
	}
	tenantByID := map[uint64]*types.Tenant{}
	if len(needsLookup) > 0 {
		if found, terr := s.tenantService.GetTenantsByIDs(ctx, needsLookup); terr == nil {
			tenantByID = found
		} else {
			logger.Warnf(ctx, "Failed to batch-load tenants for memberships (user=%s): %v",
				user.ID, terr)
		}
	}

	out := make([]types.Membership, 0, len(rows))
	for _, m := range rows {
		if m == nil || m.Status != types.TenantMemberStatusActive {
			continue
		}
		name := ""
		if activeTenant != nil && m.TenantID == activeTenant.ID {
			// A disabled workspace is not a selectable destination for
			// ordinary members; system admins keep it in the list so they
			// can still navigate there to repair it.
			if !user.IsSystemAdmin && activeTenant.Status == types.TenantStatusDisabled {
				continue
			}
			name = activeTenant.Name
		} else if t, ok := tenantByID[m.TenantID]; ok && t != nil {
			if !user.IsSystemAdmin && t.Status == types.TenantStatusDisabled {
				continue
			}
			name = t.Name
		}
		// Drop memberships whose tenant row is gone (deleted tenant or
		// stale tenant_members left over from before cascade delete).
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, types.Membership{
			TenantID:   m.TenantID,
			TenantName: name,
			Role:       m.Role,
		})
	}
	return out
}

// synthFallbackMembership returns a single-row membership list inferred
// from User.TenantID. Used only when the membership service itself is
// unavailable (partial DI graphs in tests, or a rollout window where the
// service has not been wired yet) so the response shape stays consistent.
//
// Callers that successfully queried tenant_members must NOT use this
// helper: an empty membership list is authoritative and synthesising
// from users.tenant_id would re-surface workspaces the user was removed
// from (#2586).
//
// The fallback role is intentionally TenantRoleViewer (least privilege):
// the login response only feeds UI rendering, and the backend re-derives
// the real role from tenant_members on every request. If membership data
// is temporarily unavailable, showing a Viewer UI is preferable to
// granting a misleading Owner UI that would surface admin controls the
// backend will then 403. Once the membership row appears (via the auth
// middleware's home-tenant auto-promotion or an admin invitation) the
// next /auth/me-style refresh will upgrade the UI to the real role.
func synthFallbackMembership(user *types.User, activeTenant *types.Tenant) []types.Membership {
	if user == nil || user.TenantID == 0 {
		// Always return a non-nil slice so the login response carries an
		// empty array rather than `null`, preserving the documented
		// "always populated" contract on LoginResponse.Memberships.
		return []types.Membership{}
	}
	name := ""
	if activeTenant != nil && activeTenant.ID == user.TenantID {
		name = activeTenant.Name
	}
	return []types.Membership{{
		TenantID:   user.TenantID,
		TenantName: name,
		Role:       types.TenantRoleViewer,
	}}
}

// GetUserByID gets a user by ID
func (s *userService) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	return s.userRepo.GetUserByID(ctx, id)
}

// GetUsersByIDs proxies to the repository batch fetch. Returns an empty
// map for an empty input; missing ids are absent from the result.
func (s *userService) GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error) {
	return s.userRepo.GetUsersByIDs(ctx, ids)
}

// GetUserByEmail gets a user by email
func (s *userService) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	return s.userRepo.GetUserByEmail(ctx, email)
}

// GetUserByUsername gets a user by username
func (s *userService) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	return s.userRepo.GetUserByUsername(ctx, username)
}

// GetUserByEmployeeID gets a user by employee ID (工号)
func (s *userService) GetUserByEmployeeID(ctx context.Context, employeeID string) (*types.User, error) {
	return s.userRepo.GetUserByEmployeeID(ctx, employeeID)
}

// ListUsersPage lists users for the admin user-management UI. Thin
// pass-through; the handler enforces SystemAdmin gating and normalises the
// paging parameters.
func (s *userService) ListUsersPage(
	ctx context.Context, query string, tenantID uint64, isActive *bool, offset, limit int,
) ([]*types.User, int64, error) {
	return s.userRepo.ListUsersPage(ctx, query, tenantID, isActive, offset, limit)
}

// GetUserByTenantID gets the first user (owner) of a tenant
func (s *userService) GetUserByTenantID(ctx context.Context, tenantID uint64) (*types.User, error) {
	return s.userRepo.GetUserByTenantID(ctx, tenantID)
}

// UpdateUser updates user information
func (s *userService) UpdateUser(ctx context.Context, user *types.User) error {
	user.UpdatedAt = time.Now()
	return s.userRepo.UpdateUser(ctx, user)
}

// ListSystemAdmins lists users with IsSystemAdmin=true. Thin pass-through
// to the repository; the handler enforces SystemAdmin gating, so the
// service does not duplicate the role check here.
func (s *userService) ListSystemAdmins(
	ctx context.Context, offset, limit int,
) ([]*types.User, int64, error) {
	return s.userRepo.ListSystemAdmins(ctx, offset, limit)
}

// RevokeSystemAdmin removes system-admin privileges through the
// repository's transactional guard so concurrent revokes cannot remove
// the final administrator.
func (s *userService) RevokeSystemAdmin(ctx context.Context, userID, actorID string) (*types.User, error) {
	return s.userRepo.RevokeSystemAdmin(ctx, userID, actorID)
}

// UpdateUserPreferences applies a partial update over the user's
// preferences blob. PATCH semantics: only keys present in `patch`
// (non-nil pointer fields) replace the existing value; everything else
// is preserved. This lets the front-end PUT only the preference that
// changed without having to read-modify-write the whole struct, and
// also makes the endpoint forward-compatible — older clients that
// don't know about newer keys won't accidentally erase them.
func (s *userService) UpdateUserPreferences(
	ctx context.Context,
	userID string,
	patch types.UserPreferences,
) (types.UserPreferences, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return types.UserPreferences{}, err
	}

	merged := user.Preferences
	if patch.LastActiveTenantID != nil {
		// *0 = "forget my preference, fall back to home on next login";
		// any positive value = set/replace. We do not validate membership
		// here — invalid values get culled on the next login via
		// resolveLoginTenantID, keeping this endpoint cheap.
		if *patch.LastActiveTenantID == 0 {
			merged.LastActiveTenantID = nil
		} else {
			v := *patch.LastActiveTenantID
			merged.LastActiveTenantID = &v
		}
	}

	user.Preferences = merged
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return types.UserPreferences{}, err
	}
	return merged, nil
}

// DeleteUser deletes a user
func (s *userService) DeleteUser(ctx context.Context, id string) error {
	return s.userRepo.DeleteUser(ctx, id)
}

// ChangePassword changes user password after verifying the current
// credential. The new password must satisfy ValidatePasswordPolicy so
// self-service rotation cannot introduce weaker passwords than
// registration / admin reset allow. On success every outstanding session
// is revoked so a stolen token cannot survive the rotation.
func (s *userService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify old password before policy checks so callers with a wrong
	// current credential get a clear failure instead of a policy error.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidOldPassword
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(newPassword)); err == nil {
		return ErrSamePassword
	}

	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hashedPassword)
	user.UpdatedAt = time.Now()
	// A successful rotation with the known old credential proves the caller
	// owns the account — clear the forced-rotation flag that admin-set
	// initial/reset passwords arm.
	user.MustChangePassword = false
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return err
	}

	// Invalidate every outstanding session so a stolen token cannot
	// survive a password rotation.
	return s.tokenRepo.RevokeTokensByUserID(ctx, userID)
}

// AdminResetPassword replaces a user's password without checking the previous
// credential. Authorization and the cannot-reset-self rule live at the system
// admin HTTP boundary; this service owns the security-critical persistence and
// session invalidation so no caller can accidentally update only one of them.
// The admin-set password is one the user has not chosen, so the target is
// armed with MustChangePassword and must rotate on next login.
func (s *userService) AdminResetPassword(ctx context.Context, userID, newPassword string) error {
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hashedPassword)
	user.MustChangePassword = true
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return err
	}

	return s.tokenRepo.RevokeTokensByUserID(ctx, userID)
}

// AdminCreateUser provisions a new local user on behalf of a SystemAdmin.
//
// Enterprise semantics — this is the ONLY way local accounts come into
// existence: keyed by employee ID (工号), created tenantless (workspace
// bindings are managed separately via the admin bindings API), and always
// starting with MustChangePassword=true so the admin-set or generated
// initial password must be rotated on first login.
//
// An absent password generates a random one, returned exactly once. Any
// provided password, the empty string included, must satisfy
// ValidatePasswordPolicy.
func (s *userService) AdminCreateUser(
	ctx context.Context,
	req *types.AdminCreateUserRequest,
) (*types.User, string, error) {
	if req == nil {
		return nil, "", errors.New("request is required")
	}
	employeeID := strings.TrimSpace(req.EmployeeID)
	username := strings.TrimSpace(req.Username)
	if employeeID == "" || username == "" {
		return nil, "", errors.New("employee ID and username are required")
	}

	password := ""
	generated := false

	if req.Password == nil {
		randomPassword, err := generatePolicyCompliantPassword()
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate password: %w", err)
		}
		password = randomPassword
		generated = true
	} else {
		password = *req.Password
	}
	// Generation triggers only on an absent password. Any provided
	// value, empty or whitespace-only, is hashed byte-for-byte and must
	// satisfy the password policy.
	if err := ValidatePasswordPolicy(password); err != nil {
		return nil, "", err
	}

	// Sequential duplicate check; a concurrent double-create loses at the
	// partial unique index and surfaces as a 500 whose retry then lands
	// here idempotently. Employee ID is the sole identity key, so a hit
	// always refers to the same account — no conflict variant exists.
	if existing, err := s.userRepo.GetUserByEmployeeID(ctx, employeeID); err == nil && existing != nil {
		return existing, "", ErrUserEmployeeIDExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	email := ""
	if req.Email != nil {
		email = strings.TrimSpace(*req.Email)
	}

	user := &types.User{
		ID:                 uuid.New().String(),
		EmployeeID:         employeeID,
		Username:           username,
		Email:              email,
		PasswordHash:       string(hashedPassword),
		TenantID:           0, // tenantless until an admin binds workspaces
		IsActive:           true,
		MustChangePassword: true,
	}
	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, "", err
	}

	logger.Infof(ctx, "Admin provisioned user account for employee ID %s", secutils.SanitizeForLog(employeeID))
	if generated {
		return user, password, nil
	}
	return user, "", nil
}

// EnsureBootstrapAdmin guarantees the deployment has at least one system
// administrator, creating the default admin account when none exists.
//
// Semantics (idempotent, safe to run on every startup):
//   - Any system admin already present → (false, "", nil), no writes. This
//     also keeps a bootstrap env var from silently re-granting privileges
//     an operator revoked from the UI.
//   - Account with the employee ID already exists (but no sysadmins) →
//     promote it in place; its password is deliberately NOT touched.
//   - Otherwise create a fresh tenantless account with IsSystemAdmin=true
//     and MustChangePassword=true. When password is empty a random
//     policy-compliant one is generated and returned exactly once so the
//     caller (startup bootstrap) can surface it in a one-time log line.
//
// A concurrent double-startup loses at the employee_id unique index and
// returns the error; the caller treats bootstrap as best-effort.
func (s *userService) EnsureBootstrapAdmin(ctx context.Context, employeeID, password string) (bool, string, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return false, "", errors.New("bootstrap admin employee ID is required")
	}

	if _, total, err := s.userRepo.ListSystemAdmins(ctx, 0, 1); err != nil {
		return false, "", err
	} else if total > 0 {
		return false, "", nil
	}

	// Existing account with this employee ID: promote in place.
	if existing, err := s.userRepo.GetUserByEmployeeID(ctx, employeeID); err == nil && existing != nil {
		if existing.IsSystemAdmin {
			return false, "", nil
		}
		existing.IsSystemAdmin = true
		if err := s.userRepo.UpdateUser(ctx, existing); err != nil {
			return false, "", err
		}
		logger.Infof(ctx, "Bootstrap promoted existing account %s to system admin", existing.ID)
		return true, "", nil
	}

	generated := false
	if password == "" {
		randomPassword, err := generatePolicyCompliantPassword()
		if err != nil {
			return false, "", fmt.Errorf("failed to generate bootstrap password: %w", err)
		}
		password = randomPassword
		generated = true
	}
	if err := ValidatePasswordPolicy(password); err != nil {
		return false, "", err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, "", err
	}

	user := &types.User{
		ID:                 uuid.New().String(),
		EmployeeID:         employeeID,
		Username:           "Administrator",
		Email:              "",
		PasswordHash:       string(hashedPassword),
		TenantID:           0,
		IsActive:           true,
		IsSystemAdmin:      true,
		MustChangePassword: true,
	}
	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return false, "", err
	}

	logger.Infof(ctx, "Bootstrap created default system admin account (employee ID: %s, user ID: %s)",
		secutils.SanitizeForLog(employeeID), user.ID)
	if generated {
		return true, password, nil
	}
	return true, "", nil
}

// ValidatePassword validates user password
func (s *userService) ValidatePassword(ctx context.Context, userID string, password string) error {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
}

// GenerateTokens generates access and refresh tokens for user. The
// access token's tenant_id claim defaults to user.TenantID (home), but
// if the user has persisted a still-valid "last active tenant"
// preference we honour it instead — so login (and the refresh-token
// rotation path that also calls into here) lands the user back where
// they left off across devices. SwitchTenant remains the explicit tool
// for switching to an arbitrary membership.
func (s *userService) GenerateTokens(
	ctx context.Context,
	user *types.User,
) (accessToken, refreshToken string, err error) {
	return s.generateTokensForTenant(ctx, user, s.resolveLoginTenantID(ctx, user))
}

// resolveLoginTenantID picks the tenant whose ID should be encoded in a
// freshly minted access token. The contract:
//
//  1. If the user has no LastActiveTenantID preference set (or it points
//     at home), return home — the historical behaviour. A tenantless user
//     with an active membership adopts their earliest membership instead;
//     this repairs partial invitation/admin-assignment flows.
//  2. Otherwise validate the preference: the tenant must still exist and
//     the user must still have an active membership (or be a cross-tenant
//     superuser). Validation failure logs a warning, best-effort clears
//     the stale preference (so we don't waste a DB round-trip on every
//     subsequent login), and falls back to home.
//
// This is intentionally a private method on userService so it can reach
// memberService / tenantService / userRepo. Errors from the validation
// path never fail login; the worst case is the user lands in home.
func (s *userService) resolveLoginTenantID(ctx context.Context, user *types.User) uint64 {
	if user == nil {
		return 0
	}
	pref := user.Preferences.LastActiveTenantID
	if pref == nil || *pref == 0 || *pref == user.TenantID {
		return s.homeOrFirstMembershipTenant(ctx, user)
	}
	preferred := *pref

	// Tenant must still exist and be usable as a login target. A workspace
	// disabled by the system admin is skipped for ordinary members just like
	// a deleted one (the preference is cleared and resolution falls back).
	if !s.tenantUsableForUser(ctx, preferred, user.IsSystemAdmin) {
		logger.Warnf(ctx,
			"resolveLoginTenantID: preferred tenant %d not usable for user %s, "+
				"clearing preference and falling back to home",
			preferred, user.ID)
		s.clearLastActiveTenantPreference(ctx, user)
		return s.homeOrFirstMembershipTenant(ctx, user)
	}

	// Membership (or cross-tenant superuser) must still be valid. Mirrors
	// the gate in SwitchTenant so the two entry points stay consistent.
	if !user.CanAccessAllTenants {
		if s.memberService == nil {
			logger.Warnf(ctx,
				"resolveLoginTenantID: member service unavailable; falling back to home for user %s",
				user.ID)
			return user.TenantID
		}
		member, err := s.memberService.GetMembership(ctx, user.ID, preferred)
		if err != nil || member == nil || member.Status != types.TenantMemberStatusActive {
			logger.Warnf(ctx,
				"resolveLoginTenantID: user %s no longer has active membership in tenant %d, "+
					"clearing preference and falling back to home (err=%v)",
				user.ID, preferred, err)
			s.clearLastActiveTenantPreference(ctx, user)
			return s.homeOrFirstMembershipTenant(ctx, user)
		}
	}

	return preferred
}

// homeOrFirstMembershipTenant returns the user's home tenant, or — for a
// tenantless identity (TenantID == 0) — the earliest active membership.
// Shared by the happy path and the stale-preference fallbacks so a
// tenantless session with a valid membership never gets a zero-tenant
// token when a usable tenant is available (repairs partial
// invitation/admin-assignment flows). resolveFirstMembershipTenant
// best-effort persists the resolved tenant as the new home.
//
// When users.tenant_id is non-zero we still verify an active membership
// still exists (mirroring resolveLoginTenantID's check on
// LastActiveTenantID). A dangling home pointer is common after
// RemoveMember: the membership row is soft-deleted but users.tenant_id
// was historically left untouched, which made the removed workspace
// reappear via synthFallbackMembership (#2586). Superusers that can
// access every tenant skip the membership gate.
func (s *userService) homeOrFirstMembershipTenant(ctx context.Context, user *types.User) uint64 {
	if user == nil {
		return 0
	}
	if user.TenantID == 0 {
		return s.resolveFirstMembershipTenant(ctx, user)
	}
	if user.CanAccessAllTenants || s.memberService == nil {
		return user.TenantID
	}
	member, err := s.memberService.GetMembership(ctx, user.ID, user.TenantID)
	if err == nil && member != nil && member.Status == types.TenantMemberStatusActive {
		// A disabled home workspace cannot be the session target for an
		// ordinary member — landing there would block every request. Clear
		// the stale home pointer and re-resolve to an active workspace.
		if !s.tenantUsableForUser(ctx, user.TenantID, user.IsSystemAdmin) {
			logger.Warnf(ctx,
				"homeOrFirstMembershipTenant: user %s home tenant %d is unavailable or disabled, "+
					"clearing stale home and re-resolving",
				user.ID, user.TenantID)
			s.clearStaleHomeTenant(ctx, user)
			return s.resolveFirstMembershipTenant(ctx, user)
		}
		return user.TenantID
	}
	logger.Warnf(ctx,
		"homeOrFirstMembershipTenant: user %s home tenant %d has no active membership, "+
			"clearing stale home and re-resolving (err=%v)",
		user.ID, user.TenantID, err)
	s.clearStaleHomeTenant(ctx, user)
	return s.resolveFirstMembershipTenant(ctx, user)
}

// clearStaleHomeTenant best-effort zeroes users.tenant_id (and a
// LastActiveTenantID that pointed at the same workspace) after the home
// membership is observed to be gone — or the home workspace was disabled
// by a system admin and is no longer a usable target for the member.
// Failures are logged but never fail login: the in-memory user is
// already corrected for this request.
func (s *userService) clearStaleHomeTenant(ctx context.Context, user *types.User) {
	if user == nil {
		return
	}
	staleHome := user.TenantID
	user.TenantID = 0
	if user.Preferences.LastActiveTenantID != nil && *user.Preferences.LastActiveTenantID == staleHome {
		user.Preferences.LastActiveTenantID = nil
	}
	if s.userRepo == nil {
		return
	}
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		logger.Warnf(ctx,
			"clearStaleHomeTenant: failed to persist cleared home for user %s (was tenant %d): %v",
			user.ID, staleHome, err)
	}
}

// resolveFirstMembershipTenant makes a tenantless identity usable when an
// active membership already exists (for example, an invitation was accepted
// but persisting the default tenant failed). ListByUser is stably ordered by
// join time, so the earliest valid membership is deterministic. Persisting it
// as home is best-effort: even if the repair write fails, the freshly issued
// token can still be scoped to the membership and the next login retries.
// tenantUsableForUser reports whether the workspace may serve as a
// login / session target for the given user. Two reasons it cannot:
// the tenant row is gone, or a system admin disabled it from the console.
// System admins stay exempt from the disabled check so the operator who
// disabled a workspace can still get back in and re-enable or delete it.
// A missing tenantService (partial DI graphs) answers true, mirroring the
// original nil-guard behaviour of the callers.
func (s *userService) tenantUsableForUser(ctx context.Context, tenantID uint64, systemAdmin bool) bool {
	if s.tenantService == nil {
		return true
	}
	t, err := s.tenantService.GetTenantByID(ctx, tenantID)
	if err != nil || t == nil {
		return false
	}
	return systemAdmin || t.Status != types.TenantStatusDisabled
}

func (s *userService) resolveFirstMembershipTenant(ctx context.Context, user *types.User) uint64 {
	if user == nil || s.memberService == nil {
		return 0
	}
	members, err := s.memberService.ListByUser(ctx, user.ID)
	if err != nil {
		logger.Warnf(ctx, "resolveLoginTenantID: failed to list memberships for tenantless user %s: %v", user.ID, err)
		return 0
	}
	for _, member := range members {
		if member == nil || member.TenantID == 0 || member.Status != types.TenantMemberStatusActive {
			continue
		}
		if !s.tenantUsableForUser(ctx, member.TenantID, user.IsSystemAdmin) {
			logger.Warnf(ctx, "resolveLoginTenantID: tenant %d for tenantless user %s is unavailable or disabled",
				member.TenantID, user.ID)
			continue
		}

		user.TenantID = member.TenantID
		if s.userRepo != nil {
			if err := s.userRepo.UpdateUser(ctx, user); err != nil {
				logger.Warnf(ctx, "resolveLoginTenantID: failed to persist tenant %d for tenantless user %s: %v",
					member.TenantID, user.ID, err)
				user.TenantID = 0
			}
		}
		return member.TenantID
	}
	return 0
}

// clearLastActiveTenantPreference is the best-effort cleanup half of
// resolveLoginTenantID. Failures here are logged but never propagated:
// the in-memory user already has the preference cleared for this login,
// and the next login will re-attempt the cleanup.
func (s *userService) clearLastActiveTenantPreference(ctx context.Context, user *types.User) {
	if user == nil {
		return
	}
	user.Preferences.LastActiveTenantID = nil
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		logger.Warnf(ctx,
			"clearLastActiveTenantPreference: failed to persist cleared preference for user %s: %v",
			user.ID, err)
	}
}

// generateTokensForTenant is the shared implementation behind
// GenerateTokens and SwitchTenant. It encodes activeTenantID into the
// access token's tenant_id claim so the auth middleware scopes future
// requests there.
func (s *userService) generateTokensForTenant(
	ctx context.Context,
	user *types.User,
	activeTenantID uint64,
) (accessToken, refreshToken string, err error) {
	// Generate access token (expires in 24 hours)
	accessClaims := jwt.MapClaims{
		"user_id":   user.ID,
		"email":     user.Email,
		"tenant_id": activeTenantID,
		"exp":       time.Now().Add(24 * time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"type":      "access",
	}

	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessTokenObj.SignedString([]byte(getJwtSecret()))
	if err != nil {
		return "", "", err
	}

	// Generate refresh token (expires in 7 days)
	refreshClaims := jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"type":    "refresh",
	}

	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshTokenObj.SignedString([]byte(getJwtSecret()))
	if err != nil {
		return "", "", err
	}

	// Store tokens in database
	accessTokenRecord := &types.AuthToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     accessToken,
		TokenType: "access_token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	refreshTokenRecord := &types.AuthToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     refreshToken,
		TokenType: "refresh_token",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_ = s.tokenRepo.CreateToken(ctx, accessTokenRecord)
	_ = s.tokenRepo.CreateToken(ctx, refreshTokenRecord)

	return accessToken, refreshToken, nil
}

// SwitchTenant verifies that user has an active membership in
// targetTenantID and issues a new token pair scoped to that tenant.
// The previous refresh token (if provided) is revoked so the old session
// can no longer roll forward into the source tenant.
//
// On success the target is also recorded as the user's last-active-
// tenant preference. Refresh tokens do not carry tenant_id, so
// RefreshToken / the next login re-resolve from this preference;
// a successful switch therefore has to persist it before minting
// tokens. A write failure aborts the switch so we never return a
// token pair whose later refresh would bounce to a different workspace.
//
// Returns ErrMembershipNotFound when the user is not a member of the
// target tenant. Cross-tenant superuser access (CanAccessAllTenants)
// is allowed without a membership row, mirroring the auth middleware's
// resolveTenantRole behaviour.
func (s *userService) SwitchTenant(
	ctx context.Context,
	user *types.User,
	targetTenantID uint64,
	currentRefreshToken string,
) (*types.LoginResponse, error) {
	if user == nil {
		return nil, errors.New("user is required")
	}
	if targetTenantID == 0 {
		return nil, errors.New("target workspace ID is required")
	}

	// Verify membership unless the caller is a cross-tenant superuser
	// switching outside their home tenant.
	if !user.CanAccessAllTenants || targetTenantID == user.TenantID {
		if s.memberService == nil {
			return nil, errors.New("workspace membership service unavailable")
		}
		member, err := s.memberService.GetMembership(ctx, user.ID, targetTenantID)
		if err != nil {
			return nil, fmt.Errorf("lookup membership: %w", err)
		}
		if member == nil || member.Status != types.TenantMemberStatusActive {
			return nil, ErrMembershipNotFound
		}
	}

	tenant, err := s.tenantService.GetTenantByID(ctx, targetTenantID)
	if err != nil {
		return nil, fmt.Errorf("load target workspace: %w", err)
	}
	// 已禁用空间不可作为切换目标（系统管理员可在控制台内恢复）。
	if tenant.Status == types.TenantStatusDisabled && !user.IsSystemAdmin {
		return nil, errors.New("workspace has been disabled by the system administrator")
	}

	// Persist before minting tokens so a 200 response is also a
	// durable landing-preference update. RefreshToken re-reads this
	// field; writing after issue would leave a window where access
	// is scoped to targetTenantID but refresh falls back to home.
	if err := s.recordLastActiveTenant(ctx, user, targetTenantID); err != nil {
		return nil, fmt.Errorf("record last-active-tenant preference: %w", err)
	}

	accessToken, refreshToken, err := s.generateTokensForTenant(ctx, user, targetTenantID)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	// Best-effort revoke of the previous refresh token. Failure is
	// logged but not fatal — the new tokens are already issued and the
	// old refresh token will expire naturally.
	if strings.TrimSpace(currentRefreshToken) != "" {
		if err := s.RevokeToken(ctx, currentRefreshToken); err != nil {
			logger.Warnf(ctx, "Failed to revoke previous refresh token during tenant switch: %v", err)
		}
	}

	memberships := s.buildMembershipsForUser(ctx, user, tenant)

	return &types.LoginResponse{
		Success:      true,
		Message:      "Workspace switched",
		User:         user,
		ActiveTenant: tenant,
		Memberships:  memberships,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// recordLastActiveTenant records tenantID as the user's last-active-
// tenant preference so fresh logins and refresh-token rotations land
// in the last activated workspace. Switching home writes the home ID
// rather than clearing; resolveLoginTenantID treats *home the same as
// nil today. (The SPA still sends 0 when switching home — landing is
// equivalent unless home tenant_id is later rewritten while the old
// home membership remains.) Errors are returned to the caller.
func (s *userService) recordLastActiveTenant(ctx context.Context, user *types.User, tenantID uint64) error {
	if user == nil || tenantID == 0 {
		return nil
	}
	patch := types.UserPreferences{LastActiveTenantID: &tenantID}
	merged, err := s.UpdateUserPreferences(ctx, user.ID, patch)
	if err != nil {
		return err
	}
	// Reflect the persisted preferences in the switch response.
	user.Preferences = merged
	return nil
}

// ValidateToken validates an access token. The second return value is
// the JWT's `tenant_id` claim — i.e. the tenant the token was minted
// for, which may differ from user.TenantID after a /auth/switch-tenant
// call. Tokens minted before tenant-level RBAC don't carry the claim;
// in that case we fall back to user.TenantID for backward compatibility.
func (s *userService) ValidateToken(ctx context.Context, tokenString string) (*types.User, uint64, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJwtSecret()), nil
	})

	if err != nil || !token.Valid {
		return nil, 0, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, 0, errors.New("invalid token claims")
	}

	userID, ok := claims["user_id"].(string)
	if !ok {
		return nil, 0, errors.New("invalid user ID in token")
	}

	if isRefreshTokenClaims(claims) {
		return nil, 0, errors.New("refresh token cannot be used as access token")
	}

	// Check if token is revoked
	tokenRecord, err := s.tokenRepo.GetTokenByValue(ctx, tokenString)
	if err != nil || tokenRecord == nil || tokenRecord.IsRevoked {
		return nil, 0, errors.New("token is revoked")
	}
	if tokenRecord.TokenType == "refresh_token" {
		return nil, 0, errors.New("refresh token cannot be used as access token")
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	// Extract active tenant from the JWT. Anything missing or unparseable
	// falls back to the user's home tenant so old tokens (and tokens issued
	// by code paths that don't yet set the claim) keep working.
	activeTenantID := tenantIDFromClaims(claims, user.TenantID)

	return user, activeTenantID, nil
}

func isRefreshTokenClaims(claims jwt.MapClaims) bool {
	tokenType, ok := claims["type"].(string)
	return ok && tokenType == "refresh"
}

func userIDFromSignedToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJwtSecret()), nil
	}, jwt.WithoutClaimsValidation())
	if err != nil || token == nil || !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	userID, ok := claims["user_id"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return "", errors.New("invalid user ID in token")
	}
	return userID, nil
}

// tenantIDFromClaims pulls the active tenant ID out of a parsed JWT
// claim map. Returns fallback when the claim is missing or has an
// unrecognised type. Extracted as a free function so it can be unit
// tested without standing up the full userService dependency graph.
//
// JSON numbers come back as float64 from jwt.MapClaims; the int64 /
// uint64 branches cover legacy code paths and tests that build claims
// directly. Negative values are treated as missing.
func tenantIDFromClaims(claims jwt.MapClaims, fallback uint64) uint64 {
	raw, ok := claims["tenant_id"]
	if !ok {
		return fallback
	}
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return uint64(v)
		}
	case int64:
		if v > 0 {
			return uint64(v)
		}
	case uint64:
		if v > 0 {
			return v
		}
	}
	return fallback
}

// RefreshToken refreshes access token using refresh token
func (s *userService) RefreshToken(
	ctx context.Context,
	refreshTokenString string,
) (accessToken, newRefreshToken string, err error) {
	token, err := jwt.Parse(refreshTokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJwtSecret()), nil
	})

	if err != nil || !token.Valid {
		return "", "", errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", errors.New("invalid token claims")
	}

	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != "refresh" {
		return "", "", errors.New("not a refresh token")
	}

	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", "", errors.New("invalid user ID in token")
	}

	// Check if token is revoked
	tokenRecord, err := s.tokenRepo.GetTokenByValue(ctx, refreshTokenString)
	if err != nil || tokenRecord == nil || tokenRecord.IsRevoked {
		return "", "", errors.New("refresh token is revoked")
	}
	if tokenRecord.TokenType != "refresh_token" {
		return "", "", errors.New("not a refresh token")
	}

	// Get user
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return "", "", err
	}

	// Revoke old refresh token
	tokenRecord.IsRevoked = true
	_ = s.tokenRepo.UpdateToken(ctx, tokenRecord)

	// Generate new tokens
	return s.GenerateTokens(ctx, user)
}

// Logout invalidates every outstanding session for the user identified by
// the presented JWT. Access and refresh tokens are both accepted so clients
// can end the session without refreshing first; expired tokens are allowed
// so logout still works after the access token TTL.
func (s *userService) Logout(ctx context.Context, tokenString string) error {
	userID, err := userIDFromSignedToken(tokenString)
	if err != nil {
		return err
	}
	return s.tokenRepo.RevokeTokensByUserID(ctx, userID)
}

// RevokeToken revokes a token
func (s *userService) RevokeToken(ctx context.Context, tokenString string) error {
	tokenRecord, err := s.tokenRepo.GetTokenByValue(ctx, tokenString)
	if err != nil {
		return err
	}

	tokenRecord.IsRevoked = true
	tokenRecord.UpdatedAt = time.Now()

	return s.tokenRepo.UpdateToken(ctx, tokenRecord)
}

// GetCurrentUser gets current user from context
func (s *userService) GetCurrentUser(ctx context.Context) (*types.User, error) {
	user, ok := ctx.Value(types.UserContextKey).(*types.User)
	if !ok {
		return nil, errors.New("user not found in context")
	}

	return user, nil
}

// SearchUsers searches users by username or email
func (s *userService) SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error) {
	if query == "" {
		return []*types.User{}, nil
	}
	return s.userRepo.SearchUsers(ctx, query, limit)
}

func generateRandomString(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// generatePolicyCompliantPassword returns a cryptographically random
// password: 24 random bytes base64url-encoded to 32 characters, always
// within the length-only policy.
func generatePolicyCompliantPassword() (string, error) {
	return generateRandomString(24)
}
