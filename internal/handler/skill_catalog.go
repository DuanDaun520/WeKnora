package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
)

// ListCatalog godoc
// @Summary      List workspace skills
// @Description  Returns every skill definition in this workspace and which sandbox configs it is installed on.
// @Tags         Skills
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/catalog [get]
func (h *SkillHandler) ListCatalog(c *gin.Context) {
	if h.catalog == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
		return
	}
	// include_hidden=1 keeps space-hidden skills (000108) in the listing for the
	// 技能目录 management page; everyone else (Skills/MCP browse, agent editor,
	// @mention) reads the default and never sees hidden rows.
	includeHidden := c.Query("include_hidden") == "1" || c.Query("include_hidden") == "true"
	rows, err := h.catalog.ListCatalog(c.Request.Context(), sandboxConfigTenantID(c), includeHidden)
	if err != nil {
		_ = c.Error(err)
		return
	}
	enrichSkillCreatorNames(c.Request.Context(), h.users, rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// RegisterCatalog was removed: workspaces no longer register their own skills.
// Catalog rows only materialize from the platform skill library's workspace
// assignments (see PlatformSkillService), so the tenant-side HTTP surface for
// self-registration (zip upload / source fetch) is gone.

type catalogInstallRequest struct {
	SandboxConfigIDs []string `json:"sandbox_config_ids"`
}

// UpdateCatalogMeta godoc
// @Summary      Edit the category / space visibility of a catalog skill
// @Description  Rewrites only the browser-facing category and the 000108 space-visible
// @Description  switch; bundle and provenance stay untouched.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      200  {object}  map[string]interface{}
// @Router       /skills/catalog/{id} [put]
func (h *SkillHandler) UpdateCatalogMeta(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	limitJSONBody(c, skillSourceJSONMaxBytes)
	var req catalogMetaUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillJSONRequestTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid category request"))
		return
	}
	if len(strings.TrimSpace(req.Category)) > 255 {
		_ = c.Error(apperrors.NewBadRequestError("category is too long"))
		return
	}
	ctx := c.Request.Context()
	tenantID := sandboxConfigTenantID(c)
	catalogID := c.Param("id")

	cat, err := h.catalog.UpdateCatalogMeta(ctx, tenantID, catalogID, req.Category)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	if req.Visible != nil && cat.Visible != *req.Visible {
		cat, err = h.catalog.SetCatalogVisible(ctx, tenantID, catalogID, *req.Visible)
		if err != nil {
			respondSkillServiceError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":       cat.ID,
		"category": cat.Category,
		"visible":  cat.Visible,
	}})
}

type catalogMetaUpdateRequest struct {
	Category string `json:"category"`
	// Visible is optional: absent keeps the current space visibility.
	Visible *bool `json:"visible"`
}

// InstallCatalog godoc
// @Summary      Install a catalog skill onto sandboxes
// @Description  Runs the existing snapshot install onto each named sandbox config.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      202  {object}  map[string]interface{}
// @Router       /skills/catalog/{id}/install [post]
func (h *SkillHandler) InstallCatalog(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	limitJSONBody(c, skillSourceJSONMaxBytes)
	var req catalogInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillJSONRequestTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid install request"))
		return
	}
	result, err := h.catalog.InstallCatalogToConfigs(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), req.SandboxConfigIDs,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	if result == nil {
		result = &service.CatalogInstallResult{}
	}
	data := gin.H{"installs": result.Installs}
	if len(result.Errors) > 0 {
		data["errors"] = result.Errors
	}
	c.JSON(http.StatusAccepted, gin.H{
		"success": len(result.Errors) == 0,
		"data":    data,
	})
}

// DeleteCatalog godoc
// @Summary      Delete a catalog skill
// @Description  Refused while any sandbox still has an installation of this skill.
// @Tags         Skills
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      200  {object}  map[string]interface{}
// @Router       /skills/catalog/{id} [delete]
func (h *SkillHandler) DeleteCatalog(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	if err := h.catalog.DeleteCatalog(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"),
	); err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListCatalogFiles godoc
// @Summary      List files of a catalog skill
// @Description  Lists the stored catalog bundle. Files belong to the skill definition, not a sandbox install.
// @Tags         Skills
// @Produce      json
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      200  {object}  map[string]interface{}
// @Router       /skills/catalog/{id}/files [get]
func (h *SkillHandler) ListCatalogFiles(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	files, err := h.catalog.ListCatalogFiles(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"),
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": files})
}

// GetCatalogFile godoc
// @Summary      Read one file of a catalog skill
// @Tags         Skills
// @Produce      json
// @Param        id    path   string  true  "Catalog skill ID"
// @Param        path  query  string  true  "Skill-root-relative file path"
// @Success      200   {object}  map[string]interface{}
// @Router       /skills/catalog/{id}/files/content [get]
func (h *SkillHandler) GetCatalogFile(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	file, err := h.catalog.ReadCatalogFile(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), c.Query("path"),
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": file})
}
