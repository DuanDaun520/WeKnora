package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// usageReportService folds display-only amounts on top of the ledger
// aggregates (docs/Token统计与计费设计.md). Folding rule, per design §6:
// each model's CURRENT price row multiplies its entire usage — effective_from
// records when the price was set but does not slice history (amounts are
// indicative, not invoices).
type usageReportService struct {
	repo interfaces.UsageRecordRepository
}

// NewUsageReportService constructs the report service over the ledger
// repository.
func NewUsageReportService(repo interfaces.UsageRecordRepository) interfaces.UsageReportService {
	return &usageReportService{repo: repo}
}

func (s *usageReportService) Summary(ctx context.Context, q *interfaces.UsageQuery) ([]interfaces.UsageSummaryRow, error) {
	rows, err := s.repo.Summary(ctx, q)
	if err != nil {
		return nil, err
	}
	// Collect the distinct model ids and fold amounts in one pass.
	prices := map[string]*types.ModelPrice{}
	for i := range rows {
		mid := rows[i].ModelID
		if mid == "" {
			continue
		}
		p, ok := prices[mid]
		if !ok {
			p, err = s.repo.GetPriceByModelID(ctx, mid)
			if err != nil {
				return nil, err
			}
			prices[mid] = p
		}
		rows[i].Amount = foldAmount(p, rows[i])
		if p != nil && p.Enabled {
			rows[i].HasPriceConfig = true
		}
	}
	return rows, nil
}

func (s *usageReportService) Records(ctx context.Context, q *interfaces.UsageQuery) ([]*types.ModelUsageRecord, int64, error) {
	return s.repo.Records(ctx, q)
}

func (s *usageReportService) ListPrices(ctx context.Context) ([]*types.ModelPrice, error) {
	return s.repo.ListPrices(ctx)
}

func (s *usageReportService) UpsertPrice(ctx context.Context, p *types.ModelPrice) error {
	return s.repo.UpsertPrice(ctx, p)
}

func (s *usageReportService) DeletePrice(ctx context.Context, modelID string) error {
	return s.repo.DeletePrice(ctx, modelID)
}

// foldAmount computes the display amount of one summary row. Chat-style
// categories price input/output/cached separately; embedding and rerank
// price the input side; multimedia units price per image / audio minute.
// A model without a price row folds to 0 (tokens still reported).
func foldAmount(p *types.ModelPrice, row interfaces.UsageSummaryRow) float64 {
	if p == nil || !p.Enabled {
		return 0
	}
	const perM = 1_000_000.0
	billedInput := row.InputTokens - row.CachedTokens
	if billedInput < 0 {
		billedInput = 0
	}
	amount := 0.0
	switch row.Category {
	case types.UsageCategoryChat, types.UsageCategoryVLM:
		amount += float64(billedInput) / perM * p.PriceInputPerM
		amount += float64(row.CachedTokens) / perM * p.PriceCachedPerM
		amount += float64(row.OutputTokens) / perM * p.PriceOutputPerM
		// VLM additionally bills per image when that口径 was configured.
		amount += float64(row.Images) * p.PricePerImage
	case types.UsageCategoryEmbedding, types.UsageCategoryRerank:
		amount += float64(row.InputTokens) / perM * p.PricePerM
	case types.UsageCategoryASR:
		amount += row.AudioSeconds / 60.0 * p.PricePerAudioMin
		amount += float64(billedInput) / perM * p.PriceInputPerM
		amount += float64(row.OutputTokens) / perM * p.PriceOutputPerM
	default:
		// Unknown category — fall back to the generic token prices so new
		// categories still fold instead of silently showing 0.
		amount += float64(billedInput) / perM * p.PriceInputPerM
		amount += float64(row.OutputTokens) / perM * p.PriceOutputPerM
	}
	return amount
}
