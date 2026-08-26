package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/savings"
)

type Repository struct {
	mock.Mock
}

func (m *Repository) Create(ctx context.Context, g *savings.SavingsGoal) error {
	return m.Called(ctx, g).Error(0)
}

func (m *Repository) FindByID(ctx context.Context, id string) (*savings.SavingsGoal, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*savings.SavingsGoal), args.Error(1)
}

func (m *Repository) FindByUserID(ctx context.Context, userID string) ([]savings.SavingsGoal, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]savings.SavingsGoal), args.Error(1)
}

func (m *Repository) FindActiveByUserID(ctx context.Context, userID string) ([]savings.SavingsGoal, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]savings.SavingsGoal), args.Error(1)
}

func (m *Repository) FindCompletedByUserID(ctx context.Context, userID string) ([]savings.SavingsGoal, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]savings.SavingsGoal), args.Error(1)
}

func (m *Repository) Update(ctx context.Context, g *savings.SavingsGoal) error {
	return m.Called(ctx, g).Error(0)
}

func (m *Repository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *Repository) GetSummary(ctx context.Context, userID string) (*savings.GoalSummary, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*savings.GoalSummary), args.Error(1)
}

func (m *Repository) GetUpcomingObligations(ctx context.Context, userID string, limit int) ([]savings.SavingsGoal, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]savings.SavingsGoal), args.Error(1)
}