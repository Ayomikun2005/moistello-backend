package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// RedisBridge subscribes to a Redis channel for cross-instance WebSocket
// events and relays them to the local Hub. One RedisBridge should be started
// per API server instance.
type RedisBridge struct {
	hub       *Hub
	rdb       *redis.Client
	stop      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// NewRedisBridge creates a RedisBridge and starts consuming events from the
// Redis channel in a background goroutine. Call Close to stop consumption.
func NewRedisBridge(hub *Hub, rdb *redis.Client) *RedisBridge {
	b := &RedisBridge{hub: hub, rdb: rdb, stop: make(chan struct{})}
	if rdb != nil {
		b.wg.Add(1)
		go b.consume()
	}
	return b
}

// Close stops the background goroutine that consumes Redis events and waits for it to finish.
func (b *RedisBridge) Close() {
	b.closeOnce.Do(func() {
		close(b.stop)
	})
	b.wg.Wait()
}

func (b *RedisBridge) consume() {
	defer b.wg.Done()
	pubsub := b.rdb.Subscribe(context.Background(), redisChannel)
	defer pubsub.Close()

	ch := pubsub.Channel(redis.WithChannelSize(256))

	for {
		select {
		case <-b.stop:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			b.handleMessage([]byte(msg.Payload))
		}
	}
}

func (b *RedisBridge) handleMessage(data []byte) {
	var env struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		log.Debug().Err(err).Msg("redis bridge unmarshal")
		return
	}

	var pay struct {
		CircleID    string  `json:"circleId"`
		CommunityID string  `json:"communityId"`
		UserID      string  `json:"userId"`
		Status      string  `json:"status"`
		RoundNumber int     `json:"roundNumber"`
		Amount      float64 `json:"amount"`
		Timestamp   string  `json:"timestamp"`
	}
	if err := json.Unmarshal(env.Payload, &pay); err != nil {
		log.Debug().Err(err).Msg("redis bridge payload unmarshal")
		return
	}

	// Reconstruct and relay to local Hub
	var msg Message
	msg.Type = env.Type
	msg.Payload = pay

	// Relay to circle room or user
	switch env.Type {
	case "user.updated", "notification.new":
		if pay.UserID != "" {
			b.hub.BroadcastToUser(pay.UserID, msg)
		}
	default:
		room := pay.CircleID
		if room == "" {
			room = pay.CommunityID
		}
		if room != "" {
			b.hub.Broadcast(room, msg)
		}
	}
}

// Ensure RedisBridge implements a keep-alive / stats method.
func (b *RedisBridge) Stats() (int, int) { return b.hub.Stats() }
