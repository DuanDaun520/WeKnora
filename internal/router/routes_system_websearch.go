package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminWebSearchRoutes registers the platform web-search
// governance endpoints of the admin console (000095 web search platform
// rework): the platform search-service catalog CRUD, the workspace
// assignments of a service, and the connection-test probes the console's
// service editor needs.
//
// Everything lives under /system/admin behind g.SystemAdmin() — a system
// admin may have no workspace binding, so these handlers never read the
// tenant from the request context.
//
// The legacy tenant-scoped /api/v1/web-search-providers* routes stay
// registered (routes_infra.go): reads (Viewer+) resolve a workspace's
// assigned services for chat and agent editors, and the SystemAdmin-gated
// writes keep serving as the platform-API-key surface (manage_web_search).
func RegisterSystemAdminWebSearchRoutes(
	r *gin.RouterGroup,
	h *handler.SystemWebSearchProviderHandler,
	credHandler *handler.WebSearchProviderCredentialsHandler,
	g *rbacGuards,
) {
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		// Platform search-service catalog. Provider-type metadata (registry
		// info for the console's form) has its own copy here so the console
		// never depends on the tenant-scoped Viewer+ /types route.
		adminRoutes.GET("/web-search-providers/types", h.ListProviderTypes)
		adminRoutes.GET("/web-search-providers", h.ListProviders)
		adminRoutes.POST("/web-search-providers", h.CreateProvider)
		// Raw-credential probe must be registered before /:id siblings;
		// gin's radix tree also resolves it regardless, keep it first anyway.
		adminRoutes.POST("/web-search-providers/test", h.TestProviderRaw)
		adminRoutes.GET("/web-search-providers/:id", h.GetProvider)
		adminRoutes.PUT("/web-search-providers/:id", h.UpdateProvider)
		adminRoutes.DELETE("/web-search-providers/:id", h.DeleteProvider)
		adminRoutes.POST("/web-search-providers/:id/test", h.TestProviderByID)

		// Credential subresource (tenant-context-free; also mounted on the
		// legacy group for API keys).
		adminRoutes.PUT("/web-search-providers/:id/credentials", credHandler.Put)
		adminRoutes.DELETE("/web-search-providers/:id/credentials/:field", credHandler.DeleteField)

		// Workspace assignments of a service (console service drawer).
		adminRoutes.GET("/web-search-providers/:id/tenant-assignments", h.ListTenantAssignments)
		adminRoutes.PUT("/web-search-providers/:id/tenant-assignments", h.UpdateTenantAssignments)
	}
}
