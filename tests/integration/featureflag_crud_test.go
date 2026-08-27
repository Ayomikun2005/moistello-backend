package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/featureflag"
)

// inMemoryFFRepo is a lightweight in-memory implementation of featureflag.Repository
// for integration tests that don't require a PostgreSQL connection.
type inMemoryFFRepo struct {
	flags map[string]*featureflag.FeatureFlag
}

func newInMemoryFFRepo() *inMemoryFFRepo {
	return &inMemoryFFRepo{flags: make(map[string]*featureflag.FeatureFlag)}
}

func (r *inMemoryFFRepo) Get(ctx context.Context, flag string) (*featureflag.FeatureFlag, error) {
	f, ok := r.flags[flag]
	if !ok {
		return nil, assert.AnError
	}
	return f, nil
}

func (r *inMemoryFFRepo) List(ctx context.Context) ([]featureflag.FeatureFlag, error) {
	result := make([]featureflag.FeatureFlag, 0, len(r.flags))
	for _, f := range r.flags {
		result = append(result, *f)
	}
	return result, nil
}

func (r *inMemoryFFRepo) Upsert(ctx context.Context, flag string, enabled bool, description string) error {
	if existing, ok := r.flags[flag]; ok {
		existing.Enabled = enabled
		if description != "" {
			existing.Description = description
		}
		return nil
	}
	r.flags[flag] = &featureflag.FeatureFlag{
		ID:          len(r.flags) + 1,
		Flag:        flag,
		Enabled:     enabled,
		Description: description,
	}
	return nil
}

// ── Feature Flag CRUD Integration Tests ───────────────────────────────

// TestFeatureFlagCRUD_CreateAndRead verifies creating and reading feature flags.
func TestFeatureFlagCRUD_CreateAndRead(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryFFRepo()
	svc := featureflag.NewService(repo)

	// Create a feature flag
	ff, err := svc.Set(ctx, "auction_payouts", true, "Enable auction-based payout type")
	require.NoError(t, err)
	require.NotNil(t, ff)
	assert.Equal(t, "auction_payouts", ff.Flag)
	assert.True(t, ff.Enabled)
	assert.Equal(t, "Enable auction-based payout type", ff.Description)

	// Read it back
	enabled, err := svc.IsEnabled(ctx, "auction_payouts")
	require.NoError(t, err)
	assert.True(t, enabled)

	// Disable it
	ff, err = svc.Set(ctx, "auction_payouts", false, "")
	require.NoError(t, err)
	assert.False(t, ff.Enabled)

	// Verify it's disabled
	enabled, err = svc.IsEnabled(ctx, "auction_payouts")
	require.NoError(t, err)
	assert.False(t, enabled)
}

// TestFeatureFlagCRUD_ListAll verifies listing all feature flags.
func TestFeatureFlagCRUD_ListAll(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryFFRepo()
	svc := featureflag.NewService(repo)

	// Create multiple flags
	_, err := svc.Set(ctx, "flag_a", true, "First flag")
	require.NoError(t, err)
	_, err = svc.Set(ctx, "flag_b", false, "Second flag")
	require.NoError(t, err)
	_, err = svc.Set(ctx, "flag_c", true, "Third flag")
	require.NoError(t, err)

	// List all
	flags, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, flags, 3)

	// Verify each flag has correct state
	foundFlags := make(map[string]bool)
	for _, f := range flags {
		foundFlags[f.Flag] = f.Enabled
	}
	assert.True(t, foundFlags["flag_a"])
	assert.False(t, foundFlags["flag_b"])
	assert.True(t, foundFlags["flag_c"])
}

// TestFeatureFlagCRUD_UpdateExisting verifies updating an existing flag preserves ID.
func TestFeatureFlagCRUD_UpdateExisting(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryFFRepo()
	svc := featureflag.NewService(repo)

	// Create
	ff, err := svc.Set(ctx, "premium_circles", false, "Enable premium circle type")
	require.NoError(t, err)
	origID := ff.ID

	// Update to enabled
	ff, err = svc.Set(ctx, "premium_circles", true, "Updated description")
	require.NoError(t, err)
	assert.Equal(t, origID, ff.ID) // ID should be preserved
	assert.True(t, ff.Enabled)
	assert.Equal(t, "Updated description", ff.Description)
}

// TestFeatureFlagCRUD_EmptyFlagNameRejected verifies validation of required fields.
func TestFeatureFlagCRUD_EmptyFlagNameRejected(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryFFRepo()
	svc := featureflag.NewService(repo)

	_, err := svc.Set(ctx, "", true, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flag name is required")
}

// TestFeatureFlagCRUD_NonexistentFlagReturnsError verifies error handling for missing flags.
func TestFeatureFlagCRUD_NonexistentFlagReturnsError(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryFFRepo()
	svc := featureflag.NewService(repo)

	// IsEnabled on a non-existent flag returns an error
	_, err := svc.IsEnabled(ctx, "nonexistent_flag")
	assert.Error(t, err)
}
