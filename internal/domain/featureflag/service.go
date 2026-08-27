package featureflag

import (
	"context"
	"fmt"
)

// Service provides business logic for feature flag management.
type Service interface {
	IsEnabled(ctx context.Context, flag string) (bool, error)
	List(ctx context.Context) ([]FeatureFlag, error)
	Set(ctx context.Context, flag string, enabled bool, description string) (*FeatureFlag, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) IsEnabled(ctx context.Context, flag string) (bool, error) {
	f, err := s.repo.Get(ctx, flag)
	if err != nil {
		return false, fmt.Errorf("checking feature flag %s: %w", flag, err)
	}
	return f.Enabled, nil
}

func (s *service) List(ctx context.Context) ([]FeatureFlag, error) {
	return s.repo.List(ctx)
}

func (s *service) Set(ctx context.Context, flag string, enabled bool, description string) (*FeatureFlag, error) {
	if flag == "" {
		return nil, fmt.Errorf("flag name is required")
	}
	if err := s.repo.Upsert(ctx, flag, enabled, description); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, flag)
}
