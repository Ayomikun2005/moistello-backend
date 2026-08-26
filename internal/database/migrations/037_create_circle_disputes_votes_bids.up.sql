CREATE TABLE IF NOT EXISTS disputes (
    id UUID PRIMARY KEY,
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    raiser_id UUID NOT NULL REFERENCES users(id),
    reason TEXT NOT NULL,
    evidence TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    idempotency_key VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_disputes_circle_id ON disputes(circle_id);
CREATE UNIQUE INDEX idx_disputes_idempotency ON disputes(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS circle_votes (
    id UUID PRIMARY KEY,
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    voter_id UUID NOT NULL REFERENCES users(id),
    vote_for_id UUID NOT NULL REFERENCES users(id),
    round_number INT NOT NULL,
    idempotency_key VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (circle_id, voter_id, round_number)
);

CREATE INDEX idx_circle_votes_circle_round ON circle_votes(circle_id, round_number);
CREATE UNIQUE INDEX idx_circle_votes_idempotency ON circle_votes(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS circle_auction_bids (
    id UUID PRIMARY KEY,
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    bidder_id UUID NOT NULL REFERENCES users(id),
    round_number INT NOT NULL,
    discount_bips INT NOT NULL,
    idempotency_key VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (circle_id, bidder_id, round_number)
);

CREATE INDEX idx_circle_auction_bids_circle_round ON circle_auction_bids(circle_id, round_number);
CREATE UNIQUE INDEX idx_circle_auction_bids_idempotency ON circle_auction_bids(idempotency_key) WHERE idempotency_key IS NOT NULL;
