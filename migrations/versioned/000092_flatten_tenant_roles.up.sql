-- 000092: flatten tenant roles for the enterprise user-management model.
--
-- The enterprise rework removes the per-workspace role hierarchy
-- (owner/admin/contributor/viewer): every member of a workspace is an
-- equal "admin". Normalizing to admin (not owner) on purpose — owner
-- carries hard invariants like "cannot remove the last owner" which
-- would deadlock admin-driven unbinds once everyone is the same rank.
-- With every active row at admin:
--   * Admin()/Contributor()/Viewer() route guards all pass → equal rights
--   * HasPermission code paths stay untouched
--   * Re-introducing a hierarchy later only needs new rows/migration
--
-- Idempotent and safe to re-run; soft-deleted rows are left as-is.

UPDATE tenant_members
SET role = 'admin', updated_at = NOW()
WHERE deleted_at IS NULL AND role <> 'admin';
