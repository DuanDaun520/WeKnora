-- Remove platform storage engine assignment from tenants
DROP INDEX IF EXISTS idx_tenants_platform_storage_engine_id;
ALTER TABLE tenants DROP COLUMN IF EXISTS platform_storage_engine_id;

-- Drop platform storage engines table
DROP TABLE IF EXISTS platform_storage_engines;