CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- One row per 1:1 conversation. user_a_id/user_b_id are always stored with
-- user_a_id < user_b_id (enforced by the application, not the DB, since
-- Postgres can't compare UUIDs meaningfully by value for this purpose) so
-- the same pair of users can never end up with two conversation rows.
CREATE TABLE chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_a_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_b_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (user_a_id <> user_b_id),
    UNIQUE (user_a_id, user_b_id)
);

CREATE INDEX idx_chat_conversations_user_a ON chat_conversations(user_a_id);
CREATE INDEX idx_chat_conversations_user_b ON chat_conversations(user_b_id);

-- Message bodies are always ciphertext produced client-side (X3DH + the
-- HMAC ratchet in internal/domain/chat/x3dh.go) — the server never sees or
-- stores plaintext. ephemeral_key/one_time_prekey_id are only populated on
-- the first message of a session, letting the recipient run the responder
-- side of the X3DH handshake to derive the same shared master key.
CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id),
    sequence BIGINT NOT NULL,
    ciphertext TEXT NOT NULL,
    nonce TEXT NOT NULL,
    ephemeral_key VARCHAR(64),
    one_time_prekey_id INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (conversation_id, sequence)
);

CREATE INDEX idx_chat_messages_conversation ON chat_messages(conversation_id, sequence DESC);
