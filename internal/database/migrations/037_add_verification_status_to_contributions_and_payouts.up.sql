-- Add on-chain transaction verification status columns for contributions
ALTER TABLE contributions
    ADD COLUMN IF NOT EXISTS verified_onchain BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified';

CREATE INDEX IF NOT EXISTS idx_contributions_verified_onchain ON contributions(verified_onchain);
CREATE INDEX IF NOT EXISTS idx_contributions_verification_status ON contributions(verification_status);

-- Add on-chain transaction verification status columns for payouts
ALTER TABLE payouts
    ADD COLUMN IF NOT EXISTS verified_onchain BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified';

CREATE INDEX IF NOT EXISTS idx_payouts_verified_onchain ON payouts(verified_onchain);
CREATE INDEX IF NOT EXISTS idx_payouts_verification_status ON payouts(verification_status);
