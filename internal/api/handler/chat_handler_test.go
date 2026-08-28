package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/chat"
	"github.com/moistello/backend/pkg/apperrors"
)

type fakeChatService struct {
	publishKeysFn func(ctx context.Context, userID string, input chat.PublishKeysInput) error
	getBundleFn   func(ctx context.Context, targetUserID string) (*chat.PreKeyBundle, error)
	createConvFn  func(ctx context.Context, userID, otherUserID string) (*chat.Conversation, error)
	listConvsFn   func(ctx context.Context, userID string) ([]chat.Conversation, error)
	sendMsgFn     func(ctx context.Context, userID, conversationID string, input chat.SendMessageInput) (*chat.Message, error)
	listMsgsFn    func(ctx context.Context, userID, conversationID string, before int64, limit int) ([]chat.Message, error)
}

func (f *fakeChatService) PublishKeys(ctx context.Context, userID string, input chat.PublishKeysInput) error {
	return f.publishKeysFn(ctx, userID, input)
}
func (f *fakeChatService) GetBundle(ctx context.Context, targetUserID string) (*chat.PreKeyBundle, error) {
	return f.getBundleFn(ctx, targetUserID)
}
func (f *fakeChatService) CreateConversation(ctx context.Context, userID, otherUserID string) (*chat.Conversation, error) {
	return f.createConvFn(ctx, userID, otherUserID)
}
func (f *fakeChatService) ListConversations(ctx context.Context, userID string) ([]chat.Conversation, error) {
	return f.listConvsFn(ctx, userID)
}
func (f *fakeChatService) SendMessage(ctx context.Context, userID, conversationID string, input chat.SendMessageInput) (*chat.Message, error) {
	return f.sendMsgFn(ctx, userID, conversationID, input)
}
func (f *fakeChatService) ListMessages(ctx context.Context, userID, conversationID string, before int64, limit int) ([]chat.Message, error) {
	return f.listMsgsFn(ctx, userID, conversationID, before, limit)
}

func setupChatRouter(svc chat.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := handler.NewChatHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-a"); c.Next() })
	r.POST("/v1/chat/keys", h.PublishKeys)
	r.GET("/v1/chat/keys/:userId", h.GetBundle)
	r.POST("/v1/chat/conversations", h.CreateConversation)
	r.GET("/v1/chat/conversations", h.ListConversations)
	r.POST("/v1/chat/conversations/:id/messages", h.SendMessage)
	r.GET("/v1/chat/conversations/:id/messages", h.ListMessages)
	return r
}

func TestChatHandler_PublishKeys_Success(t *testing.T) {
	svc := &fakeChatService{
		publishKeysFn: func(ctx context.Context, userID string, input chat.PublishKeysInput) error {
			assert.Equal(t, "user-a", userID)
			assert.Equal(t, "ik-pub", input.IdentityKeyPub)
			assert.Len(t, input.OneTimePreKeys, 1)
			return nil
		},
	}
	r := setupChatRouter(svc)

	body, _ := json.Marshal(map[string]any{
		"identityKeyPub": "ik-pub", "signedPreKeyId": 1, "signedPreKeyPub": "spk-pub", "signature": "sig",
		"oneTimePreKeys": []map[string]any{{"keyId": 1, "pubKey": "opk-1"}},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChatHandler_GetBundle_NotFound(t *testing.T) {
	svc := &fakeChatService{
		getBundleFn: func(ctx context.Context, targetUserID string) (*chat.PreKeyBundle, error) {
			return nil, apperrors.ErrNotFound
		},
	}
	r := setupChatRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/chat/keys/user-z", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChatHandler_CreateConversation_Success(t *testing.T) {
	svc := &fakeChatService{
		createConvFn: func(ctx context.Context, userID, otherUserID string) (*chat.Conversation, error) {
			assert.Equal(t, "user-a", userID)
			assert.Equal(t, "user-b", otherUserID)
			return &chat.Conversation{ID: "conv-1", UserAID: "user-a", UserBID: "user-b"}, nil
		},
	}
	r := setupChatRouter(svc)

	body, _ := json.Marshal(map[string]any{"userId": "user-b"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/conversations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "conv-1")
}

func TestChatHandler_SendMessage_Forbidden(t *testing.T) {
	svc := &fakeChatService{
		sendMsgFn: func(ctx context.Context, userID, conversationID string, input chat.SendMessageInput) (*chat.Message, error) {
			return nil, apperrors.ErrForbidden
		},
	}
	r := setupChatRouter(svc)

	body, _ := json.Marshal(map[string]any{"ciphertext": "ct", "nonce": "n"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/conversations/conv-1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestChatHandler_SendMessage_Success(t *testing.T) {
	svc := &fakeChatService{
		sendMsgFn: func(ctx context.Context, userID, conversationID string, input chat.SendMessageInput) (*chat.Message, error) {
			assert.Equal(t, "conv-1", conversationID)
			assert.Equal(t, "ct", input.Ciphertext)
			return &chat.Message{ID: "msg-1", ConversationID: conversationID, SenderID: userID, Sequence: 1}, nil
		},
	}
	r := setupChatRouter(svc)

	body, _ := json.Marshal(map[string]any{"ciphertext": "ct", "nonce": "n"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/conversations/conv-1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestChatHandler_ListMessages_Success(t *testing.T) {
	svc := &fakeChatService{
		listMsgsFn: func(ctx context.Context, userID, conversationID string, before int64, limit int) ([]chat.Message, error) {
			assert.Equal(t, int64(10), before)
			assert.Equal(t, 20, limit)
			return []chat.Message{{ID: "msg-1"}}, nil
		},
	}
	r := setupChatRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/chat/conversations/conv-1/messages?before=10&limit=20", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "msg-1")
}

func TestChatHandler_ListConversations_Success(t *testing.T) {
	svc := &fakeChatService{
		listConvsFn: func(ctx context.Context, userID string) ([]chat.Conversation, error) {
			return []chat.Conversation{{ID: "conv-1"}}, nil
		},
	}
	r := setupChatRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/chat/conversations", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
