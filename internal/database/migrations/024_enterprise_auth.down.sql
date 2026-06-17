DROP INDEX IF EXISTS idx_users_passkey_credential;
ALTER TABLE users DROP COLUMN IF EXISTS passkey_credential_id;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
ALTER TABLE users DROP COLUMN IF EXISTS backup_codes;
ALTER TABLE users DROP COLUMN IF EXISTS totp_enabled;
ALTER TABLE users DROP COLUMN IF EXISTS totp_secret;
