package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type stubAuthorizer struct {
	allowed bool
	err     error
}

func (s stubAuthorizer) CanSubscribe(ctx context.Context, circleID, userID string) (bool, error) {
	return s.allowed, s.err
}

func TestHub_JoinRoomDeniedForNonMember(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", UserID: "u1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(client)
	hub.SetSubscriptionAuthorizer(stubAuthorizer{allowed: false})

	assert.False(t, hub.JoinRoom("circle-123", "c1"))
	_, rooms := hub.Stats()
	assert.Equal(t, 0, rooms)
}

func TestHub_JoinRoomAllowedForMember(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", UserID: "u1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(client)
	hub.SetSubscriptionAuthorizer(stubAuthorizer{allowed: true})

	assert.True(t, hub.JoinRoom("circle-123", "c1"))
	_, rooms := hub.Stats()
	assert.Equal(t, 1, rooms)
}

func TestClient_HandleSubscribeDeniedForNonMember(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", UserID: "u1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(client)
	hub.SetSubscriptionAuthorizer(stubAuthorizer{allowed: false})

	client.handleMessage([]byte(`{"type":"subscribe","circleId":"circle-123"}`))
	_, rooms := hub.Stats()
	assert.Equal(t, 0, rooms)

	select {
	case msg := <-client.Send:
		assert.Contains(t, string(msg), "error")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected error message")
	}
}
