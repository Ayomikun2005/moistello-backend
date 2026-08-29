package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/auth"
	"github.com/moistello/backend/internal/domain/user"
	userMocks "github.com/moistello/backend/internal/domain/user/mocks"
	"github.com/moistello/backend/internal/domain/verification"
	"github.com/moistello/backend/internal/domain/wallet"
	"github.com/moistello/backend/pkg/apperrors"
	"github.com/moistello/backend/pkg/validator"
)

func init() {
	validator.Init()
}

// memoryRedisHook serves GET/SET/DEL/EXISTS from an in-memory map so handler
// tests can exercise the verification service without a live Redis.
type memoryRedisHook struct {
	mu    sync.Mutex
	store map[string]string
}

func (h *memoryRedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *memoryRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *memoryRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.mu.Lock()
		defer h.mu.Unlock()

		args := cmd.Args()
		switch cmd.Name() {
		case "get":
			key := args[1].(string)
			if val, ok := h.store[key]; ok {
				cmd.(*redis.StringCmd).SetVal(val)
				return nil
			}
			return redis.Nil
		case "set":
			key := args[1].(string)
			switch v := args[2].(type) {
			case string:
				h.store[key] = v
			case []byte:
				h.store[key] = string(v)
			}
			return nil
		case "del":
			deleted := 0
			for _, a := range args[1:] {
				if _, ok := h.store[a.(string)]; ok {
					delete(h.store, a.(string))
					deleted++
				}
			}
			cmd.(*redis.IntCmd).SetVal(int64(deleted))
			return nil
		case "exists":
			count := 0
			for _, a := range args[1:] {
				if _, ok := h.store[a.(string)]; ok {
					count++
				}
			}
			cmd.(*redis.IntCmd).SetVal(int64(count))
			return nil
		default:
			return next(ctx, cmd)
		}
	}
}

type registerTestEnv struct {
	mockAuthSvc     *mockAuthService
	mockUserRepo    *userMocks.Repository
	userSvc         user.Service
	handler         *handler.AuthHandler
	verificationSvc *verification.Service
	sentOTP         string
	wallet          *mockWalletService
}

func (e *registerTestEnv) walletMock() *mockWalletService { return e.wallet }

func newRegisterEnv(t *testing.T) *registerTestEnv {
	t.Helper()
	mockAuthSvc := new(mockAuthService)
	mockUserRepo := new(userMocks.Repository)
	userSvc := user.NewService(mockUserRepo, nil)
	wallet := new(mockWalletService)

	rdb := redis.NewClient(&redis.Options{Addr: "memory:6379"})
	rdb.AddHook(&memoryRedisHook{store: make(map[string]string)})
	t.Cleanup(func() { _ = rdb.Close() })

	verificationSvc := verification.NewService(rdb)
	env := &registerTestEnv{
		mockAuthSvc:     mockAuthSvc,
		mockUserRepo:    mockUserRepo,
		userSvc:         userSvc,
		verificationSvc: verificationSvc,
		wallet:          wallet,
	}
	verificationSvc.WithEmailSender(func(email, code string) error {
		env.sentOTP = code
		return nil
	}, nil)

	// Seed derivation lives in the wallet domain service (#166); the mock
	// returns a fixed deterministic seed so the register flows stay focused
	// on handler behaviour.
	wallet.On("DeriveWalletSeed", mock.Anything, mock.AnythingOfType("string")).Return(testWalletSeed, nil)
	env.handler = handler.NewAuthHandler(mockAuthSvc, userSvc, wallet, nil, verificationSvc, nil, nil, mockUserRepo)
	return env
}

const testEmail = "user@example.com"

func emailWalletAddr(email string) string {
	emailHash := sha256.Sum256([]byte(email))
	return "EMAIL:" + hex.EncodeToString(emailHash[:16])
}

type mockWalletService struct{ mock.Mock }

const testWalletSeed = "7b0ed4e5f2a6c9d18b3a5f7e2c4d6a8091f3b5d7e9a1c3f5b7d9e1f3a5b7c9d1e"

func (m *mockWalletService) DeriveWalletSeed(ctx context.Context, email string) (string, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return "", args.Error(1)
	}
	return args.Get(0).(string), args.Error(1)
}

func (m *mockWalletService) CreateWallet(ctx context.Context, userID string, passkeySeed []byte) (*wallet.Wallet, error) {
	args := m.Called(ctx, userID, passkeySeed)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*wallet.Wallet), args.Error(1)
}

func (m *mockWalletService) SignTransaction(ctx context.Context, walletID string, passkeySeed []byte, txnXDR string) (string, error) {
	args := m.Called(ctx, walletID, passkeySeed, txnXDR)
	return args.String(0), args.Error(1)
}

func (m *mockWalletService) GetWallets(ctx context.Context, userID string) ([]wallet.Wallet, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]wallet.Wallet), args.Error(1)
}

func (m *mockWalletService) GetBalance(ctx context.Context, userID string) (*wallet.Balance, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*wallet.Balance), args.Error(1)
}

func (m *mockWalletService) SendPayment(ctx context.Context, userID string, passkeySeed []byte, destination, asset string, amount float64, memo, ipAddress, userAgent string) (string, error) {
	args := m.Called(ctx, userID, passkeySeed, destination, asset, amount, memo, ipAddress, userAgent)
	return args.String(0), args.Error(1)
}

func (m *mockWalletService) DeleteWallet(ctx context.Context, userID, walletID string) error {
	return m.Called(ctx, userID, walletID).Error(0)
}

func TestAuthHandler_Register_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newRegisterEnv(t)

	env.mockUserRepo.On("FindByWalletAddress", mock.Anything, emailWalletAddr(testEmail)).Return(nil, apperrors.ErrNotFound)

	r := gin.New()
	r.POST("/v1/auth/register", env.handler.Register)

	body, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": "SuperSecret123!",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)
	assert.Contains(t, w.Body.String(), "verification code sent")
	assert.NotContains(t, w.Body.String(), "walletSeed")
	assert.NotContains(t, w.Body.String(), "seed")
	assert.NotEmpty(t, env.sentOTP, "OTP email should have been sent")
	env.mockUserRepo.AssertExpectations(t)
}

func TestAuthHandler_Register_DoesNotExposeWalletSeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newRegisterEnv(t)

	env.mockUserRepo.On("FindByWalletAddress", mock.Anything, emailWalletAddr("sec-test@example.com")).Return(nil, apperrors.ErrNotFound)

	r := gin.New()
	r.POST("/v1/auth/register", env.handler.Register)

	body, _ := json.Marshal(map[string]string{
		"email":    "sec-test@example.com",
		"password": "SecurePassword123!",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	data, ok := resp["data"].(map[string]any)
	assert.True(t, ok)
	assert.Nil(t, data["walletSeed"], "walletSeed must not be present in response data")
	assert.Nil(t, data["seed"], "seed must not be present in response data")
}

func TestAuthHandler_Register_MissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newRegisterEnv(t)

	r := gin.New()
	r.POST("/v1/auth/register", env.handler.Register)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/register", bytes.NewBufferString(`{"email":`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestAuthHandler_Register_ExistingUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newRegisterEnv(t)

	existingUser := &user.User{
		ID:            uuid.New(),
		WalletAddress: emailWalletAddr(testEmail),
		Role:          user.RoleUser,
	}
	env.mockUserRepo.On("FindByWalletAddress", mock.Anything, emailWalletAddr(testEmail)).Return(existingUser, nil)

	r := gin.New()
	r.POST("/v1/auth/register", env.handler.Register)

	body, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": "SuperSecret123!",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 409, w.Code)
	env.mockUserRepo.AssertExpectations(t)
}

func TestAuthHandler_RegisterVerify_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newRegisterEnv(t)

	env.mockUserRepo.On("FindByWalletAddress", mock.Anything, emailWalletAddr(testEmail)).Return(nil, apperrors.ErrNotFound)

	r := gin.New()
	r.POST("/v1/auth/register", env.handler.Register)
	r.POST("/v1/auth/register/verify", env.handler.RegisterVerify)

	body, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": "SuperSecret123!",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, 201, w.Code)
	assert.Len(t, env.sentOTP, 6)

	env.mockUserRepo.On("Create", mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)
	env.mockUserRepo.On("FindByWalletAddress", mock.Anything, emailWalletAddr(testEmail)).Return(nil, apperrors.ErrNotFound)
	env.mockAuthSvc.On("CreateSession", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.Anything, mock.Anything, mock.Anything).Return(
		&auth.TokenPair{AccessToken: "jwt-token", RefreshToken: "refresh-token", CSRFToken: "csrf-token"}, nil,
	)
	env.walletMock().On("CreateWallet", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil, nil)

	verifyBody, _ := json.Marshal(map[string]string{
		"email": testEmail,
		"code":  env.sentOTP,
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/v1/auth/register/verify", bytes.NewBuffer(verifyBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	assert.Equal(t, 201, w2.Code)
	assert.Contains(t, w2.Body.String(), "jwt-token")
	env.mockAuthSvc.AssertExpectations(t)
	env.mockUserRepo.AssertExpectations(t)
}

func TestAuthHandler_RegisterVerify_BadCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newRegisterEnv(t)

	env.mockUserRepo.On("FindByWalletAddress", mock.Anything, emailWalletAddr(testEmail)).Return(nil, apperrors.ErrNotFound)

	r := gin.New()
	r.POST("/v1/auth/register", env.handler.Register)
	r.POST("/v1/auth/register/verify", env.handler.RegisterVerify)

	body, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": "SuperSecret123!",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, 201, w.Code)

	verifyBody, _ := json.Marshal(map[string]string{
		"email": testEmail,
		"code":  "000000",
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/v1/auth/register/verify", bytes.NewBuffer(verifyBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	assert.Equal(t, 400, w2.Code)
}
