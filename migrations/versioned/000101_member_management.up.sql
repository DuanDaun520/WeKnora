-- 000101: Tenant-admin member management (成员管理弹窗).
--
-- Adds users.last_login_at so the member roster can render "最后登录时间"
-- (never logged in ⇒ NULL ⇒ frontend shows "-"). Written best-effort by the
-- login path; rows created before this migration legitimately stay NULL
-- until their next login. No backfill: inventing a login timestamp for
-- historical users would defeat the "never logged in" signal that drives
-- the 邀请 (invite) affordance.

DO $$ BEGIN RAISE NOTICE '[Migration 000101] Adding users.last_login_at'; END $$;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;

DO $$ BEGIN RAISE NOTICE '[Migration 000101] member management ready'; END $$;
