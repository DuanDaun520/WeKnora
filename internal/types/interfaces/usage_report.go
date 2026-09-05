package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// UsageQuery filters + shapes a usage-ledger report (docs/Token统计与计费设计.md).
// The three report levels are the same query with different scopes:
// platform (no filter), tenant (TenantID), user (UserID, within TenantID
// for the /me and tenant views, global for the admin user view).
type UsageQuery struct {
	TenantID uint64    // 0 = all tenants (platform level)
	UserID   string    // "" = all users
	From     time.Time // inclusive; zero = unbounded
	To       time.Time // exclusive; zero = unbounded
	Category string    // "" = all categories
	Purpose  string    // "" = all purposes
	ModelID  string    // "" = all models
	// GroupBy is one of "model", "category", "day", "user", "tenant";
	// empty returns a single total row.
	GroupBy string
	// Limit/Offset page the records listing (summary is not paged).
	Limit  int
	Offset int
}

// UsageSummaryRow is one aggregated line of a usage report. Dimension
// fields are filled per GroupBy; Amount is folded by the service from
// model_prices (display-only, CNY).
type UsageSummaryRow struct {
	TenantID       uint64  `json:"tenant_id,omitempty"`
	UserID         string  `json:"user_id,omitempty"`
	Day            string  `json:"day,omitempty"` // YYYY-MM-DD when grouped by day
	ModelID        string  `json:"model_id,omitempty"`
	ModelName      string  `json:"model_name,omitempty"`
	Category       string  `json:"category"`
	Calls          int64   `json:"calls"`
	InputTokens    int64   `json:"input_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	CachedTokens   int64   `json:"cached_tokens"`
	Images         int64   `json:"images"`
	AudioSeconds   float64 `json:"audio_seconds"`
	Amount         float64 `json:"amount"`
	Approximate    bool    `json:"approximate"`
	HasPriceConfig bool    `json:"has_price_config"`
}

// UsageRecordRepository is the persistence side of the usage report
// feature: ledger aggregation reads and price CRUD.
type UsageRecordRepository interface {
	Summary(ctx context.Context, q *UsageQuery) ([]UsageSummaryRow, error)
	Records(ctx context.Context, q *UsageQuery) ([]*types.ModelUsageRecord, int64, error)
	ListPrices(ctx context.Context) ([]*types.ModelPrice, error)
	GetPriceByModelID(ctx context.Context, modelID string) (*types.ModelPrice, error)
	UpsertPrice(ctx context.Context, p *types.ModelPrice) error
	DeletePrice(ctx context.Context, modelID string) error
}

// UsageReportService serves the three usage report levels plus the
// admin-maintained price configuration; it folds amounts from the price
// table on top of the repository aggregates.
type UsageReportService interface {
	// Summary aggregates the ledger per q.GroupBy and folds amounts.
	Summary(ctx context.Context, q *UsageQuery) ([]UsageSummaryRow, error)
	// Records returns paginated raw ledger rows (admin detail view).
	Records(ctx context.Context, q *UsageQuery) ([]*types.ModelUsageRecord, int64, error)
	// ListPrices returns every configured price row.
	ListPrices(ctx context.Context) ([]*types.ModelPrice, error)
	// UpsertPrice creates or replaces the price row of one model.
	UpsertPrice(ctx context.Context, price *types.ModelPrice) error
	// DeletePrice removes the price row of one model.
	DeletePrice(ctx context.Context, modelID string) error
}
