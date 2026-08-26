package incentives

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Circle Completion Rewards

func (s *service) GrantCircleCompletionReward(ctx context.Context, userID string, circleID string) (*Incentive, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	circleUUID, err := parseUUID(circleID)
	if err != nil {
		return nil, err
	}

	// Check if user already received completion reward for this circle
	if err := ensureNoIncentive(ctx, s.repo, uid, IncentiveTypeCircleCompletion, func(inc Incentive) bool {
		return inc.ReferenceID.Valid && inc.ReferenceID.String == circleID
	}, "already received completion reward for this circle"); err != nil {
		return nil, err
	}

	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting incentive config: %w", err)
	}

	now := time.Now().UTC()
	incentive := &Incentive{
		ID:          uuid.New(),
		UserID:      uid,
		Type:        IncentiveTypeCircleCompletion,
		Status:      IncentiveStatusPending,
		Amount:      config.CircleCompletionBonus,
		Currency:    config.CircleCompletionCurrency,
		ReferenceID: sql.NullString{String: circleUUID.String(), Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return nil, fmt.Errorf("creating circle completion incentive: %w", err)
	}

	return incentive, nil
}

// Contribution Match

func (s *service) CalculateContributionMatch(ctx context.Context, userID string, amount float64) (float64, error) {
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting incentive config: %w", err)
	}

	matchAmount := amount * (config.ContributionMatchPercent / 100.0)

	// Cap at maximum match amount
	if matchAmount > config.ContributionMatchMax {
		matchAmount = config.ContributionMatchMax
	}

	return matchAmount, nil
}

func (s *service) GrantContributionMatch(ctx context.Context, userID string, circleID string, amount float64) (*Incentive, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	matchAmount, err := s.CalculateContributionMatch(ctx, userID, amount)
	if err != nil {
		return nil, err
	}

	if matchAmount <= 0 {
		return nil, fmt.Errorf("no match amount calculated")
	}

	now := time.Now().UTC()
	incentive := &Incentive{
		ID:          uuid.New(),
		UserID:      uid,
		Type:        IncentiveTypeContributionMatch,
		Status:      IncentiveStatusPending,
		Amount:      matchAmount,
		Currency:    "USDC", // Assuming USDC for matches
		ReferenceID: sql.NullString{String: circleID, Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return nil, fmt.Errorf("creating contribution match incentive: %w", err)
	}

	return incentive, nil
}

// First Deposit Bonus

func (s *service) GrantFirstDepositBonus(ctx context.Context, userID string, depositAmount float64) (*Incentive, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	// Check if user already received first deposit bonus
	if err := ensureNoIncentive(ctx, s.repo, uid, IncentiveTypeFirstDeposit, nil, "already received first deposit bonus"); err != nil {
		return nil, err
	}

	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting incentive config: %w", err)
	}

	// Check minimum deposit requirement
	if depositAmount < config.FirstDepositMinAmount {
		return nil, fmt.Errorf("deposit amount below minimum requirement")
	}

	now := time.Now().UTC()
	incentive := &Incentive{
		ID:        uuid.New(),
		UserID:    uid,
		Type:      IncentiveTypeFirstDeposit,
		Status:    IncentiveStatusPending,
		Amount:    config.FirstDepositBonus,
		Currency:  config.FirstDepositCurrency,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return nil, fmt.Errorf("creating first deposit incentive: %w", err)
	}

	return incentive, nil
}
