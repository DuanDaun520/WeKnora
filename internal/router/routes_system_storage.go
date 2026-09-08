package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterPlatformStorageEngineRoutes registers platform-level storage engine
// management routes. These are admin-only routes for managing storage engines
// that can be assigned to workspaces.
func RegisterPlatformStorageEngineRoutes(
	r *gin.RouterGroup,
	h *handler.PlatformStorageEngineHandler,
	g *rbacGuards,
) {
	engines := r.Group("/platform/storage-engines")
	{
		// All operations are SystemAdmin-only (platform-level resource)
		engines.GET("/types", g.SystemAdmin(), h.Types)
		engines.POST("/test", g.SystemAdmin(), h.TestRaw)
		engines.POST("", g.SystemAdmin(), h.Create)
		engines.GET("", g.SystemAdmin(), h.List)
		engines.GET("/:id", g.SystemAdmin(), h.Get)
		engines.PUT("/:id", g.SystemAdmin(), h.Update)
		engines.DELETE("/:id", g.SystemAdmin(), h.Delete)
		engines.POST("/:id/test", g.SystemAdmin(), h.TestByID)
	}

	// Tenant storage engine assignment routes - under /system/admin to avoid
	// conflict with existing /tenants/:id routes
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		adminRoutes.PUT("/tenants/:tenant_id/storage-engine", h.AssignToTenant)
		adminRoutes.DELETE("/tenants/:tenant_id/storage-engine", h.UnassignFromTenant)
	}
}