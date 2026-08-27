package incentives

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
				ID:                 uuid.New(),
				UserID:             uid,
				CurrentStreak:      1,
				LongestStreak:      1,
				LastContributionAt: sql.NullTime{Time: now, Valid: true},
				BonusTier:          1,
				CreatedAt:          now,
				UpdatedAt:          now,
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
	if err := ensureNoIncentive(ctx, s.repo, uid, IncentiveTypeSavingsStreak, func(inc Incentive) bool {
		metadata := make(map[string]interface{})
		if inc.Metadata.Valid {
			_ = json.Unmarshal([]byte(inc.Metadata.String), &metadata)
		}
		if tier, ok := metadata["tier"].(float64); ok && int(tier) == streak.BonusTier {
			return true
		}
		return false
	}, fmt.Sprintf("bonus already granted for tier %d", streak.BonusTier)); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	metadata := map[string]interface{}{
		"tier":           streak.BonusTier,
		"current_streak": streak.CurrentStreak,
		"longest_streak": streak.LongestStreak,
	}
	metadataJSON, _ := json.Marshal(metadata)

	incentive := &Incentive{
		ID:        uuid.New(),
		UserID:    uid,
		Type:      IncentiveTypeSavingsStreak,
		Status:    IncentiveStatusPending,
		Amount:    bonusAmount,
		Currency:  "USDC",
		Metadata:  sql.NullString{String: string(metadataJSON), Valid: true},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateIncentive(ctx, incentive); err != nil {
		return nil, fmt.Errorf("creating streak bonus incentive: %w", err)
	}

	return incentive, nil
}
