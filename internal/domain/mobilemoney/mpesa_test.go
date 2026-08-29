package mobilemoney_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/mobilemoney"
)

func newTestMPesaProvider(t *testing.T, handler http.HandlerFunc) (*mobilemoney.MPesaProvider, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	p := mobilemoney.NewMPesaProvider(mobilemoney.MPesaConfig{
		ConsumerKey:     "key",
		ConsumerSecret:  "secret",
		Shortcode:       "174379",
		Passkey:         "passkey",
		CallbackBaseURL: "https://api.moistello.com",
		Sandbox:         true,
	})
	p.SetBaseURL(srv.URL)
	p.SetHTTPClient(srv.Client())
	return p, srv.Close
}

func TestMPesaProvider_InitiateOnramp_Success(t *testing.T) {
	p, cleanup := newTestMPesaProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v1/generate":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-123", "expires_in": "3599"})
		case "/mpesa/stkpush/v1/processrequest":
			auth := r.Header.Get("Authorization")
			assert.Equal(t, "Bearer tok-123", auth)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"MerchantRequestID":   "m-1",
				"CheckoutRequestID":   "c-1",
				"ResponseCode":        "0",
				"ResponseDescription": "Success",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	defer cleanup()

	result, err := p.InitiateOnramp(t.Context(), mobilemoney.OnrampRequest{
		Amount: 100, PhoneNumber: "254712345678", Reference: "ref-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "c-1", result.ProviderRef)
	assert.Equal(t, mobilemoney.StatusPending, result.Status)
}

func TestMPesaProvider_InitiateOnramp_RejectedByProvider(t *testing.T) {
	p, cleanup := newTestMPesaProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v1/generate":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-123", "expires_in": "3599"})
		case "/mpesa/stkpush/v1/processrequest":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"ResponseCode":        "1",
				"ResponseDescription": "Invalid PhoneNumber",
			})
		}
	})
	defer cleanup()

	_, err := p.InitiateOnramp(t.Context(), mobilemoney.OnrampRequest{Amount: 100, PhoneNumber: "bad"})
	assert.Error(t, err)
}

func TestMPesaProvider_GetStatus_MapsResultCodes(t *testing.T) {
	var resultCode string
	p, cleanup := newTestMPesaProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v1/generate":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-123", "expires_in": "3599"})
		case "/mpesa/stkpushquery/v1/query":
			_ = json.NewEncoder(w).Encode(map[string]string{"ResultCode": resultCode, "ResultDesc": "desc"})
		}
	})
	defer cleanup()

	resultCode = "0"
	res, err := p.GetStatus(t.Context(), "c-1")
	require.NoError(t, err)
	assert.Equal(t, mobilemoney.StatusCompleted, res.Status)

	resultCode = "1032"
	res, err = p.GetStatus(t.Context(), "c-1")
	require.NoError(t, err)
	assert.Equal(t, mobilemoney.StatusFailed, res.Status)
}

func TestMPesaProvider_AccessToken_CachedAcrossCalls(t *testing.T) {
	oauthCalls := 0
	p, cleanup := newTestMPesaProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v1/generate":
			oauthCalls++
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-123", "expires_in": "3599"})
		case "/mpesa/stkpush/v1/processrequest":
			_ = json.NewEncoder(w).Encode(map[string]string{"ResponseCode": "0", "CheckoutRequestID": "c-1"})
		}
	})
	defer cleanup()

	_, err := p.InitiateOnramp(t.Context(), mobilemoney.OnrampRequest{Amount: 100, PhoneNumber: "254712345678"})
	require.NoError(t, err)
	_, err = p.InitiateOnramp(t.Context(), mobilemoney.OnrampRequest{Amount: 200, PhoneNumber: "254712345678"})
	require.NoError(t, err)

	assert.Equal(t, 1, oauthCalls, "the OAuth token should be cached and reused, not fetched on every call")
}
