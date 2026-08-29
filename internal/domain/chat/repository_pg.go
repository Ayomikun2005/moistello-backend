package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/moistello/backend/pkg/apperrors"
)

type pgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &pgRepository{db: db}
}

// orderPair returns (a, b) sorted so the same two IDs always produce the
// same order, regardless of which one is "userA" from the caller's
// perspective.
func orderPair(x, y string) (string, string) {
	if x < y {
		return x, y
	}
	return y, x
}

func (r *pgRepository) GetOrCreateConversation(ctx context.Context, userA, userB string) (*Conversation, error) {
	a, b := orderPair(userA, userB)

	var conv Conversation
	err := r.db.GetContext(ctx, &conv,
		`SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE user_a_id = $1 AND user_b_id = $2`,
		a, b)
	if err == nil {
		return &conv, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("looking up conversation: %w", err)
	}

	// INSERT ... ON CONFLICT DO NOTHING then re-select handles the race
	// where two concurrent requests both try to create the same
	// conversation for the first time.
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO chat_conversations (user_a_id, user_b_id) VALUES ($1, $2) ON CONFLICT (user_a_id, user_b_id) DO NOTHING`,
		a, b)
	if err != nil {
		return nil, fmt.Errorf("creating conversation: %w", err)
	}

	err = r.db.GetContext(ctx, &conv,
		`SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE user_a_id = $1 AND user_b_id = $2`,
		a, b)
	if err != nil {
		return nil, fmt.Errorf("reloading conversation after create: %w", err)
	}
	return &conv, nil
}

func (r *pgRepository) GetConversation(ctx context.Context, id string) (*Conversation, error) {
	var conv Conversation
	err := r.db.GetContext(ctx, &conv,
		`SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting conversation: %w", err)
	}
	return &conv, nil
}

func (r *pgRepository) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	var convs []Conversation
	err := r.db.SelectContext(ctx,
		&convs,
		`SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations
		 WHERE user_a_id = $1 OR user_b_id = $1
		 ORDER BY created_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("listing conversations: %w", err)
	}
	if convs == nil {
		convs = []Conversation{}
	}
	return convs, nil
}

func (r *pgRepository) CreateMessage(ctx context.Context, msg *Message) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning message transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the conversation row so concurrent sends in the same
	// conversation serialize their sequence-number assignment instead of
	// racing to compute the same MAX(sequence)+1.
	if _, err := tx.ExecContext(ctx, `SELECT id FROM chat_conversations WHERE id = $1 FOR UPDATE`, msg.ConversationID); err != nil {
		return fmt.Errorf("locking conversation: %w", err)
	}

	var nextSeq int64
	err = tx.GetContext(ctx, &nextSeq,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM chat_messages WHERE conversation_id = $1`,
		msg.ConversationID)
	if err != nil {
		return fmt.Errorf("computing next sequence: %w", err)
	}

	query := `
		INSERT INTO chat_messages (
			id, conversation_id, sender_id, sequence, ciphertext, nonce,
			ephemeral_key, one_time_prekey_id, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = tx.ExecContext(ctx, query,
		msg.ID, msg.ConversationID, msg.SenderID, nextSeq, msg.Ciphertext, msg.Nonce,
		msg.EphemeralKey, msg.OneTimePreKeyID, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating message: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing message: %w", err)
	}

	msg.Sequence = nextSeq
	return nil
}

func (r *pgRepository) ListMessages(ctx context.Context, conversationID string, beforeSequence int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var messages []Message
	var err error
	if beforeSequence > 0 {
		err = r.db.SelectContext(ctx, &messages,
			`SELECT id, conversation_id, sender_id, sequence, ciphertext, nonce, ephemeral_key, one_time_prekey_id, created_at
			 FROM chat_messages
			 WHERE conversation_id = $1 AND sequence < $2
			 ORDER BY sequence DESC
			 LIMIT $3`,
			conversationID, beforeSequence, limit)
	} else {
		err = r.db.SelectContext(ctx, &messages,
			`SELECT id, conversation_id, sender_id, sequence, ciphertext, nonce, ephemeral_key, one_time_prekey_id, created_at
			 FROM chat_messages
			 WHERE conversation_id = $1
			 ORDER BY sequence DESC
			 LIMIT $2`,
			conversationID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	if messages == nil {
		messages = []Message{}
	}
	return messages, nil
}
