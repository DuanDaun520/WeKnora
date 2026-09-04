package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// SystemModelHandler backs the /system/admin/models* endpoints — the
// platform model catalog of the admin console (000094 model governance
// rework). Model configuration is centralized: workspace members keep
// read/usage access through the tenant-scoped /api/v1/models routes, but
// every catalog mutation and every workspace assignment lives here behind
// the SystemAdmin guard. Handlers must not read the tenant from the request
// context: a system admin may have no workspace binding at all.
type SystemModelHandler struct {
	modelSvc  interfaces.ModelService
	tenantSvc interfaces.TenantService
	// auditSvc is optional — nil in partially-wired unit tests, in which
	// case emitAudit no-ops (same convention as SystemHandler).
	auditSvc interfaces.AuditLogService
}

// NewSystemModelHandler creates a new SystemModelHandler.
func NewSystemModelHandler(
	modelSvc interfaces.ModelService,
	tenantSvc interfaces.TenantService,
	auditSvc interfaces.AuditLogService,
) *SystemModelHandler {
	return &SystemModelHandler{modelSvc: modelSvc, tenantSvc: tenantSvc, auditSvc: auditSvc}
}

// emitAudit writes one system-scope audit row for a model-governance event.
// Best-effort — a nil audit service or a write failure does not bubble up.
func (h *SystemModelHandler) emitAudit(
	ctx context.Context,
	action types.AuditAction,
	targetType, targetID string,
	details map[string]any,
) {
	if h.auditSvc == nil {
		return
	}
	actorID, _ := types.UserIDFromContext(ctx)
	var detailsJSON types.JSON
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = types.JSON(b)
		}
	}
	_ = h.auditSvc.Log(ctx, &types.AuditLog{
		// tenant_id=0 marks the row as system-scope (platform-wide event).
		TenantID:    0,
		ActorUserID: actorID,
		ActorRole:   systemAuditActorRole(ctx),
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     detailsJSON,
	})
}

// frontendModelTypeToBackend maps the frontend type vocabulary
// (chat/embedding/rerank/vllm/asr) onto the backend ModelType constants.
func frontendModelTypeToBackend(modelType string) types.ModelType {
	switch modelType {
	case "chat":
		return types.ModelTypeKnowledgeQA
	case "embedding":
		return types.ModelTypeEmbedding
	case "rerank":
		return types.ModelTypeRerank
	case "vllm":
		return types.ModelTypeVLLM
	case "asr":
		return types.ModelTypeASR
	default:
		return types.ModelType(modelType)
	}
}

// ListModels GET /system/admin/models?type=&q=
// Returns the whole platform catalog (builtin included).
func (h *SystemModelHandler) ListModels(c *gin.Context) {
	ctx := c.Request.Context()
	modelType := frontendModelTypeToBackend(c.Query("type"))
	nameQuery := secutils.SanitizeForLog(c.Query("q"))

	models, err := h.modelSvc.ListAllModels(ctx, modelType, nameQuery)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":  len(models),
		"models": dto.NewModelResponses(ctx, models),
	})
}

// GetModel GET /system/admin/models/:id
func (h *SystemModelHandler) GetModel(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(apperrors.NewBadRequestError("Model ID cannot be empty"))
		return
	}
	model, err := h.modelSvc.GetPlatformModel(ctx, id)
	if err != nil {
		if err == service.ErrModelNotFound {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": dto.NewModelResponse(ctx, model)})
}

// CreateModel POST /system/admin/models
// Body contract is identical to the (now SystemAdmin-gated) tenant route
// POST /api/v1/models; the created row is platform-owned (tenant_id = 0).
func (h *SystemModelHandler) CreateModel(c *gin.Context) {
	ctx := c.Request.Context()
	var req CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	if req.Parameters.BaseURL != "" {
		if err := secutils.ValidateURLForSSRF(req.Parameters.BaseURL); err != nil {
			logger.Warnf(ctx, "SSRF validation failed for model BaseURL: %v", err)
			c.Error(apperrors.NewBadRequestError(secutils.FormatSSRFError("Base URL", req.Parameters.BaseURL, err)))
			return
		}
	}

	model := &types.Model{
		TenantID:    0,
		Name:        secutils.SanitizeForLog(req.Name),
		DisplayName: secutils.SanitizeForLog(req.DisplayName),
		Type:        types.ModelType(secutils.SanitizeForLog(string(req.Type))),
		Source:      req.Source,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
	}
	if err := h.modelSvc.CreatePlatformModel(ctx, model); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemModelCreated, "model", model.ID, map[string]any{
		"name": model.Name, "type": string(model.Type), "source": string(model.Source),
	})
	c.JSON(http.StatusCreated, gin.H{"model": dto.NewModelResponse(ctx, model)})
}

// UpdateModel PUT /system/admin/models/:id
func (h *SystemModelHandler) UpdateModel(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(apperrors.NewBadRequestError("Model ID cannot be empty"))
		return
	}
	var req UpdateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	model, err := h.modelSvc.GetPlatformModel(ctx, id)
	if err != nil {
		if err == service.ErrModelNotFound {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if err := applyUpdateModelRequest(ctx, id, model, req); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if err := h.modelSvc.UpdatePlatformModel(ctx, model); err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemModelUpdated, "model", id, map[string]any{
		"name": model.Name, "type": string(model.Type),
	})
	c.JSON(http.StatusOK, gin.H{"model": dto.NewModelResponse(ctx, model)})
}

// DeleteModel DELETE /system/admin/models/:id
// Fails with 400 (model in use) when any workspace still references the
// model through a KB, agent, or memory config.
func (h *SystemModelHandler) DeleteModel(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(apperrors.NewBadRequestError("Model ID cannot be empty"))
		return
	}
	if err := h.modelSvc.DeletePlatformModel(ctx, id); err != nil {
		if err == service.ErrModelNotFound {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemModelDeleted, "model", id, nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Model deleted"})
}

// PutCredentials PUT /system/admin/models/:id/credentials
// Contract mirrors ModelCredentialsHandler.Put (nil-pointer = read-only
// metadata probe; secrets never travel through the main PUT body).
func (h *SystemModelHandler) PutCredentials(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))

	var req modelCredentialsPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	if req.APIKey == nil && req.AppSecret == nil {
		m, err := h.modelSvc.GetPlatformModel(ctx, id)
		if err != nil || m == nil {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.CredentialsResponse{
			Fields: map[string]dto.CredentialFieldMetadata{
				"api_key":    {Configured: m.Parameters.APIKey != ""},
				"app_secret": {Configured: m.Parameters.AppSecret != ""},
			},
		}})
		return
	}

	updated, err := h.modelSvc.UpdatePlatformModelCredentials(ctx, id, req.APIKey, req.AppSecret)
	if err != nil {
		if err == service.ErrModelNotFound {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id})
		c.Error(apperrors.NewInternalServerError("failed to update credentials: " + err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemModelCredentials, "model", id, map[string]any{
		"api_key":    req.APIKey != nil,
		"app_secret": req.AppSecret != nil,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.CredentialsResponse{
		Fields: map[string]dto.CredentialFieldMetadata{
			"api_key":    {Configured: updated.Parameters.APIKey != ""},
			"app_secret": {Configured: updated.Parameters.AppSecret != ""},
		},
	}})
}

// DeleteCredentialField DELETE /system/admin/models/:id/credentials/:field
func (h *SystemModelHandler) DeleteCredentialField(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	field := c.Param("field")
	if field != "api_key" && field != "app_secret" {
		c.Error(apperrors.NewBadRequestError("unknown credential field: "+secutils.SanitizeForLog(field)))
		return
	}
	if err := h.modelSvc.ClearPlatformModelCredential(ctx, id, field); err != nil {
		if err == service.ErrModelNotFound {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id, "field": field})
		c.Error(apperrors.NewInternalServerError("failed to clear credential: " + err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemModelCredentials, "model", id, map[string]any{
		"cleared": field,
	})
	c.Status(http.StatusNoContent)
}

// ListTenantModelAssignments GET /system/admin/tenants/:tenant_id/model-assignments
// Returns the non-builtin models assigned to the workspace.
func (h *SystemModelHandler) ListTenantModelAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := h.requireSystemAdminTenant(c)
	if !ok {
		return
	}
	models, err := h.modelSvc.ListTenantAssignedModels(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"tenant_id": tenantID})
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tenant_id": tenantID,
		"models":    dto.NewModelResponses(ctx, models),
	})
}

// SystemTenantModelAssignmentsRequest is the replace-all body of
// PUT /system/admin/tenants/:tenant_id/model-assignments.
type SystemTenantModelAssignmentsRequest struct {
	ModelIDs []string `json:"model_ids"`
}

// UpdateTenantModelAssignments PUT /system/admin/tenants/:tenant_id/model-assignments
// Replaces the full assignment list. Removing a model the workspace still
// references fails with 409 so KBs and agents never point at an invisible
// model.
func (h *SystemModelHandler) UpdateTenantModelAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := h.requireSystemAdminTenant(c)
	if !ok {
		return
	}
	var req SystemTenantModelAssignmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	actorID, _ := types.UserIDFromContext(ctx)
	if err := h.modelSvc.SetTenantModelAssignments(ctx, tenantID, req.ModelIDs, actorID); err != nil {
		if err == service.ErrModelNotFound {
			c.Error(apperrors.NewNotFoundError("Model not found"))
			return
		}
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"tenant_id": tenantID})
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemModelsAssigned, "tenant", strconv.FormatUint(tenantID, 10), map[string]any{
		"tenant_id": tenantID,
		"model_ids": req.ModelIDs,
		"count":     len(req.ModelIDs),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "tenant_id": tenantID, "count": len(req.ModelIDs)})
}

// parseSystemAdminTenantID extracts and validates the :tenant_id path
// parameter, answering 400 itself on malformed input and 404 when the
// workspace does not exist.
func parseSystemAdminTenantID(c *gin.Context) (uint64, bool) {
	raw := c.Param("tenant_id")
	tenantID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || tenantID == 0 {
		c.Error(apperrors.NewBadRequestError("Invalid workspace ID"))
		return 0, false
	}
	return tenantID, true
}

// requireSystemAdminTenant additionally verifies the workspace exists.
func (h *SystemModelHandler) requireSystemAdminTenant(c *gin.Context) (uint64, bool) {
	tenantID, ok := parseSystemAdminTenantID(c)
	if !ok {
		return 0, false
	}
	if h.tenantSvc != nil {
		tenant, err := h.tenantSvc.GetTenantByID(c.Request.Context(), tenantID)
		if err != nil || tenant == nil {
			c.Error(apperrors.NewNotFoundError("Workspace not found"))
			return 0, false
		}
	}
	return tenantID, true
}
