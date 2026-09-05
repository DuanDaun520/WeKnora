-- 000097 down: drop the platform sandbox connection catalog.
--
-- Materialized tenant_sandbox_configs rows SURVIVE the down — they are
-- ordinary workspace configs; only the provenance link columns go away
-- (their payloads, skills and sandboxes keep working as self-built rows).

DROP INDEX IF EXISTS idx_tenant_sandbox_configs_source_connection;
ALTER TABLE tenant_sandbox_configs
    DROP COLUMN IF EXISTS source_pushed_at,
    DROP COLUMN IF EXISTS source_connection_id;
DROP INDEX IF EXISTS uq_sandbox_connections_name;
DROP TABLE IF EXISTS sandbox_connections;
