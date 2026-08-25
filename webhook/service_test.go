package webhook

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignWebhookPayload(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	secret := "my-secret"

	sig1 := SignWebhookPayload(payload, secret)
	sig2 := SignWebhookPayload(payload, secret)

	assert.Equal(t, sig1, sig2, "same payload and secret should produce identical signatures")
	assert.NotEmpty(t, sig1)
	assert.Len(t, sig1, 64, "SHA-256 hex digest should be 64 chars")
}

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	payload := []byte(`{"event":"payment.completed"}`)
	secret := "super-secret-key"
	signature := SignWebhookPayload(payload, secret)

	assert.True(t, VerifyWebhookSignature(payload, signature, secret))
}

func TestVerifyWebhookSignature_Tampered(t *testing.T) {
	payload := []byte(`{"event":"payment.completed"}`)
	secret := "super-secret-key"
	signature := SignWebhookPayload(payload, secret)

	tampered := "deadbeef" + signature[8:]
	assert.False(t, VerifyWebhookSignature(payload, tampered, secret))
}

func TestVerifyWebhookSignature_WrongSecret(t *testing.T) {
	payload := []byte(`{"event":"payment.completed"}`)
	signature := SignWebhookPayload(payload, "correct-secret")

	assert.False(t, VerifyWebhookSignature(payload, signature, "wrong-secret"))
}

func TestVerifyWebhookSignature_MissingSignature(t *testing.T) {
	payload := []byte(`{"event":"payment.completed"}`)
	secret := "super-secret-key"

	assert.False(t, VerifyWebhookSignature(payload, "", secret))
	assert.False(t, VerifyWebhookSignature(payload, "   ", secret))
}

func TestConstantTimeCompare(t *testing.T) {
	a := []byte("hello")
	b := []byte("hello")
	c := []byte("world")

	assert.True(t, constantTimeCompare(a, b))
	assert.False(t, constantTimeCompare(a, c))
	assert.False(t, constantTimeCompare(a, []byte("hi")))
}

func TestVerifySignature(t *testing.T) {
	hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	assert.True(t, VerifySignature(hash, hash))
	assert.False(t, VerifySignature(hash, "00000000000000000000000000000000000000000000000000000000000000"))
	assert.False(t, VerifySignature("not-hex", hash))
	assert.False(t, VerifySignature(hash, "not-hex"))
}

type fakeWebhookRepo struct {
	mu         sync.Mutex
	webhooks   map[string]*WebhookRegistration
	active     []WebhookRegistration
	deliveries []DeliveryLog
	outcomes   map[string]bool
}

func (f *fakeWebhookRepo) Register(ctx context.Context, wh *WebhookRegistration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhooks[wh.ID] = wh
	return nil
}

func (f *fakeWebhookRepo) GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error) {
	return nil, nil
}

func (f *fakeWebhookRepo) GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active, nil
}

func (f *fakeWebhookRepo) GetByID(ctx context.Context, id string) (*WebhookRegistration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if wh, ok := f.webhooks[id]; ok {
		return wh, nil
	}
	return nil, nil
}

func (f *fakeWebhookRepo) Delete(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.webhooks, id)
	return nil
}

func (f *fakeWebhookRepo) UpdateDeliveryOutcome(ctx context.Context, id string, success bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.outcomes == nil {
		f.outcomes = map[string]bool{}
	}
	f.outcomes[id] = success
	return nil
}

func (f *fakeWebhookRepo) LogDelivery(ctx context.Context, d *DeliveryLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveries = append(f.deliveries, *d)
	return nil
}

func (f *fakeWebhookRepo) ListDeliveries(ctx context.Context, webhookID string, page, limit int) ([]DeliveryLog, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deliveries, len(f.deliveries), nil
}

// snapshotDeliveries returns a copy of the recorded delivery logs for safe
// concurrent reads from the test goroutine.
func (f *fakeWebhookRepo) snapshotDeliveries() []DeliveryLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]DeliveryLog(nil), f.deliveries...)
}

// deliveryCount returns the number of recorded delivery logs.
func (f *fakeWebhookRepo) deliveryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deliveries)
}

// outcome returns the recorded delivery outcome for a webhook ID.
func (f *fakeWebhookRepo) outcome(id string) (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.outcomes[id]
	return v, ok
}

func TestConstantTimeCompareTiming(t *testing.T) {
	a := []byte("same-length-string")
	b := []byte("same-length-string")
	c := []byte("different-str")

	iterations := 1000
	var sameTime, diffTime time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		constantTimeCompare(a, b)
		sameTime += time.Since(start)
	}

	for i := 0; i < iterations; i++ {
		start := time.Now()
		constantTimeCompare(a, c)
		diffTime += time.Since(start)
	}

	avgSame := sameTime / time.Duration(iterations)
	avgDiff := diffTime / time.Duration(iterations)

	ratio := float64(avgDiff) / float64(avgSame)
	assert.Greater(t, ratio, 0.5, "different-length comparison should not be consistently faster")
	assert.Less(t, ratio, 2.0, "different-length comparison should not be consistently slower")
	_ = subtle.ConstantTimeCompare(a, b)
}

func ExampleSignWebhookPayload() {
	payload := []byte(`{"event":"payment.completed","amount":100}`)
	secret := "whsec_1234567890"
	signature := SignWebhookPayload(payload, secret)
	println(len(signature))
}

func ExampleVerifyWebhookSignature() {
	payload := []byte(`{"event":"payment.completed","amount":100}`)
	secret := "whsec_1234567890"
	signature := SignWebhookPayload(payload, secret)
	println(VerifyWebhookSignature(payload, signature, secret))
}

func TestSubscribesTo(t *testing.T) {
	wh := &WebhookRegistration{Events: []string{"circle.created", "payout.executed"}}
	assert.True(t, wh.SubscribesTo("circle.created"))
	assert.False(t, wh.SubscribesTo("circle.closed"))

	allEvents := &WebhookRegistration{Events: nil}
	assert.True(t, allEvents.SubscribesTo("anything.at.all"))
}

func TestDispatcher_FiltersByEventType(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &fakeWebhookRepo{
		active: []WebhookRegistration{
			{ID: "wh-1", TargetURL: server.URL, Events: []string{"circle.created"}, IsActive: true},
			{ID: "wh-2", TargetURL: server.URL, Events: []string{"payout.executed"}, IsActive: true},
		},
	}

	d := NewDispatcher(repo)
	err := d.DispatchPayload(context.Background(), "circle.created", map[string]any{"id": "c1"}, 1)
	require.NoError(t, err)

	// Dispatch is asynchronous; give the goroutines a moment to finish.
	require.Eventually(t, func() bool { return received.Load() == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(1), received.Load())

	// The filtered-out webhook must have a 'skipped' delivery log entry.
	require.Eventually(t, func() bool { return repo.deliveryCount() == 2 }, time.Second, 10*time.Millisecond)
	deliveries := repo.snapshotDeliveries()
	var skipped *DeliveryLog
	for i := range deliveries {
		if deliveries[i].Status == DeliveryStatusSkipped {
			skipped = &deliveries[i]
		}
	}
	require.NotNil(t, skipped, "expected a skipped delivery log for the non-subscribed webhook")
	assert.Equal(t, "wh-2", skipped.WebhookID)
	assert.Equal(t, "circle.created", skipped.EventType)
}

func TestDispatcher_EmptySubscriptionReceivesAllEvents(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &fakeWebhookRepo{
		active: []WebhookRegistration{
			{ID: "wh-1", TargetURL: server.URL, Events: nil, IsActive: true},
		},
	}

	d := NewDispatcher(repo)
	require.NoError(t, d.DispatchPayload(context.Background(), "some.random.event", map[string]any{}, 1))
	require.Eventually(t, func() bool { return received.Load() == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(1), received.Load())
	require.Eventually(t, func() bool { return repo.deliveryCount() == 1 }, time.Second, 10*time.Millisecond)
	deliveries := repo.snapshotDeliveries()
	assert.Equal(t, DeliveryStatusDelivered, deliveries[0].Status)
}

func TestDispatcher_RetriesUntilSuccessAndLogsAttempts(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &fakeWebhookRepo{
		active: []WebhookRegistration{
			{ID: "wh-1", TargetURL: server.URL, Events: []string{"circle.created"}, IsActive: true},
		},
	}

	d := NewDispatcher(repo)
	require.NoError(t, d.DispatchPayload(context.Background(), "circle.created", map[string]any{}, 5))
	require.Eventually(t, func() bool { return attempts.Load() == 3 }, 3*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return repo.deliveryCount() == 1 }, time.Second, 10*time.Millisecond)

	deliveries := repo.snapshotDeliveries()
	assert.Equal(t, DeliveryStatusDelivered, deliveries[0].Status)
	assert.Equal(t, 3, deliveries[0].Attempt)
	assert.Equal(t, http.StatusOK, deliveries[0].StatusCode)
	success, ok := repo.outcome("wh-1")
	assert.True(t, ok)
	assert.True(t, success)
}

func TestDispatcher_ExhaustsRetriesAndLogsFailure(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	repo := &fakeWebhookRepo{
		active: []WebhookRegistration{
			{ID: "wh-1", TargetURL: server.URL, Events: []string{"circle.created"}, IsActive: true},
		},
	}

	d := NewDispatcher(repo)
	require.NoError(t, d.DispatchPayload(context.Background(), "circle.created", map[string]any{}, 2))
	require.Eventually(t, func() bool { return attempts.Load() == 2 }, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return repo.deliveryCount() == 1 }, time.Second, 10*time.Millisecond)

	deliveries := repo.snapshotDeliveries()
	assert.Equal(t, DeliveryStatusFailed, deliveries[0].Status)
	assert.Equal(t, 2, deliveries[0].Attempt)
	assert.Contains(t, deliveries[0].Error, "502")
	success, ok := repo.outcome("wh-1")
	assert.True(t, ok)
	assert.False(t, success)
}

func TestDispatcher_NetworkErrorIsRecorded(t *testing.T) {
	repo := &fakeWebhookRepo{
		active: []WebhookRegistration{
			// Point at a closed port; connection fails immediately.
			{ID: "wh-1", TargetURL: fmt.Sprintf("http://127.0.0.1:%d", nextFreePort()), Events: []string{"circle.created"}, IsActive: true},
		},
	}

	d := NewDispatcher(repo)
	require.NoError(t, d.DispatchPayload(context.Background(), "circle.created", map[string]any{}, 1))
	require.Eventually(t, func() bool { return repo.deliveryCount() == 1 }, time.Second, 10*time.Millisecond)
	deliveries := repo.snapshotDeliveries()
	assert.Equal(t, DeliveryStatusFailed, deliveries[0].Status)
	assert.NotEmpty(t, deliveries[0].Error)
}

// nextFreePort returns a port that is very likely closed so HTTP requests to
// it fail fast with a connection error.
func nextFreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 1
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}
