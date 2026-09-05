package embedding

import (
	"context"

	"github.com/Tencent/WeKnora/internal/metering"
	"github.com/Tencent/WeKnora/internal/types"
)

// meterEmbedder wraps an Embedder and appends a usage-ledger record per
// call (docs/Token统计与计费设计.md). The Embedder interface does not
// surface provider usage, so input tokens are always estimated with the
// shared runes/4 rule and flagged extra.approx=true. Failed calls carry no
// measurable consumption and are not recorded.
type meterEmbedder struct {
	inner Embedder
}

func wrapEmbedderMeter(e Embedder) Embedder {
	if e == nil {
		return nil
	}
	return &meterEmbedder{inner: e}
}

func (m *meterEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := m.inner.Embed(ctx, text)
	if err == nil {
		metering.GetManager().Submit(embedUsageRecord(ctx, m.inner, 1, text))
	}
	return result, err
}

func (m *meterEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	result, err := m.inner.BatchEmbed(ctx, texts)
	if err == nil {
		metering.GetManager().Submit(embedUsageRecord(ctx, m.inner, len(texts), texts...))
	}
	return result, err
}

func (m *meterEmbedder) BatchEmbedWithPool(ctx context.Context, model Embedder, texts []string) ([][]float32, error) {
	return m.inner.BatchEmbedWithPool(ctx, model, texts)
}

func (m *meterEmbedder) GetModelName() string { return m.inner.GetModelName() }
func (m *meterEmbedder) GetDimensions() int   { return m.inner.GetDimensions() }
func (m *meterEmbedder) GetModelID() string   { return m.inner.GetModelID() }

func embedUsageRecord(ctx context.Context, e Embedder, count int, texts ...string) *types.ModelUsageRecord {
	tenantID, userID, purpose := metering.AttributionFromContext(ctx)
	total := 0
	for _, t := range texts {
		total += metering.ApproxTokens(t)
	}
	if purpose == "" {
		purpose = "embedding"
	}
	return &types.ModelUsageRecord{
		TenantID:     tenantID,
		UserID:       userID,
		ModelID:      e.GetModelID(),
		ModelName:    e.GetModelName(),
		Category:     types.UsageCategoryEmbedding,
		Purpose:      purpose,
		Status:       types.UsageStatusSuccess,
		InputTokens:  total,
		CachedTokens: 0,
		Extra: types.JSONMap{
			"approx": true,
			"texts":  count,
		},
	}
}
