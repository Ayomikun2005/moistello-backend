CREATE TABLE IF NOT EXISTS incentives (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    amount DECIMAL(20, 8) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    metadata TEXT,
    reference_id VARCHAR(255),
    expires_at TIMESTAMP WITH TIME ZONE,
    claimed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_incentives_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_incentives_user_id ON incentives(user_id);
CREATE INDEX idx_incentives_type ON incentives(type);
CREATE INDEX idx_incentives_status ON incentives(status);
CREATE INDEX idx_incentives_reference_id ON incentives(reference_id);
CREATE INDEX idx_incentives_expires_at ON incentives(expires_at);
