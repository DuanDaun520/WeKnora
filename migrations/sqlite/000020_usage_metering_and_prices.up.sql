-- 000020 (sqlite): Token usage metering & pricing — sqlite dialect of
-- migrations/versioned/000100. See that file for the design notes.
CREATE TABLE IF NOT EXISTS model_usage_records (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id      INTEGER      NOT NULL,
    user_id        TEXT         NOT NULL DEFAULT '',
    model_id       TEXT         NOT NULL DEFAULT '',
    model_name     TEXT         NOT NULL DEFAULT '',
    category       TEXT         NOT NULL,
    purpose        TEXT         NOT NULL DEFAULT '',
    status         TEXT         NOT NULL,
    input_tokens   INTEGER      NOT NULL DEFAULT 0,
    output_tokens  INTEGER      NOT NULL DEFAULT 0,
    cached_tokens  INTEGER      NOT NULL DEFAULT 0,
    images         INTEGER      NOT NULL DEFAULT 0,
    audio_seconds  REAL         NOT NULL DEFAULT 0,
    extra          TEXT,
    ref_type       TEXT         NOT NULL DEFAULT '',
    ref_id         TEXT         NOT NULL DEFAULT '',
    occurred_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_usage_records_tenant_time ON model_usage_records(tenant_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_records_user_time   ON model_usage_records(user_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_records_time        ON model_usage_records(occurred_at);

CREATE TABLE IF NOT EXISTS model_prices (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id            TEXT      NOT NULL,
    price_input_per_m   REAL      NOT NULL DEFAULT 0,
    price_output_per_m  REAL      NOT NULL DEFAULT 0,
    price_cached_per_m  REAL      NOT NULL DEFAULT 0,
    price_per_m         REAL      NOT NULL DEFAULT 0,
    price_per_image     REAL      NOT NULL DEFAULT 0,
    price_per_audio_min REAL      NOT NULL DEFAULT 0,
    price_per_video_min REAL      NOT NULL DEFAULT 0,
    currency            TEXT      NOT NULL DEFAULT 'CNY',
    enabled             BOOLEAN   NOT NULL DEFAULT TRUE,
    effective_from      DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at          DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_model_prices_model_id UNIQUE (model_id)
);
