package webhook

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
	webhooks map[string]*WebhookRegistration
}

func (f *fakeWebhookRepo) Register(ctx context.Context, wh *WebhookRegistration) error {
	f.webhooks[wh.ID] = wh
	return nil
}

func (f *fakeWebhookRepo) GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error) {
	return nil, nil
}

func (f *fakeWebhookRepo) GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error) {
	return nil, nil
}

func (f *fakeWebhookRepo) GetByID(ctx context.Context, id string) (*WebhookRegistration, error) {
	if wh, ok := f.webhooks[id]; ok {
		return wh, nil
	}
	return nil, nil
}

func TestIncomingWebhookHandler_ReceiveWebhook(t *testing.T) {
	repo := &fakeWebhookRepo{webhooks: make(map[string]*WebhookRegistration)}
	secret := "test-secret"
	repo.Register(context.Background(), &WebhookRegistration{
		ID:        "wh-123",
		UserID:    "user-1",
		TargetURL: "https://example.com",
		Secret:    secret,
	})

	handler := NewIncomingWebhookHandler(repo)

	t.Run("valid signature returns 200", func(t *testing.T) {
		payload := []byte(`{"event":"test"}`)
		sig := SignWebhookPayload(payload, secret)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/webhooks/incoming/wh-123", bytes.NewReader(payload))
		req.Header.Set("X-Moistello-Signature", sig)
		req.Header.Set("Content-Type", "application/json")

		c := setupGinWithParam(w, req, "id", "wh-123")
		handler.ReceiveWebhook(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"received":true`)
	})

	t.Run("missing signature returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/webhooks/incoming/wh-123", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")

		c := setupGinWithParam(w, req, "id", "wh-123")
		handler.ReceiveWebhook(c)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/webhooks/incoming/wh-123", strings.NewReader("{}"))
		req.Header.Set("X-Moistello-Signature", "invalid")
		req.Header.Set("Content-Type", "application/json")

		c := setupGinWithParam(w, req, "id", "wh-123")
		handler.ReceiveWebhook(c)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown webhook returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/webhooks/incoming/unknown", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")

		c := setupGinWithParam(w, req, "id", "unknown")
		handler.ReceiveWebhook(c)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupGinWithParam(w *httptest.ResponseRecorder, req *http.Request, paramKey, paramValue string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: paramKey, Value: paramValue}}
	return c
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