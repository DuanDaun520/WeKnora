-- 000015 down: restore the flat all-admin model (mirror of Postgres 000093 down).

UPDATE tenant_members
SET role = 'admin', updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL AND role = 'contributor';
