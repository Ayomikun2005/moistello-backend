package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/deposit"
	"github.com/moistello/backend/internal/domain/withdrawal"
	"github.com/moistello/backend/webhook"
)

const testWebhookSecret = "test-webhook-secret"

// fakeDepositRepo is an in-memory deposit.Repository for handler tests.
type fakeDepositRepo struct {
	byPaymentRef map[string]*deposit.Deposit
}

func newFakeDepositRepo() *fakeDepositRepo {
	return &fakeDepositRepo{byPaymentRef: map[string]*deposit.Deposit{}}
}

func (f *fakeDepositRepo) Create(ctx context.Context, d *deposit.Deposit) error {
	f.byPaymentRef[d.PaymentRef] = d
	return nil
}
func (f *fakeDepositRepo) GetByID(ctx context.Context, id string) (*deposit.Deposit, error) {
	for _, d := range f.byPaymentRef {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, assert.AnError
}
func (f *fakeDepositRepo) GetByReceiveID(ctx context.Context, receiveID string) (*deposit.Deposit, error) {
	for _, d := range f.byPaymentRef {
		if d.ReceiveID == receiveID {
			return d, nil
		}
	}
	return nil, assert.AnError
}
func (f *fakeDepositRepo) GetByPaymentRef(ctx context.Context, paymentRef string) (*deposit.Deposit, error) {
	d, ok := f.byPaymentRef[paymentRef]
	if !ok {
		return nil, assert.AnError
	}
	return d, nil
}
func (f *fakeDepositRepo) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]deposit.Deposit, error) {
	return nil, nil
}
func (f *fakeDepositRepo) UpdateStatus(ctx context.Context, id string, status deposit.DepositStatus) error {
	for _, d := range f.byPaymentRef {
		if d.ID == id {
			d.Status = status
			return nil
		}
	}
	return assert.AnError
}
func (f *fakeDepositRepo) MarkCompleted(ctx context.Context, id string, completedAt time.Time) error {
	for _, d := range f.byPaymentRef {
		if d.ID == id {
			d.Status = deposit.DepositStatusCompleted
			d.CompletedAt = &completedAt
			return nil
		}
	}
	return assert.AnError
}
func (f *fakeDepositRepo) MarkFailed(ctx context.Context, id string, reason string) error {
	for _, d := range f.byPaymentRef {
		if d.ID == id {
			d.Status = deposit.DepositStatusFailed
			d.FailureReason = &reason
			return nil
		}
	}
	return assert.AnError
}

// fakeWithdrawalRepo is an in-memory withdrawal.Repository for handler tests.
type fakeWithdrawalRepo struct {
	byPaymentRef map[string]*withdrawal.Withdrawal
}

func newFakeWithdrawalRepo() *fakeWithdrawalRepo {
	return &fakeWithdrawalRepo{byPaymentRef: map[string]*withdrawal.Withdrawal{}}
}

func (f *fakeWithdrawalRepo) Create(ctx context.Context, w *withdrawal.Withdrawal) error {
	f.byPaymentRef[w.PaymentRef] = w
	return nil
}
func (f *fakeWithdrawalRepo) GetByID(ctx context.Context, id string) (*withdrawal.Withdrawal, error) {
	for _, w := range f.byPaymentRef {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, assert.AnError
}
func (f *fakeWithdrawalRepo) GetByPaymentRef(ctx context.Context, paymentRef string) (*withdrawal.Withdrawal, error) {
	w, ok := f.byPaymentRef[paymentRef]
	if !ok {
		return nil, assert.AnError
	}
	return w, nil
}
func (f *fakeWithdrawalRepo) GetByYellowCardTxID(ctx context.Context, txID string) (*withdrawal.Withdrawal, error) {
	for _, w := range f.byPaymentRef {
		if w.YellowCardTxID != nil && *w.YellowCardTxID == txID {
			return w, nil
		}
	}
	return nil, assert.AnError
}
func (f *fakeWithdrawalRepo) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]withdrawal.Withdrawal, error) {
	return nil, nil
}
func (f *fakeWithdrawalRepo) UpdateStatus(ctx context.Context, id string, status withdrawal.WithdrawalStatus) error {
	for _, w := range f.byPaymentRef {
		if w.ID == id {
			w.Status = status
			return nil
		}
	}
	return assert.AnError
}
func (f *fakeWithdrawalRepo) UpdateUSDCTxHash(ctx context.Context, id string, txHash string, receivedAt time.Time) error {
	return nil
}
func (f *fakeWithdrawalRepo) UpdateYellowCardTxID(ctx context.Context, id string, txID string) error {
	for _, w := range f.byPaymentRef {
		if w.ID == id {
			w.YellowCardTxID = &txID
			return nil
		}
	}
	return assert.AnError
}
func (f *fakeWithdrawalRepo) MarkCompleted(ctx context.Context, id string, completedAt time.Time) error {
	for _, w := range f.byPaymentRef {
		if w.ID == id {
			w.Status = withdrawal.WithdrawalStatusCompleted
			w.CompletedAt = &completedAt
			return nil
		}
	}
	return assert.AnError
}
func (f *fakeWithdrawalRepo) MarkFailed(ctx context.Context, id string, reason string) error {
	for _, w := range f.byPaymentRef {
		if w.ID == id {
			w.Status = withdrawal.WithdrawalStatusFailed
			w.FailureReason = &reason
			return nil
		}
	}
	return assert.AnError
}

func setupTestWebhookRouter(h *handler.YellowCardWebhookHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/yellowcard", h.HandleWebhook)
	return r
}

func postSignedWebhook(t *testing.T, r *gin.Engine, payload []byte, secret string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/webhooks/yellowcard", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-YC-Signature", webhook.SignWebhookPayload(payload, secret))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestYellowCardWebhook_InvalidSignature_Rejected(t *testing.T) {
	deposits := newFakeDepositRepo()
	withdrawals := newFakeWithdrawalRepo()
	h := handler.NewYellowCardWebhookHandler(deposits, withdrawals, testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "receive", "status": "completed", "paymentReference": "MOIST-1",
	})

	// Signed with the wrong secret.
	w := postSignedWebhook(t, r, payload, "wrong-secret")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestYellowCardWebhook_MissingSignature_Rejected(t *testing.T) {
	h := handler.NewYellowCardWebhookHandler(newFakeDepositRepo(), newFakeWithdrawalRepo(), testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "receive", "status": "completed", "paymentReference": "MOIST-1",
	})

	w := postSignedWebhook(t, r, payload, "") // no signature header

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestYellowCardWebhook_NoWebhookSecretConfigured_Rejected(t *testing.T) {
	h := handler.NewYellowCardWebhookHandler(newFakeDepositRepo(), newFakeWithdrawalRepo(), "")
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "receive", "status": "completed", "paymentReference": "MOIST-1",
	})

	w := postSignedWebhook(t, r, payload, testWebhookSecret)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestYellowCardWebhook_DepositCompleted_Reconciles(t *testing.T) {
	deposits := newFakeDepositRepo()
	deposits.byPaymentRef["MOIST-1"] = &deposit.Deposit{
		ID: "dep-1", PaymentRef: "MOIST-1", Status: deposit.DepositStatusPending,
	}
	h := handler.NewYellowCardWebhookHandler(deposits, newFakeWithdrawalRepo(), testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "receive", "status": "completed", "paymentReference": "MOIST-1", "id": "r-1",
	})

	w := postSignedWebhook(t, r, payload, testWebhookSecret)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, deposit.DepositStatusCompleted, deposits.byPaymentRef["MOIST-1"].Status)
	assert.NotNil(t, deposits.byPaymentRef["MOIST-1"].CompletedAt)
}

func TestYellowCardWebhook_DepositFailed_RecordsReason(t *testing.T) {
	deposits := newFakeDepositRepo()
	deposits.byPaymentRef["MOIST-2"] = &deposit.Deposit{
		ID: "dep-2", PaymentRef: "MOIST-2", Status: deposit.DepositStatusPending,
	}
	h := handler.NewYellowCardWebhookHandler(deposits, newFakeWithdrawalRepo(), testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "receive", "status": "failed", "paymentReference": "MOIST-2",
		"failureReason": "bank transfer rejected",
	})

	w := postSignedWebhook(t, r, payload, testWebhookSecret)

	assert.Equal(t, http.StatusOK, w.Code)
	got := deposits.byPaymentRef["MOIST-2"]
	assert.Equal(t, deposit.DepositStatusFailed, got.Status)
	require.NotNil(t, got.FailureReason)
	assert.Equal(t, "bank transfer rejected", *got.FailureReason)
}

func TestYellowCardWebhook_WithdrawalCompleted_Reconciles(t *testing.T) {
	withdrawals := newFakeWithdrawalRepo()
	withdrawals.byPaymentRef["MOIST-3"] = &withdrawal.Withdrawal{
		ID: "wd-1", PaymentRef: "MOIST-3", Status: withdrawal.WithdrawalStatusProcessing,
	}
	h := handler.NewYellowCardWebhookHandler(newFakeDepositRepo(), withdrawals, testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "send", "status": "completed", "paymentReference": "MOIST-3", "id": "s-1",
	})

	w := postSignedWebhook(t, r, payload, testWebhookSecret)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, withdrawal.WithdrawalStatusCompleted, withdrawals.byPaymentRef["MOIST-3"].Status)
}

func TestYellowCardWebhook_UnknownPaymentRef_StillReturns200(t *testing.T) {
	h := handler.NewYellowCardWebhookHandler(newFakeDepositRepo(), newFakeWithdrawalRepo(), testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{
		"type": "receive", "status": "completed", "paymentReference": "MOIST-UNKNOWN",
	})

	w := postSignedWebhook(t, r, payload, testWebhookSecret)

	// Valid signature, well-formed payload, but no matching local record —
	// Yellow Card shouldn't be made to retry forever for something we've
	// already understood (and logged) as unmatched.
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestYellowCardWebhook_MissingPaymentReference_BadRequest(t *testing.T) {
	h := handler.NewYellowCardWebhookHandler(newFakeDepositRepo(), newFakeWithdrawalRepo(), testWebhookSecret)
	r := setupTestWebhookRouter(h)

	payload, _ := json.Marshal(map[string]any{"type": "receive", "status": "completed"})

	w := postSignedWebhook(t, r, payload, testWebhookSecret)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
