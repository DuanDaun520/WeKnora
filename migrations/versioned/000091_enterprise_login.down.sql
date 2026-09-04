-- Migration: 000091_enterprise_login (down)
-- Reverts the enterprise-login changes. Restoring the legacy UNIQUE
-- constraints is best-effort: if relaxing them admitted duplicate
-- usernames/emails in the meantime, the restore is skipped with a notice
-- rather than failing the rollback.

DROP INDEX IF EXISTS idx_users_employee_id_live;

ALTER TABLE users DROP COLUMN IF EXISTS employee_id;
ALTER TABLE users DROP COLUMN IF EXISTS must_change_password;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_username_key') THEN
        BEGIN
            ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE (username);
        EXCEPTION WHEN unique_violation THEN
            RAISE NOTICE '[Migration 000091 down] duplicate usernames present; constraint not restored';
        END;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_email_key') THEN
        BEGIN
            ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
        EXCEPTION WHEN unique_violation THEN
            RAISE NOTICE '[Migration 000091 down] duplicate emails present; constraint not restored';
        END;
    END IF;
END $$;
