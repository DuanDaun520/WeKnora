-- Platform-level storage engines (managed by platform admins)
CREATE TABLE IF NOT EXISTS platform_storage_engines (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL,
    config TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_platform_storage_engines_provider ON platform_storage_engines(provider);
CREATE INDEX IF NOT EXISTS idx_platform_storage_engines_status ON platform_storage_engines(status);

-- Add platform storage engine assignment to tenants
ALTER TABLE tenants ADD COLUMN platform_storage_engine_id TEXT;