package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterUsageRoutes wires the three usage-report levels of
// docs/Token统计与计费设计.md §5.
//
//   - /me/usage/summary           — personal settings panel (every user,
//     current space, own usage only; identity from the auth context).
//   - /tenants/:tenant_id/usage/summary — space-admin panel (g.Admin() +
//     PathTenantMatch are applied by the caller in routes_auth_tenant.go;
//     scope=me narrows to the admin's own usage).
//   - /system/admin/usage/*       — platform console (g.SystemAdmin()).
//
// The endpoints are browser-session surfaces: the API-key gate (mounted on
// the v1 group) automatically denies X-API-Key principals because these
// routes are not declared through the apiKeyRoute helpers.
//
// usageHandler may be nil in environments built without the dependency
// wired; a no-op registration is preferable to a startup crash.
func RegisterUsageRoutes(r *gin.RouterGroup, usageHandler *handler.UsageReportHandler, g *rbacGuards) {
	if usageHandler == nil {
		return
	}
	me := r.Group("/me/usage")
	{
		me.GET("/summary", usageHandler.MeUsageSummary)
	}

	admin := r.Group("/system/admin", g.SystemAdmin())
	{
		admin.GET("/usage/summary", usageHandler.AdminUsageSummary)
		admin.GET("/usage/records", usageHandler.AdminUsageRecords)
		admin.GET("/model-prices", usageHandler.ListModelPrices)
		admin.PUT("/model-prices/:model_id", usageHandler.UpsertModelPrice)
		admin.DELETE("/model-prices/:model_id", usageHandler.DeleteModelPrice)
	}
}

// registerTenantUsageRoute mounts the space-level summary inside the
// tenant-scoped group built by RegisterTenantRoutes. Kept as a helper so
// that group's PathTenantMatch + role guards stay in one place.
func registerTenantUsageRoute(tenantByID *gin.RouterGroup, usageHandler *handler.UsageReportHandler, g *rbacGuards) {
	if usageHandler == nil {
		return
	}
	tenantByID.GET("/usage/summary", g.Admin(), usageHandler.TenantUsageSummary)
}
