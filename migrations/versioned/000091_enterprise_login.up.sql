-- Migration: 000091_enterprise_login
-- Description: Enterprise login — employee_id becomes the primary account
-- identifier and login name; must_change_password forces rotation of
-- admin-set initial/reset passwords on first login. Registration and email
-- login are removed at the application layer.

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Starting enterprise_login migration...'; END $$;

-- ============================================================================
-- Section 1: New columns
-- ============================================================================
DO $$ BEGIN RAISE NOTICE '[Migration 000091] Adding users.employee_id / users.must_change_password'; END $$;
ALTER TABLE users ADD COLUMN IF NOT EXISTS employee_id VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

-- ============================================================================
-- Section 2: Backfill
-- ============================================================================
-- Legacy accounts adopt their username as the employee ID. Usernames were
-- unique under the pre-migration schema, so the backfill cannot collide.
DO $$ BEGIN RAISE NOTICE '[Migration 000091] Backfilling employee_id from username'; END $$;
UPDATE users
SET employee_id = username
WHERE employee_id IS NULL OR employee_id = '';

-- ============================================================================
-- Section 3: Partial unique index on employee_id
-- ============================================================================
-- Unique among live rows only: a soft-deleted row keeps its value but frees
-- the identifier for reuse. The predicate also covers the login lookup
-- (GORM's default soft-delete scope), so no separate plain index is needed.
DO $$ BEGIN RAISE NOTICE '[Migration 000091] Creating partial unique index on users.employee_id'; END $$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_employee_id_live
    ON users(employee_id)
    WHERE deleted_at IS NULL AND employee_id IS NOT NULL AND employee_id <> '';

-- ============================================================================
-- Section 4: Relax legacy uniqueness on username / email
-- ============================================================================
-- Real names collide (中文姓名必重名) and email is demoted to an optional
-- contact field. Plain lookup indexes already exist (idx_users_username /
-- idx_users_email from migration 000001), so only the constraints go.
DO $$ BEGIN RAISE NOTICE '[Migration 000091] Dropping UNIQUE constraints on users.username / users.email'; END $$;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_key;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

COMMENT ON COLUMN users.employee_id IS 'Employee ID — primary login identifier, unique among live rows, immutable after creation';
COMMENT ON COLUMN users.must_change_password IS 'Force password rotation on next auth (admin-set initial or reset password); cleared by a successful password change';

DO $$ BEGIN RAISE NOTICE '[Migration 000091] enterprise_login migration complete'; END $$;
