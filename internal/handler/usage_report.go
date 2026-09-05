package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// UsageReportHandler serves the three usage-report levels and the price
// configuration (docs/Token统计与计费设计.md §5). Identity always comes from
// the gin context — never from a request field — so a caller cannot read
// another user's or another space's usage.
type UsageReportHandler struct {
	svc interfaces.UsageReportService
}

func NewUsageReportHandler(svc interfaces.UsageReportService) *UsageReportHandler {
	return &UsageReportHandler{svc: svc}
}

// parseUsageQuery fills the shared filter shape from query params.
// from/to accept YYYY-MM-DD (day-granular) or RFC3339.
func parseUsageQuery(c *gin.Context) (*interfaces.UsageQuery, error) {
	q := &interfaces.UsageQuery{
		Category: c.Query("category"),
		Purpose:  c.Query("purpose"),
		ModelID:  c.Query("model_id"),
		GroupBy:  c.DefaultQuery("group_by", "model"),
	}
	switch q.GroupBy {
	case "model", "category", "day", "user", "tenant", "tenant_user", "":
	default:
		return nil, apperrors.NewBadRequestError("invalid group_by")
	}
	for name, target := range map[string]*time.Time{"from": &q.From, "to": &q.To} {
		v := c.Query(name)
		if v == "" {
			continue
		}
		if len(v) == 10 { // YYYY-MM-DD
			v += "T00:00:00Z"
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, apperrors.NewBadRequestError("invalid "+name)
		}
		*target = t
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Offset = n
		}
	}
	return q, nil
}

// MeUsageSummary serves GET /api/v1/me/usage/summary — the caller's own
// usage within the current space (personal settings panel).
func (h *UsageReportHandler) MeUsageSummary(c *gin.Context) {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID := c.GetString(types.UserIDContextKey.String())
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	q, err := parseUsageQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	q.TenantID = tenantID
	q.UserID = userID
	rows, err := h.svc.Summary(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

// TenantUsageSummary serves GET /tenants/:tenant_id/usage/summary?scope=me|tenant
// — the space-admin panel. Guarded by g.Admin() on the route.
func (h *UsageReportHandler) TenantUsageSummary(c *gin.Context) {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	q, err := parseUsageQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	q.TenantID = tenantID
	if c.Query("scope") == "me" {
		q.UserID = c.GetString(types.UserIDContextKey.String())
	}
	rows, err := h.svc.Summary(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

// AdminUsageSummary serves GET /system/admin/usage/summary?level=platform|tenant|user
// — the platform console dashboard. Guarded by g.SystemAdmin() on the route.
func (h *UsageReportHandler) AdminUsageSummary(c *gin.Context) {
	q, err := parseUsageQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	level := c.DefaultQuery("level", "platform")
	switch level {
	case "platform":
		// No scope: every tenant, every user.
	case "tenant":
		if q.GroupBy == "" || q.GroupBy == "model" || q.GroupBy == "category" {
			q.GroupBy = "tenant"
		}
	case "tenant_user":
		q.GroupBy = "tenant_user"
	case "user":
		q.GroupBy = "user"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid level"})
		return
	}
	if v := c.Query("tenant_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			q.TenantID = id
		}
	}
	if v := c.Query("user_id"); v != "" {
		q.UserID = v
	}
	rows, err := h.svc.Summary(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

// AdminUsageRecords serves GET /system/admin/usage/records — the paginated
// raw ledger (admin detail view).
func (h *UsageReportHandler) AdminUsageRecords(c *gin.Context) {
	q, err := parseUsageQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if v := c.Query("tenant_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			q.TenantID = id
		}
	}
	q.UserID = c.Query("user_id")
	rows, total, err := h.svc.Records(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows, "total": total,
		"limit": q.Limit, "offset": q.Offset})
}

// ListModelPrices serves GET /system/admin/model-prices.
func (h *UsageReportHandler) ListModelPrices(c *gin.Context) {
	prices, err := h.svc.ListPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

// UpsertModelPrice serves PUT /system/admin/model-prices/:model_id.
func (h *UsageReportHandler) UpsertModelPrice(c *gin.Context) {
	modelID := c.Param("model_id")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model_id is required"})
		return
	}
	var req types.ModelPrice
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ModelID = modelID
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if req.EffectiveFrom.IsZero() {
		req.EffectiveFrom = time.Now().UTC()
	}
	if err := h.svc.UpsertPrice(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"price": req})
}

// DeleteModelPrice serves DELETE /system/admin/model-prices/:model_id.
func (h *UsageReportHandler) DeleteModelPrice(c *gin.Context) {
	modelID := c.Param("model_id")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model_id is required"})
		return
	}
	if err := h.svc.DeletePrice(c.Request.Context(), modelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": modelID})
}
