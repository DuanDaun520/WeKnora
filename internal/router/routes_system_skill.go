package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminSkillRoutes registers the platform skill library
// endpoints of the admin console (000098): the platform skill catalog CRUD
// (register from zip or source, re-register updates with an immutable name),
// the workspace assignments of a skill (each assignment materializes an
// ordinary tenant_skill_catalog row in the workspace's own storage), the
// manual push that propagates skill edits, and the file browser of the stored
// platform bundle.
//
// Everything lives under /system/admin behind g.SystemAdmin() — a system admin
// may have no workspace binding, so these handlers never read the tenant from
// the request context.
//
// Deliberately NOT mounted on the apiKeyGroup: API keys default-deny, and
// there is no platform-API-key management surface for the skill library. The
// workspace-scoped /api/v1/skills* routes (routes_skill.go) keep serving
// workspaces unchanged.
func RegisterSystemAdminSkillRoutes(
	r *gin.RouterGroup,
	h *handler.SystemSkillHandler,
	g *rbacGuards,
) {
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		// Platform skill catalog. The collection root is dual-content
		// (multipart zip or {"source":...}); gin matches static segments
		// first, same shape as the sandbox-connection routes.
		adminRoutes.GET("/skills", h.ListSkills)
		adminRoutes.POST("/skills", h.CreateSkill)
		adminRoutes.GET("/skills/:id", h.GetSkill)
		adminRoutes.PUT("/skills/:id", h.UpdateSkill)
		adminRoutes.DELETE("/skills/:id", h.DeleteSkill)

		// File browser of the stored platform bundle (drawer SKILL.md view).
		adminRoutes.GET("/skills/:id/files", h.ListSkillFiles)
		adminRoutes.GET("/skills/:id/files/content", h.GetSkillFile)

		// Workspace assignments of a skill (console skill drawer).
		adminRoutes.GET("/skills/:id/tenant-assignments", h.ListTenantAssignments)
		adminRoutes.PUT("/skills/:id/tenant-assignments", h.UpdateTenantAssignments)

		// Manual propagation of skill edits to every assignment. Never
		// installs onto a sandbox — that stays a per-workspace decision.
		adminRoutes.POST("/skills/:id/push", h.PushSkill)
	}
}
