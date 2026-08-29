package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/featureflag"
)

type Repository struct {
	mock.Mock
}

func (m *Repository) Get(ctx context.Context, flag string) (*featureflag.FeatureFlag, error) {
	args := m.Called(ctx, flag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*featureflag.FeatureFlag), args.Error(1)
}

func (m *Repository) List(ctx context.Context) ([]featureflag.FeatureFlag, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]featureflag.FeatureFlag), args.Error(1)
}

func (m *Repository) Upsert(ctx context.Context, flag string, enabled bool, description string) error {
	args := m.Called(ctx, flag, enabled, description)
	return args.Error(0)
}

func (m *Repository) Delete(ctx context.Context, flag string) error {
	args := m.Called(ctx, flag)
	return args.Error(0)
}
