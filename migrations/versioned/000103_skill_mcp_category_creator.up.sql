-- Description: Category and creator metadata for the Skills/MCP browser page.
-- Both default to '' so existing rows stay valid; creators are stamped on
-- create from the request context and never rewritten on update.
ALTER TABLE tenant_skill_catalog
    ADD COLUMN IF NOT EXISTS category VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

ALTER TABLE mcp_services
    ADD COLUMN IF NOT EXISTS category VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';
