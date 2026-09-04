-- 000093 down: restore the flat all-admin model of 000092. The demotion is
-- uniform, so promoting every active contributor back to admin faithfully
-- reverses the up migration (000092's own down is the no-op boundary).

UPDATE tenant_members
SET role = 'admin', updated_at = NOW()
WHERE deleted_at IS NULL AND role = 'contributor';
