package handler

import (
	"context"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// usableSkillLister returns the installed skills a chat turn can actually
// invoke on one sandbox config. The @ picker and the agent editor both read
// this set so they cannot offer a skill the running image does not carry.
type usableSkillLister interface {
	ListUsableSkills(ctx context.Context, tenantID uint64, configID string) []*types.TenantSkillEntity
}

// SkillHandler handles skill-related HTTP requests
type SkillHandler struct {
	usableSkills usableSkillLister
	catalog      skillCatalogService
	// users resolves CreatedBy ids into display names for catalog listings.
	// May be nil in tests that skip creator enrichment.
	users interfaces.UserService
}

type skillCatalogService interface {
	// includeHidden=false hides space-hidden skills (000108) from the browsing
	// surface; the 技能目录 management page passes true.
	ListCatalog(ctx context.Context, tenantID uint64, includeHidden bool) ([]service.SkillCatalogView, error)
	UpdateCatalogMeta(ctx context.Context, tenantID uint64, catalogID, category string) (*types.TenantSkillCatalogEntity, error)
	SetCatalogVisible(ctx context.Context, tenantID uint64, catalogID string, visible bool) (*types.TenantSkillCatalogEntity, error)
	InstallCatalogToConfigs(ctx context.Context, tenantID uint64, catalogID string, configIDs []string) (*service.CatalogInstallResult, error)
	DeleteCatalog(ctx context.Context, tenantID uint64, catalogID string) error
	ListCatalogFiles(ctx context.Context, tenantID uint64, catalogID string) ([]service.SkillFileEntry, error)
	ReadCatalogFile(ctx context.Context, tenantID uint64, catalogID, relativePath string) (*service.SkillFileContent, error)
}

// NewSkillHandler creates a new skill handler. catalog may be nil in tests
// that only exercise the chat picker; users may be nil to skip enrichment.
func NewSkillHandler(usableSkills usableSkillLister, catalog skillCatalogService, users interfaces.UserService) *SkillHandler {
	return &SkillHandler{
		usableSkills: usableSkills,
		catalog:      catalog,
		users:        users,
	}
}

// enrichSkillCreatorNames resolves view.CreatedBy into CreatorName in place.
// Views are a value slice, so we write back by index. Failures are swallowed —
// the listing stays usable without display names. Mirrors enrichAgentCreatorNames.
func enrichSkillCreatorNames(ctx context.Context, userSvc interfaces.UserService, rows []service.SkillCatalogView) {
	if userSvc == nil || len(rows) == 0 {
		return
	}
	idSet := make(map[string]struct{}, len(rows))
	for i := range rows {
		if rows[i].CreatedBy == "" {
			continue
		}
		idSet[rows[i].CreatedBy] = struct{}{}
	}
	if len(idSet) == 0 {
		return
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	users, err := userSvc.GetUsersByIDs(ctx, ids)
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve skill creator names: %v", err)
		return
	}
	for i := range rows {
		if rows[i].CreatedBy == "" {
			continue
		}
		if u, ok := users[rows[i].CreatedBy]; ok && u != nil {
			rows[i].CreatorName = pickUserDisplayName(u)
		}
	}
}

// SkillInfoResponse represents the skill info returned to frontend
type SkillInfoResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListSkills godoc
// @Summary      获取当前沙箱配置上可执行的 Skills
// @Description  返回指定沙箱配置镜像内、智能体实际能调用的已安装技能（ready 且启用）。不传 sandbox_config_id 时列表为空。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        sandbox_config_id  query     string  false  "Sandbox config ID"
// @Success      200  {object}  map[string]interface{}  "Skills列表"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills [get]
func (h *SkillHandler) ListSkills(c *gin.Context) {
	configID := c.Query("sandbox_config_id")
	if configID == "" || h.usableSkills == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"data":             []SkillInfoResponse{},
			"skills_available": false,
		})
		return
	}

	rows := h.usableSkills.ListUsableSkills(
		c.Request.Context(), sandboxConfigTenantID(c), configID,
	)
	response := make([]SkillInfoResponse, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		response = append(response, SkillInfoResponse{
			Name:        row.Name,
			Description: row.Description,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             response,
		"skills_available": true,
	})
}
