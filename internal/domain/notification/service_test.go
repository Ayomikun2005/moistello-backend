package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
)

// stubRepo satisfies Repository without a database; it records Create calls.
type stubRepo struct {
	createCalls int
	createErr   error
}

func (s *stubRepo) Create(ctx context.Context, n *Notification) error {
	s.createCalls++
	return s.createErr
}
func (s *stubRepo) List(ctx context.Context, userID uuid.UUID, page, limit int, unreadOnly bool) ([]Notification, int, error) {
	return nil, 0, nil
}
func (s *stubRepo) MarkRead(ctx context.Context, id, userID uuid.UUID) error { return nil }
func (s *stubRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) error  { return nil }

type fakePublisher struct {
	exchange   string
	routingKey string
	body       []byte
	calls      int
	err        error
}

func (f *fakePublisher) Publish(exchange, routingKey string, body []byte) error {
	f.calls++
	f.exchange = exchange
	f.routingKey = routingKey
	f.body = body
	return f.err
}

type fakeBroadcaster struct {
	userID, notificationID string
	calls                  int
}

func (f *fakeBroadcaster) NotificationCreated(_ context.Context, userID, notificationID string) {
	f.calls++
	f.userID = userID
	f.notificationID = notificationID
}

func validInput() CreateInput {
	return CreateInput{
		UserID:  "11111111-1111-1111-1111-111111111111",
		Type:    "circle.created",
		Title:   "Welcome",
		Body:    "Your circle is ready",
		Channel: ChannelInApp,
	}
}

func TestCreate_PublishesEventToQueue(t *testing.T) {
	repo := &stubRepo{}

	pub := &fakePublisher{}
	broad := &fakeBroadcaster{}
	svc := NewService(repo, pub, broad)

	n, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	require.NotNil(t, n)

	assert.Equal(t, 1, pub.calls, "event should be published exactly once")
	assert.Equal(t, EventsExchange, pub.exchange)
	assert.Equal(t, "notification.inapp", pub.routingKey)

	var payload Notification
	require.NoError(t, json.Unmarshal(pub.body, &payload))
	assert.Equal(t, n.ID, payload.ID)
	assert.Equal(t, "circle.created", string(payload.Type))

	// The queue consumer owns real-time delivery when publishing succeeds.
	assert.Equal(t, 0, broad.calls, "broadcaster must not double-deliver when queued")
}

func TestCreate_FallsBackToBroadcasterWithoutPublisher(t *testing.T) {
	repo := &stubRepo{}

	broad := &fakeBroadcaster{}
	svc := NewService(repo, nil, broad)

	n, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)

	assert.Equal(t, 0, (&fakePublisher{}).calls) // sanity: nothing published anywhere
	assert.Equal(t, 1, broad.calls)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", broad.userID)
	assert.Equal(t, n.ID.String(), broad.notificationID)
}

func TestCreate_PublishErrorFallsBackToBroadcast(t *testing.T) {
	repo := &stubRepo{}

	pub := &fakePublisher{err: errors.New("broker down")}
	broad := &fakeBroadcaster{}
	svc := NewService(repo, pub, broad)

	n, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err, "persistence succeeded so create must not fail")
	require.NotNil(t, n)
	assert.Equal(t, 1, pub.calls)
	assert.Equal(t, 1, broad.calls, "broadcast should cover the failed publish")
}

func TestCreate_InvalidUserID(t *testing.T) {
	svc := NewService(&stubRepo{}, nil, nil)

	input := validInput()
	input.UserID = "not-a-uuid"
	n, err := svc.Create(context.Background(), input)
	assert.Error(t, err)
	assert.Nil(t, n)
}
