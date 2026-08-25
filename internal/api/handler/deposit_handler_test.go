package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/wallet"
	"github.com/moistello/backend/internal/domain/yellowcard"
)

// fakeWalletService is a minimal wallet.Service returning a fixed primary wallet.
type fakeWalletService struct{}

func (f *fakeWalletService) CreateWallet(ctx context.Context, userID string, passkeySeed []byte) (*wallet.Wallet, error) {
	return &wallet.Wallet{UserID: userID, PublicKey: "GUSERWALLET..."}, nil
}
func (f *fakeWalletService) SignTransaction(ctx context.Context, walletID string, passkeySeed []byte, txnXDR string) (string, error) {
	return "", nil
}
func (f *fakeWalletService) GetWallets(ctx context.Context, userID string) ([]wallet.Wallet, error) {
	return []wallet.Wallet{{UserID: userID, PublicKey: "GUSERWALLET..."}}, nil
}
func (f *fakeWalletService) GetBalance(ctx context.Context, userID string) (*wallet.Balance, error) {
	return &wallet.Balance{}, nil
}
func (f *fakeWalletService) SendPayment(ctx context.Context, userID string, passkeySeed []byte, destination, asset string, amount float64, memo, ipAddress, userAgent string) (string, error) {
	return "", nil
}
func (f *fakeWalletService) DeleteWallet(ctx context.Context, userID, walletID string) error {
	return nil
}

// staticRoundTripper returns a canned JSON response for any request.
type staticRoundTripper struct {
	status int
	body   string
}

func (s *staticRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(s.body)),
		Request:    req,
	}, nil
}

func TestDepositHandler_InitiateWithdraw_ReturnsConfiguredYellowCardAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		configuredYCAddress = "GYCARDREALCONFIGURED123456789"
		sendBody            = `{"sendId":"send-1","status":"pending","fee":1.5,"netAmount":0}`
		quoteBody           = `{"quoteId":"q1","fromCurrency":"USDC","toCurrency":"NGN","fromAmount":100,"toAmount":1550000,"rate":15500,"fee":0,"feePercentage":0,"expiresAt":""}`
	)

	// Mock both the quote and send endpoints with the same canned response shape
	// (the handler ignores unrelated fields).
	rt := &staticRoundTripper{status: http.StatusOK, body: sendBody}
	rtQuote := &staticRoundTripper{status: http.StatusOK, body: quoteBody}

	yc := yellowcard.NewClient("key", "secret", configuredYCAddress)

	// Route quote requests to the quote roundtripper based on path.
	yc.SetHTTPClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/quotes") {
			return rtQuote.RoundTrip(req)
		}
		return rt.RoundTrip(req)
	})})

	h := handler.NewDepositHandler(yc, &fakeWalletService{})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	r.POST("/withdraw", h.InitiateWithdraw)

	body, _ := json.Marshal(map[string]interface{}{
		"amountUsdc":    100.0,
		"bankCode":      "044",
		"accountNumber": "0123456789",
		"accountName":   "John Doe",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/withdraw", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// The response must contain the real configured address, not the placeholder.
	assert.Contains(t, w.Body.String(), configuredYCAddress)
	assert.NotContains(t, w.Body.String(), "GABCDEF123")
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDepositHandler_InitiateWithdraw_MissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	yc := yellowcard.NewClient("key", "secret", "GYCARDREAL")
	h := handler.NewDepositHandler(yc, &fakeWalletService{})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	r.POST("/withdraw", h.InitiateWithdraw)

	body, _ := json.Marshal(map[string]interface{}{"amountUsdc": 100})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/withdraw", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "required")
}