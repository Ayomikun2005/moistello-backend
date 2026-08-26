package websocket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", Send: make(chan []byte, 256), Hub: hub}
	hub.Register(client)
	c, _ := hub.Stats()
	assert.Equal(t, 1, c)
	hub.Unregister(client)
	c, _ = hub.Stats()
	assert.Equal(t, 0, c)
}

func TestHub_JoinLeaveRoom(t *testing.T) {
	hub := NewHub()
	c1 := &Client{ID: "c1", Send: make(chan []byte, 256), Hub: hub}
	c2 := &Client{ID: "c2", Send: make(chan []byte, 256), Hub: hub}
	hub.Register(c1)
	hub.Register(c2)
	hub.JoinRoom("circle-123", "c1")
	hub.JoinRoom("circle-123", "c2")
	hub.JoinRoom("circle-456", "c1")
	_, rooms := hub.Stats()
	assert.Equal(t, 2, rooms)

	hub.LeaveRoom("circle-123", "c1")
	hub.Unregister(c2)
	clients, _ := hub.Stats()
	assert.GreaterOrEqual(t, clients, 1) // At least c1 remains
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	c1 := &Client{ID: "c1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(c1)
	hub.JoinRoom("circle-1", "c1")
	hub.Broadcast("circle-1", Message{Type: "test", Payload: "hello"})
	select {
	case msg := <-c1.Send:
		assert.Contains(t, string(msg), "test")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout")
	}
}

func TestHub_BroadcastToUser(t *testing.T) {
	hub := NewHub()
	// Client has random UUID ID and authentic UserID
	c1 := &Client{ID: "conn-uuid-1", UserID: "user-42", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(c1)

	hub.BroadcastToUser("user-42", Message{Type: "private", Payload: "secret"})
	select {
	case msg := <-c1.Send:
		assert.Contains(t, string(msg), "private")
		assert.Contains(t, string(msg), "secret")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for user broadcast")
	}
}

func TestHub_BroadcastToUser_MultipleConnections(t *testing.T) {
	hub := NewHub()
	// Two connections for the same user (e.g. phone and laptop)
	c1 := &Client{ID: "conn-phone", UserID: "user-99", Send: make(chan []byte, 10), Hub: hub}
	c2 := &Client{ID: "conn-laptop", UserID: "user-99", Send: make(chan []byte, 10), Hub: hub}
	cOther := &Client{ID: "conn-other", UserID: "user-other", Send: make(chan []byte, 10), Hub: hub}

	hub.Register(c1)
	hub.Register(c2)
	hub.Register(cOther)

	assert.Equal(t, 2, hub.UserClientCount("user-99"))
	assert.Equal(t, 1, hub.UserClientCount("user-other"))

	hub.BroadcastToUser("user-99", Message{Type: "notification.new", Payload: "badge"})

	// Both c1 and c2 should receive
	select {
	case msg := <-c1.Send:
		assert.Contains(t, string(msg), "notification.new")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout on c1")
	}

	select {
	case msg := <-c2.Send:
		assert.Contains(t, string(msg), "notification.new")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout on c2")
	}

	// cOther should not receive
	select {
	case <-cOther.Send:
		t.Fatal("cOther should not receive message for user-99")
	default:
	}

	// Unregister one connection
	hub.Unregister(c1)
	assert.Equal(t, 1, hub.UserClientCount("user-99"))

	// Unregister second connection
	hub.Unregister(c2)
	assert.Equal(t, 0, hub.UserClientCount("user-99"))
}

func TestHub_Broadcast_DifferentRoom(t *testing.T) {
	hub := NewHub()
	c1 := &Client{ID: "c1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(c1)
	hub.JoinRoom("circle-a", "c1")
	hub.Broadcast("circle-b", Message{Type: "test"})
	select {
	case <-c1.Send:
		t.Fatal("should not receive")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHub_Broadcast_FullChannel(t *testing.T) {
	hub := NewHub()
	c1 := &Client{ID: "c1", Send: make(chan []byte, 1), Hub: hub}
	hub.Register(c1)
	hub.JoinRoom("circle-1", "c1")
	c1.Send <- []byte("block")
	hub.Broadcast("circle-1", Message{Type: "drop"})
	time.Sleep(50 * time.Millisecond)
	_, rooms := hub.Stats()
	assert.Equal(t, 1, rooms)
}

func TestClient_HandleSubscribe(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(client)
	client.handleMessage([]byte(`{"type":"subscribe","circleId":"circle-123"}`))
	_, rooms := hub.Stats()
	assert.GreaterOrEqual(t, rooms, 1)
}

func TestClient_HandlePing(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(client)
	client.handleMessage([]byte(`{"type":"ping"}`))
	select {
	case msg := <-client.Send:
		assert.Contains(t, string(msg), "pong")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout")
	}
}

func TestClient_MissedPings(t *testing.T) {
	hub := NewHub()
	client := &Client{ID: "c1", Send: make(chan []byte, 10), Hub: hub}
	hub.Register(client)

	assert.Equal(t, int32(0), client.MissedPings())
	client.missedPings.Add(1)
	assert.Equal(t, int32(1), client.MissedPings())
	client.missedPings.Add(1)
	assert.Equal(t, int32(2), client.MissedPings())

	client.ResetMissedPings()
	assert.Equal(t, int32(0), client.MissedPings())
}

func TestMessage_Serialization(t *testing.T) {
	msg := Message{Type: "circle.updated", Payload: map[string]any{"circleId": "abc"}}
	assert.Equal(t, "circle.updated", msg.Type)
	assert.NotNil(t, msg.Payload)
}

func TestHub_Stats(t *testing.T) {
	hub := NewHub()
	c, r := hub.Stats()
	assert.Equal(t, 0, c)
	assert.Equal(t, 0, r)
}
