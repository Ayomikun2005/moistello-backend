CREATE TABLE IF NOT EXISTS savings_streaks (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE,
    current_streak INTEGER NOT NULL DEFAULT 0,
    longest_streak INTEGER NOT NULL DEFAULT 0,
    last_contribution_at TIMESTAMP WITH TIME ZONE,
    bonus_tier INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_savings_streaks_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_savings_streaks_user_id ON savings_streaks(user_id);
CREATE INDEX idx_savings_streaks_bonus_tier ON savings_streaks(bonus_tier);
