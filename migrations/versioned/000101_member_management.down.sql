-- 000101 down: drop the login-tracking column. Member management endpoints
-- degrade to rendering "-" for every member (last_login_at always NULL).
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
