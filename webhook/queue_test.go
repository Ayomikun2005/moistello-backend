package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingPublisher struct {
	exchange   string
	routingKey string
	bodies     [][]byte
	err        error
}

func (p *recordingPublisher) Publish(exchange, routingKey string, body []byte) error {
	if p.err != nil {
		return p.err
	}
	p.exchange = exchange
	p.routingKey = routingKey
	p.bodies = append(p.bodies, body)
	return nil
}

// listRepo returns every registration from DispatchPayload.
type listRepo struct {
	webhooks []WebhookRegistration
	err      error
}

func (r *listRepo) Register(ctx context.Context, wh *WebhookRegistration) error { return nil }
func (r *listRepo) GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error) {
	return nil, nil
}
func (r *listRepo) GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error) {
	return r.webhooks, r.err
}
func (r *listRepo) GetByID(ctx context.Context, id string) (*WebhookRegistration, error) {
	return nil, nil
}
func (r *listRepo) Delete(ctx context.Context, id string) error { return nil }
func (r *listRepo) ListDeliveries(ctx context.Context, webhookID string, page, limit int) ([]DeliveryLog, int, error) {
	return nil, 0, nil
}

func TestQueuedDispatcher_PublishesEnvelopePerRegistration(t *testing.T) {
	repo := &listRepo{webhooks: []WebhookRegistration{
		{ID: "wh-1", UserID: "user-1", TargetURL: "https://example.test/hook1", Secret: "s3cret"},
		{ID: "wh-2", UserID: "user-2", TargetURL: "https://example.test/hook2", Secret: "topsecret"},
	}}

	pub := &recordingPublisher{}
	d := NewQueuedDispatcher(repo, pub, "moistello.events")

	payload := map[string]string{"event": "contribution.recorded"}
	require.NoError(t, d.DispatchPayload(context.Background(), payload, "req-42", 5))

	assert.Len(t, pub.bodies, 2)
	assert.Equal(t, "moistello.events", pub.exchange)
	assert.Equal(t, WebhookRoutingKey, pub.routingKey)

	var urls []string
	for _, body := range pub.bodies {
		var msg Message
		require.NoError(t, json.Unmarshal(body, &msg))
		assert.JSONEq(t, `{"event":"contribution.recorded"}`, string(msg.Payload))
		assert.Equal(t, "req-42", msg.RequestID)
		assert.Equal(t, 5, msg.MaxRetries)
		urls = append(urls, msg.TargetURL)
	}
	assert.ElementsMatch(t, []string{"https://example.test/hook1", "https://example.test/hook2"}, urls)
}

func TestQueuedDispatcher_NoRegistrations(t *testing.T) {
	repo := &listRepo{}
	pub := &recordingPublisher{}
	d := NewQueuedDispatcher(repo, pub, "moistello.events")

	require.NoError(t, d.DispatchPayload(context.Background(), map[string]string{}, "", 3))
	assert.Empty(t, pub.bodies)
}

func TestMessageDeliver_SignsAndPosts(t *testing.T) {
	const secret = "hmac-secret"
	payload := []byte(`{"event":"test"}`)

	var gotSig, gotReqID string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Signature")
		gotReqID = r.Header.Get("X-Request-ID")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	msg := Message{
		RegistrationID: "wh-1",
		TargetURL:      srv.URL,
		Secret:         secret,
		Payload:        payload,
		RequestID:      "req-7",
		MaxRetries:     1,
	}

	require.NoError(t, msg.Deliver(context.Background(), srv.Client()))
	assert.Equal(t, SignWebhookPayload(payload, secret), gotSig)
	assert.Equal(t, "req-7", gotReqID)
	assert.JSONEq(t, string(payload), string(gotBody))
}

func TestMessageDeliver_RetriesUntilSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	msg := Message{TargetURL: srv.URL, Secret: "s", Payload: []byte("{}"), MaxRetries: 3}
	start := time.Now()
	require.NoError(t, msg.Deliver(context.Background(), srv.Client()))
	assert.Equal(t, int32(3), attempts.Load())
	assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond, "expected exponential backoff between attempts")
}

func TestMessageDeliver_ExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	msg := Message{TargetURL: srv.URL, Secret: "s", Payload: []byte("{}"), MaxRetries: 2}
	err := msg.Deliver(context.Background(), srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 2 attempts")
	assert.Equal(t, int32(2), attempts.Load())
}
