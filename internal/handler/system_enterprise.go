package handler

// Enterprise user & workspace management endpoints, mounted under
// /api/v1/system/admin/* behind the SystemAdmin() group guard (see
// RegisterSystemAdminRoutes in the router package). This file implements
// the "企业级用户管理" model of the user-system rework:
//
//   - users are provisioned by the system admin (no self-registration),
//     keyed by an immutable employee_id and created tenantless;
//   - the user↔workspace relation is a binding the system admin manages
//     (POST/DELETE bindings), reusing TenantMemberService so the existing
//     rbac.member_added / rbac.member_removed audit trail and the
//     "cannot remove the last owner" invariants keep applying;
//   - workspaces carry a two-level role model: 空间管理员 (admin) and
//     普通用户 (contributor). New bindings default to contributor — the
//     system admin designates workspace admins explicitly via
//     PUT /system/admin/tenants/:tenant_id/members/:user_id (the 000093
//     migration demoted the 000092-flattened admin rows to contributor);
//   - every workspace keeps at least one workspace admin: demoting or
//     unbinding the last one answers 409 (workspaceHasOtherAdmins);
//   - when a tenantless user receives their first binding, that workspace
//     becomes their home tenant (users.tenant_id backfill) so the login
//     flow drops them straight into a usable workspace.
//
// Handlers here follow the system.go conventions: audit via
// emitAdminAudit (system.user_* actions), secrets never logged, and
// errors returned as {error: "..."} JSON with the matching status code.

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// enterpriseUserPageDefaults / cap mirror ListSystemAdmins pagination
// conventions so both admin list UIs behave identically.
const (
	enterpriseUsersDefaultLimit = 20
	enterpriseUsersMaxLimit     = 200
)

// EnterpriseBinding is the user↔workspace binding projection shared by the
// list/create responses. Role is the actual membership role — "admin"
// (空间管理员) or "contributor" (普通用户) in the two-level enterprise
// model.
type EnterpriseBinding struct {
	TenantID   uint64 `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Role       string `json:"role"`
}

// EnterpriseUserRow extends UserInfo with the user's workspace bindings so
// the admin user list can render workspace tags without a follow-up call
// per row.
type EnterpriseUserRow struct {
	*types.UserInfo
	Bindings []EnterpriseBinding `json:"bindings"`
}

// parseEnterpriseUserFilters extracts the list filters from the query
// string. Malformed values fall back to defaults instead of 400-ing
// (mirrors ListSystemAdmins' soft pagination parsing).
func parseEnterpriseUserFilters(c *gin.Context) (query string, tenantID uint64, isActive *bool, offset, limit int) {
	query = strings.TrimSpace(c.Query("query"))
	tenantID = 0
	if v := c.Query("tenant_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			tenantID = n
		}
	}
	isActive = nil
	if v := c.Query("is_active"); v != "" {
		b := v == "true" || v == "1"
		isActive = &b
	}
	offset, limit = 0, enterpriseUsersDefaultLimit
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > enterpriseUsersMaxLimit {
		limit = enterpriseUsersMaxLimit
	}
	return query, tenantID, isActive, offset, limit
}

// tenantNamesFor is a small batch helper: resolve tenant names for the
// given ids so binding rows can carry a display name. Missing tenants
// (deleted workspaces) map to "".
func (h *SystemHandler) tenantNamesFor(c *gin.Context, ids []uint64) map[uint64]string {
	names := make(map[uint64]string, len(ids))
	if len(ids) == 0 || h.tenantSvc == nil {
		return names
	}
	ctx := c.Request.Context()
	tenants, err := h.tenantSvc.GetTenantsByIDs(ctx, ids)
	if err != nil {
		// Name enrichment is cosmetic; an empty name beats a failed list.
		return names
	}
	for id, t := range tenants {
		if t != nil {
			names[id] = t.Name
		}
	}
	return names
}

// bindingsForUser collects the user's active bindings with tenant names.
// Returns an empty (non-nil) slice when the member service is unavailable.
func (h *SystemHandler) bindingsForUser(c *gin.Context, userID string) []EnterpriseBinding {
	bindings := make([]EnterpriseBinding, 0)
	if h.memberSvc == nil {
		return bindings
	}
	ctx := c.Request.Context()
	members, err := h.memberSvc.ListByUser(ctx, userID)
	if err != nil {
		return bindings
	}
	ids := make([]uint64, 0, len(members))
	for _, m := range members {
		if m != nil {
			ids = append(ids, m.TenantID)
		}
	}
	names := h.tenantNamesFor(c, ids)
	for _, m := range members {
		if m == nil {
			continue
		}
		bindings = append(bindings, EnterpriseBinding{
			TenantID:   m.TenantID,
			TenantName: names[m.TenantID],
			Role:       string(m.Role),
		})
	}
	return bindings
}

// requireMemberService centralises the nil guard for the binding endpoints.
// Partially-wired unit tests construct SystemHandler without the member
// service; those environments answer 503 rather than panicking on a nil
// interface call.
func (h *SystemHandler) requireMemberService(c *gin.Context) bool {
	if h.memberSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "User workspace bindings are not available in this deployment"})
		return false
	}
	return true
}

// parseEnterpriseWorkspaceRole maps a wire-format role to a TenantRole.
// Empty defaults to contributor — in the two-level enterprise model,
// undesignated users are ordinary members ("没有指定的用户，都是普通用户").
// owner/viewer are not accepted: they are not part of the enterprise
// two-level surface even though legacy rows may still surface them.
func parseEnterpriseWorkspaceRole(role string) (types.TenantRole, bool) {
	switch types.TenantRole(strings.TrimSpace(role)) {
	case "", types.TenantRoleContributor:
		return types.TenantRoleContributor, true
	case types.TenantRoleAdmin:
		return types.TenantRoleAdmin, true
	default:
		return "", false
	}
}

// workspaceHasOtherAdmins reports whether the workspace keeps at least one
// active admin (or legacy owner) besides the given user. Demoting or
// unbinding the last workspace admin must be rejected (409) so a workspace
// can never end up admin-less with no recovery path inside the member UI —
// only the system admin console grants the role in the enterprise model.
// Member rosters are small, so the in-memory scan over ListByTenant is fine.
func (h *SystemHandler) workspaceHasOtherAdmins(ctx context.Context, tenantID uint64, excludeUserID string) (bool, error) {
	members, err := h.memberSvc.ListByTenant(ctx, tenantID)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		if m == nil || m.UserID == excludeUserID {
			continue
		}
		if m.Role.Level() >= types.TenantRoleAdmin.Level() {
			return true, nil
		}
	}
	return false, nil
}

// ListEnterpriseUsers godoc
// @Summary      List users (SystemAdmin)
// @Description  Paginated platform-wide user list for the enterprise user
// @Description  management UI. Filters: `query` (matches employee_id /
// @Description  username / email case-insensitively), `tenant_id` (only
// @Description  users bound to that workspace), `is_active` (true/false).
// @Description  Each row carries the user's workspace bindings.
// @Tags         System Admin
// @Produce      json
// @Param        query     query string false "Fuzzy match on employee ID / username / email"
// @Param        tenant_id query int    false "Restrict to users bound to this workspace"
// @Param        is_active query bool   false "Filter by account state"
// @Param        offset    query int    false "Page offset" default(0)
// @Param        limit     query int    false "Page size (max 200)" default(20)
// @Success      200  {object}  map[string]interface{}  "{total, users:[...]}"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      500  {object}  map[string]interface{}  "Internal error"
// @Router       /system/admin/users [get]
func (h *SystemHandler) ListEnterpriseUsers(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	query, tenantID, isActive, offset, limit := parseEnterpriseUserFilters(c)
	users, total, err := h.userSvc.ListUsersPage(ctx, query, tenantID, isActive, offset, limit)
	if err != nil {
		logger.Errorf(ctx, "Failed to list users (query=%q tenant=%d): %v", query, tenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users"})
		return
	}

	rows := make([]EnterpriseUserRow, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		rows = append(rows, EnterpriseUserRow{
			UserInfo: u.ToUserInfo(),
			Bindings: h.bindingsForUser(c, u.ID),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"users": rows,
	})
}

// EnterpriseCreateUserRequest extends the provisioning payload with the
// initial workspace bindings. Bindings are applied after the account is
// created; a failing binding does not roll the account back — the response
// reports per-binding results so the admin can retry just the binding.
type EnterpriseCreateUserRequest struct {
	types.AdminCreateUserRequest
	// TenantIDs lists workspaces the new user is bound to immediately.
	// Optional — tenantless accounts can be bound later via the bindings
	// endpoint.
	TenantIDs []uint64 `json:"tenant_ids"`
}

// EnterpriseCreateUserResponse mirrors CreateSystemUserResponse plus the
// per-binding application results.
type EnterpriseCreateUserResponse struct {
	User              *types.UserInfo          `json:"user"`
	GeneratedPassword string                   `json:"generated_password,omitempty"`
	Bindings          []EnterpriseBindResult   `json:"bindings"`
}

// EnterpriseBindResult reports one tenant_ids entry of the create request.
type EnterpriseBindResult struct {
	TenantID uint64 `json:"tenant_id"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

// bindUserToTenantIDs applies the initial bindings for a freshly created
// (or existing) user: every tenant must exist, bindings are created with
// role=contributor (ordinary member — workspace admins are designated
// explicitly afterwards), and the first successful binding becomes the
// home tenant of a tenantless user. Results are collected per tenant so a
// partial failure never fails the whole create.
func (h *SystemHandler) bindUserToTenantIDs(c *gin.Context, user *types.User, tenantIDs []uint64) []EnterpriseBindResult {
	results := make([]EnterpriseBindResult, 0, len(tenantIDs))
	if len(tenantIDs) == 0 {
		return results
	}
	if !h.requireMemberService(c) {
		for _, id := range tenantIDs {
			results = append(results, EnterpriseBindResult{TenantID: id, OK: false, Error: "binding service unavailable"})
		}
		return results
	}
	ctx := c.Request.Context()
	actorID, _ := types.UserIDFromContext(ctx)
	for _, tenantID := range tenantIDs {
		res := EnterpriseBindResult{TenantID: tenantID}
		if tenant, err := h.tenantSvc.GetTenantByID(ctx, tenantID); err != nil || tenant == nil {
			res.Error = "workspace not found"
			results = append(results, res)
			continue
		}
		if _, err := h.memberSvc.AddMember(ctx, user.ID, tenantID, types.TenantRoleContributor, &actorID); err != nil {
			res.Error = "binding failed"
			results = append(results, res)
			continue
		}
		res.OK = true
		results = append(results, res)
	}
	// Home-tenant backfill: the first bound workspace becomes the login
	// default for a tenantless account. Best-effort — a failure here only
	// means the user picks the workspace from the switcher after login.
	if user.TenantID == 0 {
		for _, res := range results {
			if res.OK {
				user.TenantID = res.TenantID
				if err := h.userSvc.UpdateUser(ctx, user); err != nil {
					logger.Errorf(ctx, "Failed to backfill home tenant %d for user %s: %v", res.TenantID, user.ID, err)
					user.TenantID = 0
				}
				break
			}
		}
	}
	return results
}

// CreateEnterpriseUser godoc
// @Summary      Create a user with initial workspace bindings (SystemAdmin)
// @Description  Provision a tenantless account keyed by employee_id (see
// @Description  POST /system/admin/users/create) and optionally bind it to
// @Description  the listed workspaces in the same call. Bindings apply with
// @Description  role=contributor (ordinary member; designate workspace
// @Description  admins via PUT /system/admin/tenants/{tenant_id}/members/{user_id});
// @Description  the first successful binding becomes the user's
// @Description  home workspace. Per-binding results are returned so a
// @Description  partial failure is visible.
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        request body EnterpriseCreateUserRequest true "User creation request"
// @Success      201  {object}  EnterpriseCreateUserResponse  "User created"
// @Success      200  {object}  EnterpriseCreateUserResponse  "Employee ID already exists (bindings still applied)"
// @Failure      400  {object}  map[string]interface{}  "Invalid request or weak password"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Router       /system/admin/users [post]
func (h *SystemHandler) CreateEnterpriseUser(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	var req EnterpriseCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user creation request"})
		return
	}
	req.EmployeeID = strings.TrimSpace(req.EmployeeID)
	req.Username = secutils.SanitizeForLog(strings.TrimSpace(req.Username))
	if req.Email != nil {
		sanitized := secutils.SanitizeForLog(strings.TrimSpace(*req.Email))
		req.Email = &sanitized
	}
	// Password is intentionally NOT trimmed or sanitized.
	if req.EmployeeID == "" || req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee ID and username are required"})
		return
	}
	if n := utf8.RuneCountInString(req.Username); n < 2 || n > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username must be 2-50 characters"})
		return
	}

	user, generatedPassword, err := h.userSvc.AdminCreateUser(ctx, &req.AdminCreateUserRequest)
	// idempotent marks the "employee ID already exists" branch: the service
	// resolved the existing account, so we still apply the requested
	// bindings and answer 200 instead of failing the whole flow.
	idempotent := false
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserEmployeeIDExists) && user != nil:
			idempotent = true
			logger.Infof(ctx, "Enterprise create user noop (employee ID exists, ID: %s)", user.ID)
		case errors.Is(err, service.ErrPasswordPolicy):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		default:
			logger.Errorf(ctx, "Failed to create user: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}
	}

	bindings := h.bindUserToTenantIDs(c, user, req.TenantIDs)

	h.emitAdminAudit(ctx, types.AuditActionSystemUserCreated, user, map[string]any{
		"target_employee_id": user.EmployeeID,
		"target_username":    user.Username,
		"password_generated": !idempotent && generatedPassword != "",
		"initial_bindings":   len(req.TenantIDs),
		"idempotent":         idempotent,
	})
	if !idempotent {
		logger.Infof(ctx, "System admin created user %s (employee ID: %s, ID: %s)", user.Username, user.EmployeeID, user.ID)
	}

	status := http.StatusCreated
	if idempotent {
		status = http.StatusOK
	}

	c.JSON(status, EnterpriseCreateUserResponse{
		User:              user.ToUserInfo(),
		GeneratedPassword: generatedPassword,
		Bindings:          bindings,
	})
}

// EnterpriseUpdateUserRequest is the PATCH-style body for
// PUT /system/admin/users/:user_id. Nil fields are left unchanged.
type EnterpriseUpdateUserRequest struct {
	Username *string `json:"username" binding:"omitempty,min=2,max=50"`
	// Email is an optional contact field. An explicit empty string clears
	// it; null/omitted leaves it alone.
	Email    *string `json:"email" binding:"omitempty,email,max=255"`
	IsActive *bool   `json:"is_active"`
}

// UpdateEnterpriseUser godoc
// @Summary      Update a user's profile or lifecycle state (SystemAdmin)
// @Description  Edit display name / email and enable or disable the
// @Description  account. Disabling revokes every outstanding session.
// @Description  System-admin accounts cannot be disabled here — revoke the
// @Description  admin role first (409 otherwise). Employee ID is immutable.
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        user_id path string true "User ID"
// @Param        request body EnterpriseUpdateUserRequest true "Fields to update"
// @Success      200  {object}  EnterpriseUserRow  "Updated user"
// @Failure      400  {object}  map[string]interface{}  "Invalid request"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "User not found"
// @Failure      409  {object}  map[string]interface{}  "Cannot disable a system admin"
// @Router       /system/admin/users/{user_id} [put]
func (h *SystemHandler) UpdateEnterpriseUser(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	user, err := h.userSvc.GetUserByID(ctx, c.Param("user_id"))
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req EnterpriseUpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user update request"})
		return
	}

	// Lifecycle guard: a disabled system admin would still hold the role
	// while being unable to log in — force the explicit revoke path first
	// so the "last admin" invariant stays observable.
	disabling := req.IsActive != nil && !*req.IsActive && user.IsActive
	if disabling && user.IsSystemAdmin {
		c.JSON(http.StatusConflict, gin.H{"error": "Revoke this user's system admin role before disabling the account"})
		return
	}

	dirty := false
	if req.Username != nil {
		username := secutils.SanitizeForLog(strings.TrimSpace(*req.Username))
		if n := utf8.RuneCountInString(username); n < 2 || n > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Username must be 2-50 characters"})
			return
		}
		user.Username = username
		dirty = true
	}
	if req.Email != nil {
		email := secutils.SanitizeForLog(strings.TrimSpace(*req.Email))
		user.Email = email
		dirty = true
	}
	if req.IsActive != nil && *req.IsActive != user.IsActive {
		user.IsActive = *req.IsActive
		dirty = true
	}
	if !dirty {
		c.JSON(http.StatusOK, EnterpriseUserRow{UserInfo: user.ToUserInfo(), Bindings: h.bindingsForUser(c, user.ID)})
		return
	}

	if err := h.userSvc.UpdateUser(ctx, user); err != nil {
		logger.Errorf(ctx, "Failed to update user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	switch {
	case disabling:
		// Kill every session so a disabled account cannot keep riding a
		// valid JWT until it expires.
		if h.tokenRepo != nil {
			if err := h.tokenRepo.RevokeTokensByUserID(ctx, user.ID); err != nil {
				logger.Errorf(ctx, "Failed to revoke sessions for disabled user %s: %v", user.ID, err)
			}
		}
		logger.Infof(ctx, "System admin disabled user %s (employee ID: %s)", user.ID, user.EmployeeID)
		h.emitAdminAudit(ctx, types.AuditActionSystemUserDisabled, user, map[string]any{
			"target_employee_id": user.EmployeeID,
			"sessions_revoked":   true,
		})
	case req.IsActive != nil && *req.IsActive && !user.MustChangePassword:
		// Re-enable. (The MustChangePassword caveat is impossible today —
		// the flag clears on first rotation — but keeps the audit honest
		// if the semantics ever change.)
		logger.Infof(ctx, "System admin enabled user %s (employee ID: %s)", user.ID, user.EmployeeID)
		h.emitAdminAudit(ctx, types.AuditActionSystemUserEnabled, user, map[string]any{
			"target_employee_id": user.EmployeeID,
		})
	default:
		logger.Infof(ctx, "System admin updated user %s (employee ID: %s)", user.ID, user.EmployeeID)
		h.emitAdminAudit(ctx, types.AuditActionSystemUserUpdated, user, map[string]any{
			"target_employee_id": user.EmployeeID,
			"target_username":    user.Username,
		})
	}

	c.JSON(http.StatusOK, EnterpriseUserRow{UserInfo: user.ToUserInfo(), Bindings: h.bindingsForUser(c, user.ID)})
}

// EnterpriseResetPasswordRequest is the body for the per-user-id password
// reset. The plaintext password never reaches logs or audit details.
type EnterpriseResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// ResetEnterpriseUserPassword godoc
// @Summary      Reset a user's password by user ID (SystemAdmin)
// @Description  Replace the user's local password, arm the first-login
// @Description  forced rotation, and revoke all of their sessions. Same
// @Description  semantics as /system/admin/users/reset-password but
// @Description  addresses the target by user_id path parameter (the row
// @Description  key used by the enterprise user list).
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        user_id path string true "User ID"
// @Param        request body EnterpriseResetPasswordRequest true "New password"
// @Success      200  {object}  map[string]interface{}  "Password reset successfully"
// @Failure      400  {object}  map[string]interface{}  "Weak password or self reset"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "User not found"
// @Router       /system/admin/users/{user_id}/reset-password [post]
func (h *SystemHandler) ResetEnterpriseUserPassword(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	user, err := h.userSvc.GetUserByID(ctx, c.Param("user_id"))
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req EnterpriseResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid password reset request"})
		return
	}
	if err := service.ValidatePasswordPolicy(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	callerID, _ := types.UserIDFromContext(ctx)
	if callerID == user.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot reset your own password here"})
		return
	}

	if err := h.userSvc.AdminResetPassword(ctx, user.ID, req.NewPassword); err != nil {
		if service.IsPasswordPolicyError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		logger.Errorf(ctx, "Failed to reset password for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset user password"})
		return
	}

	logger.Infof(ctx, "Password reset by system administrator for user ID: %s", user.ID)
	h.emitAdminAudit(ctx, types.AuditActionSystemUserPasswordReset, user, map[string]any{
		"target_employee_id": user.EmployeeID,
		"target_username":    user.Username,
		"sessions_revoked":   true,
	})
	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}

// ListUserBindings godoc
// @Summary      List a user's workspace bindings (SystemAdmin)
// @Description  Active memberships of the user with workspace names.
// @Tags         System Admin
// @Produce      json
// @Param        user_id path string true "User ID"
// @Success      200  {object}  map[string]interface{}  "{bindings:[...]}"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "User not found"
// @Router       /system/admin/users/{user_id}/bindings [get]
func (h *SystemHandler) ListUserBindings(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	user, err := h.userSvc.GetUserByID(ctx, c.Param("user_id"))
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if !h.requireMemberService(c) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"bindings": h.bindingsForUser(c, user.ID)})
}

// EnterpriseBindRequest is the body for binding a user to a workspace.
// Role is optional: omitted/empty defaults to contributor (ordinary
// member); "admin" designates a workspace administrator directly.
type EnterpriseBindRequest struct {
	TenantID uint64 `json:"tenant_id" binding:"required"`
	Role     string `json:"role" binding:"omitempty,oneof=admin contributor"`
}

// BindUserToTenant godoc
// @Summary      Bind a user to a workspace (SystemAdmin)
// @Description  Create an active membership. Role defaults to contributor
// @Description  (ordinary member); pass "admin" to designate a workspace
// @Description  administrator in the same call. The audit row
// @Description  (rbac.member_added) is emitted by TenantMemberService. If
// @Description  the user has no home workspace, the bound workspace becomes
// @Description  their login default.
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        user_id path string true "User ID"
// @Param        request body EnterpriseBindRequest true "Workspace (and role) to bind"
// @Success      201  {object}  EnterpriseBinding  "Binding created"
// @Failure      400  {object}  map[string]interface{}  "Invalid request"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "User or workspace not found"
// @Failure      409  {object}  map[string]interface{}  "Binding already exists"
// @Router       /system/admin/users/{user_id}/bindings [post]
func (h *SystemHandler) BindUserToTenant(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if !h.requireMemberService(c) {
		return
	}
	user, err := h.userSvc.GetUserByID(ctx, c.Param("user_id"))
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req EnterpriseBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid binding request"})
		return
	}
	role, ok := parseEnterpriseWorkspaceRole(req.Role)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role must be \"admin\" or \"contributor\""})
		return
	}
	tenant, err := h.tenantSvc.GetTenantByID(ctx, req.TenantID)
	if err != nil || tenant == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
		return
	}

	actorID, _ := types.UserIDFromContext(ctx)
	if _, err := h.memberSvc.AddMember(ctx, user.ID, tenant.ID, role, &actorID); err != nil {
		if errors.Is(err, service.ErrMembershipAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already bound to this workspace"})
			return
		}
		logger.Errorf(ctx, "Failed to bind user %s to tenant %d: %v", user.ID, tenant.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to bind user to workspace"})
		return
	}

	// Home-tenant backfill for tenantless accounts (admin-provisioned
	// users start with tenant_id=0). Best-effort; failure only costs the
	// login-time default, and the binding itself is already durable.
	if user.TenantID == 0 {
		user.TenantID = tenant.ID
		if err := h.userSvc.UpdateUser(ctx, user); err != nil {
			logger.Errorf(ctx, "Failed to backfill home tenant %d for user %s: %v", tenant.ID, user.ID, err)
			user.TenantID = 0
		}
	}

	c.JSON(http.StatusCreated, EnterpriseBinding{
		TenantID:   tenant.ID,
		TenantName: tenant.Name,
		Role:       string(role),
	})
}

// UnbindUserFromTenant godoc
// @Summary      Remove a user's workspace binding (SystemAdmin)
// @Description  Soft-delete the membership; audit (rbac.member_removed)
// @Description  and session revocation are handled by
// @Description  TenantMemberService. Legacy owner rows created before the
// @Description  role flattening still honour the "cannot remove the last
// @Description  owner" invariant, and removing the last workspace admin
// @Description  is rejected with 409 (workspaceHasOtherAdmins).
// @Tags         System Admin
// @Produce      json
// @Param        user_id   path string true "User ID"
// @Param        tenant_id path int    true "Workspace ID"
// @Success      200  {object}  map[string]interface{}  "Binding removed"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "Binding not found"
// @Failure      409  {object}  map[string]interface{}  "Last workspace admin/owner"
// @Router       /system/admin/users/{user_id}/bindings/{tenant_id} [delete]
func (h *SystemHandler) UnbindUserFromTenant(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if !h.requireMemberService(c) {
		return
	}
	tenantID, err := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}
	userID := c.Param("user_id")

	// Last-admin guard: unbinding the only workspace admin would leave the
	// workspace without an administrator. The system admin can always
	// designate another admin first; owners count towards the guard since
	// they outrank admins.
	if member, err := h.memberSvc.GetMembership(ctx, userID, tenantID); err == nil && member != nil &&
		member.Role.Level() >= types.TenantRoleAdmin.Level() {
		hasOthers, err := h.workspaceHasOtherAdmins(ctx, tenantID, userID)
		if err != nil {
			logger.Errorf(ctx, "Failed to check workspace admins (tenant=%d): %v", tenantID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove binding"})
			return
		}
		if !hasOthers {
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot remove the last workspace admin of this workspace"})
			return
		}
	}

	if err := h.memberSvc.RemoveMember(ctx, userID, tenantID); err != nil {
		switch {
		case errors.Is(err, service.ErrMembershipNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Binding not found"})
		case errors.Is(err, service.ErrLastOwner):
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot remove the last owner of this workspace"})
		default:
			logger.Errorf(ctx, "Failed to unbind user %s from tenant %d: %v", userID, tenantID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove binding"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Binding removed"})
}

// EnterpriseWorkspaceMember is one row of the workspace member list in the
// admin console: the membership role plus the user-facing display fields
// hydrated in one batched user lookup.
type EnterpriseWorkspaceMember struct {
	UserID     string    `json:"user_id"`
	EmployeeID string    `json:"employee_id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	JoinedAt   time.Time `json:"joined_at"`
}

// ListWorkspaceMembers godoc
// @Summary      List a workspace's members (SystemAdmin)
// @Description  Paginated member roster of one workspace for the admin
// @Description  console's workspace list — the surface where the system
// @Description  admin designates workspace admins. Supports `q` filtering
// @Description  by employee ID / username / email.
// @Tags         System Admin
// @Produce      json
// @Param        tenant_id path int true "Workspace ID"
// @Param        q         query string false "Fuzzy match on employee ID / username / email"
// @Param        page      query int    false "Page number (1-based)" default(1)
// @Param        page_size query int    false "Page size (max 100)" default(20)
// @Success      200  {object}  map[string]interface{}  "{total, members:[...]}"
// @Failure      400  {object}  map[string]interface{}  "Invalid workspace ID"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "Workspace not found"
// @Router       /system/admin/tenants/{tenant_id}/members [get]
func (h *SystemHandler) ListWorkspaceMembers(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if !h.requireMemberService(c) {
		return
	}
	tenantID, err := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}
	if tenant, err := h.tenantSvc.GetTenantByID(ctx, tenantID); err != nil || tenant == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	page, pageSize, ok := parseListPagination(c)
	if !ok {
		return
	}

	members, total, err := h.memberSvc.ListMembersPage(ctx, tenantID, q, page, pageSize)
	if err != nil {
		logger.Errorf(ctx, "Failed to list workspace members (tenant=%d): %v", tenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list workspace members"})
		return
	}

	// Batch-hydrate display fields (mirrors TenantMemberHandler.ListMembers;
	// a transient lookup error degrades names to empty rather than 500-ing).
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if m != nil {
			ids = append(ids, m.UserID)
		}
	}
	usersByID := map[string]*types.User{}
	if u, err := h.userSvc.GetUsersByIDs(ctx, ids); err == nil {
		usersByID = u
	} else {
		logger.Warnf(ctx, "Workspace member batch user lookup failed (tenant=%d): %v", tenantID, err)
	}

	rows := make([]EnterpriseWorkspaceMember, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		row := EnterpriseWorkspaceMember{
			UserID:   m.UserID,
			Role:     string(m.Role),
			JoinedAt: m.JoinedAt,
		}
		if u := usersByID[m.UserID]; u != nil {
			row.EmployeeID = u.EmployeeID
			row.Username = u.Username
			row.Email = u.Email
		}
		rows = append(rows, row)
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "members": rows})
}

// EnterpriseWorkspaceRoleRequest is the body for designating a member's
// workspace role. Only the two enterprise-visible levels are accepted.
type EnterpriseWorkspaceRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin contributor"`
}

// UpdateWorkspaceMemberRole godoc
// @Summary      Set a member's workspace role (SystemAdmin)
// @Description  Designate the member as workspace admin ("admin") or
// @Description  ordinary user ("contributor") — the two-level model driven
// @Description  from the admin console's workspace list. Demoting the last
// @Description  workspace admin is rejected with 409. Audit
// @Description  (rbac.member_role_changed) is emitted by
// @Description  TenantMemberService.UpdateRole.
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        tenant_id path string true "Workspace ID"
// @Param        user_id   path string true "User ID"
// @Param        request   body EnterpriseWorkspaceRoleRequest true "New role"
// @Success      200  {object}  EnterpriseWorkspaceMember  "Updated member"
// @Failure      400  {object}  map[string]interface{}  "Invalid request"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      404  {object}  map[string]interface{}  "Workspace, user, or membership not found"
// @Failure      409  {object}  map[string]interface{}  "Last workspace admin"
// @Router       /system/admin/tenants/{tenant_id}/members/{user_id} [put]
func (h *SystemHandler) UpdateWorkspaceMemberRole(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if !h.requireMemberService(c) {
		return
	}
	tenantID, err := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}
	if tenant, err := h.tenantSvc.GetTenantByID(ctx, tenantID); err != nil || tenant == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
		return
	}
	userID := c.Param("user_id")

	user, err := h.userSvc.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req EnterpriseWorkspaceRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role must be \"admin\" or \"contributor\""})
		return
	}
	newRole, ok := parseEnterpriseWorkspaceRole(req.Role)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role must be \"admin\" or \"contributor\""})
		return
	}

	member, err := h.memberSvc.GetMembership(ctx, userID, tenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to load membership (user=%s tenant=%d): %v", userID, tenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update member role"})
		return
	}
	if member == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User is not a member of this workspace"})
		return
	}

	// Last-admin guard: demoting (or legacy-owning) the only workspace
	// admin would leave the workspace administrator-less.
	if newRole.Level() < types.TenantRoleAdmin.Level() &&
		member.Role.Level() >= types.TenantRoleAdmin.Level() {
		hasOthers, err := h.workspaceHasOtherAdmins(ctx, tenantID, userID)
		if err != nil {
			logger.Errorf(ctx, "Failed to check workspace admins (tenant=%d): %v", tenantID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update member role"})
			return
		}
		if !hasOthers {
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot demote the last workspace admin of this workspace"})
			return
		}
	}

	if err := h.memberSvc.UpdateRole(ctx, userID, tenantID, newRole); err != nil {
		switch {
		case errors.Is(err, service.ErrMembershipNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "User is not a member of this workspace"})
		case errors.Is(err, service.ErrLastOwner):
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot demote the last owner of this workspace"})
		default:
			logger.Errorf(ctx, "Failed to update role of user %s in tenant %d: %v", userID, tenantID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update member role"})
		}
		return
	}

	logger.Infof(ctx, "System admin set workspace %d member %s role to %s", tenantID, user.ID, newRole)
	c.JSON(http.StatusOK, EnterpriseWorkspaceMember{
		UserID:     user.ID,
		EmployeeID: user.EmployeeID,
		Username:   user.Username,
		Email:      user.Email,
		Role:       string(newRole),
		JoinedAt:   member.JoinedAt,
	})
}

// ListPlatformTenants godoc
// @Summary      List all workspaces (SystemAdmin)
// @Description  Platform-level workspace catalog for the binding pickers
// @Description  in the enterprise user management UI.
// @Tags         System Admin
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "{tenants:[...]}"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Failure      500  {object}  map[string]interface{}  "Internal error"
// @Router       /system/admin/tenants [get]
func (h *SystemHandler) ListPlatformTenants(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	tenants, err := h.tenantSvc.ListAllTenants(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to list tenants: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list workspaces"})
		return
	}
	if tenants == nil {
		tenants = []*types.Tenant{}
	}
	c.JSON(http.StatusOK, gin.H{"tenants": tenants})
}

// EnterpriseCreateTenantRequest is the body for platform-level workspace
// creation. Regular users cannot create workspaces (see CreateTenant);
// the system admin provisions the shell here and binds users separately.
type EnterpriseCreateTenantRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Description    string `json:"description" binding:"max=500"`
	StorageQuotaGB *int64 `json:"storage_quota_gb" binding:"omitempty,min=1"`
}

// CreatePlatformTenant godoc
// @Summary      Create a workspace (SystemAdmin)
// @Description  Provision a workspace shell without any membership —
// @Description  users are attached via the bindings endpoints. Storage
// @Description  quota defaults to the system setting when omitted.
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        request body EnterpriseCreateTenantRequest true "Workspace info"
// @Success      201  {object}  types.Tenant  "Workspace created"
// @Failure      400  {object}  map[string]interface{}  "Invalid request"
// @Failure      403  {object}  map[string]interface{}  "Forbidden: not a system admin"
// @Router       /system/admin/tenants [post]
func (h *SystemHandler) CreatePlatformTenant(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	var req EnterpriseCreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace creation request"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Workspace name is required"})
		return
	}

	tenant := &types.Tenant{
		Name:        secutils.SanitizeForLog(req.Name),
		Description: secutils.SanitizeForLog(req.Description),
		Status:      "active",
	}
	if req.StorageQuotaGB != nil && *req.StorageQuotaGB > 0 {
		tenant.StorageQuota = *req.StorageQuotaGB * 1024 * 1024 * 1024
	} else if gb := h.systemSettingSvc.GetInt(
		ctx,
		"tenant.default_storage_quota_gb",
		"WEKNORA_TENANT_DEFAULT_STORAGE_QUOTA_GB",
		10,
	); gb > 0 {
		tenant.StorageQuota = gb * 1024 * 1024 * 1024
	}

	created, err := h.tenantSvc.CreateTenant(ctx, tenant)
	if err != nil {
		logger.Errorf(ctx, "Failed to create workspace %q: %v", req.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create workspace"})
		return
	}
	logger.Infof(ctx, "System admin created workspace ID: %d, name: %s", created.ID, secutils.SanitizeForLog(created.Name))
	c.JSON(http.StatusCreated, created)
}
