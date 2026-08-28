CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE mobile_money_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(20) NOT NULL,
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('onramp', 'offramp')),
    currency VARCHAR(3) NOT NULL,
    amount NUMERIC(20, 7) NOT NULL,
    usdc_amount NUMERIC(20, 7) NOT NULL,
    phone_number VARCHAR(20) NOT NULL,
    destination_address VARCHAR(56),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    provider_ref VARCHAR(100),
    idempotency_key VARCHAR(255) NOT NULL,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX idx_mobile_money_txns_user_id ON mobile_money_transactions(user_id);
CREATE INDEX idx_mobile_money_txns_status ON mobile_money_transactions(status) WHERE status = 'pending';
CREATE INDEX idx_mobile_money_txns_provider_ref ON mobile_money_transactions(provider_ref) WHERE provider_ref IS NOT NULL;
