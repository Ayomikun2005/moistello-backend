DROP INDEX IF EXISTS idx_payouts_verification_status;
DROP INDEX IF EXISTS idx_payouts_verified_onchain;

ALTER TABLE payouts
    DROP COLUMN IF EXISTS verification_status,
    DROP COLUMN IF EXISTS verified_onchain;

DROP INDEX IF EXISTS idx_contributions_verification_status;
DROP INDEX IF EXISTS idx_contributions_verified_onchain;

ALTER TABLE contributions
    DROP COLUMN IF EXISTS verification_status,
    DROP COLUMN IF EXISTS verified_onchain;
