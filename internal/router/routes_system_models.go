package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminModelRoutes registers the platform model-governance
// endpoints of the admin console (000094 enterprise model rework): the
// platform model catalog CRUD, the workspace model assignments, and
// SystemAdmin mirrors of the Ollama / connection-test probes that the
// console's model editor needs.
//
// Everything lives under /system/admin behind g.SystemAdmin() — a system
// admin may have no workspace binding, so these handlers never read the
// tenant from the request context. The mirrors reuse the existing
// InitializationHandler methods verbatim: those methods only touch the
// process-wide Ollama singleton / remote endpoints and are
// tenant-context-free.
//
// The legacy tenant-scoped /api/v1/models* write routes stay registered
// (routes_infra.go) but are SystemAdmin-gated as well; their remaining
// value is the platform-API-key surface. Reads stay Viewer+ so workspace
// model pickers keep working unchanged.
func RegisterSystemAdminModelRoutes(
	r *gin.RouterGroup,
	modelHandler *handler.ModelHandler,
	modelAdmin *handler.SystemModelHandler,
	initHandler *handler.InitializationHandler,
	g *rbacGuards,
) {
	adminRoutes := r.Group("/system/admin", g.SystemAdmin())
	{
		// Platform model catalog.
		adminRoutes.GET("/models", modelAdmin.ListModels)
		adminRoutes.POST("/models", modelAdmin.CreateModel)
		// Static /providers segment must be registered before /:id siblings;
		// gin's radix tree also resolves it regardless, keep it first anyway.
		adminRoutes.GET("/models/providers", modelHandler.ListModelProviders)
		adminRoutes.GET("/models/:id", modelAdmin.GetModel)
		adminRoutes.PUT("/models/:id", modelAdmin.UpdateModel)
		adminRoutes.DELETE("/models/:id", modelAdmin.DeleteModel)
		adminRoutes.POST("/models/:id/debug", modelHandler.DebugModel)
		adminRoutes.PUT("/models/:id/credentials", modelAdmin.PutCredentials)
		adminRoutes.DELETE("/models/:id/credentials/:field", modelAdmin.DeleteCredentialField)

		// Workspace model assignments (workspace row action in the console).
		adminRoutes.GET("/tenants/:tenant_id/model-assignments", modelAdmin.ListTenantModelAssignments)
		adminRoutes.PUT("/tenants/:tenant_id/model-assignments", modelAdmin.UpdateTenantModelAssignments)

		// Ollama runtime mirrors for the console panel.
		adminRoutes.GET("/ollama/status", initHandler.CheckOllamaStatus)
		adminRoutes.GET("/ollama/models", initHandler.ListOllamaModels)
		adminRoutes.POST("/ollama/models/check", initHandler.CheckOllamaModels)
		adminRoutes.POST("/ollama/models/download", initHandler.DownloadOllamaModel)
		adminRoutes.GET("/ollama/download/progress/:taskId", initHandler.GetDownloadProgress)
		adminRoutes.GET("/ollama/download/tasks", initHandler.ListDownloadTasks)

		// Connection-test mirrors used by the console's model editor /
		// debug drawer (probe external model endpoints with raw or stored
		// credentials).
		adminRoutes.POST("/initialization/remote/check", initHandler.CheckRemoteModel)
		adminRoutes.POST("/initialization/embedding/test", initHandler.TestEmbeddingModel)
		adminRoutes.POST("/initialization/rerank/check", initHandler.CheckRerankModel)
		adminRoutes.POST("/initialization/asr/check", initHandler.CheckASRModel)
		adminRoutes.POST("/initialization/multimodal/test", initHandler.TestMultimodalFunction)
	}
}
