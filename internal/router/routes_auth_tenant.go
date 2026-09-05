package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
)

// RegisterTenantRoutes 注册空间相关的路由
//
// Tenant-internal RBAC for /tenants/:id after the enterprise user-system
// rework (roles flattened to a single "admin" rank inside a workspace):
//   - GET   /:id          Viewer+ (read tenant settings — equal rights)
//   - PUT   /:id          SystemAdmin only (tenant config is a platform
//     admin surface; workspace members cannot re-shape
//     quotas/status that the sysadmin owns)
//   - DELETE /:id         SystemAdmin only (deleting a workspace affects
//     the whole platform)
//   - GET/POST/PUT/DELETE /:id/api-keys, api-principal-*  Admin+ via
//     AdminOrSystemAdmin (flat members manage their
//     integration credentials; the fallback lets a
//     tenantless system admin operate platform tenants)
//   - GET    /:id/members            Viewer+ (read-only roster; binding
//     changes go through /system/admin/users/:id/bindings)
//
// Member mutation (add/re-role/remove/leave) and the invitation flow were
// removed with the rework: user↔workspace binding is configured by the
// system admin exclusively.
//
// All /tenants/:id endpoints share g.PathTenantMatch() at the group
// level: middleware/access.go enforces "URL :id == active tenant"
// (with the cross-tenant superuser carve-out) so a member of A cannot
// drive operations against tenant B by changing the URL. This used to
// be authorizeTenantAccess in tenant.go and resolveTenantIDFromPath in
// tenant_member.go; collapsing it into one route guard means the
// declaration itself documents the rule.
//
// Cross-tenant superuser endpoints (/tenants/all, /tenants/search) use
// g.CrossTenant(): RequireCrossTenantAccess in access.go combines the
// CanAccessAllTenants user attribute with the cluster-wide
// EnableCrossTenantAccess flag, replacing the 12-line if-block that
// previously opened ListAllTenants and SearchTenants.
//
// JWT behavior for POST /tenants and GET /tenants remains unchanged. Platform
// API keys may create tenants through system_tenants_manage; workspace keys
// remain default-denied on tenant-catalog operations.
func RegisterTenantRoutes(
	r *gin.RouterGroup,
	handler *handler.TenantHandler,
	memberHandler *handler.TenantMemberHandler,
	auditLogHandler *handler.AuditLogHandler,
	usageReportHandler *handler.UsageReportHandler,
	g *rbacGuards,
) {
	// Cross-tenant superuser endpoints — promoted from handler if-blocks
	// to middleware.RequireCrossTenantAccess at the route layer.
	g.apiKeyRoute(r, http.MethodGet, "/tenants/all",
		apiKeyPlatform(types.APIKeyCapabilitySystemTenantsRead, types.APIKeyCapabilitySystemTenantsManage),
		g.CrossTenant(), handler.ListAllTenants)
	g.apiKeyRoute(r, http.MethodGet, "/tenants/search",
		apiKeyPlatform(types.APIKeyCapabilitySystemTenantsRead, types.APIKeyCapabilitySystemTenantsManage),
		g.CrossTenant(), handler.SearchTenants)

	// 空间路由组
	tenantRoutes := r.Group("/tenants")
	{
		// 创建空间仅系统管理员（handler 内部按 IsSystemAdmin 收口）；
		// platform API key 经 system_tenants_manage 能力走同一端点。
		// 普通用户的自助开户已在企业化改造中移除。
		g.apiKeyRoute(tenantRoutes, http.MethodPost, "",
			apiKeyPlatform(types.APIKeyCapabilitySystemTenantsManage), handler.CreateTenant)
		g.apiKeyRoute(tenantRoutes, http.MethodGet, "", apiKeyManageTenantSettings(apiKeyFullAccess()), handler.ListTenants)

		// Generic KV configuration management (tenant-level). Tenant ID
		// is obtained from authentication context; the URL :key is a
		// config key, not a tenant ID, so these stay outside the
		// PathTenantMatch group. Tenant-level surface: full-access keys may
		// call it, and scoped keys need manage_tenant_settings. PUT is
		// AdminOrSystemAdmin: since the admin-console 按空间代管 migration a
		// system admin manages the integration keys (parser-engine-config
		// etc., gated further in-handler via CanManageIntegrationSecrets)
		// through X-Tenant-ID with a Viewer tenant role, while the remaining
		// keys keep their tenant-Admin semantics.
		g.apiKeyRoute(tenantRoutes, http.MethodGet, "/kv/:key", apiKeyManageTenantSettings(apiKeyFullAccess()), g.Viewer(), handler.GetTenantKV)
		g.apiKeyRoute(tenantRoutes, http.MethodPut, "/kv/:key", apiKeyManageTenantSettings(apiKeyFullAccess()), g.AdminOrSystemAdmin(), handler.UpdateTenantKV)

		// Per-tenant endpoints share PathTenantMatch at the group level.
		// Most /tenants/:id/* endpoints stay undeclared for API keys by
		// default — tenant lifecycle and key/principal management require
		// full tenant access or JWT ownership. Member/invitation management
		// opts in below through the manage_members capability.
		tenantByID := tenantRoutes.Group("/:id", g.PathTenantMatch())
		{
			g.apiKeyRoute(tenantByID, http.MethodGet, "",
				apiKeyPlatform(types.APIKeyCapabilitySystemTenantsRead, types.APIKeyCapabilitySystemTenantsManage),
				g.Viewer(), handler.GetTenant)
			// 空间级配置/删除是平台管理面：仅系统管理员（或 platform key）。
			g.apiKeyRoute(tenantByID, http.MethodPut, "",
				apiKeyPlatform(types.APIKeyCapabilitySystemTenantsManage), g.SystemAdmin(), handler.UpdateTenant)
			g.apiKeyRoute(tenantByID, http.MethodDelete, "",
				apiKeyPlatform(types.APIKeyCapabilitySystemTenantsManage), g.SystemAdmin(), handler.DeleteTenant)
			// 集成凭证管理随角色扁平化放宽到 Admin+；AdminOrSystemAdmin
			// 兜底覆盖未绑定该空间的系统管理员。
			tenantByID.GET("/api-keys", g.AdminOrSystemAdmin(), handler.ListAPIKeys)
			tenantByID.POST("/api-keys", g.AdminOrSystemAdmin(), handler.CreateAPIKey)
			tenantByID.PUT("/api-keys/:key_id", g.AdminOrSystemAdmin(), handler.UpdateAPIKey)
			tenantByID.DELETE("/api-keys/:key_id", g.AdminOrSystemAdmin(), handler.DeleteAPIKey)
			tenantByID.GET("/api-principal-config", g.AdminOrSystemAdmin(), handler.GetAPIPrincipalConfig)
			tenantByID.PUT("/api-principal-config", g.AdminOrSystemAdmin(), handler.UpdateAPIPrincipalConfig)
			tenantByID.POST("/api-principal-test-token", g.AdminOrSystemAdmin(), handler.CreateAPIPrincipalTestToken)

			// 成员名册只读（PR 3 of #1303 保留的列表面）。企业化改造后
			// 用户与空间的绑定统一由系统管理员经
			// /system/admin/users/:user_id/bindings 配置；000101 成员管理
			// 弹窗为空间管理员重开了一个收窄的管理面（加人/重置密码/
			// 移出/统计），角色调整与自助退出仍不开放。
			if memberHandler != nil {
				g.apiKeyRoute(tenantByID, http.MethodGet, "/members", apiKeyManageMembers(apiKeyFullAccess()), g.Viewer(), memberHandler.ListMembers)
				// 000101 空间管理员成员管理：Admin+（PathTenantMatch 已在
				// 组级保证 :id 即调用者所在空间）；API-key 侧沿用
				// manage_members 能力（全量 key 或受权 key）。
				g.apiKeyRoute(tenantByID, http.MethodPost, "/members", apiKeyManageMembers(apiKeyFullAccess()), g.Admin(), memberHandler.AddTenantMember)
				g.apiKeyRoute(tenantByID, http.MethodDelete, "/members/:user_id", apiKeyManageMembers(apiKeyFullAccess()), g.Admin(), memberHandler.RemoveTenantMember)
				g.apiKeyRoute(tenantByID, http.MethodPost, "/members/:user_id/reset-password", apiKeyManageMembers(apiKeyFullAccess()), g.Admin(), memberHandler.ResetTenantMemberPassword)
				g.apiKeyRoute(tenantByID, http.MethodGet, "/members/:user_id/stats", apiKeyManageMembers(apiKeyFullAccess()), g.Admin(), memberHandler.GetTenantMemberStats)
			}

			// Audit log feed (PR 6 of #1303). Admin+ so denied-action
			// histories don't surface to ordinary members; the
			// PathTenantMatch group already prevents cross-tenant
			// reads. nil-skip mirrors the memberHandler pattern above
			// for environments wired without the audit dependency.
			if auditLogHandler != nil {
				tenantByID.GET("/audit-log", g.Admin(), auditLogHandler.ListTenantAuditLog)
			}

			// Space-level usage report (space-admin panel,
			// docs/Token统计与计费设计.md §5.2). PathTenantMatch above
			// already pins the caller to their own space.
			registerTenantUsageRoute(tenantByID, usageReportHandler, g)
		}
	}
}

// RegisterMyEnvVarRoutes wires the caller's own environment variables under
// /me/env-vars. The v1 group already applies middleware.Auth, and no role gate
// is added on purpose: these are the caller's own values, and the service
// derives whose they are from the context rather than the request.
//
// This deliberately does not reuse /sandbox-configs/:id/skills*, which is
// Admin+ even for reads (see routes_infra.go): an upload there drives a root
// shell whose output is baked into the image, and the listing names what that
// image carries. This endpoint returns declarations and set/unset status only.
//
// h may be nil in environments built without the dependency wired; a no-op
// registration is preferable to a startup crash, as with the invitation inbox.
func RegisterMyEnvVarRoutes(r *gin.RouterGroup, h *handler.MeEnvVarHandler) {
	if h == nil {
		return
	}
	me := r.Group("/me/env-vars")
	{
		me.GET("", h.List)
		me.PUT("/skill", h.SetSkill)
		me.DELETE("/skill", h.DeleteSkill)
		me.PUT("/sandbox", h.SetSandbox)
		me.DELETE("/sandbox", h.DeleteSandbox)
	}
}

// RegisterAuthRoutes registers authentication routes.
//
// Enterprise rework: self-service registration, invite-link registration,
// OIDC and Lite AutoSetup are gone — accounts are provisioned exclusively
// by the system admin via /system/admin/users, and login is
// employee_id + password. /auth/login is the only unauthenticated
// mutation left (plus token refresh/validate/logout), so the public
// rate limiter that used to guard the share-link endpoints retired
// with them.
func RegisterAuthRoutes(r *gin.RouterGroup, handler *handler.AuthHandler, g *rbacGuards) {
	r.POST("/auth/login", handler.Login)
	r.POST("/auth/switch-tenant", handler.SwitchTenant)
	r.POST("/auth/refresh", handler.RefreshToken)
	r.GET("/auth/validate", handler.ValidateToken)
	r.POST("/auth/logout", handler.Logout)
	// auth/me returns only the caller's own identity/profile, so it is safe
	// for any valid API key. Chat clients / MCP call it to discover "who am I";
	// leaving it default-deny was why scoped keys got a 403 here.
	g.apiKeyRoute(r, http.MethodGet, "/auth/me", apiKeyAny(), handler.GetCurrentUser)
	r.PUT("/auth/me/preferences", handler.UpdateMyPreferences)
	r.POST("/auth/change-password", handler.ChangePassword)
}

// RegisterSystemRoutes registers system information routes
//
// Reads (GetSystemInfo / ListParserEngines / GetStorageEngineStatus)
// are gated to Viewer+ — any tenant member can see "is the parser
// reachable". The /*-check / /reconnect endpoints actively probe
// remote services with tenant credentials and could trigger network
// fanout; since the admin-console 按空间代管 migration they are
// SystemAdmin-only (the console drives them through X-Tenant-ID).
func RegisterSystemRoutes(
	r *gin.RouterGroup,
	handler *handler.SystemHandler,
	g *rbacGuards,
) {
	systemRoutes := g.apiKeyGroup(r.Group("/system"), apiKeyManageVectorStores(apiKeyFullAccess()))
	{
		systemRoutes.With(apiKeyAny()).GET("/capabilities", g.Viewer(), handler.GetDeploymentCapabilities)
		systemRoutes.GET("/info", g.Viewer(), handler.GetSystemInfo)
		systemRoutes.GET("/parser-engines", g.Viewer(), handler.ListParserEngines)
		systemRoutes.POST("/parser-engines/check", g.SystemAdmin(), handler.CheckParserEngines)
		systemRoutes.POST("/docreader/reconnect", g.SystemAdmin(), handler.ReconnectDocReader)
		systemRoutes.GET("/storage-engine-status", g.Viewer(), handler.GetStorageEngineStatus)
		systemRoutes.POST("/storage-engine-check", g.SystemAdmin(), handler.CheckStorageEngine)
		systemRoutes.POST("/sandbox-check", g.SystemAdmin(), handler.CheckSandboxConfig)
	}
}

// RegisterSystemAdminRoutes registers system administration routes.
//
// All endpoints under this group are gated to SystemAdmin users (i.e.
// User.IsSystemAdmin == true). These are platform-wide operations
// independent of per-tenant Owner/Admin/Contributor/Viewer roles —
// they let org-level superusers grant/revoke system-admin status and,
// in later milestones, will host global settings, built-in models, and
// cross-tenant observability.
//
// Mounted under /api/v1/system/admin/* so the URL scheme stays aligned
// with the existing /api/v1/system/info family. Front-end clients live
// in frontend/src/api/system/index.ts.
//
// auditLogHandler may be nil in environments wired without the audit
// dependency; the /audit-log subroute is then omitted. This mirrors
// the optional wiring in RegisterTenantRoutes.
func RegisterSystemAdminRoutes(
	r *gin.RouterGroup,
	handler *handler.SystemHandler,
	auditLogHandler *handler.AuditLogHandler,
	g *rbacGuards,
) {
	// Apply SystemAdmin() at the group level — every route below inherits
	// the guard, so adding new endpoints can't accidentally drop the gate.
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		// P0: SystemAdmin role management
		adminRoutes.POST("/promote", handler.PromoteUserToSystemAdmin)
		adminRoutes.POST("/revoke", handler.RevokeSystemAdmin)
		adminRoutes.GET("/list", handler.ListSystemAdmins)
		adminRoutes.POST("/users/reset-password", handler.ResetUserPassword)
		adminRoutes.POST("/users/create", handler.CreateSystemUser)

		// Enterprise user & workspace management (user-system rework):
		// paginated user list with workspace tags, provisioning with
		// initial bindings, profile/lifecycle updates, per-user password
		// reset, user↔workspace binding management, and the platform
		// workspace catalog. Static siblings (/users/create,
		// /users/reset-password) take precedence over the :user_id
		// wildcard in gin's radix tree, so the legacy email-era routes
		// keep resolving until Phase 5 removes them.
		adminRoutes.GET("/users", handler.ListEnterpriseUsers)
		adminRoutes.POST("/users", handler.CreateEnterpriseUser)
		adminRoutes.PUT("/users/:user_id", handler.UpdateEnterpriseUser)
		adminRoutes.POST("/users/:user_id/reset-password", handler.ResetEnterpriseUserPassword)
		adminRoutes.GET("/users/:user_id/bindings", handler.ListUserBindings)
		adminRoutes.POST("/users/:user_id/bindings", handler.BindUserToTenant)
		adminRoutes.DELETE("/users/:user_id/bindings/:tenant_id", handler.UnbindUserFromTenant)
		adminRoutes.GET("/tenants", handler.ListPlatformTenants)
		adminRoutes.POST("/tenants", handler.CreatePlatformTenant)
		// Two-level workspace roles: the workspace member roster and the
		// 空间管理员/普通用户 designation surface of the admin console.
		adminRoutes.GET("/tenants/:tenant_id/members", handler.ListWorkspaceMembers)
		adminRoutes.PUT("/tenants/:tenant_id/members/:user_id", handler.UpdateWorkspaceMemberRole)

		adminRoutes.GET("/api-keys", handler.ListPlatformAPIKeys)
		adminRoutes.POST("/api-keys", handler.CreatePlatformAPIKey)
		adminRoutes.DELETE("/api-keys/:key_id", handler.DeletePlatformAPIKey)

		// P1: platform-wide system settings (DB-backed runtime tunables).
		// Reads return raw model rows / arrays (no `gin.H{"data":...}`
		// wrapping), matching the project's axios interceptor convention
		// — see frontend/src/utils/request.ts:97.
		g.apiKeyRoute(adminRoutes, http.MethodGet, "/settings",
			apiKeyPlatform(types.APIKeyCapabilitySystemSettingsRead, types.APIKeyCapabilitySystemSettingsManage),
			handler.ListSystemSettings)
		g.apiKeyRoute(adminRoutes, http.MethodGet, "/settings/:key",
			apiKeyPlatform(types.APIKeyCapabilitySystemSettingsRead, types.APIKeyCapabilitySystemSettingsManage),
			handler.GetSystemSetting)
		g.apiKeyRoute(adminRoutes, http.MethodPut, "/settings/:key",
			apiKeyPlatform(types.APIKeyCapabilitySystemSettingsManage), handler.UpdateSystemSetting)
		g.apiKeyRoute(adminRoutes, http.MethodDelete, "/settings/:key",
			apiKeyPlatform(types.APIKeyCapabilitySystemSettingsManage), handler.ResetSystemSetting)

		// Runtime operations: live asynq queue depths, safe task projections,
		// and state-checked task actions for the SystemAdmin dashboard. Lite
		// mode returns available=false.
		g.apiKeyRoute(adminRoutes, http.MethodGet, "/runtime/queues",
			apiKeyPlatform(types.APIKeyCapabilitySystemRuntimeRead, types.APIKeyCapabilitySystemRuntimeManage),
			handler.GetRuntimeQueues)
		g.apiKeyRoute(adminRoutes, http.MethodGet, "/runtime/queues/:queue/tasks",
			apiKeyPlatform(types.APIKeyCapabilitySystemRuntimeRead, types.APIKeyCapabilitySystemRuntimeManage),
			handler.ListRuntimeTasks)
		g.apiKeyRoute(adminRoutes, http.MethodPost, "/runtime/queues/:queue/tasks/:task_id/actions/:action",
			apiKeyPlatform(types.APIKeyCapabilitySystemRuntimeManage), handler.MutateRuntimeTask)
		g.apiKeyRoute(adminRoutes, http.MethodDelete, "/runtime/queues/:queue/archived",
			apiKeyPlatform(types.APIKeyCapabilitySystemRuntimeManage), handler.PurgeArchivedRuntimeTasks)

		// Bulk action — write the current default-quota setting onto
		// every existing tenant. Lives under /tenants instead of
		// /settings because it changes tenants, not the setting row.
		g.apiKeyRoute(adminRoutes, http.MethodPost, "/tenants/apply-default-storage-quota",
			apiKeyPlatform(types.APIKeyCapabilitySystemTenantsManage),
			handler.ApplyDefaultStorageQuotaToAllTenants)

		// Platform-wide audit feed (tenant_id=0 rows). Covers
		// system.setting_changed / system.admin_promoted /
		// system.admin_revoked etc. — events written by the routes
		// above. Without this endpoint those audit rows would have
		// no UI surface (per-tenant ListTenantAuditLog filters them
		// out by tenant_id). Optional: skip when audit deps are
		// absent, matching RegisterTenantRoutes' /audit-log handling.
		if auditLogHandler != nil {
			g.apiKeyRoute(adminRoutes, http.MethodGet, "/audit-log",
				apiKeyPlatform(types.APIKeyCapabilitySystemAuditRead), auditLogHandler.ListSystemAuditLog)
		}
	}
}
