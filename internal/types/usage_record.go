package types

import "time"

// Usage record categories — the closed set of billable model-call kinds.
// Every meter wrapper maps its package onto exactly one of these.
const (
	UsageCategoryChat      = "chat"
	UsageCategoryEmbedding = "embedding"
	UsageCategoryRerank    = "rerank"
	UsageCategoryVLM       = "vlm"
	UsageCategoryASR       = "asr"
)

// Usage record statuses. Billing counts consumed tokens regardless of the
// business outcome, so "failed"/"interrupted" rows still carry the tokens
// that were spent before the call went wrong; only zero-consumption calls
// are skipped entirely.
const (
	UsageStatusSuccess     = "success"
	UsageStatusFailed      = "failed"
	UsageStatusInterrupted = "interrupted"
)

// ModelUsageRecord is one row in model_usage_records: the append-only,
// Langfuse-independent ledger every meter wrapper appends to. Aggregation
// for the three report levels (platform / tenant / user) is plain SQL over
// this table; nothing here is ever mutated after insert.
type ModelUsageRecord struct {
	ID int64 `gorm:"primaryKey;column:id" json:"-"`
	// TenantID is the space the call belongs to; 0 should not occur (the
	// meter skips records without a tenant to keep reports honest).
	TenantID uint64 `gorm:"column:tenant_id;index:idx_usage_records_tenant_time,priority:1" json:"tenant_id"`
	// UserID is the acting user; empty for system/background tasks (the
	// record then aggregates under the space only).
	UserID string `gorm:"column:user_id;size:36;index:idx_usage_records_user_time,priority:1" json:"user_id,omitempty"`
	// ModelID joins models.id when the call went through a configured
	// model; ModelName is the stable fallback for pricing lookups.
	ModelID   string `gorm:"column:model_id;size:36" json:"model_id,omitempty"`
	ModelName string `gorm:"column:model_name;size:255" json:"model_name"`
	// Category is one of UsageCategory*; Purpose is the LLMCallPurpose
	// scene label (agent_round, document_summary, ...) when known.
	Category string `gorm:"column:category;size:16" json:"category"`
	Purpose  string `gorm:"column:purpose;size:64" json:"purpose,omitempty"`
	// Status is one of UsageStatus*.
	Status string `gorm:"column:status;size:16" json:"status"`
	// Token counters. Input is prompt/request tokens, Output is
	// completion/response tokens, Cached is the cache-read subset of
	// input reported by prompt-caching providers.
	InputTokens  int `gorm:"column:input_tokens;default:0"  json:"input_tokens"`
	OutputTokens int `gorm:"column:output_tokens;default:0" json:"output_tokens"`
	CachedTokens int `gorm:"column:cached_tokens;default:0" json:"cached_tokens"`
	// Images counts image/video-frame units (VLM and multimodal chat);
	// AudioSeconds accumulates transcribed audio length. Both are real
	// columns (not buried in Extra) so report aggregation stays plain
	// SUM() across the postgres and sqlite dialects.
	Images       int     `gorm:"column:images;default:0"         json:"images,omitempty"`
	AudioSeconds float64 `gorm:"column:audio_seconds;default:0"  json:"audio_seconds,omitempty"`
	// Extra carries the remaining non-token details (audio_bytes,
	// rerank document counts, the approx:true estimation marker, ...).
	Extra JSONMap `gorm:"column:extra;type:jsonb" json:"extra,omitempty"`
	// RefType/RefID optionally link the record to its business object
	// ("session", "message", "knowledge", ...).
	RefType string `gorm:"column:ref_type;size:32" json:"ref_type,omitempty"`
	RefID   string `gorm:"column:ref_id;size:64" json:"ref_id,omitempty"`
	// OccurredAt is when the model call finished (report grouping uses
	// this, not the insert time — the writer flushes in batches).
	OccurredAt time.Time `gorm:"column:occurred_at;index:idx_usage_records_tenant_time,priority:2;index:idx_usage_records_user_time,priority:2;index:idx_usage_records_time,priority:1" json:"occurred_at"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (ModelUsageRecord) TableName() string { return "model_usage_records" }

// ModelPrice is one row in model_prices: the admin-maintained per-model
// unit prices used to fold usage into display-only amounts. Prices are
// matched by event time via EffectiveFrom — changing a price never
// rewrites history. Multimedia fields are filled per the pricing口径 the
// admin picked for that model; unused ones stay zero.
type ModelPrice struct {
	ID int64 `gorm:"primaryKey;column:id" json:"-"`
	// ModelID joins models.id. One price row per model.
	ModelID string `gorm:"column:model_id;size:36;uniqueIndex:uk_model_prices_model_id" json:"model_id"`
	// Chat-style prices, CNY per million tokens.
	PriceInputPerM  float64 `gorm:"column:price_input_per_m;default:0"  json:"price_input_per_m"`
	PriceOutputPerM float64 `gorm:"column:price_output_per_m;default:0" json:"price_output_per_m"`
	PriceCachedPerM float64 `gorm:"column:price_cached_per_m;default:0" json:"price_cached_per_m"`
	// Embedding/rerank price, CNY per million tokens.
	PricePerM float64 `gorm:"column:price_per_m;default:0" json:"price_per_m"`
	// Multimedia prices: per image/frame, per audio minute, per video minute (CNY).
	PricePerImage     float64 `gorm:"column:price_per_image;default:0"     json:"price_per_image"`
	PricePerAudioMin  float64 `gorm:"column:price_per_audio_min;default:0"  json:"price_per_audio_min"`
	PricePerVideoMin  float64 `gorm:"column:price_per_video_min;default:0"  json:"price_per_video_min"`
	// Currency is display-only; default CNY.
	Currency string `gorm:"column:currency;size:8;default:CNY" json:"currency"`
	// Enabled=false keeps the row but excludes it from folding (usage
	// then shows tokens with zero amount).
	Enabled bool `gorm:"column:enabled;default:true" json:"enabled"`
	// EffectiveFrom is when this price takes effect for event matching.
	EffectiveFrom time.Time `gorm:"column:effective_from" json:"effective_from"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (ModelPrice) TableName() string { return "model_prices" }
