package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/logger"
	mcpsecurity "github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// SystemMCPServiceHandler backs the /system/admin/mcp-services* endpoints —
// the platform MCP catalog of the admin console (000096 MCP platform rework).
// An MCP service is configured once here and shared with workspaces through
// explicit assignments; workspace members keep read/usage access through the
// tenant-scoped /api/v1/mcp-services routes. Handlers must not read the tenant
// from the request context: a system admin may have no workspace binding at
// all.
type SystemMCPServiceHandler struct {
	repo      interfaces.MCPServiceRepository
	svc       interfaces.MCPServiceService
	tenantSvc interfaces.TenantService
	// auditSvc is optional — nil in partially-wired unit tests, in which
	// case emitAudit no-ops (same convention as SystemModelHandler).
	auditSvc interfaces.AuditLogService
}

// NewSystemMCPServiceHandler creates a new SystemMCPServiceHandler.
func NewSystemMCPServiceHandler(
	repo interfaces.MCPServiceRepository,
	svc interfaces.MCPServiceService,
	tenantSvc interfaces.TenantService,
	auditSvc interfaces.AuditLogService,
) *SystemMCPServiceHandler {
	return &SystemMCPServiceHandler{
		repo:      repo,
		svc:       svc,
		tenantSvc: tenantSvc,
		auditSvc:  auditSvc,
	}
}

// emitAudit writes one system-scope audit row for an MCP governance event.
// Best-effort — a nil audit service or a write failure does not bubble up.
func (h *SystemMCPServiceHandler) emitAudit(
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

// assignmentsFor builds the assignment DTO input of one service.
func (h *SystemMCPServiceHandler) assignmentsFor(
	ctx context.Context, serviceID string,
) ([]types.MCPServiceAssignmentInfo, error) {
	rows, err := h.svc.ListServiceAssignments(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []types.MCPServiceAssignmentInfo{}
	}
	return rows, nil
}

// ListServices GET /system/admin/mcp-services
// Returns the whole platform catalog with each service's assignments. Builtin
// rows are included; they are visible to every workspace by design and carry
// no assignments.
func (h *SystemMCPServiceHandler) ListServices(c *gin.Context) {
	ctx := c.Request.Context()
	services, err := h.repo.ListAll(ctx)
	if err != nil {
		logger.Warnf(ctx, "Failed to list platform MCP services: %v", err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	out := make([]*dto.SystemMCPServiceResponse, 0, len(services))
	for _, svc := range services {
		assignments, err := h.assignmentsFor(ctx, svc.ID)
		if err != nil {
			logger.Warnf(ctx, "Failed to list assignments of MCP service %s: %v", svc.ID, err)
			c.Error(apperrors.NewInternalServerError(err.Error()))
			return
		}
		out = append(out, dto.NewSystemMCPServiceResponse(ctx, svc, assignments))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// GetService GET /system/admin/mcp-services/:id
func (h *SystemMCPServiceHandler) GetService(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	svc, err := h.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if svc == nil {
		c.Error(apperrors.NewNotFoundError("MCP service not found"))
		return
	}
	assignments, err := h.assignmentsFor(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewSystemMCPServiceResponse(ctx, svc, assignments),
	})
}

// CreateService POST /system/admin/mcp-services
// Body contract is identical to the tenant route; the created row is
// platform-owned (tenant_id = 0) and starts unassigned. Secrets may ride the
// POST on create (same as the tenant route); afterwards they only flow
// through /credentials.
func (h *SystemMCPServiceHandler) CreateService(c *gin.Context) {
	ctx := c.Request.Context()
	var svc types.MCPService
	if err := c.ShouldBindJSON(&svc); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	svc.Name = secutils.SanitizeForLog(svc.Name)
	svc.Description = secutils.SanitizeForLog(svc.Description)

	if err := validateMCPServiceShape(&svc); err != nil {
		c.Error(err)
		return
	}

	if svc.URL != nil && *svc.URL != "" {
		if err := secutils.ValidateURLForSSRF(*svc.URL); err != nil {
			logger.Warnf(ctx, "SSRF validation failed for MCP service URL: %v", err)
			c.Error(apperrors.NewBadRequestError(secutils.FormatSSRFError("MCP service URL", *svc.URL, err)))
			return
		}
	}
	if err := mcpsecurity.ValidateServiceOutboundURLs(&svc); err != nil {
		logger.Warnf(ctx, "SSRF validation failed for MCP service configuration: %v", err)
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	if err := h.svc.CreateMCPService(ctx, &svc); err != nil {
		logger.Warnf(ctx, "Failed to create platform MCP service: %v", err)
		c.Error(apperrors.NewInternalServerError("Failed to create MCP service: " + err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemMCPServiceCreated, "mcp_service", svc.ID, map[string]any{
		"name": svc.Name, "transport_type": string(svc.TransportType),
	})
	assignments, _ := h.assignmentsFor(ctx, svc.ID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    dto.NewSystemMCPServiceResponse(ctx, &svc, assignments),
	})
}

// UpdateService PUT /system/admin/mcp-services/:id
// Partial-update semantics mirror the tenant route exactly (shared parser);
// secrets never flow through this endpoint — warn when a stale caller passes
// one (parser logs it).
func (h *SystemMCPServiceHandler) UpdateService(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))

	existing, err := h.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	if existing == nil {
		c.Error(apperrors.NewNotFoundError("MCP service not found"))
		return
	}

	var updateData map[string]interface{}
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	service, updateFields, err := parseMCPServiceUpdateBody(ctx, id, updateData)
	if err != nil {
		c.Error(err)
		return
	}
	service.TenantID = 0 // platform scope

	if err := h.svc.UpdateMCPService(ctx, service, updateFields); err != nil {
		logger.Warnf(ctx, "Failed to update platform MCP service %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError("Failed to update MCP service: " + err.Error()))
		return
	}

	updated, _ := h.repo.GetByIDAnyTenant(ctx, id)
	if updated == nil {
		updated = service
	}
	h.emitAudit(ctx, types.AuditActionSystemMCPServiceUpdated, "mcp_service", id, map[string]any{
		"name": updated.Name,
	})
	assignments, _ := h.assignmentsFor(ctx, id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dto.NewSystemMCPServiceResponse(ctx, updated, assignments),
	})
}

// DeleteService DELETE /system/admin/mcp-services/:id
// Assignments are purged with the row; builtin services are rejected by the
// service layer.
func (h *SystemMCPServiceHandler) DeleteService(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if err := h.svc.DeleteMCPService(ctx, 0, id); err != nil {
		if err == service.ErrMCPServiceNotFound {
			c.Error(apperrors.NewNotFoundError("MCP service not found"))
			return
		}
		logger.Warnf(ctx, "Failed to delete platform MCP service %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemMCPServiceDeleted, "mcp_service", id, nil)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestService POST /system/admin/mcp-services/:id/test
// Probes a saved service. OAuth services will surface an
// "authorization required" result here because per-user OAuth needs a
// workspace + principal context that the console cannot supply — that
// authorization happens in-conversation (McpOAuthCard) after assignment.
func (h *SystemMCPServiceHandler) TestService(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	result, err := h.svc.TestMCPService(ctx, 0, id)
	if err != nil {
		logger.Warnf(ctx, "Platform MCP service test failed for %s: %v", id, err)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": types.MCPTestResult{
				Success: false,
				Message: "Test failed: " + err.Error(),
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ListTenantAssignments GET /system/admin/mcp-services/:id/tenant-assignments
// Returns the workspaces this service is assigned to.
func (h *SystemMCPServiceHandler) ListTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if _, err := h.repo.GetByIDAnyTenant(ctx, id); err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	rows, err := h.assignmentsFor(ctx, id)
	if err != nil {
		logger.Warnf(ctx, "Failed to list assignments of MCP service %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// SystemMCPTenantAssignmentInput is one row of the replace-all body of
// PUT /system/admin/mcp-services/:id/tenant-assignments. Unlike web search
// there is no default flag — MCP has no per-workspace default semantics.
type SystemMCPTenantAssignmentInput struct {
	TenantID uint64 `json:"tenant_id" binding:"required"`
}

// SystemMCPTenantAssignmentsRequest is the replace-all body.
type SystemMCPTenantAssignmentsRequest struct {
	Assignments []SystemMCPTenantAssignmentInput `json:"assignments"`
}

// UpdateTenantAssignments PUT /system/admin/mcp-services/:id/tenant-assignments
// Replaces the full assignment list of the service. Builtin services are
// rejected — they are visible to every workspace without assignments.
func (h *SystemMCPServiceHandler) UpdateTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	var req SystemMCPTenantAssignmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	rows := make([]types.TenantMCPServiceAssignment, 0, len(req.Assignments))
	tenantIDs := make(map[uint64]bool, len(req.Assignments))
	for _, input := range req.Assignments {
		if input.TenantID == 0 || tenantIDs[input.TenantID] {
			c.Error(apperrors.NewBadRequestError("invalid or duplicate workspace ID in assignments"))
			return
		}
		tenantIDs[input.TenantID] = true
		rows = append(rows, types.TenantMCPServiceAssignment{
			TenantID: input.TenantID,
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
	if err := h.svc.SetServiceAssignments(ctx, id, rows, actorID); err != nil {
		if err == service.ErrMCPServiceNotFound {
			c.Error(apperrors.NewNotFoundError("MCP service not found"))
			return
		}
		logger.Warnf(ctx, "Failed to set assignments of MCP service %s: %v", id, err)
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	assigned := make([]uint64, 0, len(rows))
	for _, row := range rows {
		assigned = append(assigned, row.TenantID)
	}
	h.emitAudit(ctx, types.AuditActionSystemMCPServiceAssigned, "mcp_service", id, map[string]any{
		"tenants": assigned,
	})
	updated, _ := h.assignmentsFor(ctx, id)
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"service_id": id,
		"data":       updated,
	})
}
