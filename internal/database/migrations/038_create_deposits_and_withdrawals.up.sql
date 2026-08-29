CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Yellow Card NGN -> USDC deposits (receive requests). Persisted so a
-- deposit's state survives process restarts and can be reconciled against
-- Yellow Card webhook notifications instead of only living in Redis/memory.
CREATE TABLE deposits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_ngn NUMERIC(20,2) NOT NULL,
    estimated_usdc NUMERIC(20,7) NOT NULL,
    destination_address VARCHAR(56) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    receive_id VARCHAR(100) NOT NULL,
    payment_ref VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    failure_reason TEXT
);

CREATE INDEX idx_deposits_user_id ON deposits(user_id, created_at DESC);
CREATE UNIQUE INDEX idx_deposits_receive_id ON deposits(receive_id);
CREATE UNIQUE INDEX idx_deposits_payment_ref ON deposits(payment_ref);

-- Yellow Card USDC -> NGN withdrawals (send requests).
CREATE TABLE withdrawals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_usdc NUMERIC(20,7) NOT NULL,
    estimated_ngn NUMERIC(20,2) NOT NULL,
    bank_code VARCHAR(20) NOT NULL,
    account_number VARCHAR(30) NOT NULL,
    account_name VARCHAR(200) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    platform_address VARCHAR(56),
    usdc_tx_hash VARCHAR(64),
    yellow_card_tx_id VARCHAR(100),
    payment_ref VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failure_reason TEXT
);

CREATE INDEX idx_withdrawals_user_id ON withdrawals(user_id, created_at DESC);
CREATE UNIQUE INDEX idx_withdrawals_payment_ref ON withdrawals(payment_ref);
CREATE UNIQUE INDEX idx_withdrawals_yellow_card_tx_id ON withdrawals(yellow_card_tx_id) WHERE yellow_card_tx_id IS NOT NULL;
