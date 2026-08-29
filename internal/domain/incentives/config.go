package incentives

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/moistello/backend/pkg/apperrors"
)

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
