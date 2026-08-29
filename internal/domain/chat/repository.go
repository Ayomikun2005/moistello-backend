package chat

import "context"

// Repository persists conversations and messages.
type Repository interface {
	// GetOrCreateConversation returns the existing 1:1 conversation between
	// userA and userB, creating it if none exists yet. Order of userA/userB
	// doesn't matter — the same pair always maps to the same conversation.
	GetOrCreateConversation(ctx context.Context, userA, userB string) (*Conversation, error)
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	ListConversations(ctx context.Context, userID string) ([]Conversation, error)

	// CreateMessage assigns the next sequence number within the
	// conversation and persists msg. msg.Sequence is ignored on input and
	// populated on the returned value.
	CreateMessage(ctx context.Context, msg *Message) error
	// ListMessages returns up to limit messages from conversationID with
	// sequence < beforeSequence (pass 0 to start from the most recent),
	// newest first.
	ListMessages(ctx context.Context, conversationID string, beforeSequence int64, limit int) ([]Message, error)
}
