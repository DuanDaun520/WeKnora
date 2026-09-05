package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// Models are platform-managed infrastructure since the 000094 enterprise
// model-governance rework: reads stay Viewer+ (workspace model pickers),
// every mutation is SystemAdmin-only and the admin console drives the
// platform catalog through /system/admin/models* (routes_system_models.go).
// These tenant-scoped routes remain registered so platform API keys with
// the manage_models capability keep their surface.
func RegisterModelRoutes(
	r *gin.RouterGroup,
	handler *handler.ModelHandler,
	credHandler *handler.ModelCredentialsHandler,
	g *rbacGuards,
) {
	// 模型路由组。空间级基础设施：仅完全访问（Owner）API key 可访问。
	models := g.apiKeyGroup(r.Group("/models"), apiKeyManageModels(apiKeyFullAccess()))
	{
		// 获取模型厂商列表 — Viewer+
		models.GET("/providers", g.Viewer(), handler.ListModelProviders)
		// 创建模型 — SystemAdmin（模型由系统管理员统一配置）
		models.POST("", g.SystemAdmin(), handler.CreateModel)
		// 获取模型列表 — Viewer+
		models.GET("", g.Viewer(), handler.ListModels)
		// 调试已保存模型会发起真实上游调用并产生费用 — SystemAdmin
		models.POST("/:id/debug", g.SystemAdmin(), handler.DebugModel)
		// 获取单个模型 — Viewer+
		models.GET("/:id", g.Viewer(), handler.GetModel)
		// 更新模型 — SystemAdmin（模型由系统管理员统一配置）
		models.PUT("/:id", g.SystemAdmin(), handler.UpdateModel)
		// 删除模型 — SystemAdmin
		models.DELETE("/:id", g.SystemAdmin(), handler.DeleteModel)
		// Per-field credential subresource (see internal/handler/model_credentials.go) — SystemAdmin
		models.PUT("/:id/credentials", g.SystemAdmin(), credHandler.Put)
		models.DELETE("/:id/credentials/:field", g.SystemAdmin(), credHandler.DeleteField)
	}
}

// Sandbox configs are workspace infrastructure that hold provider credentials.
// Since the admin-console 按空间代管 migration the whole management surface
// (config CRUD, inventory, config-scoped skills) is SystemAdmin-only and the
// admin console drives it through X-Tenant-ID; workspace admins keep the
// Viewer reads (agent editors resolve sandbox configs). Scoped API keys
// cannot safely receive partial authority over them yet because mutation can
// strand remote sandboxes.
func RegisterSandboxConfigRoutes(
	r *gin.RouterGroup,
	h *handler.SandboxConfigHandler,
	skills *handler.SandboxSkillHandler,
	g *rbacGuards,
) {
	configs := g.apiKeyGroup(r.Group("/sandbox-configs"), apiKeyFullAccess())
	{
		configs.GET("", g.Viewer(), h.List)
		configs.PUT("/workspace-policy", g.SystemAdmin(), h.SetWorkspacePolicy)
		configs.POST("/templates/query", g.SystemAdmin(), h.QueryTemplates)
		configs.POST("", g.SystemAdmin(), h.Create)
		configs.GET("/:id", g.Viewer(), h.Get)
		configs.PUT("/:id", g.SystemAdmin(), h.Update)
		configs.DELETE("/:id", g.SystemAdmin(), h.Delete)
		configs.GET("/:id/sandboxes", g.SystemAdmin(), h.Inventory)
		// Skills are SystemAdmin throughout, reads included: an upload drives
		// a root shell whose output is baked into the image every session of
		// this config boots, and the listing names what that image carries.
		configs.GET("/:id/skills", g.SystemAdmin(), skills.List)
		configs.POST("/:id/skills", g.SystemAdmin(), skills.Upload)
		configs.GET("/:id/skills/:skillId", g.SystemAdmin(), skills.Get)
		configs.GET("/:id/skills/:skillId/files", g.SystemAdmin(), skills.ListFiles)
		configs.GET("/:id/skills/:skillId/files/content", g.SystemAdmin(), skills.GetFile)
		configs.POST("/:id/skills/:skillId/reinstall", g.SystemAdmin(), skills.Reinstall)
		configs.POST("/:id/skills/:skillId/stop", g.SystemAdmin(), skills.Stop)
		configs.PATCH("/:id/skills/:skillId", g.SystemAdmin(), skills.Patch)
		configs.DELETE("/:id/skills/:skillId", g.SystemAdmin(), skills.Delete)
		configs.GET("/:id/skills/:skillId/install-events", g.SystemAdmin(), skills.InstallEvents)
		configs.GET("/:id/skills/:skillId/transcript", g.SystemAdmin(), skills.InstallTranscript)
	}
}

// RegisterEvaluationRoutes registers evaluation endpoints. Running an
// evaluation drives LLM calls (cost) and reads from KBs across the
// tenant; gate to Admin+ until product asks for a finer-grained
// matrix.
func RegisterEvaluationRoutes(r *gin.RouterGroup, handler *handler.EvaluationHandler, g *rbacGuards) {
	evaluationRoutes := g.apiKeyGroup(r.Group("/evaluation"), apiKeyRunEvaluations(apiKeyFullAccess()))
	{
		evaluationRoutes.POST("", g.Admin(), handler.Evaluation)
		evaluationRoutes.GET("", g.Viewer(), handler.GetEvaluationResult)
	}
}

func RegisterInitializationRoutes(r *gin.RouterGroup, handler *handler.InitializationHandler, g *rbacGuards) {
	// 初始化接口
	// GetCurrentConfigByKB 是只读，Viewer+ 即可（KB 受限 key 可读其范围内的 KB）。
	g.apiKeyRoute(r, http.MethodGet, "/initialization/config/:kbId",
		apiKeyRetrieve(apiKeyFullAccess()), g.Viewer(), g.KBAccessRead("kbId"), handler.GetCurrentConfigByKB)
	// InitializeByKB / UpdateKBConfig 都是改 KB 的核心模型/storage 配置 —
	// 跟 PUT /knowledge-bases/:id 同等敏感，挂同款 OwnedKB 矩阵 + KBAccessWrite
	//（API-key 主体短路 Owned* 守卫，KB allow-list 只能靠 KBAccess 兜底）。
	g.apiKeyRoute(r, http.MethodPost, "/initialization/initialize/:kbId",
		apiKeyManageKnowledgeBases(apiKeyFullAccess()), g.OwnedKBOrAdminFromKbIDParam(), g.KBAccessWrite("kbId"), handler.InitializeByKB)
	g.apiKeyRoute(r, http.MethodPut, "/initialization/config/:kbId",
		apiKeyManageKnowledgeBases(apiKeyFullAccess()), g.OwnedKBOrAdminFromKbIDParam(), g.KBAccessWrite("kbId"), handler.UpdateKBConfig)

	// Ollama / 远程 API / 抽取等系统级检测/下载操作。这些不绑某个 KB，
	// 会改空间级模型配置或拉远端模型；JWT 侧只读探测 Viewer+、变更自
	// 000094 模型收权后为 SystemAdmin（管理台镜像路由见
	// routes_system_models.go）。对 API key 均为空间级：full-access key
	// 可用，scoped key 需要 manage_models。
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/status", apiKeyManageModels(apiKeyFullAccess()), g.Viewer(), handler.CheckOllamaStatus)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/models", apiKeyManageModels(apiKeyFullAccess()), g.Viewer(), handler.ListOllamaModels)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/ollama/models/check", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.CheckOllamaModels)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/ollama/models/download", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.DownloadOllamaModel)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/download/progress/:taskId", apiKeyManageModels(apiKeyFullAccess()), g.Viewer(), handler.GetDownloadProgress)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/download/tasks", apiKeyManageModels(apiKeyFullAccess()), g.Viewer(), handler.ListDownloadTasks)

	// 远程API相关接口
	g.apiKeyRoute(r, http.MethodPost, "/initialization/remote/check", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.CheckRemoteModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/embedding/test", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.TestEmbeddingModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/rerank/check", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.CheckRerankModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/asr/check", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.CheckASRModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/multimodal/test", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.TestMultimodalFunction)

	g.apiKeyRoute(r, http.MethodPost, "/initialization/extract/text-relation", apiKeyManageModels(apiKeyFullAccess()), g.Admin(), handler.ExtractTextRelations)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/extract/fabri-tag", apiKeyManageModels(apiKeyFullAccess()), g.Admin(), handler.FabriTag)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/extract/fabri-text", apiKeyManageModels(apiKeyFullAccess()), g.Admin(), handler.FabriText)
}

// RegisterMCPServiceRoutes registers MCP service routes.
//
// MCP services are tenant-level integrations (external tool servers).
// Since the admin-console 按空间代管 migration the management surface
// (CRUD, connection tests, credential subresource, tool-approval policy)
// is SystemAdmin-only, driven from the console through X-Tenant-ID;
// workspace members keep the Viewer reads (agent editors list services,
// tools and per-user OAuth state). Interactive /agent flows stay Viewer+ —
// they are runtime conversations, not configuration.
func RegisterMCPServiceRoutes(
	r *gin.RouterGroup,
	handler *handler.MCPServiceHandler,
	credHandler *handler.MCPCredentialsHandler,
	oauthHandler *handler.MCPOAuthHandler,
	g *rbacGuards,
) {
	// MCP OAuth provider redirect. Registered OUTSIDE the /mcp-services group
	// to avoid a static-vs-":id" route conflict, and left unauthenticated
	// (allow-listed in middleware/auth.go) because the third-party browser
	// redirect carries no WeKnora bearer — the single-use state authenticates.
	r.GET("/mcp-oauth/callback", oauthHandler.Callback)

	mcpServices := g.apiKeyGroup(r.Group("/mcp-services"), apiKeyManageMCPServices(apiKeyFullAccess()))
	{
		// Create MCP service — SystemAdmin
		mcpServices.POST("", g.SystemAdmin(), handler.CreateMCPService)
		// List MCP services — Viewer+
		mcpServices.GET("", g.Viewer(), handler.ListMCPServices)
		// Get MCP service by ID — Viewer+
		mcpServices.GET("/:id", g.Viewer(), handler.GetMCPService)
		// Update MCP service — SystemAdmin
		mcpServices.PUT("/:id", g.SystemAdmin(), handler.UpdateMCPService)
		// Delete MCP service — SystemAdmin
		mcpServices.DELETE("/:id", g.SystemAdmin(), handler.DeleteMCPService)
		// Test MCP service connection — SystemAdmin (probes external infra)
		mcpServices.POST("/:id/test", g.SystemAdmin(), handler.TestMCPService)
		// Get MCP service tools — Viewer+
		mcpServices.GET("/:id/tools", g.Viewer(), handler.GetMCPServiceTools)
		// Get MCP service resources — Viewer+
		mcpServices.GET("/:id/resources", g.Viewer(), handler.GetMCPServiceResources)
		// Per-field credential subresource: secrets never travel via the main
		// PUT body. See internal/handler/mcp_credentials.go for the contract. — SystemAdmin
		mcpServices.PUT("/:id/credentials", g.SystemAdmin(), credHandler.Put)
		mcpServices.DELETE("/:id/credentials/:field", g.SystemAdmin(), credHandler.DeleteField)
		// MCP tool human approval (issue #1173) — Viewer+ to read, SystemAdmin to set policy
		mcpServices.GET("/:id/tool-approvals", g.Viewer(), handler.ListMCPToolApprovals)
		mcpServices.PUT("/:id/tool-approvals/:tool_name", g.SystemAdmin(), handler.SetMCPToolApproval)
		// Per-user OAuth authorization flow. Viewer+ may authorize/inspect/
		// revoke their own token; the callback is the separate public route
		// registered above.
		mcpServices.POST("/:id/oauth/authorize-url", g.Viewer(), oauthHandler.AuthorizeURL)
		mcpServices.GET("/:id/oauth/status", g.Viewer(), oauthHandler.Status)
		mcpServices.DELETE("/:id/oauth/token", g.Viewer(), oauthHandler.Revoke)
	}

	// /agent tool-approval + OAuth resolution are interactive human flows;
	// not declared for API keys (default-deny).
	agentTool := r.Group("/agent")
	{
		// Resolving a pending tool-approval is gated to tenant members
		// (Viewer+). The approval card surfaces inside an agent chat the
		// caller initiated — restricting it to Admin+ blocks the only
		// people who actually have context to approve, so the gate is
		// kept at "anyone in the tenant" instead.
		agentTool.POST("/tool-approvals/:pending_id", g.Viewer(), handler.ResolveToolApproval)
		// Resume an agent run paused on an in-conversation MCP OAuth prompt.
		// Same tenant-member (Viewer+) gating rationale as tool-approvals.
		agentTool.POST("/mcp-oauth-resolutions/:pending_id", g.Viewer(), oauthHandler.ResolveMCPOAuth)
		agentTool.POST("/mcp-oauth-resolutions/:pending_id/cancel", g.Viewer(), oauthHandler.CancelMCPOAuth)
	}
}

// RegisterWebSearchRoutes registers web search routes
func RegisterWebSearchRoutes(r *gin.RouterGroup, webSearchHandler *handler.WebSearchHandler, g *rbacGuards) {
	// Web search providers — Viewer+ (read-only listing of provider catalog).
	webSearch := r.Group("/web-search")
	{
		webSearch.GET("/providers", g.Viewer(), webSearchHandler.GetProviders)
	}
}

// RegisterWebSearchProviderRoutes registers the tenant-scoped web search
// provider routes.
//
// Since the 000095 platform rework provider rows are platform-owned and
// shared with workspaces through tenant_web_search_provider_assignments
// (see routes_system_websearch.go for the admin-console CRUD + assignment
// endpoints). This group keeps two duties: reads (Viewer+) resolve a
// workspace's assigned services for the chat input and agent editors, and
// the SystemAdmin-gated writes remain registered as the platform-API-key
// surface (manage_web_search) — their handlers operate on the platform
// catalog, with the X-Tenant-ID only scoping the visibility check.
func RegisterWebSearchProviderRoutes(
	r *gin.RouterGroup,
	h *handler.WebSearchProviderHandler,
	credHandler *handler.WebSearchProviderCredentialsHandler,
	g *rbacGuards,
) {
	providers := g.apiKeyGroup(r.Group("/web-search-providers"), apiKeyManageWebSearch(apiKeyFullAccess()))
	{
		// List available provider types (metadata for UI forms) — Viewer+
		providers.GET("/types", g.Viewer(), h.ListProviderTypes)
		// Test with raw credentials (no persistence) — SystemAdmin
		providers.POST("/test", g.SystemAdmin(), h.TestProviderRaw)
		// CRUD
		providers.POST("", g.SystemAdmin(), h.CreateProvider)
		providers.GET("", g.Viewer(), h.ListProviders)
		providers.GET("/:id", g.Viewer(), h.GetProvider)
		providers.PUT("/:id", g.SystemAdmin(), h.UpdateProvider)
		providers.DELETE("/:id", g.SystemAdmin(), h.DeleteProvider)
		// Per-field credential subresource — SystemAdmin
		providers.PUT("/:id/credentials", g.SystemAdmin(), credHandler.Put)
		providers.DELETE("/:id/credentials/:field", g.SystemAdmin(), credHandler.DeleteField)
		// Test existing saved provider — SystemAdmin
		providers.POST("/:id/test", g.SystemAdmin(), h.TestProviderByID)
	}
}

// RegisterVectorStoreRoutes registers CRUD routes for vector store configurations.
//
// Vector stores are tenant-level infrastructure. Since the admin-console
// 按空间代管 migration the management surface is SystemAdmin-only; reads
// stay Viewer+ (KB creation resolves candidate stores).
func RegisterVectorStoreRoutes(r *gin.RouterGroup, h *handler.VectorStoreHandler, g *rbacGuards) {
	stores := g.apiKeyGroup(r.Group("/vector-stores"), apiKeyManageVectorStores(apiKeyFullAccess()))
	{
		// List available engine types (metadata for UI forms) — Viewer+
		stores.GET("/types", g.Viewer(), h.ListStoreTypes)
		// Test with raw credentials (no persistence) — SystemAdmin
		stores.POST("/test", g.SystemAdmin(), h.TestStoreRaw)
		// CRUD
		stores.POST("", g.SystemAdmin(), h.CreateStore)
		stores.GET("", g.Viewer(), h.ListStores)
		stores.GET("/:id", g.Viewer(), h.GetStore)
		stores.PUT("/:id", g.SystemAdmin(), h.UpdateStore)
		stores.DELETE("/:id", g.SystemAdmin(), h.DeleteStore)
		// Test existing saved or env store — SystemAdmin
		stores.POST("/:id/test", g.SystemAdmin(), h.TestStoreByID)
	}
}

// RegisterStorageBackendRoutes manages concrete object/file storage instances.
// Management surface is SystemAdmin since the admin-console 按空间代管
// migration (console drives it through X-Tenant-ID); reads stay Viewer+.
func RegisterStorageBackendRoutes(r *gin.RouterGroup, h *handler.StorageBackendHandler, g *rbacGuards) {
	backends := g.apiKeyGroup(r.Group("/storage-backends"), apiKeyManageStorageBackends(apiKeyFullAccess()))
	{
		backends.GET("/types", g.Viewer(), h.Types)
		backends.POST("/test", g.SystemAdmin(), h.TestRaw)
		backends.POST("", g.SystemAdmin(), h.Create)
		backends.GET("", g.Viewer(), h.List)
		backends.GET("/:id", g.Viewer(), h.Get)
		backends.PUT("/:id", g.SystemAdmin(), h.Update)
		backends.DELETE("/:id", g.SystemAdmin(), h.Delete)
		backends.POST("/:id/test", g.SystemAdmin(), h.TestByID)
		backends.PUT("/:id/default", g.SystemAdmin(), h.SetDefault)
	}
}

// RegisterDataSourceRoutes 注册数据源相关的路由
//
// Data sources hold external service credentials (Feishu/Notion/Yuque)
// and trigger sync jobs that mutate KB content tenant-wide. Reads are
// Viewer+; everything else (CRUD, validation, sync control, credential
// subresource) is Admin+.
func RegisterDataSourceRoutes(
	r *gin.RouterGroup,
	handler *handler.DataSourceHandler,
	credHandler *handler.DataSourceCredentialsHandler,
	g *rbacGuards,
) {
	// Data source routes
	ds := g.apiKeyGroup(r.Group("/datasource"), apiKeyManageDataSources(apiKeyFullAccess()))
	{
		// Get available connector types — Viewer+
		ds.GET("/types", g.Viewer(), handler.GetAvailableConnectors)

		// Validate credentials without persistence (for "Test Connection" button) — Admin+
		ds.POST("/validate-credentials", g.Admin(), handler.ValidateCredentials)

		// CRUD operations
		ds.POST("", g.Admin(), handler.CreateDataSource)
		ds.GET("", g.Viewer(), handler.ListDataSources)
		ds.GET("/:id", g.Viewer(), handler.GetDataSource)
		ds.PUT("/:id", g.Admin(), handler.UpdateDataSource)
		ds.DELETE("/:id", g.Admin(), handler.DeleteDataSource)

		// Credential subresource. Single logical field "credentials" because
		// connector credentials are a per-connector atomic map (see
		// internal/handler/datasource_credentials.go). — Admin+
		ds.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		ds.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)

		// Connection and resource management — Admin+
		ds.POST("/:id/validate", g.Admin(), handler.ValidateConnection)
		ds.GET("/:id/resources", g.Admin(), handler.ListAvailableResources)
		ds.POST("/:id/resource-ancestors", g.Admin(), handler.ResolveResourceAncestors)

		// Sync management — Admin+
		ds.POST("/:id/sync", g.Admin(), handler.ManualSync)
		ds.POST("/:id/pause", g.Admin(), handler.PauseDataSource)
		ds.POST("/:id/resume", g.Admin(), handler.ResumeDataSource)

		// Sync logs — Viewer+ (read-only audit trail)
		ds.GET("/:id/logs", g.Viewer(), handler.GetSyncLogs)
		ds.GET("/logs/:log_id", g.Viewer(), handler.GetSyncLog)
	}
}

// RegisterWeKnoraCloudRoutes 注册 WeKnoraCloud 初始化路由
// RegisterWeKnoraCloudRoutes registers the WeKnoraCloud credential
// management endpoints. SaveCredentials persists external SaaS keys
// for the tenant — SystemAdmin since the 000094 model-governance rework
// (the workspace settings page is gone; WeKnoraCloud models carry their
// credentials on the platform catalog rows). Status stays a low-risk
// readiness probe (Viewer+).
func RegisterWeKnoraCloudRoutes(r *gin.RouterGroup, handler *handler.WeKnoraCloudHandler, g *rbacGuards) {
	g.apiKeyRoute(r, http.MethodPost, "/weknoracloud/credentials", apiKeyManageModels(apiKeyFullAccess()), g.SystemAdmin(), handler.SaveCredentials)
	g.apiKeyRoute(r, http.MethodGet, "/models/weknoracloud/status", apiKeyManageModels(apiKeyFullAccess()), g.Viewer(), handler.Status)
}
