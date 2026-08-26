package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/api/handler"
)

// Mock Token Service for Handler testing
type mockTokenServiceForHandler struct {
	balance      uint64
	stakedAmount uint64
	stakeTxHash  string
	unstakeTx    string
	err          error
}

func (m *mockTokenServiceForHandler) GetBalance(ctx context.Context, address string) (uint64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.balance, nil
}

func (m *mockTokenServiceForHandler) GetStakedAmount(ctx context.Context, address string) (uint64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.stakedAmount, nil
}

func (m *mockTokenServiceForHandler) Stake(ctx context.Context, userID string, passkeySeed []byte, amount uint64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.stakeTxHash, nil
}

func (m *mockTokenServiceForHandler) Unstake(ctx context.Context, userID string, passkeySeed []byte, amount uint64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.unstakeTx, nil
}

func setupTokenTestRouter(svc *mockTokenServiceForHandler, authUserID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if authUserID != "" {
			c.Set("userID", authUserID)
		}
		c.Next()
	})

	h := handler.NewTokenHandler(svc)
	r.GET("/v1/token/balance/:address", h.GetBalance)
	r.GET("/v1/token/stakes/:address", h.GetStakes)
	r.POST("/v1/token/stake", h.Stake)
	r.POST("/v1/token/unstake", h.Unstake)

	return r
}

func TestTokenHandler_GetBalance_Success(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{balance: 5000}
	r := setupTokenTestRouter(mockSvc, "")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/token/balance/GBTESTADDR12345", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp["success"].(bool))
}

func TestTokenHandler_GetBalance_Error(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{err: errors.New("contract RPC failed")}
	r := setupTokenTestRouter(mockSvc, "")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/token/balance/GBTESTADDR12345", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTokenHandler_GetStakes_Success(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{stakedAmount: 1200}
	r := setupTokenTestRouter(mockSvc, "")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/token/stakes/GBTESTADDR12345", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp["success"].(bool))
}

func TestTokenHandler_Stake_Success(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{stakeTxHash: "tx-stake-hash-123"}
	userID := uuid.NewString()
	r := setupTokenTestRouter(mockSvc, userID)

	body := `{"amount":100,"passkeySeed":"my-secret-seed"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/token/stake", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp["success"].(bool))
}

func TestTokenHandler_Stake_InvalidPayload(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{}
	r := setupTokenTestRouter(mockSvc, uuid.NewString())

	// Missing amount / zero amount
	body := `{"amount":0,"passkeySeed":"my-secret-seed"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/token/stake", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTokenHandler_Stake_ServiceError(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{err: errors.New("contract execution failed")}
	r := setupTokenTestRouter(mockSvc, uuid.NewString())

	body := `{"amount":100,"passkeySeed":"my-secret-seed"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/token/stake", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTokenHandler_Unstake_Success(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{unstakeTx: "tx-unstake-hash-456"}
	userID := uuid.NewString()
	r := setupTokenTestRouter(mockSvc, userID)

	body := `{"amount":50,"passkeySeed":"my-secret-seed"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/token/unstake", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp["success"].(bool))
}

func TestTokenHandler_Unstake_InvalidPayload(t *testing.T) {
	mockSvc := &mockTokenServiceForHandler{}
	r := setupTokenTestRouter(mockSvc, uuid.NewString())

	// Missing passkeySeed
	body := `{"amount":50}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/token/unstake", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
