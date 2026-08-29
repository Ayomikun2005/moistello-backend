package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/mobilemoney"
)

type Repository struct {
	mock.Mock
}

func (m *Repository) Create(ctx context.Context, t *mobilemoney.Transaction) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *Repository) GetByID(ctx context.Context, id string) (*mobilemoney.Transaction, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mobilemoney.Transaction), args.Error(1)
}

func (m *Repository) GetByIdempotencyKey(ctx context.Context, userID, idempotencyKey string) (*mobilemoney.Transaction, error) {
	args := m.Called(ctx, userID, idempotencyKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mobilemoney.Transaction), args.Error(1)
}

func (m *Repository) UpdateStatus(ctx context.Context, id string, status mobilemoney.Status, failureReason *string) error {
	args := m.Called(ctx, id, status, failureReason)
	return args.Error(0)
}

func (m *Repository) ListPending(ctx context.Context, olderThanSeconds int) ([]mobilemoney.Transaction, error) {
	args := m.Called(ctx, olderThanSeconds)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]mobilemoney.Transaction), args.Error(1)
}
