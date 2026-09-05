package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminSandboxConnectionRoutes registers the platform sandbox
// connection endpoints of the admin console (000097): the platform connection
// catalog CRUD, the workspace assignments of a connection (each assignment is
// a materialized tenant_sandbox_configs row), the manual push that propagates
// connection edits, and the connectivity/template probes of the connection
// drawer.
//
// Everything lives under /system/admin behind g.SystemAdmin() — a system admin
// may have no workspace binding, so these handlers never read the tenant from
// the request context.
//
// Deliberately NOT mounted on the apiKeyGroup: API keys default-deny, and
// unlike MCP there is no platform-API-key management surface for sandbox
// connections. The workspace-scoped /api/v1/sandbox-configs* routes
// (routes_infra.go) keep serving materialized rows unchanged.
func RegisterSystemAdminSandboxConnectionRoutes(
	r *gin.RouterGroup,
	h *handler.SystemSandboxConnectionHandler,
	g *rbacGuards,
) {
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		// Platform sandbox connection catalog. Static "check" and
		// "templates/query" sit next to ":id"; gin matches static segments
		// first (same shape as workspace-policy + :id on the tenant group).
		adminRoutes.GET("/sandbox-connections", h.ListConnections)
		adminRoutes.POST("/sandbox-connections", h.CreateConnection)
		adminRoutes.GET("/sandbox-connections/check", h.CheckDraftConnection)
		adminRoutes.POST("/sandbox-connections/check", h.CheckDraftConnection)
		adminRoutes.POST("/sandbox-connections/templates/query", h.QueryTemplates)
		adminRoutes.GET("/sandbox-connections/:id", h.GetConnection)
		adminRoutes.PUT("/sandbox-connections/:id", h.UpdateConnection)
		adminRoutes.DELETE("/sandbox-connections/:id", h.DeleteConnection)
		adminRoutes.POST("/sandbox-connections/:id/check", h.CheckConnection)

		// Workspace assignments of a connection (console connection drawer).
		adminRoutes.GET("/sandbox-connections/:id/tenant-assignments", h.ListTenantAssignments)
		adminRoutes.PUT("/sandbox-connections/:id/tenant-assignments", h.UpdateTenantAssignments)

		// Manual propagation of connection edits to every materialized row.
		adminRoutes.POST("/sandbox-connections/:id/push", h.PushConnection)
	}
}
