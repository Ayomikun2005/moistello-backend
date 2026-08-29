package websocket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/chat"
)

func TestBroadcaster_ChatMessageDelivered_DeliversToRecipientOnly(t *testing.T) {
	hub := NewHub()
	recipient := &Client{ID: "c1", UserID: "user-b", Send: make(chan []byte, 256), Hub: hub}
	other := &Client{ID: "c2", UserID: "user-c", Send: make(chan []byte, 256), Hub: hub}
	hub.Register(recipient)
	hub.Register(other)

	b := NewBroadcaster(hub, nil)
	msg := &chat.Message{ID: "msg-1", ConversationID: "conv-1", SenderID: "user-a", Ciphertext: "ct", Nonce: "n"}
	b.ChatMessageDelivered(t.Context(), "user-b", msg)

	select {
	case data := <-recipient.Send:
		var envelope Message
		require.NoError(t, json.Unmarshal(data, &envelope))
		assert.Equal(t, "chat.message", envelope.Type)
	default:
		t.Fatal("expected recipient to receive a message")
	}

	select {
	case <-other.Send:
		t.Fatal("a different user must not receive another user's chat message")
	default:
	}
}

func TestBroadcaster_ChatMessageDelivered_NoOpWhenRecipientOffline(t *testing.T) {
	hub := NewHub()
	b := NewBroadcaster(hub, nil)

	assert.NotPanics(t, func() {
		b.ChatMessageDelivered(t.Context(), "no-such-user", &chat.Message{ID: "msg-1"})
	})
}
