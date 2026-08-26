package swap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/circle"
	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/pkg/apperrors"
)

// Mock Swap Repository
type mockSwapRepo struct {
	offers map[string]*SwapOffer
}

func newMockSwapRepo() *mockSwapRepo {
	return &mockSwapRepo{
		offers: make(map[string]*SwapOffer),
	}
}

func (m *mockSwapRepo) CreateSwapOffer(ctx context.Context, offer *SwapOffer) error {
	m.offers[offer.ID] = offer
	return nil
}

func (m *mockSwapRepo) GetSwapOfferByID(ctx context.Context, id string) (*SwapOffer, error) {
	offer, ok := m.offers[id]
	if !ok {
		return nil, errors.New("offer not found")
	}
	return offer, nil
}

func (m *mockSwapRepo) UpdateSwapOfferStatus(ctx context.Context, id string, status SwapOfferStatus, transactionHash *string) error {
	offer, ok := m.offers[id]
	if !ok {
		return errors.New("offer not found")
	}
	offer.Status = status
	if transactionHash != nil {
		offer.TransactionHash = transactionHash
	}
	return nil
}

func (m *mockSwapRepo) ListUserSwapOffers(ctx context.Context, userID string, filter SwapHistoryFilter) ([]SwapOffer, int, error) {
	var result []SwapOffer
	for _, o := range m.offers {
		if o.OfferorUserID == userID || (o.OffereeUserID != nil && *o.OffereeUserID == userID) {
			result = append(result, *o)
		}
	}
	return result, len(result), nil
}

func (m *mockSwapRepo) ListCircleSwapOffers(ctx context.Context, circleID string, filter SwapHistoryFilter) ([]SwapOffer, int, error) {
	var result []SwapOffer
	for _, o := range m.offers {
		if o.CircleID == circleID {
			result = append(result, *o)
		}
	}
	return result, len(result), nil
}

func (m *mockSwapRepo) CancelExpiredOffers(ctx context.Context) error {
	for _, o := range m.offers {
		if o.Status == SwapOfferStatusCreated && time.Now().After(o.ExpiresAt) {
			o.Status = SwapOfferStatusExpired
		}
	}
	return nil
}

// Mock Circle Service
type mockCircleService struct {
	circles map[string]*circle.Circle
	members map[string]map[string]bool // circleID -> userID -> isMember
}

func newMockCircleService() *mockCircleService {
	return &mockCircleService{
		circles: make(map[string]*circle.Circle),
		members: make(map[string]map[string]bool),
	}
}

func (m *mockCircleService) Get(ctx context.Context, id string) (*circle.Circle, error) {
	c, ok := m.circles[id]
	if !ok {
		return nil, errors.New("circle not found")
	}
	return c, nil
}

func (m *mockCircleService) IsMember(ctx context.Context, circleID, userID string) (bool, error) {
	if circleMembers, ok := m.members[circleID]; ok {
		return circleMembers[userID], nil
	}
	return false, nil
}

func (m *mockCircleService) List(ctx context.Context, filter circle.CircleFilter) ([]circle.Circle, int, error) {
	return nil, 0, nil
}
func (m *mockCircleService) Create(ctx context.Context, organizerID string, input circle.CreateCircleInput) (*circle.Circle, error) {
	return nil, nil
}
func (m *mockCircleService) Update(ctx context.Context, id, userID string, input circle.UpdateCircleInput) (*circle.Circle, error) {
	return nil, nil
}
func (m *mockCircleService) Start(ctx context.Context, id, userID string) error { return nil }
func (m *mockCircleService) Close(ctx context.Context, id, userID string) error { return nil }
func (m *mockCircleService) Cancel(ctx context.Context, id, userID string) error { return nil }
func (m *mockCircleService) Join(ctx context.Context, circleID, userID string, inviteCode string) error {
	return nil
}
func (m *mockCircleService) Exit(ctx context.Context, circleID, userID string) error { return nil }
func (m *mockCircleService) GetMembers(ctx context.Context, circleID string) ([]circle.CircleMember, error) {
	return nil, nil
}
func (m *mockCircleService) RemoveMember(ctx context.Context, circleID, callerID, memberAddress string, reason string) error {
	return nil
}

// Mock User Service
type mockUserService struct {
	users map[string]*user.User
}

func newMockUserService() *mockUserService {
	return &mockUserService{
		users: make(map[string]*user.User),
	}
}

func (m *mockUserService) GetByID(ctx context.Context, id string) (*user.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (m *mockUserService) GetByWallet(ctx context.Context, wallet string) (*user.User, error) {
	for _, u := range m.users {
		if u.WalletAddress == wallet {
			return u, nil
		}
	}
	return nil, errors.New("user not found")
}
func (m *mockUserService) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserService) Create(ctx context.Context, wallet string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserService) Delete(ctx context.Context, id string) error { return nil }
func (m *mockUserService) UpdateProfile(ctx context.Context, id string, updates user.UpdateProfileInput) (*user.User, error) {
	return nil, nil
}
func (m *mockUserService) UpdateNotificationPreferences(ctx context.Context, id string, prefs user.NotificationPrefsInput) (*user.User, error) {
	return nil, nil
}
func (m *mockUserService) IsEmailTaken(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (m *mockUserService) GetMoiScore(ctx context.Context, id string) (*user.MoiScoreResponse, error) {
	return nil, nil
}
func (m *mockUserService) GetCircles(ctx context.Context, id string) ([]any, error) {
	return nil, nil
}
func (m *mockUserService) ClaimName(ctx context.Context) (string, error) { return "", nil }

// ─── Swap Service Lifecycle Tests ─────────────────────────────────────────────

func TestSwapService_CreateSwapOffer_Validation(t *testing.T) {
	repo := newMockSwapRepo()
	circleSvc := newMockCircleService()
	userSvc := newMockUserService()

	svc := NewService(repo, circleSvc, userSvc, nil)
	ctx := context.Background()

	circleID := uuid.NewString()
	userA := uuid.NewString()
	userB := uuid.NewString()

	// 1. Invalid circle ID
	_, err := svc.CreateSwapOffer(ctx, userA, SwapOfferRequest{
		CircleID: circleID,
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidInput))

	// Setup circle
	circleSvc.circles[circleID] = &circle.Circle{ID: uuid.MustParse(circleID), Name: "Circle 1"}

	// 2. User is not a member of the circle
	_, err = svc.CreateSwapOffer(ctx, userA, SwapOfferRequest{
		CircleID: circleID,
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrForbidden))

	// Add userA as member
	circleSvc.members[circleID] = map[string]bool{userA: true}

	// 3. Offeree is specified but is not a member
	_, err = svc.CreateSwapOffer(ctx, userA, SwapOfferRequest{
		CircleID:      circleID,
		OffereeUserID: &userB,
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidInput))

	// Add userB as member
	circleSvc.members[circleID][userB] = true

	// 4. User has no wallet address
	userSvc.users[userA] = &user.User{ID: uuid.MustParse(userA), WalletAddress: ""}
	_, err = svc.CreateSwapOffer(ctx, userA, SwapOfferRequest{
		CircleID:        circleID,
		OffereeUserID:   &userB,
		OfferorAsset:    "USDC",
		OfferorAmount:   100,
		RequestedAsset:  "XLM",
		RequestedAmount: 500,
		ExpiresIn:       24,
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidInput))
}

func TestSwapService_AcceptSwapOffer_Validation(t *testing.T) {
	repo := newMockSwapRepo()
	circleSvc := newMockCircleService()
	userSvc := newMockUserService()

	svc := NewService(repo, circleSvc, userSvc, nil)
	ctx := context.Background()

	circleID := uuid.NewString()
	userA := uuid.NewString()
	userB := uuid.NewString()
	userC := uuid.NewString()

	// 1. Offer not found
	_, err := svc.AcceptSwapOffer(ctx, userB, "non-existent-id")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))

	// Create an offer in repo
	offerID := uuid.NewString()
	repo.offers[offerID] = &SwapOffer{
		ID:            offerID,
		CircleID:      circleID,
		OfferorUserID: userA,
		OffereeUserID: &userB,
		Status:        SwapOfferStatusCreated,
	}

	// 2. Acceptor is not the specified offeree
	_, err = svc.AcceptSwapOffer(ctx, userC, offerID)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrForbidden))

	// 3. Acceptor is not a member of the circle
	_, err = svc.AcceptSwapOffer(ctx, userB, offerID)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrForbidden))

	// Setup circle members
	circleSvc.members[circleID] = map[string]bool{userA: true, userB: true}

	// 4. Offeror cannot accept own offer
	repo.offers[offerID].OffereeUserID = nil // open offer
	_, err = svc.AcceptSwapOffer(ctx, userA, offerID)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidInput))

	// 5. Offer not in created status
	repo.offers[offerID].Status = SwapOfferStatusCompleted
	_, err = svc.AcceptSwapOffer(ctx, userB, offerID)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidInput))
}

func TestSwapService_GetSwapHistory(t *testing.T) {
	repo := newMockSwapRepo()
	circleSvc := newMockCircleService()
	userSvc := newMockUserService()

	svc := NewService(repo, circleSvc, userSvc, nil)
	ctx := context.Background()

	userA := uuid.NewString()
	userB := uuid.NewString()

	repo.offers["o1"] = &SwapOffer{ID: "o1", OfferorUserID: userA, CircleID: "c1", Status: SwapOfferStatusCreated}
	repo.offers["o2"] = &SwapOffer{ID: "o2", OfferorUserID: userA, OffereeUserID: &userB, CircleID: "c1", Status: SwapOfferStatusCompleted}
	repo.offers["o3"] = &SwapOffer{ID: "o3", OfferorUserID: userB, CircleID: "c2", Status: SwapOfferStatusCreated}

	resp, err := svc.GetSwapHistory(ctx, userA, SwapHistoryFilter{Limit: 20, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Swaps, 2)
}
