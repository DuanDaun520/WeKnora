package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// SystemSandboxConnectionHandler backs the /system/admin/sandbox-connections
// endpoints — the platform sandbox catalog of the admin console (000097). A
// connection is configured once here and materialized into workspaces through
// explicit assignments; edits propagate only via the manual push action.
// Handlers must not read the tenant from the request context: a system admin
// may have no workspace binding at all.
type SystemSandboxConnectionHandler struct {
	svc       *service.SandboxConnectionService
	tenantSvc interfaces.TenantService
	// auditSvc is optional — nil in partially-wired unit tests, in which
	// case emitAudit no-ops (same convention as SystemMCPServiceHandler).
	auditSvc interfaces.AuditLogService
}

// NewSystemSandboxConnectionHandler creates a new SystemSandboxConnectionHandler.
func NewSystemSandboxConnectionHandler(
	svc *service.SandboxConnectionService,
	tenantSvc interfaces.TenantService,
	auditSvc interfaces.AuditLogService,
) *SystemSandboxConnectionHandler {
	return &SystemSandboxConnectionHandler{
		svc:       svc,
		tenantSvc: tenantSvc,
		auditSvc:  auditSvc,
	}
}

// emitAudit writes one system-scope audit row for a sandbox-connection
// governance event. Best-effort — a nil audit service or a write failure does
// not bubble up.
func (h *SystemSandboxConnectionHandler) emitAudit(
	ctx context.Context,
	action types.AuditAction,
	targetID string,
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
		TargetType:  "sandbox_connection",
		TargetID:    targetID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     detailsJSON,
	})
}

// systemSandboxConnectionResponse is the console projection of one connection.
// Config goes through the same masking as workspace configs; assignments is
// never null so the card list renders without a nil guard.
type systemSandboxConnectionResponse struct {
	ID          string                                `json:"id"`
	Name        string                                `json:"name"`
	Description string                                `json:"description,omitempty"`
	SandboxType string                                `json:"sandbox_type"`
	Config      *types.TenantSandboxConfig            `json:"config"`
	Assignments []service.SandboxConnectionAssignment `json:"assignments"`
	CreatedAt   time.Time                             `json:"created_at"`
	UpdatedAt   time.Time                             `json:"updated_at"`
}

func (h *SystemSandboxConnectionHandler) toResponse(
	ctx context.Context, e *types.SandboxConnectionEntity,
) systemSandboxConnectionResponse {
	assignments, err := h.svc.ListAssignments(ctx, e.ID)
	if err != nil {
		// The connection itself is fine; its assignment view is not worth a 500.
		logger.Warnf(ctx, "[sandbox-connection] listing assignments of %s failed: %v", e.ID, err)
		assignments = []service.SandboxConnectionAssignment{}
	}
	if assignments == nil {
		assignments = []service.SandboxConnectionAssignment{}
	}
	return systemSandboxConnectionResponse{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		SandboxType: e.SandboxType,
		Config:      types.SandboxConfigForResponse(e.Config, true),
		Assignments: assignments,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

type systemSandboxConnectionRequest struct {
	Name        string                     `json:"name" binding:"required"`
	Description string                     `json:"description,omitempty"`
	Config      *types.TenantSandboxConfig `json:"config"`
}

// respondSandboxConnectionError maps the connection service's errors onto the
// HTTP surface. Returns false for errors it did not recognize so callers can
// fall through to the shared sandbox-config classifiers.
func respondSandboxConnectionError(c *gin.Context, err error) bool {
	switch {
	case stderrors.Is(err, service.ErrSandboxConnectionNotFound):
		c.Error(apperrors.NewNotFoundError("sandbox connection not found"))
	case stderrors.Is(err, service.ErrSandboxConnectionNameConflict):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "name_conflict",
				"message": "已存在同名沙箱连接",
			},
		})
	default:
		var assigned *service.AssignmentsExistError
		if !stderrors.As(err, &assigned) {
			return false
		}
		assignments := assigned.Assignments
		if assignments == nil {
			assignments = []service.SandboxConnectionAssignment{}
		}
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "assignments_exist",
				"message": "该连接仍被分配给空间，请先取消分配",
				"data":    gin.H{"assignments": assignments},
			},
		})
	}
	return true
}

// respondSandboxConnectionServiceError layers the connection-specific mapping
// on top of the two classifiers the workspace sandbox surface already uses, so
// input sentinels stay 400 and provider refusals keep their exact shapes.
func respondSandboxConnectionServiceError(c *gin.Context, err error) {
	if respondSandboxConnectionError(c, err) {
		return
	}
	if respondSandboxConfigRefusal(c, err) {
		return
	}
	respondSandboxConfigServiceError(c, err)
}

// ListConnections GET /system/admin/sandbox-connections
func (h *SystemSandboxConnectionHandler) ListConnections(c *gin.Context) {
	ctx := c.Request.Context()
	connections, err := h.svc.List(ctx)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	out := make([]systemSandboxConnectionResponse, 0, len(connections))
	for _, e := range connections {
		out = append(out, h.toResponse(ctx, e))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// GetConnection GET /system/admin/sandbox-connections/:id
func (h *SystemSandboxConnectionHandler) GetConnection(c *gin.Context) {
	ctx := c.Request.Context()
	e, err := h.svc.Get(ctx, secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": h.toResponse(ctx, e)})
}

// CreateConnection POST /system/admin/sandbox-connections
// Body contract mirrors the workspace config route; the connection starts
// unassigned and carries no row-local state (SkillImage/VolumeMount are
// stripped by the service).
func (h *SystemSandboxConnectionHandler) CreateConnection(c *gin.Context) {
	ctx := c.Request.Context()
	var req systemSandboxConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	created, err := h.svc.Create(ctx, service.CreateSandboxConnectionInput{
		Name:        req.Name,
		Description: req.Description,
		Config:      req.Config,
	})
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemSandboxConnectionCreated, created.ID, map[string]any{
		"name": created.Name, "sandbox_type": created.SandboxType,
	})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": h.toResponse(ctx, created)})
}

// UpdateConnection PUT /system/admin/sandbox-connections/:id
// Assignments are NOT touched here; the edit lights drift markers on stale
// materialized rows, cleared by the explicit push action.
func (h *SystemSandboxConnectionHandler) UpdateConnection(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	var req systemSandboxConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	updated, err := h.svc.Update(ctx, id, service.UpdateSandboxConnectionInput{
		Name:        req.Name,
		Description: req.Description,
		Config:      req.Config,
	})
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemSandboxConnectionUpdated, id, map[string]any{
		"name": updated.Name,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": h.toResponse(ctx, updated)})
}

// DeleteConnection DELETE /system/admin/sandbox-connections/:id
// Refused with 409 assignments_exist while live materialized rows exist — they
// are real workspace configs that may own skills and sandboxes, so unassigning
// them is a guarded per-workspace decision, not a delete-side cascade.
func (h *SystemSandboxConnectionHandler) DeleteConnection(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if err := h.svc.Delete(ctx, id); err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemSandboxConnectionDeleted, id, nil)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListTenantAssignments GET /system/admin/sandbox-connections/:id/tenant-assignments
func (h *SystemSandboxConnectionHandler) ListTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	assignments, err := h.svc.ListAssignments(ctx, id)
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	if assignments == nil {
		assignments = []service.SandboxConnectionAssignment{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": assignments})
}

// systemSandboxConnectionAssignmentInput is one row of the replace-all body of
// PUT /system/admin/sandbox-connections/:id/tenant-assignments.
type systemSandboxConnectionAssignmentInput struct {
	TenantID uint64 `json:"tenant_id" binding:"required"`
}

// systemSandboxConnectionAssignmentsRequest is the replace-all body.
type systemSandboxConnectionAssignmentsRequest struct {
	Assignments []systemSandboxConnectionAssignmentInput `json:"assignments"`
}

// UpdateTenantAssignments PUT /system/admin/sandbox-connections/:id/tenant-assignments
// Replaces the full assignment set. Partial success is the norm: adding one
// workspace and removing another report independently, and a blocked removal
// (installed skills / live sandboxes) never rolls back the rest.
func (h *SystemSandboxConnectionHandler) UpdateTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	var req systemSandboxConnectionAssignmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	tenantIDs := make([]uint64, 0, len(req.Assignments))
	seen := make(map[uint64]bool, len(req.Assignments))
	for _, input := range req.Assignments {
		if input.TenantID == 0 || seen[input.TenantID] {
			c.Error(apperrors.NewBadRequestError("invalid or duplicate workspace ID in assignments"))
			return
		}
		seen[input.TenantID] = true
		tenantIDs = append(tenantIDs, input.TenantID)
	}

	// Reject unknown workspace IDs with 404 rather than silently dropping
	// them — the console lists workspaces that may have been deleted since.
	if h.tenantSvc != nil {
		for _, tenantID := range tenantIDs {
			tenant, err := h.tenantSvc.GetTenantByID(ctx, tenantID)
			if err != nil || tenant == nil {
				c.Error(apperrors.NewNotFoundError("workspace not found"))
				return
			}
		}
	}

	results, assignments, err := h.svc.ReplaceAssignments(ctx, id, tenantIDs)
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	if results == nil {
		results = []service.AssignmentOutcome{}
	}
	if assignments == nil {
		assignments = []service.SandboxConnectionAssignment{}
	}
	h.emitAudit(ctx, types.AuditActionSystemSandboxConnectionAssigned, id, map[string]any{
		"tenants": tenantIDs,
	})
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"assignments": assignments,
		"results":     results,
	})
}

// PushConnection POST /system/admin/sandbox-connections/:id/push
// Propagates the connection's current payload to every materialized row
// through the workspace update flow. Per-workspace outcomes are always 200 —
// a blocked workspace is a result, not a request failure.
func (h *SystemSandboxConnectionHandler) PushConnection(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	results, err := h.svc.Push(ctx, id)
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	if results == nil {
		results = []service.PushOutcome{}
	}
	updated, blocked := 0, 0
	for _, r := range results {
		switch r.Status {
		case "updated":
			updated++
		case "blocked_live_sandboxes", "blocked_skill_snapshot":
			blocked++
		}
	}
	h.emitAudit(ctx, types.AuditActionSystemSandboxConnectionPushed, id, map[string]any{
		"updated": updated, "blocked": blocked,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"results": results,
		"updated": updated,
		"blocked": blocked,
	})
}

// CheckConnection POST /system/admin/sandbox-connections/:id/check
// Probes a saved connection. Masked secrets in the body resolve against the
// stored payload, so the drawer can test edits without retyping the API key.
func (h *SystemSandboxConnectionHandler) CheckConnection(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	id := secutils.SanitizeForLog(c.Param("id"))
	var req SandboxCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	conn, err := h.svc.Get(ctx, id)
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	stored := conn.Config
	incoming := req.Config
	if incoming == nil {
		incoming = stored
	}
	result, err := runSandboxCheck(ctx, incoming, stored, req.Deep)
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// CheckDraftConnection POST /system/admin/sandbox-connections/check
// Probes an unsaved draft. There is no stored payload to resolve masked
// secrets against, so the form must carry real values.
func (h *SystemSandboxConnectionHandler) CheckDraftConnection(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	var req SandboxCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	result, err := runSandboxCheck(ctx, req.Config, nil, req.Deep)
	if err != nil {
		respondSandboxConnectionServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type systemSandboxConnectionTemplateQueryRequest struct {
	Config          *types.TenantSandboxConfig `json:"config"`
	ConnectionID    string                     `json:"connection_id,omitempty"`
	EnsureStandard  bool                       `json:"ensure_standard"`
	ReplaceStandard bool                       `json:"replace_standard"`
}

// QueryTemplates POST /system/admin/sandbox-connections/templates/query
// The connection drawer's template catalog: same provider view and the same
// ensure/replace standard-template actions as the workspace drawer.
func (h *SystemSandboxConnectionHandler) QueryTemplates(c *gin.Context) {
	var req systemSandboxConnectionTemplateQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	result, err := h.svc.QueryTemplates(c.Request.Context(), service.PlatformTemplateQueryInput{
		Config:          req.Config,
		ConnectionID:    req.ConnectionID,
		EnsureStandard:  req.EnsureStandard,
		ReplaceStandard: req.ReplaceStandard,
	})
	if err != nil {
		if respondSandboxConfigRefusal(c, err) {
			return
		}
		respondSandboxConnectionServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
