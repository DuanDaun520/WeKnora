package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// SystemWebSearchProviderHandler backs the
// /system/admin/web-search-providers* endpoints — the platform search-service
// catalog of the admin console (000095 web search platform rework). A search
// service is configured once here and shared with workspaces through explicit
// assignments; workspace members keep read/usage access through the
// tenant-scoped /api/v1/web-search-providers routes. Handlers must not read
// the tenant from the request context: a system admin may have no workspace
// binding at all.
type SystemWebSearchProviderHandler struct {
	repo      interfaces.WebSearchProviderRepository
	svc       interfaces.WebSearchProviderService
	registry  *infra_web_search.Registry
	tenantSvc interfaces.TenantService
	// auditSvc is optional — nil in partially-wired unit tests, in which
	// case emitAudit no-ops (same convention as SystemModelHandler).
	auditSvc interfaces.AuditLogService
}

// NewSystemWebSearchProviderHandler creates a new SystemWebSearchProviderHandler.
func NewSystemWebSearchProviderHandler(
	repo interfaces.WebSearchProviderRepository,
	svc interfaces.WebSearchProviderService,
	registry *infra_web_search.Registry,
	tenantSvc interfaces.TenantService,
	auditSvc interfaces.AuditLogService,
) *SystemWebSearchProviderHandler {
	return &SystemWebSearchProviderHandler{
		repo:      repo,
		svc:       svc,
		registry:  registry,
		tenantSvc: tenantSvc,
		auditSvc:  auditSvc,
	}
}

// emitAudit writes one system-scope audit row for a web-search governance
// event. Best-effort — a nil audit service or a write failure does not
// bubble up.
func (h *SystemWebSearchProviderHandler) emitAudit(
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

// assignmentsFor builds the assignment DTO input of one provider.
func (h *SystemWebSearchProviderHandler) assignmentsFor(
	ctx context.Context, providerID string,
) ([]types.WebSearchProviderAssignmentInfo, error) {
	rows, err := h.svc.ListProviderAssignments(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []types.WebSearchProviderAssignmentInfo{}
	}
	return rows, nil
}

// ListProviderTypes GET /system/admin/web-search-providers/types
// Registry metadata for the console form — same payload as the tenant-scoped
// route, no tenant involvement.
func (h *SystemWebSearchProviderHandler) ListProviderTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.GetWebSearchProviderTypes(),
	})
}

// ListProviders GET /system/admin/web-search-providers
// Returns the whole platform catalog with each service's assignments.
func (h *SystemWebSearchProviderHandler) ListProviders(c *gin.Context) {
	ctx := c.Request.Context()
	providers, err := h.repo.ListAll(ctx)
	if err != nil {
		logger.Warnf(ctx, "Failed to list platform web search providers: %v", err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	out := make([]*dto.SystemWebSearchProviderResponse, 0, len(providers))
	for _, provider := range providers {
		assignments, err := h.assignmentsFor(ctx, provider.ID)
		if err != nil {
			logger.Warnf(ctx, "Failed to list assignments of provider %s: %v", provider.ID, err)
			c.Error(apperrors.NewInternalServerError(err.Error()))
			return
		}
		out = append(out, dto.NewSystemWebSearchProviderResponse(ctx, provider, assignments))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// GetProvider GET /system/admin/web-search-providers/:id
func (h *SystemWebSearchProviderHandler) GetProvider(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	provider, err := h.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if provider == nil {
		c.Error(apperrors.NewNotFoundError("web search provider not found"))
		return
	}
	assignments, err := h.assignmentsFor(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewSystemWebSearchProviderResponse(ctx, provider, assignments),
	})
}

// CreateProvider POST /system/admin/web-search-providers
// Body contract is identical to the (SystemAdmin-gated) tenant route; the
// created row is platform-owned (tenant_id = 0) and starts unassigned.
func (h *SystemWebSearchProviderHandler) CreateProvider(c *gin.Context) {
	ctx := c.Request.Context()
	var req CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	provider := &types.WebSearchProviderEntity{
		Name:        secutils.SanitizeForLog(req.Name),
		Provider:    req.Provider,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
	}
	if err := h.svc.CreateProvider(ctx, provider); err != nil {
		logger.Warnf(ctx, "Failed to create platform web search provider: %v", err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemWebSearchProviderCreated, "web_search_provider", provider.ID, map[string]any{
		"name": provider.Name, "provider": string(provider.Provider),
	})
	assignments, _ := h.assignmentsFor(ctx, provider.ID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    dto.NewSystemWebSearchProviderResponse(ctx, provider, assignments),
	})
}

// UpdateProvider PUT /system/admin/web-search-providers/:id
// Merge semantics mirror the tenant route: api_key never flows through this
// endpoint (warn when a stale caller passes one), ExtraConfig/Name/
// Description are preserved when omitted, provider type is immutable.
func (h *SystemWebSearchProviderHandler) UpdateProvider(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))

	existing, err := h.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if existing == nil {
		c.Error(apperrors.NewNotFoundError("web search provider not found"))
		return
	}

	var req UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	if req.Parameters.APIKey != "" && req.Parameters.APIKey != existing.Parameters.APIKey {
		logger.Warnf(ctx,
			"deprecated: api_key in PUT /system/admin/web-search-providers/%s body is ignored; use PUT /credentials instead",
			id)
	}
	mergedParams := req.Parameters
	mergedParams.APIKey = existing.Parameters.APIKey
	if mergedParams.ExtraConfig == nil {
		mergedParams.ExtraConfig = existing.Parameters.ExtraConfig
	}
	mergedName := req.Name
	if mergedName == "" {
		mergedName = existing.Name
	}
	mergedDescription := req.Description
	if mergedDescription == "" {
		mergedDescription = existing.Description
	}

	provider := &types.WebSearchProviderEntity{
		ID:          id,
		Name:        secutils.SanitizeForLog(mergedName),
		Provider:    existing.Provider, // Provider type is immutable after creation
		Description: secutils.SanitizeForLog(mergedDescription),
		Parameters:  mergedParams,
	}
	if err := h.svc.UpdateProvider(ctx, provider); err != nil {
		logger.Warnf(ctx, "Failed to update platform web search provider %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	updated, _ := h.repo.GetByIDAnyTenant(ctx, id)
	if updated == nil {
		updated = provider
	}
	h.emitAudit(ctx, types.AuditActionSystemWebSearchProviderUpdated, "web_search_provider", id, map[string]any{
		"name": updated.Name,
	})
	assignments, _ := h.assignmentsFor(ctx, id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewSystemWebSearchProviderResponse(ctx, updated, assignments),
	})
}

// DeleteProvider DELETE /system/admin/web-search-providers/:id
// Assignments are purged with the row; agents pinning the deleted service
// fall back to their workspace default at runtime.
func (h *SystemWebSearchProviderHandler) DeleteProvider(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if err := h.svc.DeleteProvider(ctx, id); err != nil {
		if err == service.ErrWebSearchProviderNotFound {
			c.Error(apperrors.NewNotFoundError("web search provider not found"))
			return
		}
		logger.Warnf(ctx, "Failed to delete platform web search provider %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemWebSearchProviderDeleted, "web_search_provider", id, nil)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestProviderRaw POST /system/admin/web-search-providers/test
// Probes raw credentials without persistence (console "测试连接" on an
// unsaved form).
func (h *SystemWebSearchProviderHandler) TestProviderRaw(c *gin.Context) {
	ctx := c.Request.Context()
	var req TestProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	if err := doTestSearch(ctx, h.registry, req.Provider, req.Parameters); err != nil {
		logger.Warnf(ctx, "Web search provider test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestProviderByID POST /system/admin/web-search-providers/:id/test
// Probes a saved provider with its stored credentials.
func (h *SystemWebSearchProviderHandler) TestProviderByID(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	provider, err := h.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if provider == nil {
		c.Error(apperrors.NewNotFoundError("web search provider not found"))
		return
	}
	if err := doTestSearch(ctx, h.registry, string(provider.Provider), provider.Parameters); err != nil {
		logger.Warnf(ctx, "Web search provider test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListTenantAssignments GET /system/admin/web-search-providers/:id/tenant-assignments
// Returns the workspaces this service is assigned to.
func (h *SystemWebSearchProviderHandler) ListTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if _, err := h.repo.GetByIDAnyTenant(ctx, id); err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	rows, err := h.assignmentsFor(ctx, id)
	if err != nil {
		logger.Warnf(ctx, "Failed to list assignments of provider %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// SystemProviderTenantAssignmentInput is one row of the replace-all body of
// PUT /system/admin/web-search-providers/:id/tenant-assignments.
type SystemProviderTenantAssignmentInput struct {
	TenantID  uint64 `json:"tenant_id" binding:"required"`
	IsDefault bool   `json:"is_default"`
}

// SystemProviderTenantAssignmentsRequest is the replace-all body.
type SystemProviderTenantAssignmentsRequest struct {
	Assignments []SystemProviderTenantAssignmentInput `json:"assignments"`
}

// UpdateTenantAssignments PUT /system/admin/web-search-providers/:id/tenant-assignments
// Replaces the full assignment list of the service. Marking a workspace's
// default here clears the default flag on every other service of that
// workspace (at most one default per workspace, enforced by the repository).
func (h *SystemWebSearchProviderHandler) UpdateTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	var req SystemProviderTenantAssignmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	rows := make([]types.TenantWebSearchProviderAssignment, 0, len(req.Assignments))
	tenantIDs := make(map[uint64]bool, len(req.Assignments))
	for _, input := range req.Assignments {
		if input.TenantID == 0 || tenantIDs[input.TenantID] {
			c.Error(apperrors.NewBadRequestError("invalid or duplicate workspace ID in assignments"))
			return
		}
		tenantIDs[input.TenantID] = true
		rows = append(rows, types.TenantWebSearchProviderAssignment{
			TenantID:  input.TenantID,
			IsDefault: input.IsDefault,
		})
	}

	// Reject unknown workspace IDs with 404 rather than silently dropping
	// them — the console lists workspaces that may have been deleted since.
	if h.tenantSvc != nil {
		for tenantID := range tenantIDs {
			tenant, err := h.tenantSvc.GetTenantByID(ctx, tenantID)
			if err != nil || tenant == nil {
				c.Error(apperrors.NewNotFoundError("workspace not found"))
				return
			}
		}
	}

	actorID, _ := types.UserIDFromContext(ctx)
	if err := h.svc.SetProviderAssignments(ctx, id, rows, actorID); err != nil {
		if err == service.ErrWebSearchProviderNotFound {
			c.Error(apperrors.NewNotFoundError("web search provider not found"))
			return
		}
		logger.Warnf(ctx, "Failed to set assignments of provider %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	defaults := make([]uint64, 0)
	assigned := make([]uint64, 0, len(rows))
	for _, row := range rows {
		assigned = append(assigned, row.TenantID)
		if row.IsDefault {
			defaults = append(defaults, row.TenantID)
		}
	}
	h.emitAudit(ctx, types.AuditActionSystemWebSearchProviderAssigned, "web_search_provider", id, map[string]any{
		"tenants":  assigned,
		"defaults": defaults,
	})
	updated, _ := h.assignmentsFor(ctx, id)
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"provider_id": id,
		"data":        updated,
	})
}
