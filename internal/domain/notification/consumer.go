package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/moistello/backend/pkg/rabbitmq"
)

// NotificationsQueue is the durable queue notification events are consumed
// from. Config default: "moistello.notifications".
const notificationsRoutingPattern = "notification.*"

// StartQueueConsumer ensures the durable notifications queue exists, binds it
// to the events exchange and consumes notification.* events, fanning each one
// out to connected WebSocket clients. It blocks until ctx is cancelled or the
// broker connection drops.
func StartQueueConsumer(ctx context.Context, client *rabbitmq.Client, exchange, queue string, broadcaster Broadcaster) error {
	if err := client.EnsureQueue(queue, exchange, notificationsRoutingPattern); err != nil {
		return fmt.Errorf("ensuring notifications queue: %w", err)
	}

	return client.Consume(ctx, queue, func(ctx context.Context, body []byte) error {
		var n Notification
		if err := json.Unmarshal(body, &n); err != nil {
			// Malformed payloads can never become valid; dropping them keeps
			// the queue flowing.
			return fmt.Errorf("decoding notification event: %w", err)
		}
		if broadcaster != nil {
			broadcaster.NotificationCreated(ctx, n.UserID.String(), n.ID.String())
		}
		return nil
	})
}
