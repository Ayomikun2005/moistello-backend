package incentives

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/moistello/backend/pkg/apperrors"
)

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

// Referral System

func (s *service) GenerateReferralCode(ctx context.Context, userID string) (string, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return "", err
	}
	
	// Generate a unique referral code
	code := generateReferralCode(uid)
	
	// Check if referral already exists for this user
	existing, err := s.repo.FindByReferrerID(ctx, uid)
	if err == nil && len(existing) > 0 {
		return existing[0].ReferralCode, nil
	}
	
	// Create new referral record (pending until someone uses it)
	referral := &Referral{
		ID:           uuid.New(),
		ReferrerID:   uid,
		ReferredID:   uuid.Nil, // Will be set when someone uses the code
		ReferralCode: code,
		Status:       "pending",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	
	if err := s.repo.CreateReferral(ctx, referral); err != nil {
		return "", fmt.Errorf("creating referral: %w", err)
	}
	
	return code, nil
}

func (s *service) ApplyReferralCode(ctx context.Context, userID string, code string) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return err
	}
	
	// Check if user already used a referral
	existingReferral, err := s.repo.FindByReferredID(ctx, uid)
	if err == nil && existingReferral != nil {
		return fmt.Errorf("user already used a referral code")
	}
	
	// Find the referral by code
	referral, err := s.repo.FindByReferralCode(ctx, code)
	if err != nil {
		return fmt.Errorf("invalid referral code: %w", err)
	}
	
	// Cannot refer yourself
	if referral.ReferrerID == uid {
		return fmt.Errorf("cannot refer yourself")
	}
	
	// Check if referral is still available
	if referral.Status != "pending" || referral.ReferredID != uuid.Nil {
		return fmt.Errorf("referral code already used")
	}
	
	// Get config for bonus amount
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("getting incentive config: %w", err)
	}
	
	// Update referral with referred user
	referral.ReferredID = uid
	referral.Status = "completed"
	now := time.Now().UTC()
	referral.CompletedAt = sql.NullTime{Time: now, Valid: true}
	referral.UpdatedAt = now
	
	if err := s.repo.UpdateReferralStatus(ctx, referral.ID, "completed"); err != nil {
		return fmt.Errorf("updating referral status: %w", err)
	}
	
	// Grant bonus to referrer
	incentive := &Incentive{
		ID:          uuid.New(),
		UserID:      referral.ReferrerID,
		Type:        IncentiveTypeReferral,
		Status:      IncentiveStatusPending,
		Amount:      config.ReferralBonusAmount,
		Currency:    config.ReferralBonusCurrency,
		ReferenceID: sql.NullString{String: code, Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return fmt.Errorf("creating referral incentive: %w", err)
	}
	
	return nil
}

func (s *service) GetReferrals(ctx context.Context, userID string) ([]Referral, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	
	return s.repo.FindByReferrerID(ctx, uid)
}

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
	existing, err := s.repo.FindByUserIDAndType(ctx, uid, IncentiveTypeCircleCompletion)
	if err == nil {
		for _, inc := range existing {
			if inc.ReferenceID.Valid && inc.ReferenceID.String == circleID {
				return nil, fmt.Errorf("already received completion reward for this circle")
			}
		}
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

// Savings Streak Bonuses

func (s *service) RecordContribution(ctx context.Context, userID string) (*SavingsStreak, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting incentive config: %w", err)
	}
	
	now := time.Now().UTC()
	
	// Get existing streak or create new one
	streak, err := s.repo.FindStreakByUserID(ctx, uid)
	if err != nil {
		if err == ErrIncentiveNotFound {
			// Create new streak
			streak = &SavingsStreak{
				ID:        uuid.New(),
				UserID:    uid,
				CurrentStreak: 1,
				LongestStreak: 1,
				LastContributionAt: sql.NullTime{Time: now, Valid: true},
				BonusTier: 1,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.repo.CreateSavingsStreak(ctx, streak); err != nil {
				return nil, fmt.Errorf("creating savings streak: %w", err)
			}
			return streak, nil
		}
		return nil, fmt.Errorf("finding savings streak: %w", err)
	}
	
	// Check if contribution is within streak window (7 days)
	streakWindow := 7 * 24 * time.Hour
	if streak.LastContributionAt.Valid && now.Sub(streak.LastContributionAt.Time) <= streakWindow {
		// Continue streak
		streak.CurrentStreak++
		if streak.CurrentStreak > streak.LongestStreak {
			streak.LongestStreak = streak.CurrentStreak
		}
	} else {
		// Reset streak
		streak.CurrentStreak = 1
	}
	
	streak.LastContributionAt = sql.NullTime{Time: now, Valid: true}
	streak.UpdatedAt = now
	
	// Calculate bonus tier
	if streak.CurrentStreak >= config.StreakBonusTier3 {
		streak.BonusTier = 3
	} else if streak.CurrentStreak >= config.StreakBonusTier2 {
		streak.BonusTier = 2
	} else {
		streak.BonusTier = 1
	}
	
	if err := s.repo.UpdateSavingsStreak(ctx, streak); err != nil {
		return nil, fmt.Errorf("updating savings streak: %w", err)
	}
	
	return streak, nil
}

func (s *service) GrantStreakBonus(ctx context.Context, userID string) (*Incentive, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	
	streak, err := s.repo.FindStreakByUserID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("finding savings streak: %w", err)
	}
	
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting incentive config: %w", err)
	}
	
	var bonusAmount float64
	switch streak.BonusTier {
	case 3:
		bonusAmount = config.StreakBonusTier3Amount
	case 2:
		bonusAmount = config.StreakBonusTier2Amount
	case 1:
		bonusAmount = config.StreakBonusTier1Amount
	default:
		return nil, fmt.Errorf("invalid bonus tier")
	}
	
	if bonusAmount <= 0 {
		return nil, fmt.Errorf("no bonus amount for current tier")
	}
	
	// Check if bonus already granted for this tier
	existing, err := s.repo.FindByUserIDAndType(ctx, uid, IncentiveTypeSavingsStreak)
	if err == nil {
		for _, inc := range existing {
			metadata := make(map[string]interface{})
			if inc.Metadata.Valid {
				json.Unmarshal([]byte(inc.Metadata.String), &metadata)
			}
			if tier, ok := metadata["tier"].(float64); ok && int(tier) == streak.BonusTier {
				return nil, fmt.Errorf("bonus already granted for tier %d", streak.BonusTier)
			}
		}
	}
	
	now := time.Now().UTC()
	metadata := map[string]interface{}{
		"tier":            streak.BonusTier,
		"current_streak":  streak.CurrentStreak,
		"longest_streak":  streak.LongestStreak,
	}
	metadataJSON, _ := json.Marshal(metadata)
	
	incentive := &Incentive{
		ID:          uuid.New(),
		UserID:      uid,
		Type:        IncentiveTypeSavingsStreak,
		Status:      IncentiveStatusPending,
		Amount:      bonusAmount,
		Currency:    "USDC",
		Metadata:    sql.NullString{String: string(metadataJSON), Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return nil, fmt.Errorf("creating streak bonus incentive: %w", err)
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
	existing, err := s.repo.FindByUserIDAndType(ctx, uid, IncentiveTypeFirstDeposit)
	if err == nil && len(existing) > 0 {
		return nil, fmt.Errorf("already received first deposit bonus")
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
		ID:          uuid.New(),
		UserID:      uid,
		Type:        IncentiveTypeFirstDeposit,
		Status:      IncentiveStatusPending,
		Amount:      config.FirstDepositBonus,
		Currency:    config.FirstDepositCurrency,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return nil, fmt.Errorf("creating first deposit incentive: %w", err)
	}
	
	return incentive, nil
}

// General Incentive Operations

func (s *service) ClaimIncentive(ctx context.Context, userID string, incentiveID string) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return err
	}
	
	incentiveUUID, err := parseUUID(incentiveID)
	if err != nil {
		return err
	}
	
	// Get incentive
	incentive, err := s.repo.FindByID(ctx, incentiveUUID)
	if err != nil {
		return fmt.Errorf("finding incentive: %w", err)
	}
	
	// Verify ownership
	if incentive.UserID != uid {
		return fmt.Errorf("incentive does not belong to user")
	}
	
	// Check if already claimed
	if incentive.Status == IncentiveStatusClaimed {
		return fmt.Errorf("incentive already claimed")
	}
	
	// Check if expired
	if incentive.ExpiresAt.Valid && time.Now().UTC().After(incentive.ExpiresAt.Time) {
		return fmt.Errorf("incentive has expired")
	}
	
	// Update status to claimed
	if err := s.repo.UpdateIncentiveStatus(ctx, incentiveUUID, IncentiveStatusClaimed); err != nil {
		return fmt.Errorf("claiming incentive: %w", err)
	}
	
	return nil
}

func (s *service) GetUserIncentives(ctx context.Context, userID string) ([]Incentive, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	
	return s.repo.FindByUserID(ctx, uid)
}

func (s *service) GetPendingIncentives(ctx context.Context, userID string) ([]Incentive, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	
	return s.repo.GetPendingIncentives(ctx, uid)
}

func (s *service) GetUserSummary(ctx context.Context, userID string) (*UserIncentiveSummary, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	
	return s.repo.GetUserIncentiveSummary(ctx, uid)
}

// Admin Configuration

func (s *service) GetConfig(ctx context.Context) (*IncentiveConfig, error) {
	return s.repo.GetConfig(ctx)
}

func (s *service) UpdateConfig(ctx context.Context, config *IncentiveConfig) error {
	config.UpdatedAt = time.Now().UTC()
	
	// Try to update existing config
	err := s.repo.UpdateConfig(ctx, config)
	if err == apperrors.ErrNotFound {
		// Create new config if none exists
		config.ID = uuid.New()
		config.CreatedAt = time.Now().UTC()
		return s.repo.CreateConfig(ctx, config)
	}
	
	return err
}

// Helper function to generate referral code
func generateReferralCode(userID uuid.UUID) string {
	// Simple implementation: use first 8 characters of userID
	return userID.String()[:8]
}
