CREATE TABLE IF NOT EXISTS incentive_configs (
    id UUID PRIMARY KEY,
    referral_bonus_amount DECIMAL(20, 8) NOT NULL DEFAULT 5.0,
    referral_bonus_currency VARCHAR(10) NOT NULL DEFAULT 'USDC',
    circle_completion_bonus DECIMAL(20, 8) NOT NULL DEFAULT 10.0,
    circle_completion_currency VARCHAR(10) NOT NULL DEFAULT 'USDC',
    contribution_match_percent DECIMAL(5, 2) NOT NULL DEFAULT 10.0,
    contribution_match_max DECIMAL(20, 8) NOT NULL DEFAULT 50.0,
    streak_bonus_tier1 INTEGER NOT NULL DEFAULT 4,
    streak_bonus_tier1_amount DECIMAL(20, 8) NOT NULL DEFAULT 2.0,
    streak_bonus_tier2 INTEGER NOT NULL DEFAULT 8,
    streak_bonus_tier2_amount DECIMAL(20, 8) NOT NULL DEFAULT 5.0,
    streak_bonus_tier3 INTEGER NOT NULL DEFAULT 12,
    streak_bonus_tier3_amount DECIMAL(20, 8) NOT NULL DEFAULT 10.0,
    first_deposit_bonus DECIMAL(20, 8) NOT NULL DEFAULT 5.0,
    first_deposit_currency VARCHAR(10) NOT NULL DEFAULT 'USDC',
    first_deposit_min_amount DECIMAL(20, 8) NOT NULL DEFAULT 10.0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_incentive_configs_is_active ON incentive_configs(is_active);

-- Insert default config
INSERT INTO incentive_configs (id, referral_bonus_amount, referral_bonus_currency, circle_completion_bonus, circle_completion_currency,
    contribution_match_percent, contribution_match_max, streak_bonus_tier1, streak_bonus_tier1_amount,
    streak_bonus_tier2, streak_bonus_tier2_amount, streak_bonus_tier3, streak_bonus_tier3_amount,
    first_deposit_bonus, first_deposit_currency, first_deposit_min_amount, is_active)
VALUES (
    gen_random_uuid(),
    5.0, 'USDC',
    10.0, 'USDC',
    10.0, 50.0,
    4, 2.0,
    8, 5.0,
    12, 10.0,
    5.0, 'USDC', 10.0,
    true
);
