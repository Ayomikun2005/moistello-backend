package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/wallet"
)

func TestWalletHandler_Withdraw_WithoutPasskeySeed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := new(mockWalletService)
	svc.On(
		"SendPayment",
		mock.Anything,
		"user-123",
		mock.AnythingOfType("[]uint8"), // empty seed when omitted
		"GDEST...",
		"XLM",
		float64(10.5),
		"",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
	).Return("txhash-wallet", nil)

	h := handler.NewWalletHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", "user-123")
		c.Next()
	})
	r.POST("/v1/wallets/withdraw", h.Withdraw)

	// No passkeySeed in the body — must not be rejected.
	body, _ := json.Marshal(map[string]any{
		"destination": "GDEST...",
		"asset":       "XLM",
		"amount":      10.5,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/wallets/withdraw", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "txhash-wallet")
	assert.NotContains(t, w.Body.String(), "passkeySeed")
	svc.AssertExpectations(t)
}

var _ wallet.Service = (*mockWalletService)(nil)
