package featureflag_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/featureflag"
	"github.com/moistello/backend/internal/domain/featureflag/mocks"
)

func TestCache_IsEnabled_UnknownFlagDefaultsFalse(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	cache := featureflag.NewCache(svc, time.Hour)

	assert.False(t, cache.IsEnabled("never_loaded"))
}

func TestCache_Refresh_PopulatesFlags(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	cache := featureflag.NewCache(svc, time.Hour)
	ctx := context.Background()

	repo.On("List", ctx).Return([]featureflag.FeatureFlag{
		{Flag: "kyc_required", Enabled: true},
		{Flag: "premium_circles", Enabled: false},
	}, nil).Once()

	require.NoError(t, cache.Refresh(ctx))
	assert.True(t, cache.IsEnabled("kyc_required"))
	assert.False(t, cache.IsEnabled("premium_circles"))
	assert.False(t, cache.IsEnabled("nonexistent"))
	repo.AssertExpectations(t)
}

func TestCache_Refresh_KeepsStaleSnapshotOnError(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	cache := featureflag.NewCache(svc, time.Hour)
	ctx := context.Background()

	repo.On("List", ctx).Return([]featureflag.FeatureFlag{
		{Flag: "kyc_required", Enabled: true},
	}, nil).Once()
	require.NoError(t, cache.Refresh(ctx))
	assert.True(t, cache.IsEnabled("kyc_required"))

	repo.On("List", ctx).Return(nil, context.DeadlineExceeded).Once()
	err := cache.Refresh(ctx)
	assert.Error(t, err)
	// Stale value from the first successful refresh must still be served.
	assert.True(t, cache.IsEnabled("kyc_required"))
	repo.AssertExpectations(t)
}

func TestCache_Start_ReloadsOnInterval(t *testing.T) {
	repo := new(mocks.Repository)
	svc := featureflag.NewService(repo)
	cache := featureflag.NewCache(svc, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo.On("List", ctx).Return([]featureflag.FeatureFlag{{Flag: "kyc_required", Enabled: false}}, nil).Once()
	require.NoError(t, cache.Start(ctx))
	assert.False(t, cache.IsEnabled("kyc_required"))

	repo.On("List", ctx).Return([]featureflag.FeatureFlag{{Flag: "kyc_required", Enabled: true}}, nil)

	require.Eventually(t, func() bool {
		return cache.IsEnabled("kyc_required")
	}, time.Second, 10*time.Millisecond, "cache should pick up the flag flip within a couple of reload intervals")

	cache.Stop()
}
