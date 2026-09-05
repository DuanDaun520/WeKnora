package handler

import (
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// TenantMemberHandler exposes the member roster under /tenants/:id/members.
//
// History: the enterprise user-system rework had removed every member
// mutation route (binding users to workspaces was system-admin-only via
// /system/admin/users/:user_id/bindings). Migration 000101 reopens a
// deliberately narrowed tenant-side surface for the 空间管理员 member
// management dialog: add (工号+姓名+默认密码, existing platform accounts
// are simply bound), reset password (8-digit), remove (admins/self are
// protected), and per-member stats. All of it is Admin+ on the
// PathTenantMatch group; re-roleing and self-leave stay closed.
//
// Tenant scoping: the auth middleware resolves the caller's role against
// the *active* tenant (JWT / X-Tenant-ID switch / API-key). The URL :id
// is independent and MUST be cross-checked: a member of tenant A could
// otherwise GET /tenants/B/members and have the role gate happily accept
// their tenant-A role for an operation that targets tenant B. That
// cross-check now lives in middleware.RequirePathTenantMatch (mounted at
// the /tenants/:id route group); by the time a request reaches the
// method below, :id is guaranteed to either match the active tenant or
// carry a cross-tenant superuser bypass.
type TenantMemberHandler struct {
	memberService interfaces.TenantMemberService
	userService   interfaces.UserService
}

// NewTenantMemberHandler wires the dependencies. PR 1 already provides
// both services through the dig container; we just consume them. The
// previous *config.Config argument was removed once
// middleware.RequirePathTenantMatch took over the cross-tenant
// superuser carve-out.
func NewTenantMemberHandler(
	memberService interfaces.TenantMemberService,
	userService interfaces.UserService,
) *TenantMemberHandler {
	return &TenantMemberHandler{
		memberService: memberService,
		userService:   userService,
	}
}

// parseTenantIDFromPath reads :id from the gin route and validates it as
// a tenant ID. Returning (0, false) means we already wrote the error to
// the gin context and the caller should `return` immediately.
func parseTenantIDFromPath(c *gin.Context) (uint64, bool) {
	raw := strings.TrimSpace(c.Param("id"))
	if raw == "" {
		c.Error(apperrors.NewValidationError("workspace id is required"))
		return 0, false
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		c.Error(apperrors.NewValidationError("workspace id must be a positive integer"))
		return 0, false
	}
	return v, true
}

// ListMembers godoc
// @Summary      列出空间成员
// @Description  分页返回当前空间内 active 成员（含每位成员的工号、姓名、邮箱、头像）；支持 q 按工号/姓名/邮箱筛选
// @Tags         空间成员
// @Produce      json
// @Param        id         path   string  true   "空间 ID"
// @Param        q          query  string  false  "按工号/姓名/邮箱模糊筛选"
// @Param        page       query  int     false  "页码（从 1 起）"  default(1)
// @Param        page_size  query  int     false  "每页数量（最大 100）"  default(20)
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /tenants/{id}/members [get]
func (h *TenantMemberHandler) ListMembers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	page, pageSize, ok := parseListPagination(c)
	if !ok {
		return
	}

	members, total, err := h.memberService.ListMembersPage(ctx, tenantID, q, page, pageSize)
	if err != nil {
		logger.Errorf(ctx, "ListMembersPage failed: tenant=%d err=%v", tenantID, err)
		c.Error(apperrors.NewInternalServerError("failed to list members").WithDetails(err.Error()))
		return
	}

	// Hydrate user-facing fields in one batched query. Before this we
	// did N+1 GetUserByID calls; tenants with hundreds of members
	// pressed the user repo hard for no good reason. Failure is
	// best-effort — a transient batch error degrades to "no employee
	// id / name on this page" rather than dropping rows, so dangling
	// memberships can still be seen and cleaned up by the admin.
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	usersByID := map[string]*types.User{}
	if u, err := h.userService.GetUsersByIDs(ctx, ids); err == nil {
		usersByID = u
	} else {
		logger.Warnf(ctx, "ListMembers batch user lookup failed: tenant=%d err=%v", tenantID, err)
	}

	resp := make([]types.TenantMemberResponse, 0, len(members))
	for _, m := range members {
		row := types.TenantMemberResponse{
			UserID:    m.UserID,
			Role:      m.Role,
			Status:    m.Status,
			InvitedBy: m.InvitedBy,
			JoinedAt:  m.JoinedAt,
		}
		if u, ok := usersByID[m.UserID]; ok && u != nil {
			row.EmployeeID = u.EmployeeID
			row.Email = u.Email
			row.Username = u.Username
			row.Avatar = u.Avatar
			row.LastLoginAt = u.LastLoginAt
		}
		resp = append(resp, row)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"members":   resp,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// memberDefaultPassword is the fixed initial password handed out by the
// 添加成员 dialog (000101). Passed through AdminCreateUser so the normal
// password policy still applies; users must rotate it at first login.
const memberDefaultPassword = "abc1234#"

// AddTenantMemberRequest is the body of POST /tenants/:id/members.
type AddTenantMemberRequest struct {
	EmployeeID string `json:"employee_id" binding:"required,min=1,max=64"`
	Username   string `json:"username"    binding:"required,min=2,max=50"`
	// Password is optional; omitted ⇒ memberDefaultPassword.
	Password string `json:"password" binding:"omitempty,min=6,max=64"`
}

// buildMemberResponse hydrates one TenantMemberResponse from the member
// row plus its user record (nil-safe: the roster degrades to bare ids).
func buildMemberResponse(m *types.TenantMember, u *types.User) types.TenantMemberResponse {
	row := types.TenantMemberResponse{
		UserID:     m.UserID,
		Role:       m.Role,
		Status:     m.Status,
		InvitedBy:  m.InvitedBy,
		JoinedAt:   m.JoinedAt,
	}
	if u != nil {
		row.EmployeeID = u.EmployeeID
		row.Email = u.Email
		row.Username = u.Username
		row.Avatar = u.Avatar
		row.LastLoginAt = u.LastLoginAt
	}
	return row
}

// ensureHomeTenant points a tenantless user's home workspace (users.tenant_id)
// at the tenant they were just added to, mirroring the system-admin
// bindUserToTenantIDs behaviour: the first workspace a user joins becomes
// their login default. Best-effort — a failure only costs the convenience.
func (h *TenantMemberHandler) ensureHomeTenant(c *gin.Context, u *types.User, tenantID uint64) {
	if u == nil || u.TenantID != 0 {
		return
	}
	u.TenantID = tenantID
	if err := h.userService.UpdateUser(c.Request.Context(), u); err != nil {
		logger.Warnf(c.Request.Context(),
			"AddMember: failed to set home tenant %d for user %s: %v", tenantID, u.ID, err)
	}
}

// AddTenantMember godoc
// @Summary      添加成员（空间管理员）
// @Description  按工号添加成员：已有平台账号则将其加入本空间（角色固定为普通成员），否则创建新账号（默认密码 abc1234#，首次登录需修改）
// @Tags         空间成员
// @Accept       json
// @Produce      json
// @Param        id       path  string                 true  "空间 ID"
// @Param        request  body  AddTenantMemberRequest true  "工号 + 姓名（+可选密码）"
// @Success      201      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError  "参数错误 / 工号已存在 / 已是成员"
// @Failure      500      {object}  errors.AppError
// @Security     Bearer
// @Router       /tenants/{id}/members [post]
func (h *TenantMemberHandler) AddTenantMember(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}
	var req AddTenantMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	req.EmployeeID = strings.TrimSpace(req.EmployeeID)
	req.Username = strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	if password == "" {
		password = memberDefaultPassword
	}
	actor, _ := types.UserIDFromContext(ctx)

	created := false
	user, err := h.userService.GetUserByEmployeeID(ctx, req.EmployeeID)
	if err != nil {
		// ErrUserNotFound is the expected "no platform account yet" case —
		// fall through to provisioning. Anything else is a lookup failure.
		if !stderrors.Is(err, apprepo.ErrUserNotFound) {
			logger.Errorf(ctx, "AddTenantMember: lookup employee %s failed: %v", req.EmployeeID, err)
			c.Error(apperrors.NewInternalServerError("failed to look up user"))
			return
		}
		user = nil
	}
	if user != nil {
		// Existing platform account: only bind it into this workspace.
		if !user.IsActive {
			c.Error(apperrors.NewBadRequestError("该账号已被停用，无法添加"))
			return
		}
	} else {
		pwd := password
		user, _, err = h.userService.AdminCreateUser(ctx, &types.AdminCreateUserRequest{
			EmployeeID: req.EmployeeID,
			Username:   req.Username,
			Password:   &pwd,
		})
		if err != nil {
			// Race-lost duplicate (checked above, index-backed) and policy
			// violations are user-correctable → 400; anything else is infra.
			if stderrors.Is(err, service.ErrUserEmployeeIDExists) {
				c.Error(apperrors.NewBadRequestError("该工号已存在，请直接添加为成员"))
				return
			}
			if _, ok := apperrors.IsAppError(err); !ok {
				c.Error(apperrors.NewBadRequestError(err.Error()))
				return
			}
			c.Error(err)
			return
		}
		created = true
	}

	invitedBy := actor
	member, err := h.memberService.AddMember(ctx, user.ID, tenantID, types.TenantRoleContributor, &invitedBy)
	if err != nil {
		if stderrors.Is(err, service.ErrMembershipAlreadyExists) {
			c.Error(apperrors.NewBadRequestError("该用户已是本空间成员"))
			return
		}
		logger.Errorf(ctx, "AddTenantMember: bind user %s to tenant %d failed: %v", user.ID, tenantID, err)
		c.Error(apperrors.NewInternalServerError("failed to add member"))
		return
	}
	h.ensureHomeTenant(c, user, tenantID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"member":   buildMemberResponse(member, user),
			"created":  created,
			"password": password,
		},
	})
}

// RemoveTenantMember godoc
// @Summary      移出空间（空间管理员）
// @Description  将成员移出本空间（软删 tenant_members 行并吊销其会话）。空间管理员（含本人）不可移出，需系统管理员操作
// @Tags         空间成员
// @Param        id        path  string  true  "空间 ID"
// @Param        user_id   path  string  true  "用户 ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /tenants/{id}/members/{user_id} [delete]
func (h *TenantMemberHandler) RemoveTenantMember(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}
	target := strings.TrimSpace(c.Param("user_id"))
	if target == "" {
		c.Error(apperrors.NewBadRequestError("user id is required"))
		return
	}
	actor, _ := types.UserIDFromContext(ctx)

	membership, err := h.memberService.GetMembership(ctx, target, tenantID)
	if err != nil {
		logger.Errorf(ctx, "RemoveTenantMember: lookup failed: %v", err)
		c.Error(apperrors.NewInternalServerError("failed to look up membership"))
		return
	}
	if membership == nil {
		c.Error(apperrors.NewNotFoundError("该用户不是本空间成员"))
		return
	}
	// 管理员与本人不可由空间管理员移出（需求原话提示语）。
	if target == actor || membership.Role == types.TenantRoleAdmin || membership.Role == types.TenantRoleOwner {
		c.Error(apperrors.NewForbiddenError("此是空间管理员，需要系统管理员才可以移出"))
		return
	}
	if err := h.memberService.RemoveMember(ctx, target, tenantID); err != nil {
		logger.Errorf(ctx, "RemoveTenantMember: remove %s from tenant %d failed: %v", target, tenantID, err)
		c.Error(apperrors.NewInternalServerError("failed to remove member"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已移出空间"})
}

// ResetTenantMemberPassword godoc
// @Summary      重置成员密码（空间管理员）
// @Description  将成员密码重置为随机 8 位数字并吊销其会话；新密码仅本次返回，成员下次登录需修改密码
// @Tags         空间成员
// @Param        id        path  string  true  "空间 ID"
// @Param        user_id   path  string  true  "用户 ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /tenants/{id}/members/{user_id}/reset-password [post]
func (h *TenantMemberHandler) ResetTenantMemberPassword(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}
	target := strings.TrimSpace(c.Param("user_id"))
	actor, _ := types.UserIDFromContext(ctx)

	membership, err := h.memberService.GetMembership(ctx, target, tenantID)
	if err != nil {
		logger.Errorf(ctx, "ResetTenantMemberPassword: lookup failed: %v", err)
		c.Error(apperrors.NewInternalServerError("failed to look up membership"))
		return
	}
	if membership == nil {
		c.Error(apperrors.NewNotFoundError("该用户不是本空间成员"))
		return
	}
	if target == actor {
		c.Error(apperrors.NewForbiddenError("不能重置自己的密码，请在个人设置中修改密码"))
		return
	}
	plain, err := h.memberService.ResetMemberPassword(ctx, tenantID, target)
	if err != nil {
		logger.Errorf(ctx, "ResetTenantMemberPassword: reset %s failed: %v", target, err)
		c.Error(apperrors.NewInternalServerError("failed to reset password"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"new_password": plain},
	})
}

// GetTenantMemberStats godoc
// @Summary      成员统计（空间管理员）
// @Description  返回成员在本空间上传的知识数量与参与的会话次数
// @Tags         空间成员
// @Param        id        path  string  true  "空间 ID"
// @Param        user_id   path  string  true  "用户 ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /tenants/{id}/members/{user_id}/stats [get]
func (h *TenantMemberHandler) GetTenantMemberStats(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}
	target := strings.TrimSpace(c.Param("user_id"))

	membership, err := h.memberService.GetMembership(ctx, target, tenantID)
	if err != nil {
		logger.Errorf(ctx, "GetTenantMemberStats: lookup failed: %v", err)
		c.Error(apperrors.NewInternalServerError("failed to look up membership"))
		return
	}
	if membership == nil {
		c.Error(apperrors.NewNotFoundError("该用户不是本空间成员"))
		return
	}
	stats, err := h.memberService.GetMemberStats(ctx, tenantID, target)
	if err != nil {
		logger.Errorf(ctx, "GetTenantMemberStats: stats for %s tenant %d failed: %v", target, tenantID, err)
		c.Error(apperrors.NewInternalServerError("failed to load member stats"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}
