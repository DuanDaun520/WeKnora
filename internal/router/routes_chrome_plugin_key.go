package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterChromePluginKeyRoutes wires user-level Chrome plugin API key routes
// under /me/chrome-plugin-key. Any authenticated user can generate, query, or
// delete their own Chrome plugin key. The handler derives the owner from the
// authenticated user context, so no additional role gate is needed.
//
// The Chrome plugin key is a convenience API key with predefined capabilities
// (retrieve, chat, read_agents, ingest) and automatic knowledge base scoping
// (KBs with AllowMemberContribute=true). Naming follows "{EmployeeID}-{Username}-ApiKey".
func RegisterChromePluginKeyRoutes(r *gin.RouterGroup, h *handler.ChromePluginKeyHandler) {
	if h == nil {
		return
	}
	// r is already the /api/v1 group, so routes will be at /api/v1/me/chrome-plugin-key
	me := r.Group("/me/chrome-plugin-key")
	{
		me.GET("", h.GetKey)
		me.POST("", h.GenerateKey)
		me.DELETE("", h.DeleteKey)
	}
}