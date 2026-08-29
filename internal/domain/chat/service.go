package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/moistello/backend/pkg/apperrors"
)

// Broadcaster delivers a newly-sent message to the recipient's live
// WebSocket connection(s), if any are open. Implemented by
// internal/websocket.Broadcaster; mirrors the pattern used by
// community.Broadcaster.
type Broadcaster interface {
	ChatMessageDelivered(ctx context.Context, recipientID string, msg *Message)
}

// PublishKeysInput is a client's full X3DH key bundle publish/replenish
// request: identity key (only meaningful the first time), current signed
// prekey, and any new one-time prekeys to add to the pool.
type PublishKeysInput struct {
	IdentityKeyPub  string
	SignedPreKeyID  int
	SignedPreKeyPub string
	Signature       string
	OneTimePreKeys  []OneTimePreKeyInput
}

// SendMessageInput is a pre-encrypted message envelope from the client.
// EphemeralKey/OneTimePreKeyID must be set on the first message of a new
// X3DH session and omitted afterward.
type SendMessageInput struct {
	Ciphertext      string
	Nonce           string
	EphemeralKey    string
	OneTimePreKeyID *int
}

type Service interface {
	PublishKeys(ctx context.Context, userID string, input PublishKeysInput) error
	// GetBundle fetches targetUserID's prekey bundle so the caller can
	// initiate an X3DH handshake with them. Consumes one one-time prekey
	// from targetUserID's pool if one is available.
	GetBundle(ctx context.Context, targetUserID string) (*PreKeyBundle, error)

	CreateConversation(ctx context.Context, userID, otherUserID string) (*Conversation, error)
	ListConversations(ctx context.Context, userID string) ([]Conversation, error)

	// SendMessage persists a message in conversationID on behalf of userID
	// and delivers it to the other participant's live WebSocket connection
	// if one exists. Fails if userID is not a participant.
	SendMessage(ctx context.Context, userID, conversationID string, input SendMessageInput) (*Message, error)
	// ListMessages returns userID's message history for a conversation
	// they participate in.
	ListMessages(ctx context.Context, userID, conversationID string, beforeSequence int64, limit int) ([]Message, error)
}

type service struct {
	keys        KeyRepository
	messages    Repository
	broadcaster Broadcaster
}

func NewService(keys KeyRepository, messages Repository, broadcaster Broadcaster) Service {
	return &service{keys: keys, messages: messages, broadcaster: broadcaster}
}

func (s *service) PublishKeys(ctx context.Context, userID string, input PublishKeysInput) error {
	if input.IdentityKeyPub == "" {
		return fmt.Errorf("identityKeyPub is required")
	}
	if input.SignedPreKeyPub == "" || input.Signature == "" {
		return fmt.Errorf("signedPreKeyPub and signature are required")
	}

	if err := s.keys.UpsertIdentityKey(ctx, userID, input.IdentityKeyPub); err != nil {
		return err
	}
	if err := s.keys.UpsertSignedPreKey(ctx, userID, input.SignedPreKeyID, input.SignedPreKeyPub, input.Signature); err != nil {
		return err
	}
	if len(input.OneTimePreKeys) > 0 {
		if err := s.keys.AddOneTimePreKeys(ctx, userID, input.OneTimePreKeys); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) GetBundle(ctx context.Context, targetUserID string) (*PreKeyBundle, error) {
	return s.keys.GetBundle(ctx, targetUserID)
}

func (s *service) CreateConversation(ctx context.Context, userID, otherUserID string) (*Conversation, error) {
	if userID == otherUserID {
		return nil, fmt.Errorf("cannot start a conversation with yourself")
	}
	return s.messages.GetOrCreateConversation(ctx, userID, otherUserID)
}

func (s *service) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	return s.messages.ListConversations(ctx, userID)
}

func (s *service) SendMessage(ctx context.Context, userID, conversationID string, input SendMessageInput) (*Message, error) {
	if input.Ciphertext == "" || input.Nonce == "" {
		return nil, fmt.Errorf("ciphertext and nonce are required")
	}

	conv, err := s.messages.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if !conv.HasParticipant(userID) {
		return nil, apperrors.ErrForbidden
	}

	msg := &Message{
		ID:             uuid.New().String(),
		ConversationID: conversationID,
		SenderID:       userID,
		Ciphertext:     input.Ciphertext,
		Nonce:          input.Nonce,
		CreatedAt:      time.Now().UTC(),
	}
	if input.EphemeralKey != "" {
		msg.EphemeralKey = &input.EphemeralKey
	}
	if input.OneTimePreKeyID != nil {
		msg.OneTimePreKeyID = input.OneTimePreKeyID
	}

	if err := s.messages.CreateMessage(ctx, msg); err != nil {
		return nil, err
	}

	if s.broadcaster != nil {
		s.broadcaster.ChatMessageDelivered(ctx, conv.OtherParticipant(userID), msg)
	}

	return msg, nil
}

func (s *service) ListMessages(ctx context.Context, userID, conversationID string, beforeSequence int64, limit int) ([]Message, error) {
	conv, err := s.messages.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if !conv.HasParticipant(userID) {
		return nil, apperrors.ErrForbidden
	}
	return s.messages.ListMessages(ctx, conversationID, beforeSequence, limit)
}
