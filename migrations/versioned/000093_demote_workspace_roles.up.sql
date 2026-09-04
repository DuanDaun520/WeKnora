-- 000093: reintroduce the two-level workspace role model.
--
-- 000092 flattened every active membership to 'admin' (equal rights).
-- The enterprise model now splits workspace membership into two visible
-- levels: 空间管理员 (admin) and 普通用户 (contributor). The system admin
-- designates workspace admins explicitly in the admin console; everyone
-- else is an ordinary member.
--
-- Per the confirmed rollout decision, existing rows are demoted to
-- 'contributor' wholesale — the system admin re-assigns workspace admins
-- via PUT /system/admin/tenants/:tenant_id/members/:user_id afterwards.
-- A workspace can temporarily have no admin; only the system admin can
-- grant the role, so the state is always recoverable from the console.
--
-- Idempotent and safe to re-run; soft-deleted rows are left as-is.

UPDATE tenant_members
SET role = 'contributor', updated_at = NOW()
WHERE deleted_at IS NULL AND role = 'admin';
