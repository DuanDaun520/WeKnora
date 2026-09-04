-- 000094: platform-wide model catalog + per-workspace model assignments.
--
-- Enterprise model governance rework:
--   1. Model configuration is centralized: only the system admin may
--      create/modify models (guards tightened at the router layer).
--   2. All non-builtin models become platform-owned (tenant_id = 0) and
--      are shared with workspaces through explicit assignments in the new
--      tenant_model_assignments table.
--   3. Existing tenant-owned models are migrated losslessly: an assignment
--      to the original workspace is created BEFORE the tenant_id reset, so
--      every workspace keeps seeing exactly the models it could see before
--      (knowledge bases / agents keep resolving their model references).
--
-- Built-in (is_builtin = true, YAML-managed) models stay visible to every
-- workspace without an assignment row — the out-of-box behaviour is kept.
--
-- tenant_model_assignments rows are hard-deleted on removal; the audit log
-- (system.tenant_models_assigned) is the history of record.

DO $$ BEGIN RAISE NOTICE '[Migration 000094] Starting platform models + assignments...'; END $$;

DO $$ BEGIN RAISE NOTICE '[Migration 000094] Creating table: tenant_model_assignments'; END $$;
CREATE TABLE IF NOT EXISTS tenant_model_assignments (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   INTEGER NOT NULL,
    model_id    VARCHAR(64) NOT NULL,
    assigned_by VARCHAR(64) NOT NULL DEFAULT '',
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_tenant_model_assignments_tenant_model UNIQUE (tenant_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_model_assignments_model_id ON tenant_model_assignments(model_id);

-- Preserve current per-workspace visibility before resetting ownership.
DO $$ BEGIN RAISE NOTICE '[Migration 000094] Backfilling assignments from legacy model ownership'; END $$;
INSERT INTO tenant_model_assignments (tenant_id, model_id, assigned_by)
SELECT DISTINCT m.tenant_id, m.id, 'system'
FROM models m
WHERE m.is_builtin = false AND m.deleted_at IS NULL AND m.tenant_id > 0
ON CONFLICT DO NOTHING;

DO $$ BEGIN RAISE NOTICE '[Migration 000094] Resetting non-builtin models to platform ownership (tenant_id = 0)'; END $$;
UPDATE models
SET tenant_id = 0, updated_at = CURRENT_TIMESTAMP
WHERE is_builtin = false AND tenant_id > 0;
