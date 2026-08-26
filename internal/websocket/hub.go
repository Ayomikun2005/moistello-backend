package websocket

import (
	"encoding/json"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/moistello/backend/pkg/metrics"
)

// Message is a structured WebSocket message sent to clients.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// Hub maintains the set of active WebSocket clients and manages circle-based
// rooms for targeted broadcasts.
type Hub struct {
	mu          sync.RWMutex
	clients     map[string]*Client            // clientID -> Client
	userClients map[string]map[string]*Client // userID -> clientID -> Client
	rooms       map[string]map[string]*Client // circleID -> clientID -> Client
}

// NewHub creates a new Hub with empty client, user client, and room registries.
func NewHub() *Hub {
	return &Hub{
		clients:     make(map[string]*Client),
		userClients: make(map[string]map[string]*Client),
		rooms:       make(map[string]map[string]*Client),
	}
}

// Register adds a client to the hub so it can receive broadcasts and updates metrics.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client.ID]; !ok {
		h.clients[client.ID] = client
		metrics.WSActiveConnections.Inc()
	}
	if client.UserID != "" {
		if _, ok := h.userClients[client.UserID]; !ok {
			h.userClients[client.UserID] = make(map[string]*Client)
		}
		h.userClients[client.UserID][client.ID] = client
	}
	h.mu.Unlock()
	log.Debug().Str("clientID", client.ID).Str("userID", client.UserID).Msg("client registered")
}

// Unregister removes a client from the hub, user registry, and all rooms it has joined.
// It is safe to call from any goroutine.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client.ID]; ok {
		delete(h.clients, client.ID)
		metrics.WSActiveConnections.Dec()
	}
	if client.UserID != "" {
		if conns, ok := h.userClients[client.UserID]; ok {
			delete(conns, client.ID)
			if len(conns) == 0 {
				delete(h.userClients, client.UserID)
			}
		}
	}
	for _, room := range h.rooms {
		delete(room, client.ID)
	}
	h.mu.Unlock()
	log.Debug().Str("clientID", client.ID).Str("userID", client.UserID).Msg("client unregistered")
}

// JoinRoom subscribes a client to a circle's broadcast room.
func (h *Hub) JoinRoom(circleID, clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[circleID]; !ok {
		h.rooms[circleID] = make(map[string]*Client)
	}
	if client, ok := h.clients[clientID]; ok {
		h.rooms[circleID][clientID] = client
	}
	log.Debug().Str("circleID", circleID).Str("clientID", clientID).Msg("client joined room")
}

// LeaveRoom unsubscribes a client from a circle's broadcast room.
func (h *Hub) LeaveRoom(circleID, clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[circleID]; ok {
		delete(room, clientID)
	}
	log.Debug().Str("circleID", circleID).Str("clientID", clientID).Msg("client left room")
}

// Broadcast sends a message to all clients currently subscribed to a circle
// room. If the circle has no subscribers the message is silently dropped.
func (h *Hub) Broadcast(circleID string, msg Message) {
	h.mu.RLock()
	room, ok := h.rooms[circleID]
	if !ok || len(room) == 0 {
		h.mu.RUnlock()
		return
	}

	targets := make([]*Client, 0, len(room))
	for _, client := range room {
		targets = append(targets, client)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Warn().Err(err).Str("type", msg.Type).Msg("marshaling broadcast message")
		return
	}

	var slowClients []*Client
	for _, client := range targets {
		select {
		case client.Send <- data:
		default:
			// Client's send buffer is full — assume disconnected
			slowClients = append(slowClients, client)
		}
	}

	for _, client := range slowClients {
		h.Unregister(client)
	}
}

// BroadcastToUser sends a message to all active connections for the specified userID.
// If the user has no connected clients the message is silently dropped.
func (h *Hub) BroadcastToUser(userID string, msg Message) {
	h.mu.RLock()
	userConns, ok := h.userClients[userID]
	if !ok || len(userConns) == 0 {
		h.mu.RUnlock()
		return
	}

	targets := make([]*Client, 0, len(userConns))
	for _, client := range userConns {
		targets = append(targets, client)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Warn().Err(err).Str("type", msg.Type).Str("userID", userID).Msg("marshaling user message")
		return
	}

	var slowClients []*Client
	for _, client := range targets {
		select {
		case client.Send <- data:
		default:
			slowClients = append(slowClients, client)
		}
	}

	for _, client := range slowClients {
		h.Unregister(client)
	}
}

// UserClientCount returns the number of active connections for a given user.
func (h *Hub) UserClientCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if conns, ok := h.userClients[userID]; ok {
		return len(conns)
	}
	return 0
}

// Stats returns the current number of connected clients and active rooms.
func (h *Hub) Stats() (clients int, rooms int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients), len(h.rooms)
}

// ClientCount returns the total number of registered clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// RoomCount returns the total number of active rooms.
func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}
