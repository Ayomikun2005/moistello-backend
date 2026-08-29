package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/chat"
)

type KeyRepository struct {
	mock.Mock
}

func (m *KeyRepository) UpsertIdentityKey(ctx context.Context, userID, identityKeyPub string) error {
	args := m.Called(ctx, userID, identityKeyPub)
	return args.Error(0)
}

func (m *KeyRepository) UpsertSignedPreKey(ctx context.Context, userID string, keyID int, signedPreKeyPub, signature string) error {
	args := m.Called(ctx, userID, keyID, signedPreKeyPub, signature)
	return args.Error(0)
}

func (m *KeyRepository) AddOneTimePreKeys(ctx context.Context, userID string, keys []chat.OneTimePreKeyInput) error {
	args := m.Called(ctx, userID, keys)
	return args.Error(0)
}

func (m *KeyRepository) CountUnusedOneTimePreKeys(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *KeyRepository) GetBundle(ctx context.Context, userID string) (*chat.PreKeyBundle, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*chat.PreKeyBundle), args.Error(1)
}

type Repository struct {
	mock.Mock
}

func (m *Repository) GetOrCreateConversation(ctx context.Context, userA, userB string) (*chat.Conversation, error) {
	args := m.Called(ctx, userA, userB)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*chat.Conversation), args.Error(1)
}

func (m *Repository) GetConversation(ctx context.Context, id string) (*chat.Conversation, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*chat.Conversation), args.Error(1)
}

func (m *Repository) ListConversations(ctx context.Context, userID string) ([]chat.Conversation, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]chat.Conversation), args.Error(1)
}

func (m *Repository) CreateMessage(ctx context.Context, msg *chat.Message) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *Repository) ListMessages(ctx context.Context, conversationID string, beforeSequence int64, limit int) ([]chat.Message, error) {
	args := m.Called(ctx, conversationID, beforeSequence, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]chat.Message), args.Error(1)
}

type Broadcaster struct {
	mock.Mock
}

func (m *Broadcaster) ChatMessageDelivered(ctx context.Context, recipientID string, msg *chat.Message) {
	m.Called(ctx, recipientID, msg)
}
