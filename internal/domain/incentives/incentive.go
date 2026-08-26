package incentives

import (
	"context"
	"fmt"
	"time"
)

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
