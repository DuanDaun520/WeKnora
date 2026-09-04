-- 000015 (sqlite): demote all active memberships to 'contributor' for the
-- two-level workspace role model (空间管理员 vs 普通用户). Mirror of the
-- Postgres 000093 migration — see that file for the design rationale.

UPDATE tenant_members
SET role = 'contributor', updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL AND role = 'admin';
