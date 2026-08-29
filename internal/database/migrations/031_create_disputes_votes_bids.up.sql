CREATE TABLE IF NOT EXISTS circle_disputes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    raiser_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(255) NOT NULL,
    details TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_circle_disputes_circle ON circle_disputes(circle_id);
CREATE INDEX idx_circle_disputes_raiser ON circle_disputes(raiser_id);

CREATE TABLE IF NOT EXISTS circle_votes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    voter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    round_number INTEGER NOT NULL CHECK (round_number >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_circle_votes_voter_round UNIQUE (circle_id, voter_id, round_number)
);

CREATE INDEX idx_circle_votes_circle_round ON circle_votes(circle_id, round_number);

CREATE TABLE IF NOT EXISTS circle_auction_bids (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    bidder_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    round_number INTEGER NOT NULL CHECK (round_number >= 1),
    bid_amount NUMERIC(18,7) NOT NULL CHECK (bid_amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_circle_auction_bids_bidder_round UNIQUE (circle_id, bidder_id, round_number)
);

CREATE INDEX idx_circle_auction_bids_circle_round ON circle_auction_bids(circle_id, round_number);
