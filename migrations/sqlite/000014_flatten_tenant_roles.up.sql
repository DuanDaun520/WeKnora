-- 000014 (sqlite): flatten tenant roles for the enterprise user-management
-- model. Mirror of the Postgres 000092 migration: every active membership
-- becomes 'admin'. See the Postgres file for the design rationale.

UPDATE tenant_members
SET role = 'admin', updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL AND role <> 'admin';
