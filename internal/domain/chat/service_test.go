package chat_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/chat"
	"github.com/moistello/backend/internal/domain/chat/mocks"
	"github.com/moistello/backend/pkg/apperrors"
)

func TestService_PublishKeys(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)
	ctx := context.Background()

	input := chat.PublishKeysInput{
		IdentityKeyPub:  "ik-pub",
		SignedPreKeyID:  1,
		SignedPreKeyPub: "spk-pub",
		Signature:       "sig",
		OneTimePreKeys:  []chat.OneTimePreKeyInput{{KeyID: 1, PubKey: "opk-1"}},
	}

	keys.On("UpsertIdentityKey", ctx, "user-1", "ik-pub").Return(nil)
	keys.On("UpsertSignedPreKey", ctx, "user-1", 1, "spk-pub", "sig").Return(nil)
	keys.On("AddOneTimePreKeys", ctx, "user-1", input.OneTimePreKeys).Return(nil)

	err := svc.PublishKeys(ctx, "user-1", input)
	require.NoError(t, err)
	keys.AssertExpectations(t)
}

func TestService_PublishKeys_RequiresIdentityKey(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)

	err := svc.PublishKeys(context.Background(), "user-1", chat.PublishKeysInput{
		SignedPreKeyPub: "spk", Signature: "sig",
	})
	assert.Error(t, err)
	keys.AssertNotCalled(t, "UpsertIdentityKey", mock.Anything, mock.Anything, mock.Anything)
}

func TestService_CreateConversation_RejectsSelf(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)

	_, err := svc.CreateConversation(context.Background(), "user-1", "user-1")
	assert.Error(t, err)
	msgs.AssertNotCalled(t, "GetOrCreateConversation", mock.Anything, mock.Anything, mock.Anything)
}

func TestService_SendMessage_RejectsNonParticipant(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)
	ctx := context.Background()

	conv := &chat.Conversation{ID: "conv-1", UserAID: "user-a", UserBID: "user-b"}
	msgs.On("GetConversation", ctx, "conv-1").Return(conv, nil)

	_, err := svc.SendMessage(ctx, "user-c", "conv-1", chat.SendMessageInput{Ciphertext: "ct", Nonce: "n"})
	assert.ErrorIs(t, err, apperrors.ErrForbidden)
	msgs.AssertNotCalled(t, "CreateMessage", mock.Anything, mock.Anything)
}

func TestService_SendMessage_PersistsAndBroadcastsToOtherParticipant(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	broadcaster := new(mocks.Broadcaster)
	svc := chat.NewService(keys, msgs, broadcaster)
	ctx := context.Background()

	conv := &chat.Conversation{ID: "conv-1", UserAID: "user-a", UserBID: "user-b"}
	msgs.On("GetConversation", ctx, "conv-1").Return(conv, nil)
	msgs.On("CreateMessage", ctx, mock.MatchedBy(func(m *chat.Message) bool {
		return m.SenderID == "user-a" && m.Ciphertext == "ct" && m.Nonce == "n"
	})).Return(nil)
	broadcaster.On("ChatMessageDelivered", ctx, "user-b", mock.AnythingOfType("*chat.Message")).Return()

	msg, err := svc.SendMessage(ctx, "user-a", "conv-1", chat.SendMessageInput{Ciphertext: "ct", Nonce: "n"})
	require.NoError(t, err)
	assert.Equal(t, "user-a", msg.SenderID)
	msgs.AssertExpectations(t)
	broadcaster.AssertExpectations(t)
}

func TestService_SendMessage_RequiresCiphertextAndNonce(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)

	_, err := svc.SendMessage(context.Background(), "user-a", "conv-1", chat.SendMessageInput{})
	assert.Error(t, err)
	msgs.AssertNotCalled(t, "GetConversation", mock.Anything, mock.Anything)
}

func TestService_ListMessages_RejectsNonParticipant(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)
	ctx := context.Background()

	conv := &chat.Conversation{ID: "conv-1", UserAID: "user-a", UserBID: "user-b"}
	msgs.On("GetConversation", ctx, "conv-1").Return(conv, nil)

	_, err := svc.ListMessages(ctx, "user-c", "conv-1", 0, 50)
	assert.ErrorIs(t, err, apperrors.ErrForbidden)
}

func TestService_GetBundle_Delegates(t *testing.T) {
	keys := new(mocks.KeyRepository)
	msgs := new(mocks.Repository)
	svc := chat.NewService(keys, msgs, nil)
	ctx := context.Background()

	bundle := &chat.PreKeyBundle{UserID: "user-b", IdentityKey: "ik"}
	keys.On("GetBundle", ctx, "user-b").Return(bundle, nil)

	got, err := svc.GetBundle(ctx, "user-b")
	require.NoError(t, err)
	assert.Equal(t, bundle, got)
}
