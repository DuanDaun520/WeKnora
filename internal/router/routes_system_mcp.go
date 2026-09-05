package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminMCPRoutes registers the platform MCP governance
// endpoints of the admin console (000096 MCP platform rework): the platform
// MCP service catalog CRUD, the workspace assignments of a service, the
// connection test, the credential subresource, and the platform-default
// tool-approval policy.
//
// Everything lives under /system/admin behind g.SystemAdmin() — a system
// admin may have no workspace binding, so these handlers never read the
// tenant from the request context.
//
// The legacy tenant-scoped /api/v1/mcp-services* routes stay registered
// (routes_infra.go): reads (Viewer+) resolve a workspace's assigned + builtin
// services for chat and agent editors, and the SystemAdmin-gated writes keep
// serving as the platform-API-key surface (manage_mcp). Per-user OAuth
// (authorize/status/revoke) stays tenant-only — it needs a workspace +
// principal context.
func RegisterSystemAdminMCPRoutes(
	r *gin.RouterGroup,
	h *handler.SystemMCPServiceHandler,
	credHandler *handler.MCPCredentialsHandler,
	mcpHandler *handler.MCPServiceHandler,
	g *rbacGuards,
) {
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		// Platform MCP service catalog.
		adminRoutes.GET("/mcp-services", h.ListServices)
		adminRoutes.POST("/mcp-services", h.CreateService)
		adminRoutes.GET("/mcp-services/:id", h.GetService)
		adminRoutes.PUT("/mcp-services/:id", h.UpdateService)
		adminRoutes.DELETE("/mcp-services/:id", h.DeleteService)
		adminRoutes.POST("/mcp-services/:id/test", h.TestService)

		// Credential subresource — MCPCredentialsHandler treats a missing
		// workspace context as platform scope (also mounted on the legacy
		// group for API keys).
		adminRoutes.PUT("/mcp-services/:id/credentials", credHandler.Put)
		adminRoutes.DELETE("/mcp-services/:id/credentials/:field", credHandler.DeleteField)

		// Workspace assignments of a service (console service drawer).
		adminRoutes.GET("/mcp-services/:id/tenant-assignments", h.ListTenantAssignments)
		adminRoutes.PUT("/mcp-services/:id/tenant-assignments", h.UpdateTenantAssignments)

		// Platform-default tool-approval policy (tenant_id = 0 rows). The
		// same MCPServiceHandler methods serve the tenant route with a
		// workspace override; runtime resolution prefers the workspace row
		// and falls back to the platform row.
		adminRoutes.GET("/mcp-services/:id/tool-approvals", mcpHandler.ListMCPToolApprovals)
		adminRoutes.PUT("/mcp-services/:id/tool-approvals/:tool_name", mcpHandler.SetMCPToolApproval)
	}
}
