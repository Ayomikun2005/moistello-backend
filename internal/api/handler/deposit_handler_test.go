package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/moistello/backend/config"
	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/wallet"
	"github.com/moistello/backend/internal/domain/yellowcard"
)

type mockDepositWalletService struct {
	wallets []wallet.Wallet
}

func (m *mockDepositWalletService) CreateWallet(ctx context.Context, userID string, passkeySeed []byte) (*wallet.Wallet, error) {
	return nil, nil
}
func (m *mockDepositWalletService) SignTransaction(ctx context.Context, walletID string, passkeySeed []byte, txnXDR string) (string, error) {
	return "", nil
}
func (m *mockDepositWalletService) GetWallets(ctx context.Context, userID string) ([]wallet.Wallet, error) {
	return m.wallets, nil
}
func (m *mockDepositWalletService) GetBalance(ctx context.Context, userID string) (*wallet.Balance, error) {
	return nil, nil
}
func (m *mockDepositWalletService) SendPayment(ctx context.Context, userID string, passkeySeed []byte, destination, asset string, amount float64, memo, ipAddress, userAgent string) (string, error) {
	return "", nil
}
func (m *mockDepositWalletService) DeleteWallet(ctx context.Context, userID, walletID string) error {
	return nil
}

func setupTestDepositRouter(h *handler.DepositHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-123")
		c.Next()
	})
	r.POST("/v1/wallet/deposit", h.InitiateDeposit)
	r.POST("/v1/wallet/withdraw", h.InitiateWithdraw)
	return r
}

func TestDepositHandler_AmountCaps(t *testing.T) {
	ycClient := yellowcard.NewClient("", "")
	mockWallet := &mockDepositWalletService{
		wallets: []wallet.Wallet{{PublicKey: "GABC12345"}},
	}

	h := handler.NewDepositHandler(ycClient, mockWallet).WithConfig(config.YellowCardConfig{
		MaxDepositNGN:   100_000,
		MaxWithdrawUSDC: 500,
	})

	r := setupTestDepositRouter(h)

	t.Run("deposit exceeding max cap is rejected", func(t *testing.T) {
		reqBody, _ := json.Marshal(map[string]any{
			"amountNgn": 150_000,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/deposit", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "exceeds maximum allowed limit")
	})

	t.Run("withdraw exceeding max cap is rejected", func(t *testing.T) {
		reqBody, _ := json.Marshal(map[string]any{
			"amountUsdc":    600,
			"bankCode":      "044",
			"accountNumber": "0123456789",
			"accountName":   "John Doe",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/withdraw", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "exceeds maximum allowed limit")
	})
}

func TestDepositHandler_DailyCapsAndIdempotency(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skip("Redis is not available, skipping live daily caps & idempotency test")
	}

	ctx := context.Background()
	ycClient := yellowcard.NewClient("", "")
	mockWallet := &mockDepositWalletService{
		wallets: []wallet.Wallet{{PublicKey: "GABC12345"}},
	}

	h := handler.NewDepositHandler(ycClient, mockWallet).
		WithRedis(rdb).
		WithConfig(config.YellowCardConfig{
			MaxDepositNGN:      500_000,
			DailyDepositCapNGN: 100_000,
		})

	r := setupTestDepositRouter(h)

	// Pre-fill daily usage to 90,000 NGN
	todayKey := "yc:daily:deposit:test-user-123:" + "2006-01-02"
	rdb.Set(ctx, todayKey, 90_000, 0)
	defer rdb.Del(ctx, todayKey)

	// Requesting 20,000 should exceed 100,000 daily cap
	reqBody, _ := json.Marshal(map[string]any{
		"amountNgn": 20_000,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/wallet/deposit", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Verify daily limit error
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "exceeds daily limit")
}
