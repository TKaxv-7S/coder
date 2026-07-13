-- Convert legacy users created with login_type 'none' into service accounts.
--
-- Historically, admins created machine-to-machine users with login_type
-- 'none'. That path is now deprecated in favour of premium service accounts,
-- so this migration grandfathers those existing accounts onto the service
-- account model (is_service_account = true).
--
-- BREAKING / DESTRUCTIVE: the users_email_not_empty CHECK constraint (added in
-- migration 000433) requires that service accounts have an empty email. We
-- therefore MUST blank the email of every converted user. The original email
-- address is NOT recoverable. Email uniqueness indexes already exclude empty
-- emails (WHERE email != ''), so blanking multiple rows does not conflict.
--
-- System users (is_system = true) are intentionally left untouched.
UPDATE users
SET is_service_account = true,
	email = ''
WHERE login_type = 'none'
	AND is_service_account = false
	AND is_system = false;
