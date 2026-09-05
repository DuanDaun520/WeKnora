package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// SystemSkillHandler backs the /system/admin/skills endpoints — the platform
// skill library of the admin console (000098). A skill is registered once
// here and materialized into workspaces through explicit assignments; edits
// propagate only via the manual push action, installs stay a per-workspace
// decision. Handlers must not read the tenant from the request context: a
// system admin may have no workspace binding at all.
type SystemSkillHandler struct {
	svc       *service.PlatformSkillService
	tenantSvc interfaces.TenantService
	// auditSvc is optional — nil in partially-wired unit tests, in which
	// case emitAudit no-ops (same convention as SystemMCPServiceHandler).
	auditSvc interfaces.AuditLogService
}

// NewSystemSkillHandler creates a new SystemSkillHandler.
func NewSystemSkillHandler(
	svc *service.PlatformSkillService,
	tenantSvc interfaces.TenantService,
	auditSvc interfaces.AuditLogService,
) *SystemSkillHandler {
	return &SystemSkillHandler{
		svc:       svc,
		tenantSvc: tenantSvc,
		auditSvc:  auditSvc,
	}
}

// emitAudit writes one system-scope audit row for a skill-library governance
// event. Best-effort — a nil audit service or a write failure does not bubble
// up.
func (h *SystemSkillHandler) emitAudit(
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
		TargetType:  "platform_skill",
		TargetID:    targetID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     detailsJSON,
	})
}

// systemPlatformSkillResponse is the console projection of one platform
// skill. Instructions are deliberately absent: the file browser already
// serves SKILL.md out of the stored bundle.
type systemPlatformSkillResponse struct {
	ID           string                            `json:"id"`
	Name         string                            `json:"name"`
	Version      string                            `json:"version,omitempty"`
	Description  string                            `json:"description,omitempty"`
	Source       string                            `json:"source,omitempty"`
	BundleSHA256 string                            `json:"bundle_sha256,omitempty"`
	Assignments  []service.PlatformSkillAssignment `json:"assignments"`
	CreatedAt    time.Time                         `json:"created_at"`
	UpdatedAt    time.Time                         `json:"updated_at"`
}

func (h *SystemSkillHandler) toResponse(
	ctx context.Context, e *types.PlatformSkillEntity,
) systemPlatformSkillResponse {
	assignments, err := h.svc.ListAssignments(ctx, e.ID)
	if err != nil {
		// The skill itself is fine; its assignment view is not worth a 500.
		logger.Warnf(ctx, "[platform-skill] listing assignments of %s failed: %v", e.ID, err)
		assignments = []service.PlatformSkillAssignment{}
	}
	if assignments == nil {
		assignments = []service.PlatformSkillAssignment{}
	}
	return systemPlatformSkillResponse{
		ID:           e.ID,
		Name:         e.Name,
		Version:      e.Version,
		Description:  e.Description,
		Source:       e.Source,
		BundleSHA256: e.BundleSHA256,
		Assignments:  assignments,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

// respondPlatformSkillError maps the skill library service's errors onto the
// HTTP surface. Returns false for errors it did not recognize so callers can
// fall through to the shared skill classifiers.
func respondPlatformSkillError(c *gin.Context, err error) bool {
	switch {
	case stderrors.Is(err, service.ErrPlatformSkillNotFound):
		c.Error(apperrors.NewNotFoundError("platform skill not found"))
	case stderrors.Is(err, service.ErrPlatformSkillNameConflict):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "name_conflict",
				"message": "已存在同名平台技能",
			},
		})
	case stderrors.Is(err, service.ErrPlatformSkillNameImmutable):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "name_immutable",
				"message": "技能名称不可修改；如需改名请注册新技能",
			},
		})
	default:
		var assigned *service.PlatformSkillAssignmentsExistError
		if !stderrors.As(err, &assigned) {
			return false
		}
		assignments := assigned.Assignments
		if assignments == nil {
			assignments = []service.PlatformSkillAssignment{}
		}
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "assignments_exist",
				"message": "该技能仍被分配给空间，请先取消分配",
				"data":    gin.H{"assignments": assignments},
			},
		})
	}
	return true
}

// respondPlatformSkillServiceError layers the library-specific mapping on top
// of the shared skill classifier, so bundle/source input sentinels stay 400.
func respondPlatformSkillServiceError(c *gin.Context, err error) {
	if respondPlatformSkillError(c, err) {
		return
	}
	respondSkillServiceError(c, err)
}

// readSkillUploadBody reads the multipart "file" field with the same limits
// as the workspace catalog register route.
func readSkillUploadBody(c *gin.Context) ([]byte, bool) {
	maxBytes := secutils.GetMaxSkillBundleSize()
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillTooLargeError())
			return nil, false
		}
		_ = c.Error(apperrors.NewBadRequestError("file is required"))
		return nil, false
	}
	defer func() { _ = file.Close() }()
	if header.Size > maxBytes {
		_ = c.Error(skillTooLargeError())
		return nil, false
	}
	archive, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError("failed to read the uploaded skill bundle"))
		return nil, false
	}
	if int64(len(archive)) > maxBytes {
		_ = c.Error(skillTooLargeError())
		return nil, false
	}
	return archive, true
}

// bindSkillSourceRequest binds the {"source":"..."} JSON branch with the same
// cap as the workspace register route. Returns false after writing an error.
func bindSkillSourceRequest(c *gin.Context, req *skillSourceRequest) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		var tooLarge *http.MaxBytesError
		if stderrors.As(err, &tooLarge) {
			_ = c.Error(skillSourceRequestTooLargeError())
			return false
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid skill source request"))
		return false
	}
	if strings.TrimSpace(req.Source) == "" {
		_ = c.Error(apperrors.NewBadRequestError("source is required"))
		return false
	}
	return true
}

// ListSkills GET /system/admin/skills
func (h *SystemSkillHandler) ListSkills(c *gin.Context) {
	ctx := c.Request.Context()
	skills, err := h.svc.List(ctx)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	out := make([]systemPlatformSkillResponse, 0, len(skills))
	for _, e := range skills {
		out = append(out, h.toResponse(ctx, e))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// GetSkill GET /system/admin/skills/:id
func (h *SystemSkillHandler) GetSkill(c *gin.Context) {
	ctx := c.Request.Context()
	e, err := h.svc.Get(ctx, secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		respondPlatformSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": h.toResponse(ctx, e)})
}

// CreateSkill POST /system/admin/skills
// Dual-content exactly like the workspace catalog register route: multipart
// "file" (a zip) or JSON {"source":"..."}.
func (h *SystemSkillHandler) CreateSkill(c *gin.Context) {
	ctx := c.Request.Context()
	created, ok := h.registerBody(ctx, c, "")
	if !ok {
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemPlatformSkillCreated, created.ID, map[string]any{
		"name": created.Name, "source": created.Source,
	})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": h.toResponse(ctx, created)})
}

// UpdateSkill PUT /system/admin/skills/:id
// Re-registers the skill's bundle (name immutable). Assignments are NOT
// touched here; the update lights drift markers on assignments, cleared by
// the explicit push action.
func (h *SystemSkillHandler) UpdateSkill(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	updated, ok := h.registerBody(ctx, c, id)
	if !ok {
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemPlatformSkillUpdated, id, map[string]any{
		"name": updated.Name,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": h.toResponse(ctx, updated)})
}

// registerBody runs the shared dual-content body of create and update: an
// empty updateID means create. It owns the whole error path and writes the
// response itself; ok=false means "done, stop".
func (h *SystemSkillHandler) registerBody(
	ctx context.Context, c *gin.Context, updateID string,
) (*types.PlatformSkillEntity, bool) {
	maxBytes := secutils.GetMaxSkillBundleSize()
	limitSkillUploadBody(c, maxBytes)

	var (
		skill   *types.PlatformSkillEntity
		err     error
		source  string
		archive []byte
	)
	if strings.HasPrefix(c.ContentType(), "application/json") {
		var req skillSourceRequest
		if !bindSkillSourceRequest(c, &req) {
			return nil, false
		}
		source = strings.TrimSpace(req.Source)
	} else {
		body, ok := readSkillUploadBody(c)
		if !ok {
			return nil, false
		}
		archive = body
		source = strings.TrimSpace(c.PostForm("source"))
		if source == "" {
			source = "upload"
		}
	}

	if updateID == "" {
		if archive != nil {
			skill, err = h.svc.CreateFromArchive(ctx, archive, source)
		} else {
			skill, err = h.svc.CreateFromSource(ctx, source)
		}
	} else {
		if archive != nil {
			skill, err = h.svc.UpdateFromArchive(ctx, updateID, archive, source)
		} else {
			skill, err = h.svc.UpdateFromSource(ctx, updateID, source)
		}
	}
	if err != nil {
		respondPlatformSkillServiceError(c, err)
		return nil, false
	}
	return skill, true
}

// DeleteSkill DELETE /system/admin/skills/:id
// Refused with 409 assignments_exist while live assignments exist — the
// materialized rows are real workspace catalog rows that may own installs, so
// unassigning them is a guarded per-workspace decision, not a delete-side
// cascade.
func (h *SystemSkillHandler) DeleteSkill(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if err := h.svc.Delete(ctx, id); err != nil {
		respondPlatformSkillServiceError(c, err)
		return
	}
	h.emitAudit(ctx, types.AuditActionSystemPlatformSkillDeleted, id, nil)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListSkillFiles GET /system/admin/skills/:id/files
func (h *SystemSkillHandler) ListSkillFiles(c *gin.Context) {
	files, err := h.svc.ListFiles(c.Request.Context(), secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		respondPlatformSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": files})
}

// GetSkillFile GET /system/admin/skills/:id/files/content?path=
func (h *SystemSkillHandler) GetSkillFile(c *gin.Context) {
	file, err := h.svc.ReadFile(
		c.Request.Context(),
		secutils.SanitizeForLog(c.Param("id")),
		c.Query("path"),
	)
	if err != nil {
		respondPlatformSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": file})
}

// ListTenantAssignments GET /system/admin/skills/:id/tenant-assignments
func (h *SystemSkillHandler) ListTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	assignments, err := h.svc.ListAssignments(ctx, id)
	if err != nil {
		respondPlatformSkillServiceError(c, err)
		return
	}
	if assignments == nil {
		assignments = []service.PlatformSkillAssignment{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": assignments})
}

// systemSkillAssignmentInput is one row of the replace-all body of PUT
// /system/admin/skills/:id/tenant-assignments.
type systemSkillAssignmentInput struct {
	TenantID uint64 `json:"tenant_id" binding:"required"`
}

// systemSkillAssignmentsRequest is the replace-all body.
type systemSkillAssignmentsRequest struct {
	Assignments []systemSkillAssignmentInput `json:"assignments"`
}

// UpdateTenantAssignments PUT /system/admin/skills/:id/tenant-assignments
// Replaces the full assignment set. Partial success is the norm: adding one
// workspace and removing another report independently, and a blocked removal
// (installed skills) or a blocked add (self-built name takeover) never rolls
// back the rest.
func (h *SystemSkillHandler) UpdateTenantAssignments(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	var req systemSkillAssignmentsRequest
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
		respondPlatformSkillServiceError(c, err)
		return
	}
	if results == nil {
		results = []service.PlatformSkillAssignmentOutcome{}
	}
	if assignments == nil {
		assignments = []service.PlatformSkillAssignment{}
	}
	h.emitAudit(ctx, types.AuditActionSystemPlatformSkillAssigned, id, map[string]any{
		"tenants": tenantIDs,
	})
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"assignments": assignments,
		"results":     results,
	})
}

// PushSkill POST /system/admin/skills/:id/push
// Propagates the skill's current bundle to every assignment through the
// workspace register path. Per-workspace outcomes are always 200 — a blocked
// workspace is a result, not a request failure. Push never installs onto any
// sandbox: that stays a per-workspace decision.
func (h *SystemSkillHandler) PushSkill(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	results, err := h.svc.Push(ctx, id)
	if err != nil {
		respondPlatformSkillServiceError(c, err)
		return
	}
	if results == nil {
		results = []service.PlatformSkillPushOutcome{}
	}
	updated, healed, blocked := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "updated":
			updated++
		case "healed":
			updated++
			healed++
		case "blocked":
			blocked++
		}
	}
	h.emitAudit(ctx, types.AuditActionSystemPlatformSkillPushed, id, map[string]any{
		"updated": updated, "blocked": blocked, "healed": healed,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"results": results,
		"updated": updated,
		"healed":  healed,
		"blocked": blocked,
	})
}
