package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/internal/domain/chat"
	"github.com/moistello/backend/pkg/apperrors"
	"github.com/moistello/backend/pkg/response"
)

// ChatHandler exposes the E2EE chat API (#188) on top of the X3DH
// primitives in internal/domain/chat: publishing/fetching key bundles,
// creating conversations, and sending/listing already-encrypted messages.
// The server never sees plaintext — Ciphertext/Nonce are opaque blobs
// produced client-side.
type ChatHandler struct {
	svc chat.Service
}

func NewChatHandler(svc chat.Service) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// @Summary Publish X3DH key bundle
// @Description Publishes or updates the caller's identity key, signed prekey, and replenishes their one-time prekey pool.
// @Tags Chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object{identityKeyPub=string,signedPreKeyId=int,signedPreKeyPub=string,signature=string,oneTimePreKeys=array} true "Key bundle"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Router /chat/keys [post]
func (h *ChatHandler) PublishKeys(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		IdentityKeyPub  string `json:"identityKeyPub" binding:"required"`
		SignedPreKeyID  int    `json:"signedPreKeyId" binding:"required"`
		SignedPreKeyPub string `json:"signedPreKeyPub" binding:"required"`
		Signature       string `json:"signature" binding:"required"`
		OneTimePreKeys  []struct {
			KeyID  int    `json:"keyId" binding:"required"`
			PubKey string `json:"pubKey" binding:"required"`
		} `json:"oneTimePreKeys"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	input := chat.PublishKeysInput{
		IdentityKeyPub:  req.IdentityKeyPub,
		SignedPreKeyID:  req.SignedPreKeyID,
		SignedPreKeyPub: req.SignedPreKeyPub,
		Signature:       req.Signature,
	}
	for _, k := range req.OneTimePreKeys {
		input.OneTimePreKeys = append(input.OneTimePreKeys, chat.OneTimePreKeyInput{KeyID: k.KeyID, PubKey: k.PubKey})
	}

	if err := h.svc.PublishKeys(c.Request.Context(), userID, input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, gin.H{"published": true})
}

// @Summary Get a user's X3DH prekey bundle
// @Description Fetches userId's current prekey bundle to initiate an X3DH handshake with them. Consumes one one-time prekey from their pool if available.
// @Tags Chat
// @Produce json
// @Security BearerAuth
// @Param userId path string true "Target user ID"
// @Success 200 {object} response.Envelope{data=object{bundle=object}}
// @Failure 404 {object} response.Envelope
// @Router /chat/keys/{userId} [get]
func (h *ChatHandler) GetBundle(c *gin.Context) {
	bundle, err := h.svc.GetBundle(c.Request.Context(), c.Param("userId"))
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "user has not published chat keys")
			return
		}
		response.InternalError(c, "failed to get key bundle")
		return
	}
	response.OK(c, gin.H{"bundle": bundle})
}

// @Summary Create or get a conversation
// @Description Creates a 1:1 conversation with another user, or returns the existing one if it already exists.
// @Tags Chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object{userId=string} true "Other participant's user ID"
// @Success 201 {object} response.Envelope{data=object{conversation=object}}
// @Failure 400 {object} response.Envelope
// @Router /chat/conversations [post]
func (h *ChatHandler) CreateConversation(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		UserID string `json:"userId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "userId is required")
		return
	}

	conv, err := h.svc.CreateConversation(c.Request.Context(), userID, req.UserID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Created(c, gin.H{"conversation": conv})
}

// @Summary List my conversations
// @Description Lists all conversations the caller participates in, most recent first.
// @Tags Chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=object{conversations=array}}
// @Router /chat/conversations [get]
func (h *ChatHandler) ListConversations(c *gin.Context) {
	convs, err := h.svc.ListConversations(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.InternalError(c, "failed to list conversations")
		return
	}
	response.OK(c, gin.H{"conversations": convs})
}

// @Summary Send an encrypted message
// @Description Persists a client-encrypted message in a conversation and delivers it to the recipient's live WebSocket connection if one is open.
// @Tags Chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Conversation ID"
// @Param body body object{ciphertext=string,nonce=string,ephemeralKey=string,oneTimePreKeyId=int} true "Encrypted message envelope"
// @Success 201 {object} response.Envelope{data=object{message=object}}
// @Failure 400 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /chat/conversations/{id}/messages [post]
func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		Ciphertext      string `json:"ciphertext" binding:"required"`
		Nonce           string `json:"nonce" binding:"required"`
		EphemeralKey    string `json:"ephemeralKey"`
		OneTimePreKeyID *int   `json:"oneTimePreKeyId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "ciphertext and nonce are required")
		return
	}

	msg, err := h.svc.SendMessage(c.Request.Context(), userID, c.Param("id"), chat.SendMessageInput{
		Ciphertext:      req.Ciphertext,
		Nonce:           req.Nonce,
		EphemeralKey:    req.EphemeralKey,
		OneTimePreKeyID: req.OneTimePreKeyID,
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrForbidden) {
			response.Forbidden(c, "not a participant in this conversation")
			return
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "conversation not found")
			return
		}
		response.BadRequest(c, err.Error())
		return
	}

	response.Created(c, gin.H{"message": msg})
}

// @Summary List messages in a conversation
// @Description Returns paginated message history for a conversation, newest first. Pass `before` (a message sequence number) to page backward.
// @Tags Chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "Conversation ID"
// @Param before query int false "Return messages with sequence < before"
// @Param limit query int false "Max messages to return (default 50, max 200)"
// @Success 200 {object} response.Envelope{data=object{messages=array}}
// @Failure 403 {object} response.Envelope
// @Router /chat/conversations/{id}/messages [get]
func (h *ChatHandler) ListMessages(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var before int64
	if raw := c.Query("before"); raw != "" {
		before, _ = strconv.ParseInt(raw, 10, 64)
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if l, err := strconv.Atoi(raw); err == nil {
			limit = l
		}
	}

	messages, err := h.svc.ListMessages(c.Request.Context(), userID, c.Param("id"), before, limit)
	if err != nil {
		if errors.Is(err, apperrors.ErrForbidden) {
			response.Forbidden(c, "not a participant in this conversation")
			return
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "conversation not found")
			return
		}
		response.InternalError(c, "failed to list messages")
		return
	}

	response.OK(c, gin.H{"messages": messages})
}
