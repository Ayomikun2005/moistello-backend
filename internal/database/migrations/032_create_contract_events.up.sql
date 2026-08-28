-- Append-only audit log of all Soroban contract events processed by the indexer.
-- Used for debugging, replay, and analytics. Rows are never updated or deleted.

CREATE TABLE IF NOT EXISTS contract_events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tx_hash      TEXT        NOT NULL,
    ledger       BIGINT      NOT NULL,
    contract_id  TEXT        NOT NULL,
    event_type   TEXT        NOT NULL,
    payload      JSONB       NOT NULL DEFAULT '{}',
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fast lookup by transaction hash (deduplication checks).
CREATE INDEX IF NOT EXISTS idx_contract_events_tx_hash
    ON contract_events (tx_hash);

-- Fast lookup by event type (analytics, debugging).
CREATE INDEX IF NOT EXISTS idx_contract_events_event_type
    ON contract_events (event_type);

-- Fast lookup by contract (per-circle event history).
CREATE INDEX IF NOT EXISTS idx_contract_events_contract_id
    ON contract_events (contract_id);

-- Prevent duplicate events for the same tx + contract + type combination.
CREATE UNIQUE INDEX IF NOT EXISTS idx_contract_events_unique
    ON contract_events (tx_hash, contract_id, event_type);
