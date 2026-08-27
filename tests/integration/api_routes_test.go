package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/internal/domain/circle"
	"github.com/moistello/backend/internal/domain/community"
	"github.com/moistello/backend/internal/domain/contribution"
	"github.com/moistello/backend/internal/domain/invite"
	"github.com/moistello/backend/internal/domain/notification"
	"github.com/moistello/backend/internal/domain/payout"
	"github.com/moistello/backend/internal/domain/savings"
	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/pkg/apperrors"
	"github.com/moistello/backend/webhook"
)

// ── Mock implementations ──────────────────────────────────────────────

type mockUserRepo struct {
	user *user.User
}

func (m *mockUserRepo) FindByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if m.user != nil && m.user.ID == id {
		return m.user, nil
	}
	return &user.User{ID: id, WalletAddress: "GMOCK1234567890ABCDEF", MoiScore: 100, Role: user.RoleUser}, nil
}
func (m *mockUserRepo) FindByWalletAddress(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) FindByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) FindByPasskeyCredentialID(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserRepo) Create(_ context.Context, _ *user.User) error { return nil }
func (m *mockUserRepo) Delete(_ context.Context, _ uuid.UUID) error  { return nil }
func (m *mockUserRepo) Update(_ context.Context, _ *user.User) error { return nil }
func (m *mockUserRepo) UpdateMoiScore(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}
func (m *mockUserRepo) List(_ context.Context, _ user.UserFilter) ([]user.User, error) {
	return []user.User{}, nil
}
func (m *mockUserRepo) Count(_ context.Context, _ user.UserFilter) (int, error) { return 0, nil }
func (m *mockUserRepo) ClaimNextName(_ context.Context) (int64, error)          { return 1, nil }

type mockUserService struct{}

func (m *mockUserService) GetByID(_ context.Context, id string) (*user.User, error) {
	return &user.User{ID: uuid.MustParse(id), WalletAddress: "GMOCK", MoiScore: 100, Role: user.RoleUser}, nil
}
func (m *mockUserService) GetByWallet(_ context.Context, _ string) (*user.User, error) {
	return &user.User{ID: uuid.New(), WalletAddress: "GMOCK", MoiScore: 100}, nil
}
func (m *mockUserService) GetByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserService) Create(_ context.Context, _ string) (*user.User, error) {
	return &user.User{ID: uuid.New(), WalletAddress: "GMOCK", MoiScore: 0}, nil
}
func (m *mockUserService) Delete(_ context.Context, _ string) error { return nil }
func (m *mockUserService) UpdateProfile(_ context.Context, id string, _ user.UpdateProfileInput) (*user.User, error) {
	return &user.User{ID: uuid.MustParse(id), WalletAddress: "GMOCK", MoiScore: 100}, nil
}
func (m *mockUserService) UpdateNotificationPreferences(_ context.Context, id string, _ user.NotificationPrefsInput) (*user.User, error) {
	return &user.User{ID: uuid.MustParse(id), WalletAddress: "GMOCK", NotificationChannels: []string{"inapp"}}, nil
}
func (m *mockUserService) IsEmailTaken(_ context.Context, _ string) (bool, error) { return false, nil }
func (m *mockUserService) GetMoiScore(_ context.Context, id string) (*user.MoiScoreResponse, error) {
	return &user.MoiScoreResponse{Score: 100, Level: "Silver"}, nil
}
func (m *mockUserService) GetCircles(_ context.Context, _ string) ([]any, error) { return []any{}, nil }
func (m *mockUserService) ClaimName(_ context.Context) (string, error)           { return "MoistelloUser1", nil }

type mockCircleRepo struct{}

func (m *mockCircleRepo) FindByID(_ context.Context, id uuid.UUID) (*circle.Circle, error) {
	return &circle.Circle{ID: id, Name: "Mock Circle", Status: circle.CircleStatusActive, MaxMembers: 10, CircleType: circle.CircleTypePublic, OrganizerID: uuid.New()}, nil
}
func (m *mockCircleRepo) FindByContractID(_ context.Context, _ string) (*circle.Circle, error) {
	return nil, circle.ErrCircleNotFound
}
func (m *mockCircleRepo) List(_ context.Context, _ circle.CircleFilter) ([]circle.Circle, error) {
	return []circle.Circle{}, nil
}
func (m *mockCircleRepo) Count(_ context.Context, _ circle.CircleFilter) (int, error)  { return 0, nil }
func (m *mockCircleRepo) Create(_ context.Context, _ *circle.Circle) error             { return nil }
func (m *mockCircleRepo) Update(_ context.Context, _ *circle.Circle) error             { return nil }
func (m *mockCircleRepo) Delete(_ context.Context, _ uuid.UUID) error                  { return nil }
func (m *mockCircleRepo) CreateMember(_ context.Context, _ *circle.CircleMember) error { return nil }
func (m *mockCircleRepo) GetMembers(_ context.Context, _ uuid.UUID) ([]circle.CircleMember, error) {
	return []circle.CircleMember{}, nil
}
func (m *mockCircleRepo) GetMemberCount(_ context.Context, _ uuid.UUID) (int, error) { return 1, nil }
func (m *mockCircleRepo) UpdateMemberStatus(_ context.Context, _, _ uuid.UUID, _ circle.MemberStatus) error {
	return nil
}
func (m *mockCircleRepo) FindMemberByCircleAndUser(_ context.Context, _, _ uuid.UUID) (*circle.CircleMember, error) {
	return nil, nil
}
func (m *mockCircleRepo) FindCirclesByUserID(_ context.Context, _ uuid.UUID) ([]circle.Circle, error) {
	return []circle.Circle{}, nil
}

type fakeTestWebhookRepo struct{}

func (r *fakeTestWebhookRepo) Register(_ context.Context, _ *webhook.WebhookRegistration) error {
	return nil
}
func (r *fakeTestWebhookRepo) GetByUserID(_ context.Context, _ string) ([]webhook.WebhookRegistration, error) {
	return nil, nil
}
func (r *fakeTestWebhookRepo) GetActiveWebhooks(_ context.Context) ([]webhook.WebhookRegistration, error) {
	return nil, nil
}
func (r *fakeTestWebhookRepo) GetByID(_ context.Context, _ string) (*webhook.WebhookRegistration, error) {
	return nil, nil
}
func (r *fakeTestWebhookRepo) Delete(_ context.Context, _ string) error { return nil }
func (r *fakeTestWebhookRepo) ListDeliveries(_ context.Context, _ string, _, _ int) ([]webhook.DeliveryLog, int, error) {
	return nil, 0, nil
}

type mockCircleService struct{}

func (m *mockCircleService) Get(_ context.Context, id string) (*circle.Circle, error) {
	return &circle.Circle{ID: uuid.MustParse(id), Name: "Mock Circle", Status: circle.CircleStatusActive, MaxMembers: 10, CircleType: circle.CircleTypePublic, OrganizerID: uuid.New()}, nil
}
func (m *mockCircleService) List(_ context.Context, _ circle.CircleFilter) ([]circle.Circle, int, error) {
	return []circle.Circle{}, 0, nil
}
func (m *mockCircleService) Create(_ context.Context, organizerID string, input circle.CreateCircleInput) (*circle.Circle, error) {
	return &circle.Circle{ID: uuid.New(), Name: input.Name, Status: circle.CircleStatusPending, OrganizerID: uuid.MustParse(organizerID)}, nil
}
func (m *mockCircleService) Update(_ context.Context, id, userID string, _ circle.UpdateCircleInput) (*circle.Circle, error) {
	return &circle.Circle{ID: uuid.MustParse(id), Name: "Updated"}, nil
}
func (m *mockCircleService) Start(_ context.Context, _, _ string) error   { return nil }
func (m *mockCircleService) Close(_ context.Context, _, _ string) error   { return nil }
func (m *mockCircleService) Cancel(_ context.Context, _, _ string) error  { return nil }
func (m *mockCircleService) Join(_ context.Context, _, _, _ string) error { return nil }
func (m *mockCircleService) Exit(_ context.Context, _, _ string) error    { return nil }
func (m *mockCircleService) GetMembers(_ context.Context, _ string) ([]circle.CircleMember, error) {
	return []circle.CircleMember{}, nil
}
func (m *mockCircleService) IsMember(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockCircleService) RemoveMember(_ context.Context, _, _, _ string, _ string) error {
	return nil
}
func (m *mockCircleService) ProcessMissedContributions(_ context.Context, _ string, _ int) error {
	return nil
}

type mockInviteService struct{}

func (m *mockInviteService) Generate(_ context.Context, _ invite.GenerateInput) (*invite.Invite, error) {
	return &invite.Invite{ID: uuid.New(), Code: "INVITE-123"}, nil
}
func (m *mockInviteService) Validate(_ context.Context, _ string) (*invite.Invite, error) {
	return &invite.Invite{ID: uuid.New(), Code: "valid-code"}, nil
}
func (m *mockInviteService) List(_ context.Context, _ string) ([]invite.Invite, error) {
	return []invite.Invite{}, nil
}
func (m *mockInviteService) Revoke(_ context.Context, _, _ string) error { return nil }

type mockContribService struct{}

func (m *mockContribService) Record(_ context.Context, input contribution.RecordInput) (*contribution.Contribution, error) {
	return &contribution.Contribution{ID: uuid.New(), RoundNumber: input.RoundNumber, Amount: input.Amount, Status: contribution.StatusPending}, nil
}
func (m *mockContribService) UpdateVerification(_ context.Context, _ string, _ bool, _ contribution.VerificationStatus) error {
	return nil
}
func (m *mockContribService) GetUserHistory(_ context.Context, _ string, _, _ int) ([]contribution.Contribution, int, error) {
	return []contribution.Contribution{}, 0, nil
}
func (m *mockContribService) GetCircleHistory(_ context.Context, _ string, _, _ int) ([]contribution.Contribution, int, error) {
	return []contribution.Contribution{}, 0, nil
}

type mockContribRepo struct{}

func (m *mockContribRepo) FindByID(_ context.Context, _ uuid.UUID) (*contribution.Contribution, error) {
	return nil, apperrors.ErrNotFound
}
func (m *mockContribRepo) FindByCircleAndUser(_ context.Context, _, _ uuid.UUID) (*contribution.Contribution, error) {
	return nil, nil
}
func (m *mockContribRepo) Create(_ context.Context, _ *contribution.Contribution) error { return nil }
func (m *mockContribRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ contribution.ContributionStatus, _ string) error {
	return nil
}
func (m *mockContribRepo) UpdateVerificationStatus(_ context.Context, _ uuid.UUID, _ bool, _ contribution.VerificationStatus) error {
	return nil
}
func (m *mockContribRepo) ListByUser(_ context.Context, _ uuid.UUID, _, _ int) ([]contribution.Contribution, int, error) {
	return []contribution.Contribution{}, 0, nil
}
func (m *mockContribRepo) ListByCircle(_ context.Context, _ uuid.UUID, _, _ int) ([]contribution.Contribution, int, error) {
	return []contribution.Contribution{}, 0, nil
}

type mockPayoutService struct{}

func (m *mockPayoutService) Record(_ context.Context, input payout.RecordInput) (*payout.Payout, error) {
	return &payout.Payout{ID: uuid.New(), RoundNumber: input.RoundNumber, Amount: input.Amount, PayoutType: input.PayoutType}, nil
}
func (m *mockPayoutService) UpdateVerification(_ context.Context, _ string, _ bool, _ payout.VerificationStatus) error {
	return nil
}
func (m *mockPayoutService) GetUserHistory(_ context.Context, _ string, _, _ int) ([]payout.Payout, int, error) {
	return []payout.Payout{}, 0, nil
}
func (m *mockPayoutService) GetCircleHistory(_ context.Context, _ string, _, _ int) ([]payout.Payout, int, error) {
	return []payout.Payout{{ID: uuid.New()}}, 1, nil
}

type mockPayoutRepo struct{}

func (m *mockPayoutRepo) FindByID(_ context.Context, _ uuid.UUID) (*payout.Payout, error) {
	return nil, apperrors.ErrNotFound
}
func (m *mockPayoutRepo) Create(_ context.Context, _ *payout.Payout) error { return nil }
func (m *mockPayoutRepo) UpdateVerificationStatus(_ context.Context, _ uuid.UUID, _ bool, _ payout.VerificationStatus) error {
	return nil
}
func (m *mockPayoutRepo) ListByUser(_ context.Context, _ uuid.UUID, _, _ int) ([]payout.Payout, int, error) {
	return []payout.Payout{}, 0, nil
}
func (m *mockPayoutRepo) ListByCircle(_ context.Context, _ uuid.UUID, _, _ int) ([]payout.Payout, int, error) {
	return []payout.Payout{}, 0, nil
}

type mockNotificationService struct{}

func (m *mockNotificationService) Create(_ context.Context, _ notification.CreateInput) (*notification.Notification, error) {
	return &notification.Notification{ID: uuid.New()}, nil
}
func (m *mockNotificationService) List(_ context.Context, _ string, _, _ int, _ bool) ([]notification.Notification, int, error) {
	return []notification.Notification{}, 0, nil
}
func (m *mockNotificationService) MarkRead(_ context.Context, _, _ string) error { return nil }
func (m *mockNotificationService) MarkAllRead(_ context.Context, _ string) error { return nil }

type mockCommunityService struct{}

func (m *mockCommunityService) Create(_ context.Context, _ string, _ community.CreateCommunityInput) (*community.Community, error) {
	return &community.Community{ID: uuid.New(), Name: "Test Community"}, nil
}
func (m *mockCommunityService) Get(_ context.Context, id string) (*community.Community, error) {
	return &community.Community{ID: uuid.MustParse(id), Name: "Test Community"}, nil
}
func (m *mockCommunityService) GetBySlug(_ context.Context, _ string) (*community.Community, error) {
	return &community.Community{ID: uuid.New(), Name: "Test Community"}, nil
}
func (m *mockCommunityService) List(_ context.Context, _ community.CommunityFilter) ([]community.Community, int, error) {
	return []community.Community{}, 0, nil
}
func (m *mockCommunityService) Update(_ context.Context, _, _ string, _ community.UpdateCommunityInput) (*community.Community, error) {
	return &community.Community{ID: uuid.New(), Name: "Updated"}, nil
}
func (m *mockCommunityService) Delete(_ context.Context, _, _ string) error { return nil }
func (m *mockCommunityService) Join(_ context.Context, _, _ string) error   { return nil }
func (m *mockCommunityService) Leave(_ context.Context, _, _ string) error  { return nil }
func (m *mockCommunityService) GetMembers(_ context.Context, _ string) ([]community.CommunityMember, error) {
	return []community.CommunityMember{}, nil
}
func (m *mockCommunityService) IsMember(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}
func (m *mockCommunityService) CreateAnnouncement(_ context.Context, _, _, _ string) (*community.Announcement, error) {
	return &community.Announcement{ID: uuid.New()}, nil
}
func (m *mockCommunityService) GetAnnouncements(_ context.Context, _ string) ([]community.Announcement, error) {
	return []community.Announcement{}, nil
}
func (m *mockCommunityService) DeleteAnnouncement(_ context.Context, _, _ string) error { return nil }
func (m *mockCommunityService) LikeAnnouncement(_ context.Context, _ string) error      { return nil }
func (m *mockCommunityService) PinAnnouncement(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (m *mockCommunityService) RemoveMember(_ context.Context, _, _, _ string) error      { return nil }
func (m *mockCommunityService) TransferOwnership(_ context.Context, _, _, _ string) error { return nil }
func (m *mockCommunityService) GetActivity(_ context.Context, _ string, _ int) ([]community.ActivityEvent, error) {
	return []community.ActivityEvent{}, nil
}
func (m *mockCommunityService) GetMyCommunities(_ context.Context, _ string) ([]community.Community, error) {
	return []community.Community{}, nil
}
func (m *mockCommunityService) RecordActivity(_ context.Context, _, _, _, _ string, _ map[string]interface{}) error {
	return nil
}
func (m *mockCommunityService) UpdateTotalSaved(_ context.Context, _ string) error { return nil }

type mockSavingsService struct{}

func (m *mockSavingsService) CreateGoal(_ context.Context, _ string, req savings.CreateGoalRequest) (*savings.SavingsGoal, error) {
	return &savings.SavingsGoal{ID: uuid.New().String(), Name: req.Name, TargetAmount: req.TargetAmount}, nil
}
func (m *mockSavingsService) GetGoal(_ context.Context, _, goalID string) (*savings.SavingsGoal, error) {
	return &savings.SavingsGoal{ID: goalID, Name: "Test Goal"}, nil
}
func (m *mockSavingsService) ListGoals(_ context.Context, _ string) ([]savings.SavingsGoal, error) {
	return []savings.SavingsGoal{}, nil
}
func (m *mockSavingsService) ListActiveGoals(_ context.Context, _ string) ([]savings.SavingsGoal, error) {
	return []savings.SavingsGoal{}, nil
}
func (m *mockSavingsService) UpdateGoal(_ context.Context, _, goalID string, _ savings.UpdateGoalRequest) (*savings.SavingsGoal, error) {
	return &savings.SavingsGoal{ID: goalID, Name: "Updated Goal"}, nil
}
func (m *mockSavingsService) DeleteGoal(_ context.Context, _, _ string) error { return nil }
func (m *mockSavingsService) CompleteGoal(_ context.Context, _, goalID string) (*savings.SavingsGoal, error) {
	return &savings.SavingsGoal{ID: goalID, Status: "completed"}, nil
}
func (m *mockSavingsService) GetSummary(_ context.Context, _ string) (*savings.GoalSummary, error) {
	return &savings.GoalSummary{TotalGoals: 0}, nil
}
func (m *mockSavingsService) GetUpcomingObligations(_ context.Context, _ string) ([]savings.SavingsGoal, error) {
	return []savings.SavingsGoal{}, nil
}

// ── Test helpers ──────────────────────────────────────────────────────

func authedRequest(t *testing.T, router http.Handler, method, path, userID string, body any) (int, map[string]any) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func unauthRequest(t *testing.T, router http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	circleSvc := &mockCircleService{}
	inviteSvc := &mockInviteService{}
	contribSvc := &mockContribService{}
	contribRepo := &mockContribRepo{}
	payoutSvc := &mockPayoutService{}
	payoutRepo := &mockPayoutRepo{}
	userSvc := &mockUserService{}
	notifSvc := &mockNotificationService{}
	communitySvc := &mockCommunityService{}
	savingsSvc := &mockSavingsService{}

	circleHandler := handler.NewCircleHandler(circleSvc, inviteSvc, contribSvc, payoutSvc)
	inviteHandler := handler.NewInviteHandler(inviteSvc)
	contributionHandler := handler.NewContributionHandler(contribSvc, contribRepo)
	payoutHandler := handler.NewPayoutHandler(payoutSvc, payoutRepo)
	communityHandler := handler.NewCommunityHandler(communitySvc)
	notificationHandler := handler.NewNotificationHandler(notifSvc, userSvc)
	savingsHandler := handler.NewSavingsGoalHandler(savingsSvc)
	webhookHandler := handler.NewWebhookHandler(&fakeTestWebhookRepo{})
	healthHandler := handler.NewHealthHandler(nil, nil, "", "")

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", c.GetHeader("X-Test-User"))
		c.Set("role", c.GetHeader("X-Test-Role"))
		c.Next()
	})

	r.GET("/health", healthHandler.Health)
	r.GET("/health/ready", healthHandler.Ready)

	api := r.Group("/v1")
	{
		// Circles
		api.POST("/circles", circleHandler.CreateCircle)
		api.GET("/circles/:id", circleHandler.GetCircle)
		api.PATCH("/circles/:id", circleHandler.UpdateCircle)
		api.POST("/circles/:id/start", circleHandler.StartCircle)
		api.POST("/circles/:id/payout", circleHandler.TriggerPayout)
		api.POST("/circles/:id/close", circleHandler.CloseCircle)
		api.DELETE("/circles/:id", circleHandler.CancelCircle)
		api.POST("/circles/:id/join", circleHandler.JoinCircle)
		api.POST("/circles/:id/contribute", circleHandler.Contribute)
		api.POST("/circles/:id/exit", circleHandler.ExitCircle)
		api.GET("/circles/:id/members", circleHandler.GetMembers)
		api.GET("/circles/:id/rounds", circleHandler.GetRounds)
		api.GET("/circles/:id/payouts", circleHandler.GetPayouts)
		api.POST("/circles/:id/dispute", circleHandler.Dispute)
		api.POST("/circles/:id/vote", circleHandler.Vote)
		api.POST("/circles/:id/auction-bid", circleHandler.AuctionBid)

		// Invites
		api.GET("/circles/:id/invites", inviteHandler.ListInvites)
		api.POST("/circles/:id/invites", inviteHandler.CreateInvite)
		api.DELETE("/invites/:code", inviteHandler.RevokeInvite)

		// Contributions
		api.GET("/contributions", contributionHandler.ListContributions)
		api.GET("/contributions/:id", contributionHandler.GetContribution)

		// Payouts
		api.GET("/payouts", payoutHandler.ListPayouts)
		api.GET("/payouts/:id", payoutHandler.GetPayout)

		// Communities
		api.POST("/communities", communityHandler.Create)
		api.GET("/communities", communityHandler.List)
		api.GET("/communities/:id", communityHandler.Get)
		api.PATCH("/communities/:id", communityHandler.Update)
		api.DELETE("/communities/:id", communityHandler.Delete)
		api.POST("/communities/:id/join", communityHandler.Join)
		api.POST("/communities/:id/leave", communityHandler.Leave)
		api.GET("/communities/:id/members", communityHandler.GetMembers)
		api.GET("/communities/:id/membership", communityHandler.IsMember)
		api.POST("/communities/:id/announcements", communityHandler.CreateAnnouncement)
		api.GET("/communities/:id/announcements", communityHandler.GetAnnouncements)
		api.DELETE("/communities/:id/announcements/:announcementId", communityHandler.DeleteAnnouncement)
		api.POST("/communities/:id/announcements/:announcementId/like", communityHandler.LikeAnnouncement)
		api.PATCH("/communities/:id/announcements/:announcementId/pin", communityHandler.PinAnnouncement)
		api.DELETE("/communities/:id/members/:memberId", communityHandler.RemoveMember)
		api.POST("/communities/:id/transfer-ownership", communityHandler.TransferOwnership)
		api.GET("/communities/:id/activity", communityHandler.GetActivity)

		// Notifications
		api.GET("/notifications", notificationHandler.ListNotifications)
		api.PATCH("/notifications/:id/read", notificationHandler.MarkRead)
		api.PATCH("/notifications/read-all", notificationHandler.MarkAllRead)
		api.PUT("/notifications/preferences", notificationHandler.UpdatePreferences)

		// Savings
		api.POST("/savings/goals", savingsHandler.Create)
		api.GET("/savings/goals", savingsHandler.List)
		api.GET("/savings/goals/active", savingsHandler.ListActive)
		api.GET("/savings/goals/summary", savingsHandler.Summary)
		api.GET("/savings/goals/obligations", savingsHandler.UpcomingObligations)
		api.GET("/savings/goals/:id", savingsHandler.Get)
		api.PATCH("/savings/goals/:id", savingsHandler.Update)
		api.DELETE("/savings/goals/:id", savingsHandler.Delete)
		api.POST("/savings/goals/:id/complete", savingsHandler.Complete)

		// Webhooks
		api.POST("/webhooks", webhookHandler.RegisterWebhook)
		api.GET("/webhooks", webhookHandler.ListWebhooks)
		api.DELETE("/webhooks/:id", webhookHandler.DeleteWebhook)
	}

	return r
}

// ── Health routes ─────────────────────────────────────────────────────

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	r := setupTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "status")
}

func TestReadyEndpoint_ReturnsOK(t *testing.T) {
	r := setupTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/ready", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ready")
}

// ── Circle routes ─────────────────────────────────────────────────────

func TestCircleRoutes_CreateCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	orgID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/circles", orgID, map[string]any{
		"name": "Test Circle", "circleType": "public", "payoutType": "random",
		"contributionAmount": 100, "currency": "USDC", "frequency": "weekly",
		"maxMembers": 5, "maxStrikes": 3,
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_CreateCircle_MissingName(t *testing.T) {
	r := setupTestRouter()
	orgID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles", orgID, map[string]any{
		"circleType": "public", "payoutType": "random",
		"contributionAmount": 100, "currency": "USDC", "frequency": "weekly",
		"maxMembers": 5,
	})
	assert.Equal(t, http.StatusUnprocessableEntity, code)
}

func TestCircleRoutes_GetCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/circles/"+circleID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_UpdateCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	orgID := uuid.New().String()
	code, resp := authedRequest(t, r, "PATCH", "/v1/circles/"+circleID, orgID, map[string]any{
		"name": "Updated Circle",
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_StartCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	orgID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/start", orgID, nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_CloseCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	orgID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/close", orgID, nil)
	assert.Equal(t, http.StatusOK, code)
}

func TestCircleRoutes_CancelCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	orgID := uuid.New().String()
	code, resp := authedRequest(t, r, "DELETE", "/v1/circles/"+circleID, orgID, nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_JoinCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/join", userID, map[string]any{})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_Contribute_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/contribute", userID, map[string]any{
		"amount": 100, "txnHash": "txn-123", "roundNumber": 1,
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_Contribute_MissingFields(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/contribute", userID, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestCircleRoutes_ExitCircle_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/exit", userID, nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCircleRoutes_GetMembers_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/circles/"+circleID+"/members", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCircleRoutes_GetRounds_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/circles/"+circleID+"/rounds", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCircleRoutes_GetPayouts_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/circles/"+circleID+"/payouts", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCircleRoutes_Dispute_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/dispute", userID, map[string]any{
		"reason": "Suspicious activity",
	})
	assert.Equal(t, http.StatusNotImplemented, code)
}

func TestCircleRoutes_Dispute_MissingReason(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/dispute", userID, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestCircleRoutes_Vote_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/vote", userID, map[string]any{
		"recipientId": uuid.New().String(),
	})
	assert.Equal(t, http.StatusNotImplemented, code)
}

func TestCircleRoutes_Vote_MissingRecipient(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/vote", userID, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestCircleRoutes_AuctionBid_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/auction-bid", userID, map[string]any{
		"bidAmount": 150,
	})
	assert.Equal(t, http.StatusNotImplemented, code)
}

func TestCircleRoutes_AuctionBid_MissingAmount(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	userID := uuid.New().String()
	code, _ := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/auction-bid", userID, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)
}

// ── Invite routes ─────────────────────────────────────────────────────

func TestInviteRoutes_ListInvites_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/circles/"+circleID+"/invites", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestInviteRoutes_CreateInvite_HappyPath(t *testing.T) {
	r := setupTestRouter()
	circleID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/circles/"+circleID+"/invites", uuid.New().String(), map[string]any{
		"maxUses": 5,
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Contains(t, resp, "data")
}

func TestInviteRoutes_RevokeInvite_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "DELETE", "/v1/invites/INVITE-123", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

// ── Contribution routes ───────────────────────────────────────────────

func TestContributionRoutes_ListContributions_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/contributions", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestContributionRoutes_GetContribution_InvalidID(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "GET", "/v1/contributions/not-a-uuid", uuid.New().String(), nil)
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestContributionRoutes_GetContribution_NotFound(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "GET", "/v1/contributions/"+uuid.New().String(), uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, code)
}

// ── Payout routes ─────────────────────────────────────────────────────

func TestPayoutRoutes_ListPayouts_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/payouts", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestPayoutRoutes_GetPayout_InvalidID(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "GET", "/v1/payouts/not-a-uuid", uuid.New().String(), nil)
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestPayoutRoutes_GetPayout_NotFound(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "GET", "/v1/payouts/"+uuid.New().String(), uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, code)
}

// ── Community routes ──────────────────────────────────────────────────

func TestCommunityRoutes_Create_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "POST", "/v1/communities", uuid.New().String(), map[string]any{
		"name":     "Test Community",
		"slug":     "test-community",
		"category": "finance",
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_List_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/communities", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_Get_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/communities/"+communityID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_Update_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "PATCH", "/v1/communities/"+communityID, uuid.New().String(), map[string]any{
		"name": "Updated Community",
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_Delete_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "DELETE", "/v1/communities/"+communityID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_Join_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/communities/"+communityID+"/join", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_Leave_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/communities/"+communityID+"/leave", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_GetMembers_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/communities/"+communityID+"/members", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_IsMember_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/communities/"+communityID+"/membership", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_CreateAnnouncement_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/communities/"+communityID+"/announcements", uuid.New().String(), map[string]any{
		"content": "Hello community!",
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_GetAnnouncements_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/communities/"+communityID+"/announcements", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestCommunityRoutes_DeleteAnnouncement_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	announcementID := uuid.New().String()
	code, resp := authedRequest(t, r, "DELETE", "/v1/communities/"+communityID+"/announcements/"+announcementID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_LikeAnnouncement_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	announcementID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/communities/"+communityID+"/announcements/"+announcementID+"/like", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_PinAnnouncement_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	announcementID := uuid.New().String()
	code, resp := authedRequest(t, r, "PATCH", "/v1/communities/"+communityID+"/announcements/"+announcementID+"/pin", uuid.New().String(), map[string]any{
		"pinned": true,
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_RemoveMember_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	memberID := uuid.New().String()
	code, resp := authedRequest(t, r, "DELETE", "/v1/communities/"+communityID+"/members/"+memberID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_TransferOwnership_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/communities/"+communityID+"/transfer-ownership", uuid.New().String(), map[string]any{
		"newOwnerId": uuid.New().String(),
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestCommunityRoutes_GetActivity_HappyPath(t *testing.T) {
	r := setupTestRouter()
	communityID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/communities/"+communityID+"/activity", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

// ── Notification routes ───────────────────────────────────────────────

func TestNotificationRoutes_List_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/notifications", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestNotificationRoutes_MarkRead_HappyPath(t *testing.T) {
	r := setupTestRouter()
	notifID := uuid.New().String()
	code, resp := authedRequest(t, r, "PATCH", "/v1/notifications/"+notifID+"/read", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestNotificationRoutes_MarkAllRead_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "PATCH", "/v1/notifications/read-all", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestNotificationRoutes_UpdatePreferences_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "PUT", "/v1/notifications/preferences", uuid.New().String(), map[string]any{
		"channels": []string{"inapp", "email"},
		"muted":    false,
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestNotificationRoutes_UpdatePreferences_MissingChannels(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "PUT", "/v1/notifications/preferences", uuid.New().String(), map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)
}

// ── Savings routes ────────────────────────────────────────────────────

func TestSavingsRoutes_CreateGoal_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "POST", "/v1/savings/goals", uuid.New().String(), map[string]any{
		"name": "Vacation Fund", "targetAmount": 5000,
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_CreateGoal_MissingName(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "POST", "/v1/savings/goals", uuid.New().String(), map[string]any{
		"targetAmount": 5000,
	})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestSavingsRoutes_ListGoals_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/savings/goals", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_ListActiveGoals_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/savings/goals/active", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_Summary_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/savings/goals/summary", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_UpcomingObligations_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/savings/goals/obligations", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_GetGoal_HappyPath(t *testing.T) {
	r := setupTestRouter()
	goalID := uuid.New().String()
	code, resp := authedRequest(t, r, "GET", "/v1/savings/goals/"+goalID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_UpdateGoal_HappyPath(t *testing.T) {
	r := setupTestRouter()
	goalID := uuid.New().String()
	code, resp := authedRequest(t, r, "PATCH", "/v1/savings/goals/"+goalID, uuid.New().String(), map[string]any{
		"name": "Updated Goal",
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestSavingsRoutes_DeleteGoal_HappyPath(t *testing.T) {
	r := setupTestRouter()
	goalID := uuid.New().String()
	code, resp := authedRequest(t, r, "DELETE", "/v1/savings/goals/"+goalID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["success"])
}

func TestSavingsRoutes_CompleteGoal_HappyPath(t *testing.T) {
	r := setupTestRouter()
	goalID := uuid.New().String()
	code, resp := authedRequest(t, r, "POST", "/v1/savings/goals/"+goalID+"/complete", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

// ── Webhook routes ────────────────────────────────────────────────────

func TestWebhookRoutes_Register_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "POST", "/v1/webhooks", uuid.New().String(), map[string]any{
		"url": "https://example.com/webhook", "events": []string{"circle.created"},
	})
	assert.Equal(t, http.StatusCreated, code)
	assert.Contains(t, resp, "data")
}

func TestWebhookRoutes_Register_MissingURL(t *testing.T) {
	r := setupTestRouter()
	code, _ := authedRequest(t, r, "POST", "/v1/webhooks", uuid.New().String(), map[string]any{
		"events": []string{"circle.created"},
	})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestWebhookRoutes_List_HappyPath(t *testing.T) {
	r := setupTestRouter()
	code, resp := authedRequest(t, r, "GET", "/v1/webhooks", uuid.New().String(), nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp, "data")
}

func TestWebhookRoutes_Delete_NotFound(t *testing.T) {
	r := setupTestRouter()
	webhookID := uuid.New().String()
	code, _ := authedRequest(t, r, "DELETE", "/v1/webhooks/"+webhookID, uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, code)
}

// ── Unauthenticated access ────────────────────────────────────────────

// testPublicKeyPEM returns a fresh RSA public key in PEM form, matching the
// key format the JWT auth middleware expects.
func testPublicKeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
}

func TestUnauthenticatedAccess_CircleCreate(t *testing.T) {
	r := gin.New()
	r.Use(middleware.AuthMiddleware(testPublicKeyPEM(t)))
	circleHandler := handler.NewCircleHandler(&mockCircleService{}, nil, nil, nil)
	r.POST("/v1/circles", circleHandler.CreateCircle)
	code, _ := unauthRequest(t, r, "POST", "/v1/circles", map[string]any{
		"name": "Test Circle",
	})
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestUnauthenticatedAccess_Contributions(t *testing.T) {
	r := gin.New()
	r.Use(middleware.AuthMiddleware(testPublicKeyPEM(t)))
	contribHandler := handler.NewContributionHandler(&mockContribService{}, &mockContribRepo{})
	r.GET("/v1/contributions", contribHandler.ListContributions)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/contributions", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUnauthenticatedAccess_Notifications(t *testing.T) {
	r := gin.New()
	r.Use(middleware.AuthMiddleware(testPublicKeyPEM(t)))
	notifHandler := handler.NewNotificationHandler(&mockNotificationService{}, &mockUserService{})
	r.GET("/v1/notifications", notifHandler.ListNotifications)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/notifications", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
