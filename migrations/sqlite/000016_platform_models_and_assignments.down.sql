-- 000016 (sqlite, down): restore per-workspace model ownership and drop
-- assignments. Best-effort mirror of Postgres 000094 down.

UPDATE models
SET tenant_id = (
        SELECT MIN(a.tenant_id) FROM tenant_model_assignments a WHERE a.model_id = models.id
    ),
    updated_at = CURRENT_TIMESTAMP
WHERE is_builtin = 0
  AND tenant_id = 0
  AND EXISTS (SELECT 1 FROM tenant_model_assignments a WHERE a.model_id = models.id);

DROP TABLE IF EXISTS tenant_model_assignments;
