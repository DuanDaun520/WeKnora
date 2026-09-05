package asr

import (
	"context"

	"github.com/Tencent/WeKnora/internal/metering"
	"github.com/Tencent/WeKnora/internal/types"
)

// meterASR wraps an ASR and appends a usage-ledger record per call
// (docs/Token统计与计费设计.md). Audio duration is not uniformly available
// before transcription, so the record carries audio_bytes (+ the
// transcription's text-token estimate, extra.approx=true); per-minute
// pricing folds from the duration a provider reports in Segments when
// present (extra.audio_seconds). Failed calls are not recorded.
type meterASR struct {
	inner ASR
}

func wrapASRMeter(a ASR, err error) (ASR, error) {
	if err != nil || a == nil {
		return a, err
	}
	return &meterASR{inner: a}, nil
}

func (m *meterASR) Transcribe(ctx context.Context, audioBytes []byte, fileName string) (*TranscriptionResult, error) {
	result, err := m.inner.Transcribe(ctx, audioBytes, fileName)
	if err == nil && result != nil {
		tenantID, userID, purpose := metering.AttributionFromContext(ctx)
		if purpose == "" {
			purpose = "asr"
		}
		extra := types.JSONMap{
			"approx":      true,
			"audio_bytes": len(audioBytes),
		}
		metering.GetManager().Submit(&types.ModelUsageRecord{
			TenantID:     tenantID,
			UserID:       userID,
			ModelID:      m.inner.GetModelID(),
			ModelName:    m.inner.GetModelName(),
			Category:     types.UsageCategoryASR,
			Purpose:      purpose,
			Status:       types.UsageStatusSuccess,
			OutputTokens: metering.ApproxTokens(result.Text),
			AudioSeconds: segmentSeconds(result),
			Extra:        extra,
		})
	}
	return result, err
}

func (m *meterASR) GetModelName() string { return m.inner.GetModelName() }
func (m *meterASR) GetModelID() string   { return m.inner.GetModelID() }

// segmentSeconds sums the segment durations when the provider reports
// them (Start..End pairs); returns 0 otherwise.
func segmentSeconds(result *TranscriptionResult) float64 {
	var total float64
	for _, seg := range result.Segments {
		if seg.End > seg.Start {
			total += seg.End - seg.Start
		}
	}
	return total
}
