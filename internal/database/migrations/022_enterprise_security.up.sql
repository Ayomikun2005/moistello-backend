CREATE TABLE IF NOT EXISTS withdrawal_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    destination VARCHAR(56) NOT NULL,
    asset VARCHAR(10) NOT NULL,
    amount NUMERIC(20,7) NOT NULL,
    fee_amount NUMERIC(20,7) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    ip_address VARCHAR(45),
    user_agent TEXT,
    failure_reason TEXT,
    tx_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_withdrawal_audit_user ON withdrawal_audit(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS spending_limits (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    daily_limit_usdc NUMERIC(20,7) NOT NULL DEFAULT 1000,
    weekly_limit_usdc NUMERIC(20,7) NOT NULL DEFAULT 5000,
    whitelist_only BOOLEAN NOT NULL DEFAULT false,
    large_tx_threshold NUMERIC(20,7) NOT NULL DEFAULT 500,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS withdrawal_rate_limits (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    window_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempt_count INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, window_start)
);
