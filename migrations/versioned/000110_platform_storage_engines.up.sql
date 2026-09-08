-- Platform-level storage engines (managed by platform admins)
CREATE TABLE IF NOT EXISTS platform_storage_engines (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    provider VARCHAR(32) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

CREATE INDEX IF NOT EXISTS idx_platform_storage_engines_provider ON platform_storage_engines(provider);
CREATE INDEX IF NOT EXISTS idx_platform_storage_engines_status ON platform_storage_engines(status);
CREATE INDEX IF NOT EXISTS idx_platform_storage_engines_deleted_at ON platform_storage_engines(deleted_at);

-- Add platform storage engine assignment to tenants
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS platform_storage_engine_id VARCHAR(36);

-- Add comment explaining the relationship
COMMENT ON COLUMN tenants.platform_storage_engine_id IS 'Platform-level storage engine assigned to this workspace. Takes precedence over default_storage_backend_id when set.';

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_tenants_platform_storage_engine_id ON tenants(platform_storage_engine_id);