package handler

import (
	"net/http"
	"strconv"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/storageallowlist"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type PlatformStorageEngineHandler struct {
	service *service.PlatformStorageEngineService
}

func NewPlatformStorageEngineHandler(service *service.PlatformStorageEngineService) *PlatformStorageEngineHandler {
	return &PlatformStorageEngineHandler{service: service}
}

type platformStorageEngineRequest struct {
	Name     string                     `json:"name" binding:"required"`
	Provider string                     `json:"provider" binding:"required"`
	Config   types.StorageBackendConfig `json:"config"`
	Status   string                     `json:"status,omitempty"`
}

// List godoc
// @Summary      List platform storage engines
// @Description  List all platform-level storage engines (admin only). Credentials are masked.
// @Tags         PlatformStorageEngine
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of platform storage engines"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Security     Bearer
// @Router       /platform/storage-engines [get]
func (h *PlatformStorageEngineHandler) List(c *gin.Context) {
	engines, err := h.service.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	result := make([]types.PlatformStorageEngine, 0, len(engines))
	for _, engine := range engines {
		result = append(result, types.NewPlatformStorageEngineResponse(engine))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// Get godoc
// @Summary      Get platform storage engine
// @Description  Retrieve a single platform storage engine by ID (admin only). Credentials are masked.
// @Tags         PlatformStorageEngine
// @Produce      json
// @Param        id   path      string  true  "Platform storage engine ID"
// @Success      200  {object}  map[string]interface{}   "Platform storage engine details"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Failure      404  {object}  apperrors.AppError          "Storage engine not found"
// @Security     Bearer
// @Router       /platform/storage-engines/{id} [get]
func (h *PlatformStorageEngineHandler) Get(c *gin.Context) {
	engine, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	if engine == nil {
		c.Error(apperrors.NewNotFoundError("storage engine not found"))
		return
	}
	// Get tenant count for this engine
	count, err := h.service.CountTenants(c.Request.Context(), engine.ID)
	if err != nil {
		c.Error(err)
		return
	}
	response := types.NewPlatformStorageEngineResponse(engine)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response, "tenant_count": count})
}

// Create godoc
// @Summary      Create platform storage engine
// @Description  Create a new platform-level storage engine (admin only). Configuration is validated and connectivity tested.
// @Tags         PlatformStorageEngine
// @Accept       json
// @Produce      json
// @Param        request  body      platformStorageEngineRequest    true  "Storage engine configuration"
// @Success      201      {object}  map[string]interface{}   "Created storage engine"
// @Failure      400      {object}  apperrors.AppError          "Invalid request, validation, or connectivity test failure"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      403      {object}  map[string]interface{}   "Forbidden"
// @Failure      409      {object}  apperrors.AppError          "A storage engine with this name already exists"
// @Security     Bearer
// @Router       /platform/storage-engines [post]
func (h *PlatformStorageEngineHandler) Create(c *gin.Context) {
	var req platformStorageEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	engine := &types.PlatformStorageEngine{Name: req.Name, Provider: req.Provider, Config: req.Config, Status: req.Status}
	if err := h.service.Create(c.Request.Context(), engine); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": types.NewPlatformStorageEngineResponse(engine)})
}

// Update godoc
// @Summary      Update platform storage engine
// @Description  Update a platform storage engine's mutable fields (name, credentials, status). Provider and physical location are immutable.
// @Tags         PlatformStorageEngine
// @Accept       json
// @Produce      json
// @Param        id       path      string                           true  "Platform storage engine ID"
// @Param        request  body      platformStorageEngineRequest    true  "Updated storage engine fields"
// @Success      200      {object}  map[string]interface{}   "Updated storage engine"
// @Failure      400      {object}  apperrors.AppError          "Immutable field change, validation, or connectivity failure"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      403      {object}  map[string]interface{}   "Forbidden"
// @Failure      404      {object}  apperrors.AppError          "Storage engine not found"
// @Security     Bearer
// @Router       /platform/storage-engines/{id} [put]
func (h *PlatformStorageEngineHandler) Update(c *gin.Context) {
	var req platformStorageEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	engine := &types.PlatformStorageEngine{ID: c.Param("id"), Name: req.Name, Provider: req.Provider, Config: req.Config, Status: req.Status}
	if err := h.service.Update(c.Request.Context(), engine); err != nil {
		c.Error(err)
		return
	}
	updated, err := h.service.GetByID(c.Request.Context(), engine.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": types.NewPlatformStorageEngineResponse(updated)})
}

// Delete godoc
// @Summary      Delete platform storage engine
// @Description  Soft-delete a platform storage engine. Cannot delete if workspaces are using it.
// @Tags         PlatformStorageEngine
// @Produce      json
// @Param        id   path      string  true  "Platform storage engine ID"
// @Success      200  {object}  map[string]interface{}   "Deletion success"
// @Failure      400  {object}  apperrors.AppError          "Engine is in use"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Failure      404  {object}  apperrors.AppError          "Storage engine not found"
// @Security     Bearer
// @Router       /platform/storage-engines/{id} [delete]
func (h *PlatformStorageEngineHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestRaw godoc
// @Summary      Test platform storage engine with raw config
// @Description  Test connectivity for the provided storage configuration without persisting it (admin only).
// @Tags         PlatformStorageEngine
// @Accept       json
// @Produce      json
// @Param        request  body      platformStorageEngineRequest    true  "Storage engine configuration to test"
// @Success      200      {object}  map[string]interface{}   "Connectivity test result (success, error)"
// @Failure      400      {object}  apperrors.AppError          "Invalid request or validation error"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Security     Bearer
// @Router       /platform/storage-engines/test [post]
func (h *PlatformStorageEngineHandler) TestRaw(c *gin.Context) {
	var req platformStorageEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	engine := &types.PlatformStorageEngine{Name: req.Name, Provider: req.Provider, Config: req.Config}
	if err := engine.Validate(); err != nil {
		c.Error(err)
		return
	}
	if err := h.service.Test(c.Request.Context(), engine); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": storageTestErrorMessage(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestByID godoc
// @Summary      Test platform storage engine by ID
// @Description  Test connectivity of an existing platform storage engine (admin only).
// @Tags         PlatformStorageEngine
// @Produce      json
// @Param        id   path      string  true  "Platform storage engine ID"
// @Success      200  {object}  map[string]interface{}   "Connectivity test result (success, error)"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Failure      404  {object}  apperrors.AppError          "Storage engine not found"
// @Security     Bearer
// @Router       /platform/storage-engines/{id}/test [post]
func (h *PlatformStorageEngineHandler) TestByID(c *gin.Context) {
	engine, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	if engine == nil {
		c.Error(apperrors.NewNotFoundError("storage engine not found"))
		return
	}
	if err := h.service.Test(c.Request.Context(), engine); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": storageTestErrorMessage(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Types godoc
// @Summary      List allowed storage provider types
// @Description  Return the storage provider types allowed by STORAGE_ALLOW_LIST.
// @Tags         PlatformStorageEngine
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of allowed storage provider types"
// @Security     Bearer
// @Router       /platform/storage-engines/types [get]
func (h *PlatformStorageEngineHandler) Types(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": storageallowlist.AllowedList()})
}

// AssignToTenant godoc
// @Summary      Assign storage engine to tenant
// @Description  Assign a platform storage engine to a specific workspace (admin only).
// @Tags         PlatformStorageEngine
// @Accept       json
// @Produce      json
// @Param        tenantId   path      int  true  "Tenant ID"
// @Param        request    body      map[string]string  true  "Engine ID to assign"
// @Success      200  {object}  map[string]interface{}   "Assignment success"
// @Failure      400  {object}  apperrors.AppError          "Invalid request"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Security     Bearer
// @Router       /tenants/{tenantId}/storage-engine [put]
func (h *PlatformStorageEngineHandler) AssignToTenant(c *gin.Context) {
	tenantIDStr := c.Param("tenantId")
	tenantID, err := strconv.ParseUint(tenantIDStr, 10, 64)
	if err != nil {
		c.Error(apperrors.NewBadRequestError("invalid tenant ID"))
		return
	}
	var req struct {
		PlatformStorageEngineID string `json:"platform_storage_engine_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	// Get admin user ID from context
	adminUserID := c.GetUint64(types.UserIDContextKey.String())
	if err := h.service.AssignToTenant(c.Request.Context(), tenantID, req.PlatformStorageEngineID, adminUserID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// UnassignFromTenant godoc
// @Summary      Unassign storage engine from tenant
// @Description  Remove the platform storage engine assignment from a workspace (admin only).
// @Tags         PlatformStorageEngine
// @Produce      json
// @Param        tenantId   path      int  true  "Tenant ID"
// @Success      200  {object}  map[string]interface{}   "Unassignment success"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      403  {object}  map[string]interface{}   "Forbidden"
// @Security     Bearer
// @Router       /tenants/{tenantId}/storage-engine [delete]
func (h *PlatformStorageEngineHandler) UnassignFromTenant(c *gin.Context) {
	tenantIDStr := c.Param("tenantId")
	tenantID, err := strconv.ParseUint(tenantIDStr, 10, 64)
	if err != nil {
		c.Error(apperrors.NewBadRequestError("invalid tenant ID"))
		return
	}
	if err := h.service.UnassignFromTenant(c.Request.Context(), tenantID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}