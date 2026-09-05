-- 000100: Token usage metering & pricing (docs/Token统计与计费设计.md).
--
-- Two tables:
--   model_usage_records — append-only ledger written by the meter wrappers
--     in internal/models/{chat,embedding,rerank,vlm,asr}. Independent of
--     Langfuse; failed/interrupted calls are recorded too whenever tokens
--     were consumed.
--   model_prices — admin-maintained per-model unit prices (CNY per million
--     tokens / per image / per audio minute). Display-only folding; no
--     deduction, no payment.

DO $$ BEGIN RAISE NOTICE '[Migration 000100] Creating model_usage_records'; END $$;
CREATE TABLE IF NOT EXISTS model_usage_records (
    id             BIGSERIAL PRIMARY KEY,
    tenant_id      BIGINT       NOT NULL,
    user_id        VARCHAR(36)  NOT NULL DEFAULT '',
    model_id       VARCHAR(36)  NOT NULL DEFAULT '',
    model_name     VARCHAR(255) NOT NULL DEFAULT '',
    category       VARCHAR(16)  NOT NULL,
    purpose        VARCHAR(64)  NOT NULL DEFAULT '',
    status         VARCHAR(16)  NOT NULL,
    input_tokens   INT          NOT NULL DEFAULT 0,
    output_tokens  INT          NOT NULL DEFAULT 0,
    cached_tokens  INT          NOT NULL DEFAULT 0,
    images         INT          NOT NULL DEFAULT 0,
    audio_seconds  DOUBLE PRECISION NOT NULL DEFAULT 0,
    extra          JSONB,
    ref_type       VARCHAR(32)  NOT NULL DEFAULT '',
    ref_id         VARCHAR(64)  NOT NULL DEFAULT '',
    occurred_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_usage_records_tenant_time ON model_usage_records(tenant_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_records_user_time   ON model_usage_records(user_id, occurred_at) WHERE user_id <> '';
CREATE INDEX IF NOT EXISTS idx_usage_records_time        ON model_usage_records(occurred_at);

DO $$ BEGIN RAISE NOTICE '[Migration 000100] Creating model_prices'; END $$;
CREATE TABLE IF NOT EXISTS model_prices (
    id                  BIGSERIAL PRIMARY KEY,
    model_id            VARCHAR(36)  NOT NULL,
    price_input_per_m   DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_output_per_m  DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_cached_per_m  DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_per_m         DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_per_image     DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_per_audio_min DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_per_video_min DOUBLE PRECISION NOT NULL DEFAULT 0,
    currency            VARCHAR(8)   NOT NULL DEFAULT 'CNY',
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    effective_from      TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_model_prices_model_id UNIQUE (model_id)
);

DO $$ BEGIN RAISE NOTICE '[Migration 000100] usage metering ready'; END $$;
