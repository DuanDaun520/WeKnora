package vlm

import (
	"context"

	"github.com/Tencent/WeKnora/internal/metering"
	"github.com/Tencent/WeKnora/internal/types"
)

// meterVLM wraps a VLM and appends a usage-ledger record per call
// (docs/Token统计与计费设计.md). Image tokens are provider-specific and
// not surfaced by the interface, so the record counts the prompt/output
// text tokens (estimated, extra.approx=true) plus the image/frame count in
// extra.images for per-image pricing. Failed calls are not recorded.
type meterVLM struct {
	inner VLM
}

func wrapVLMMeter(v VLM, err error) (VLM, error) {
	if err != nil || v == nil {
		return v, err
	}
	return &meterVLM{inner: v}, nil
}

func (m *meterVLM) Predict(ctx context.Context, imgBytes [][]byte, prompt string) (string, error) {
	result, err := m.inner.Predict(ctx, imgBytes, prompt)
	if err == nil {
		tenantID, userID, purpose := metering.AttributionFromContext(ctx)
		if purpose == "" {
			purpose = "vlm"
		}
		in := metering.ApproxTokens(prompt)
		out := metering.ApproxTokens(result)
		extra := types.JSONMap{
			"approx": true,
		}
		if len(imgBytes) > 0 {
			// Frame-level size info helps per-image pricing when a caller
			// batches video frames into one request.
			extra["image_bytes"] = totalBytes(imgBytes)
		}
		metering.GetManager().Submit(&types.ModelUsageRecord{
			TenantID:     tenantID,
			UserID:       userID,
			ModelID:      m.inner.GetModelID(),
			ModelName:    m.inner.GetModelName(),
			Category:     types.UsageCategoryVLM,
			Purpose:      purpose,
			Status:       types.UsageStatusSuccess,
			InputTokens:  in,
			OutputTokens: out,
			Images:       len(imgBytes),
			Extra:        extra,
		})
	}
	return result, err
}

func (m *meterVLM) GetModelName() string { return m.inner.GetModelName() }
func (m *meterVLM) GetModelID() string   { return m.inner.GetModelID() }

func totalBytes(batches [][]byte) int {
	n := 0
	for _, b := range batches {
		n += len(b)
	}
	return n
}
