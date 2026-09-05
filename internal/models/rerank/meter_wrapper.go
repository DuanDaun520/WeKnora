package rerank

import (
	"context"

	"github.com/Tencent/WeKnora/internal/metering"
	"github.com/Tencent/WeKnora/internal/types"
)

// meterReranker wraps a Reranker and appends a usage-ledger record per call
// (docs/Token统计与计费设计.md). Provider usage is not surfaced by the
// interface, so tokens (query + candidate documents) are estimated and
// flagged extra.approx=true; the document count rides in extra.documents
// for per-1k-request style pricing. Failed calls are not recorded.
type meterReranker struct {
	inner Reranker
}

func wrapRerankerMeter(r Reranker, err error) (Reranker, error) {
	if err != nil || r == nil {
		return r, err
	}
	return &meterReranker{inner: r}, nil
}

func (m *meterReranker) Rerank(ctx context.Context, query string, documents []string) ([]RankResult, error) {
	results, err := m.inner.Rerank(ctx, query, documents)
	if err == nil {
		tenantID, userID, purpose := metering.AttributionFromContext(ctx)
		total := metering.ApproxTokens(query)
		for _, doc := range documents {
			total += metering.ApproxTokens(doc)
		}
		if purpose == "" {
			purpose = "rerank"
		}
		metering.GetManager().Submit(&types.ModelUsageRecord{
			TenantID:    tenantID,
			UserID:      userID,
			ModelID:     m.inner.GetModelID(),
			ModelName:   m.inner.GetModelName(),
			Category:    types.UsageCategoryRerank,
			Purpose:     purpose,
			Status:      types.UsageStatusSuccess,
			InputTokens: total,
			Extra: types.JSONMap{
				"approx":    true,
				"documents": len(documents),
			},
		})
	}
	return results, err
}

func (m *meterReranker) GetModelName() string { return m.inner.GetModelName() }
func (m *meterReranker) GetModelID() string   { return m.inner.GetModelID() }
