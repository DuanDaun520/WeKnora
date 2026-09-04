-- 000016 (sqlite): platform-wide model catalog + per-workspace model
-- assignments. Mirror of the Postgres 000094 migration — see that file for
-- the design rationale.

CREATE TABLE IF NOT EXISTS tenant_model_assignments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   INTEGER NOT NULL,
    model_id    VARCHAR(64) NOT NULL,
    assigned_by VARCHAR(64) NOT NULL DEFAULT '',
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_model_assignments_model_id
    ON tenant_model_assignments(model_id);

-- Preserve current per-workspace visibility before resetting ownership.
INSERT OR IGNORE INTO tenant_model_assignments (tenant_id, model_id, assigned_by)
SELECT DISTINCT m.tenant_id, m.id, 'system'
FROM models m
WHERE m.is_builtin = 0 AND m.deleted_at IS NULL AND m.tenant_id > 0;

UPDATE models
SET tenant_id = 0, updated_at = CURRENT_TIMESTAMP
WHERE is_builtin = 0 AND tenant_id > 0;
