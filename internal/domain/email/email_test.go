package email_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/email"
)

func TestEmailService_DevModeFallbackWhenNoAPIKey(t *testing.T) {
	svc := email.NewService(email.Config{
		APIKey: "",
	})

	err := svc.SendOTP(context.Background(), "user@example.com", "123456")
	assert.NoError(t, err, "empty api key should gracefully fallback in dev mode without error")

	err = svc.SendRecoveryCode(context.Background(), "user@example.com", "12345678")
	assert.NoError(t, err)

	err = svc.SendBackupCodes(context.Background(), "user@example.com", []string{"code1", "code2"})
	assert.NoError(t, err)
}

func TestEmailService_SendOTPSuccess(t *testing.T) {
	var receivedBody map[string]any
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messageId":"<msg123@brevo>"}`))
	}))
	defer server.Close()

	svc := email.NewService(email.Config{
		APIKey:      "test-brevo-key",
		FromAddress: "support@moistello.com",
		FromName:    "Moistello App",
		BaseURL:     server.URL,
	})

	err := svc.SendOTP(context.Background(), "recipient@example.com", "654321")
	require.NoError(t, err)

	assert.Equal(t, "test-brevo-key", receivedHeaders.Get("api-key"))
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))

	sender := receivedBody["sender"].(map[string]any)
	assert.Equal(t, "Moistello App", sender["name"])
	assert.Equal(t, "support@moistello.com", sender["email"])

	to := receivedBody["to"].([]any)
	assert.Len(t, to, 1)
	assert.Equal(t, "recipient@example.com", to[0].(map[string]any)["email"])

	htmlContent := receivedBody["htmlContent"].(string)
	assert.Contains(t, htmlContent, "654321")
}

func TestEmailService_RetryOnTransientServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&attempts, 1)
		if current < 3 {
			// Fail first two attempts with 503 Service Unavailable
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
			return
		}
		// Succeed on 3rd attempt
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messageId":"<ok123>"}`))
	}))
	defer server.Close()

	svc := email.NewService(email.Config{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 3,
	})

	err := svc.SendRecoveryCode(context.Background(), "user@example.com", "87654321")
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

func TestEmailService_PermanentFailureWhenRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal crash"}`))
	}))
	defer server.Close()

	svc := email.NewService(email.Config{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: 2,
	})

	err := svc.SendOTP(context.Background(), "user@example.com", "123456")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "attempts to send email via brevo failed")
}
