package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// EventsExchange is the topic exchange notification events are published to.
const EventsExchange = "moistello.events"

// Publisher abstracts the RabbitMQ producer so the service can be tested
// with a mock. *rabbitmq.Client satisfies it.
type Publisher interface {
	Publish(exchange, routingKey string, body []byte) error
}

type Service interface {
	Create(ctx context.Context, input CreateInput) (*Notification, error)
	List(ctx context.Context, userID string, page, limit int, unreadOnly bool) ([]Notification, int, error)
	MarkRead(ctx context.Context, id, userID string) error
	MarkAllRead(ctx context.Context, userID string) error
}

type CreateInput struct {
	UserID  string             `json:"userId" validate:"required"`
	Type    NotificationType   `json:"type" validate:"required"`
	Title   string             `json:"title" validate:"required"`
	Body    string             `json:"body" validate:"required"`
	Data    json.RawMessage    `json:"data"`
	Channel NotificationChannel `json:"channel" validate:"required,oneof=inapp email sms push"`
}

// Broadcaster defines the interface for real-time notification delivery.
type Broadcaster interface {
	NotificationCreated(ctx context.Context, userID, notificationID string)
}

type notificationService struct {
	repo         Repository
	rabbitClient Publisher
	broadcaster  Broadcaster
}

func NewService(repo Repository, rabbitClient Publisher, broadcaster Broadcaster) Service {
	return &notificationService{repo: repo, rabbitClient: rabbitClient, broadcaster: broadcaster}
}

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID: %w", err)
	}
	return id, nil
}

func (s *notificationService) Create(ctx context.Context, input CreateInput) (*Notification, error) {
	userID, err := parseUUID(input.UserID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	n := &Notification{
		ID:        uuid.New(),
		UserID:    userID,
		Type:      input.Type,
		Title:     input.Title,
		Body:      input.Body,
		Data:      input.Data,
		IsRead:    false,
		Channel:   input.Channel,
		CreatedAt: now,
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("creating notification: %w", err)
	}

	// When a publisher is wired, the durable notification queue drives
	// real-time delivery (the consumer fans out over WebSockets) — the direct
	// broadcast is skipped to avoid duplicate deliveries. Without a publisher
	// the broadcaster is used in-process as a graceful fallback.
	if s.rabbitClient != nil {
		payload, err := json.Marshal(n)
		if err == nil {
			routingKey := fmt.Sprintf("notification.%s", input.Channel)
			if pubErr := s.rabbitClient.Publish(EventsExchange, routingKey, payload); pubErr != nil {
				// Persistence already succeeded; fall back to an immediate
				// broadcast so the user does not miss the event.
				log.Warn().Err(pubErr).Msg("publishing notification event")
				if s.broadcaster != nil {
					s.broadcaster.NotificationCreated(ctx, input.UserID, n.ID.String())
				}
				return n, nil
			}
			return n, nil
		}
	}

	if s.broadcaster != nil {
		s.broadcaster.NotificationCreated(ctx, input.UserID, n.ID.String())
	}

	return n, nil
}

func (s *notificationService) List(ctx context.Context, userID string, page, limit int, unreadOnly bool) ([]Notification, int, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	notifications, total, err := s.repo.List(ctx, uid, page, limit, unreadOnly)
	if err != nil {
		return nil, 0, fmt.Errorf("listing notifications: %w", err)
	}
	return notifications, total, nil
}

func (s *notificationService) MarkRead(ctx context.Context, id, userID string) error {
	nid, err := parseUUID(id)
	if err != nil {
		return err
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return err
	}
	if err := s.repo.MarkRead(ctx, nid, uid); err != nil {
		return fmt.Errorf("marking notification read: %w", err)
	}
	return nil
}

func (s *notificationService) MarkAllRead(ctx context.Context, userID string) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return err
	}
	if err := s.repo.MarkAllRead(ctx, uid); err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}
	return nil
}
