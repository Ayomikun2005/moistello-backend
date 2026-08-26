package incentives

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Service is the incentives domain entry point. Implementations are split
// across themed files in this package: referral.go, bonus.go, streak.go,
// incentive.go and config.go. All grant flows share the idempotency guard in
// idempotency.go.
type Service interface {
	// Referral System
	GenerateReferralCode(ctx context.Context, userID string) (string, error)
	ApplyReferralCode(ctx context.Context, userID string, code string) error
	GetReferrals(ctx context.Context, userID string) ([]Referral, error)

	// Circle Completion Rewards
	GrantCircleCompletionReward(ctx context.Context, userID string, circleID string) (*Incentive, error)

	// Contribution Match
	CalculateContributionMatch(ctx context.Context, userID string, amount float64) (float64, error)
	GrantContributionMatch(ctx context.Context, userID string, circleID string, amount float64) (*Incentive, error)

	// Savings Streak Bonuses
	RecordContribution(ctx context.Context, userID string) (*SavingsStreak, error)
	GrantStreakBonus(ctx context.Context, userID string) (*Incentive, error)

	// First Deposit Bonus
	GrantFirstDepositBonus(ctx context.Context, userID string, depositAmount float64) (*Incentive, error)

	// General Incentive Operations
	ClaimIncentive(ctx context.Context, userID string, incentiveID string) error
	GetUserIncentives(ctx context.Context, userID string) ([]Incentive, error)
	GetPendingIncentives(ctx context.Context, userID string) ([]Incentive, error)
	GetUserSummary(ctx context.Context, userID string) (*UserIncentiveSummary, error)

	// Admin Configuration
	GetConfig(ctx context.Context) (*IncentiveConfig, error)
	UpdateConfig(ctx context.Context, config *IncentiveConfig) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func parseUUID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid uuid: %w", err)
	}
	return parsed, nil
}
