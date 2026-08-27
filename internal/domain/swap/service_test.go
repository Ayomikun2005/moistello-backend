package swap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/pkg/apperrors"
)

// ── Fakes ─────────────────────────────────────────────────────────────────

type fakeUserService struct {
	getByIDFn func(ctx context.Context, id string) (*user.User, error)
}

func (f *fakeUserService) GetByID(ctx context.Context, id string) (*user.User, error) {
	return f.getByIDFn(ctx, id)
}

type fakeEscrow struct {
	createSwapFn  func(ctx context.Context, circleID, offeror, offeree string, offerorAsset string, offerorAmount int64, requestedAsset string, requestedAmount int64, expiresAt uint64) (string, error)
	acceptSwapFn  func(ctx context.Context, swapID string, acceptor string) (string, error)
	cancelSwapFn  func(ctx context.Context, swapID string, canceller string) (string, error)
	executeSwapFn func(ctx context.Context, swapID string) (string, error)
}

func (f *fakeEscrow) CreateSwap(ctx context.Context, circleID, offeror, offeree string, offerorAsset string, offerorAmount int64, requestedAsset string, requestedAmount int64, expiresAt uint64) (string, error) {
	return f.createSwapFn(ctx, circleID, offeror, offeree, offerorAsset, offerorAmount, requestedAsset, requestedAmount, expiresAt)
}

func (f *fakeEscrow) AcceptSwap(ctx context.Context, swapID string, acceptor string) (string, error) {
	return f.acceptSwapFn(ctx, swapID, acceptor)
}

func (f *fakeEscrow) CancelSwap(ctx context.Context, swapID string, canceller string) (string, error) {
	return f.cancelSwapFn(ctx, swapID, canceller)
}

func (f *fakeEscrow) ExecuteSwap(ctx context.Context, swapID string) (string, error) {
	return f.executeSwapFn(ctx, swapID)
}

type fakeRepo struct {
	createFn        func(ctx context.Context, offer *SwapOffer) error
	getByIDFn       func(ctx context.Context, id string) (*SwapOffer, error)
	updateFn        func(ctx context.Context, id string, status SwapOfferStatus, transactionHash *string) error
	listUserFn      func(ctx context.Context, userID string, filter SwapHistoryFilter) ([]SwapOffer, int, error)
	listCircleFn    func(ctx context.Context, circleID string, filter SwapHistoryFilter) ([]SwapOffer, int, error)
	listExpiredFn   func(ctx context.Context, now time.Time) ([]SwapOffer, error)
	updatedStatuses []SwapOfferStatus
	updatedIDs      []string
}

func (f *fakeRepo) CreateSwapOffer(ctx context.Context, offer *SwapOffer) error {
	return f.createFn(ctx, offer)
}

func (f *fakeRepo) GetSwapOfferByID(ctx context.Context, id string) (*SwapOffer, error) {
	return f.getByIDFn(ctx, id)
}

func (f *fakeRepo) UpdateSwapOfferStatus(ctx context.Context, id string, status SwapOfferStatus, transactionHash *string) error {
	f.updatedStatuses = append(f.updatedStatuses, status)
	f.updatedIDs = append(f.updatedIDs, id)
	if f.updateFn != nil {
		return f.updateFn(ctx, id, status, transactionHash)
	}
	return nil
}

func (f *fakeRepo) ListUserSwapOffers(ctx context.Context, userID string, filter SwapHistoryFilter) ([]SwapOffer, int, error) {
	return f.listUserFn(ctx, userID, filter)
}

func (f *fakeRepo) ListCircleSwapOffers(ctx context.Context, circleID string, filter SwapHistoryFilter) ([]SwapOffer, int, error) {
	return f.listCircleFn(ctx, circleID, filter)
}

func (f *fakeRepo) ListExpiredCreatedOffers(ctx context.Context, now time.Time) ([]SwapOffer, error) {
	return f.listExpiredFn(ctx, now)
}

func walletUser(id string) *user.User {
	return &user.User{WalletAddress: "G" + id + "WALLET"}
}

func createdOffer(id, offeror string) *SwapOffer {
	return &SwapOffer{
		ID:            id,
		OfferorUserID: offeror,
		Status:        SwapOfferStatusCreated,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}
}

// ── Sweep worker ──────────────────────────────────────────────────────────

func TestSweepExpiredOffers_ReleasesEscrowAndMarksExpired(t *testing.T) {
	ctx := context.Background()
	offers := []SwapOffer{*createdOffer("offer-1", "u1"), *createdOffer("offer-2", "u2")}
	repo := &fakeRepo{listExpiredFn: func(ctx context.Context, now time.Time) ([]SwapOffer, error) {
		return offers, nil
	}}
	users := &fakeUserService{getByIDFn: func(ctx context.Context, id string) (*user.User, error) {
		return walletUser(id), nil
	}}
	var cancelled []string
	escrow := &fakeEscrow{cancelSwapFn: func(ctx context.Context, swapID, canceller string) (string, error) {
		cancelled = append(cancelled, swapID+":"+canceller)
		return "tx-" + swapID, nil
	}}

	svc := NewService(repo, nil, users, escrow)
	swept, err := svc.SweepExpiredOffers(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, swept)
	assert.Equal(t, []string{"offer-1:Gu1WALLET", "offer-2:Gu2WALLET"}, cancelled)
	// Both offers marked expired in the same order.
	assert.Equal(t, []SwapOfferStatus{SwapOfferStatusExpired, SwapOfferStatusExpired}, repo.updatedStatuses)
	assert.Equal(t, []string{"offer-1", "offer-2"}, repo.updatedIDs)
}

func TestSweepExpiredOffers_SkipsOfferWhenOnChainCancelFails(t *testing.T) {
	ctx := context.Background()
	offers := []SwapOffer{*createdOffer("offer-1", "u1"), *createdOffer("offer-2", "u2")}
	repo := &fakeRepo{listExpiredFn: func(ctx context.Context, now time.Time) ([]SwapOffer, error) {
		return offers, nil
	}}
	users := &fakeUserService{getByIDFn: func(ctx context.Context, id string) (*user.User, error) {
		return walletUser(id), nil
	}}
	escrow := &fakeEscrow{cancelSwapFn: func(ctx context.Context, swapID, canceller string) (string, error) {
		if swapID == "offer-1" {
			return "", errors.New("simulation failed")
		}
		return "tx-" + swapID, nil
	}}

	svc := NewService(repo, nil, users, escrow)
	swept, err := svc.SweepExpiredOffers(ctx)
	require.NoError(t, err)

	// Only the offer that cancelled on-chain was swept; the failed one stays
	// created so the next sweep retries it.
	assert.Equal(t, 1, swept)
	assert.Equal(t, []SwapOfferStatus{SwapOfferStatusExpired}, repo.updatedStatuses)
	assert.Equal(t, []string{"offer-2"}, repo.updatedIDs)
}

func TestSweepExpiredOffers_SkipsOfferWhenOfferorUnresolvable(t *testing.T) {
	ctx := context.Background()
	offers := []SwapOffer{*createdOffer("offer-1", "gone-user")}
	repo := &fakeRepo{listExpiredFn: func(ctx context.Context, now time.Time) ([]SwapOffer, error) {
		return offers, nil
	}}
	users := &fakeUserService{getByIDFn: func(ctx context.Context, id string) (*user.User, error) {
		return nil, errors.New("user not found")
	}}
	escrow := &fakeEscrow{cancelSwapFn: func(ctx context.Context, swapID, canceller string) (string, error) {
		return "tx", nil
	}}

	svc := NewService(repo, nil, users, escrow)
	swept, err := svc.SweepExpiredOffers(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, swept)
	assert.Empty(t, repo.updatedStatuses)
}

// ── Offeror cancel ────────────────────────────────────────────────────────

func TestCancelSwapOffer_Success(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{getByIDFn: func(ctx context.Context, id string) (*SwapOffer, error) {
		return createdOffer("offer-1", "u1"), nil
	}}
	users := &fakeUserService{getByIDFn: func(ctx context.Context, id string) (*user.User, error) {
		return walletUser(id), nil
	}}
	var cancelled []string
	escrow := &fakeEscrow{cancelSwapFn: func(ctx context.Context, swapID, canceller string) (string, error) {
		cancelled = append(cancelled, swapID+":"+canceller)
		return "tx", nil
	}}

	svc := NewService(repo, nil, users, escrow)
	offer, err := svc.CancelSwapOffer(ctx, "u1", "offer-1")
	require.NoError(t, err)

	assert.Equal(t, SwapOfferStatusCancelled, offer.Status)
	assert.Equal(t, []string{"offer-1:Gu1WALLET"}, cancelled)
	assert.Equal(t, []SwapOfferStatus{SwapOfferStatusCancelled}, repo.updatedStatuses)
}

func TestCancelSwapOffer_OnlyOfferorCanCancel(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{getByIDFn: func(ctx context.Context, id string) (*SwapOffer, error) {
		return createdOffer("offer-1", "u1"), nil
	}}
	escrow := &fakeEscrow{cancelSwapFn: func(ctx context.Context, swapID, canceller string) (string, error) {
		t.Fatal("escrow must not be called for a non-offeror")
		return "", nil
	}}

	svc := NewService(repo, nil, &fakeUserService{}, escrow)
	_, err := svc.CancelSwapOffer(ctx, "attacker", "offer-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrForbidden)
}

func TestCancelSwapOffer_RejectsNonCreatedOffer(t *testing.T) {
	ctx := context.Background()
	accepted := createdOffer("offer-1", "u1")
	accepted.Status = SwapOfferStatusAccepted
	repo := &fakeRepo{getByIDFn: func(ctx context.Context, id string) (*SwapOffer, error) {
		return accepted, nil
	}}
	escrow := &fakeEscrow{cancelSwapFn: func(ctx context.Context, swapID, canceller string) (string, error) {
		t.Fatal("escrow must not be called for a non-created offer")
		return "", nil
	}}

	svc := NewService(repo, nil, &fakeUserService{}, escrow)
	_, err := svc.CancelSwapOffer(ctx, "u1", "offer-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
}

// ── Acceptance vs expiry ──────────────────────────────────────────────────

func TestAcceptSwapOffer_RejectsExpiredOffer(t *testing.T) {
	ctx := context.Background()
	expired := createdOffer("offer-1", "u1")
	expired.ExpiresAt = time.Now().Add(-time.Hour) // expired but not yet swept
	repo := &fakeRepo{getByIDFn: func(ctx context.Context, id string) (*SwapOffer, error) {
		return expired, nil
	}}
	escrow := &fakeEscrow{acceptSwapFn: func(ctx context.Context, swapID, acceptor string) (string, error) {
		t.Fatal("escrow must not be called for an expired offer")
		return "", nil
	}}

	svc := NewService(repo, nil, &fakeUserService{}, escrow)
	_, err := svc.AcceptSwapOffer(ctx, "u2", "offer-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
	assert.Contains(t, err.Error(), "expired")
}
