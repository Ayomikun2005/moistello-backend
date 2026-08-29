package featureflag_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/featureflag"
	"github.com/moistello/backend/internal/domain/featureflag/mocks"
	"github.com/moistello/backend/pkg/apperrors"
)

func TestService_IsEnabled(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	ctx := context.Background()

	repo.On("Get", ctx, "kyc_required").Return(&featureflag.FeatureFlag{Flag: "kyc_required", Enabled: true}, nil)

	enabled, err := svc.IsEnabled(ctx, "kyc_required")
	require.NoError(t, err)
	assert.True(t, enabled)
	repo.AssertExpectations(t)
}

func TestService_IsEnabled_NotFound(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	ctx := context.Background()

	repo.On("Get", ctx, "unknown_flag").Return(nil, apperrors.ErrNotFound)

	_, err := svc.IsEnabled(ctx, "unknown_flag")
	assert.Error(t, err)
	repo.AssertExpectations(t)
}

func TestService_Set_CreatesOrUpdates(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	ctx := context.Background()

	repo.On("Upsert", ctx, "premium_circles", true, "Enable premium circle type").Return(nil)
	repo.On("Get", ctx, "premium_circles").Return(&featureflag.FeatureFlag{Flag: "premium_circles", Enabled: true, Description: "Enable premium circle type"}, nil)

	f, err := svc.Set(ctx, "premium_circles", true, "Enable premium circle type")
	require.NoError(t, err)
	assert.True(t, f.Enabled)
	repo.AssertExpectations(t)
}

func TestService_Set_RequiresFlagName(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)

	_, err := svc.Set(context.Background(), "", true, "")
	assert.Error(t, err)
	repo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestService_List(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	ctx := context.Background()

	repo.On("List", ctx).Return([]featureflag.FeatureFlag{
		{Flag: "auction_payouts", Enabled: false},
		{Flag: "vote_payouts", Enabled: true},
	}, nil)

	flags, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, flags, 2)
	repo.AssertExpectations(t)
}

func TestService_Delete(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	ctx := context.Background()

	repo.On("Delete", ctx, "premium_circles").Return(nil)

	err := svc.Delete(ctx, "premium_circles")
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestService_Delete_RequiresFlagName(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)

	err := svc.Delete(context.Background(), "")
	assert.Error(t, err)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}
