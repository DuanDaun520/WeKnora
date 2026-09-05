package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// usageRecordRepository reads the usage ledger and the price table.
// Ledger writes come from the metering manager (batch inserts); this
// repository owns the report/aggregate reads and the price CRUD.
type usageRecordRepository struct {
	db *gorm.DB
}

// NewUsageRecordRepository constructs the production repository backed by
// the shared GORM connection.
func NewUsageRecordRepository(db *gorm.DB) interfaces.UsageRecordRepository {
	return &usageRecordRepository{db: db}
}

// Summary aggregates the ledger. The dimension list is derived from
// q.GroupBy; category is always included so each billable kind stays
// visible in one pass. Day grouping uses a dialect-specific expression.
func (r *usageRecordRepository) Summary(ctx context.Context, q *interfaces.UsageQuery) ([]interfaces.UsageSummaryRow, error) {
	dims := summaryDims(r.db, q)

	selects := append([]string{}, dims...)
	selects = append(selects,
		"COUNT(*) AS calls",
		"COALESCE(SUM(input_tokens), 0) AS input_tokens",
		"COALESCE(SUM(output_tokens), 0) AS output_tokens",
		"COALESCE(SUM(cached_tokens), 0) AS cached_tokens",
		"COALESCE(SUM(images), 0) AS images",
		"COALESCE(SUM(audio_seconds), 0) AS audio_seconds",
	)

	where, args := usageWhereClause(q)
	sql := fmt.Sprintf("SELECT %s FROM model_usage_records", joinStrings(selects, ", "))
	if where != "" {
		sql += " WHERE " + where
	}
	sql += fmt.Sprintf(" GROUP BY %s ORDER BY 1", joinStrings(dims, ", "))

	rows, err := r.db.WithContext(ctx).Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []interfaces.UsageSummaryRow{}
	for rows.Next() {
		var (
			row                interfaces.UsageSummaryRow
			day, modelID       string
			modelName          string
			category, userID   string
			tenantID           int64
		)
		targets := make([]interface{}, 0, len(dims)+6)
		for _, d := range dims {
			switch d {
			case "tenant_id":
				targets = append(targets, &tenantID)
			case "user_id":
				targets = append(targets, &userID)
			case "model_id":
				targets = append(targets, &modelID)
			case "model_name":
				targets = append(targets, &modelName)
			case "category":
				targets = append(targets, &category)
			default: // the day expression
				targets = append(targets, &day)
			}
		}
		targets = append(targets,
			&row.Calls, &row.InputTokens, &row.OutputTokens, &row.CachedTokens,
			&row.Images, &row.AudioSeconds,
		)
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		row.TenantID = uint64(tenantID)
		row.UserID = userID
		row.Day = day
		row.ModelID = modelID
		row.ModelName = modelName
		row.Category = category
		out = append(out, row)
	}
	return out, rows.Err()
}

// Records returns paginated raw ledger rows plus the unpaginated total.
func (r *usageRecordRepository) Records(ctx context.Context, q *interfaces.UsageQuery) ([]*types.ModelUsageRecord, int64, error) {
	limit := 50
	if q != nil && q.Limit > 0 {
		limit = q.Limit
	}
	if limit > 200 {
		limit = 200
	}
	offset := 0
	if q != nil && q.Offset > 0 {
		offset = q.Offset
	}

	var total int64
	if err := usageScope(r.db.WithContext(ctx).Model(&types.ModelUsageRecord{}), q).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*types.ModelUsageRecord
	if err := usageScope(r.db.WithContext(ctx).Model(&types.ModelUsageRecord{}), q).
		Order("occurred_at DESC, id DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *usageRecordRepository) ListPrices(ctx context.Context) ([]*types.ModelPrice, error) {
	var prices []*types.ModelPrice
	err := r.db.WithContext(ctx).Order("model_id").Find(&prices).Error
	return prices, err
}

func (r *usageRecordRepository) GetPriceByModelID(ctx context.Context, modelID string) (*types.ModelPrice, error) {
	var price types.ModelPrice
	err := r.db.WithContext(ctx).Where("model_id = ?", modelID).First(&price).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &price, nil
}

func (r *usageRecordRepository) UpsertPrice(ctx context.Context, p *types.ModelPrice) error {
	if p == nil || p.ModelID == "" {
		return errors.New("model_id is required")
	}
	var existing types.ModelPrice
	err := r.db.WithContext(ctx).Where("model_id = ?", p.ModelID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.WithContext(ctx).Create(p).Error
	}
	if err != nil {
		return err
	}
	existing.PriceInputPerM = p.PriceInputPerM
	existing.PriceOutputPerM = p.PriceOutputPerM
	existing.PriceCachedPerM = p.PriceCachedPerM
	existing.PricePerM = p.PricePerM
	existing.PricePerImage = p.PricePerImage
	existing.PricePerAudioMin = p.PricePerAudioMin
	existing.PricePerVideoMin = p.PricePerVideoMin
	existing.Currency = p.Currency
	existing.Enabled = p.Enabled
	existing.EffectiveFrom = p.EffectiveFrom
	return r.db.WithContext(ctx).Save(&existing).Error
}

func (r *usageRecordRepository) DeletePrice(ctx context.Context, modelID string) error {
	return r.db.WithContext(ctx).
		Where("model_id = ?", modelID).
		Delete(&types.ModelPrice{}).Error
}

// summaryDims derives the GROUP BY dimension list. Model and category are
// ALWAYS present: pricing is per-model (设计 §6 折算口径), so a row without
// model_id could never fold an amount — the "按类型" view therefore returns
// model × category granularity, same as the day view.
func summaryDims(db *gorm.DB, q *interfaces.UsageQuery) []string {
	var dims []string
	if q != nil {
		switch q.GroupBy {
		case "day":
			dims = []string{dayExpr(db)}
		case "user":
			dims = []string{"user_id"}
		case "tenant":
			dims = []string{"tenant_id"}
		case "tenant_user":
			dims = []string{"tenant_id", "user_id"}
		}
	}
	return append(dims, "model_id", "model_name", "category")
}

// dayExpr renders the YYYY-MM-DD grouping key for the active dialect.
func dayExpr(db *gorm.DB) string {
	if db.Dialector.Name() == "postgres" {
		return "to_char(occurred_at, 'YYYY-MM-DD')"
	}
	return "strftime('%Y-%m-%d', occurred_at)"
}

// usageScope adds the shared WHERE dimensions to a GORM query builder.
func usageScope(tx *gorm.DB, q *interfaces.UsageQuery) *gorm.DB {
	if q == nil {
		return tx
	}
	if q.TenantID != 0 {
		tx = tx.Where("tenant_id = ?", q.TenantID)
	}
	if q.UserID != "" {
		tx = tx.Where("user_id = ?", q.UserID)
	}
	if !q.From.IsZero() {
		tx = tx.Where("occurred_at >= ?", q.From)
	}
	if !q.To.IsZero() {
		tx = tx.Where("occurred_at < ?", q.To)
	}
	if q.Category != "" {
		tx = tx.Where("category = ?", q.Category)
	}
	if q.Purpose != "" {
		tx = tx.Where("purpose = ?", q.Purpose)
	}
	if q.ModelID != "" {
		tx = tx.Where("model_id = ?", q.ModelID)
	}
	return tx
}

// usageWhereClause is the raw-SQL twin of usageScope for Summary.
func usageWhereClause(q *interfaces.UsageQuery) (string, []interface{}) {
	clauses := []string{}
	args := []interface{}{}
	if q == nil {
		return "", args
	}
	if q.TenantID != 0 {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, q.TenantID)
	}
	if q.UserID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, q.UserID)
	}
	if !q.From.IsZero() {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, q.From)
	}
	if !q.To.IsZero() {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, q.To)
	}
	if q.Category != "" {
		clauses = append(clauses, "category = ?")
		args = append(args, q.Category)
	}
	if q.Purpose != "" {
		clauses = append(clauses, "purpose = ?")
		args = append(args, q.Purpose)
	}
	if q.ModelID != "" {
		clauses = append(clauses, "model_id = ?")
		args = append(args, q.ModelID)
	}
	return joinStrings(clauses, " AND "), args
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
