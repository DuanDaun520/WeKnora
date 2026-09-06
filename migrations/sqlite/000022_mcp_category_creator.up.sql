-- Description: Category and creator metadata for the Skills/MCP browser page.
-- Lite has no skill tables; only mcp_services needs the columns here.
ALTER TABLE mcp_services ADD COLUMN category VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE mcp_services ADD COLUMN created_by VARCHAR(36) NOT NULL DEFAULT '';
