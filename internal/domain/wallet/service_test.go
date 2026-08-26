package wallet

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestService builds a wallet service for unit tests. DeriveWalletSeed does
// not touch the repository or Horizon, so a nil repo and a throwaway master
// keypair suffice.
func newTestService(t *testing.T, cfg Config) Service {
	t.Helper()
	kp, err := keypair.Random()
	require.NoError(t, err)
	if cfg.MasterSecretKey == "" {
		cfg.MasterSecretKey = kp.Seed()
	}
	svc, err := NewService(nil, cfg)
	require.NoError(t, err)
	return svc
}

func TestDeriveWalletSeed_MissingPepper(t *testing.T) {
	svc := newTestService(t, Config{})

	seed, err := svc.DeriveWalletSeed(context.Background(), "user@example.com")
	assert.Error(t, err)
	assert.Empty(t, seed)
	assert.Contains(t, err.Error(), "wallet pepper is not configured")
}

func TestDeriveWalletSeed_DeterministicAndUniquePerEmail(t *testing.T) {
	svc := newTestService(t, Config{
		WalletPepper:  "test-secret-pepper-123",
		Argon2Time:    1,
		Argon2Memory:  64 * 1024,
		Argon2Threads: 4,
	})

	seed, err := svc.DeriveWalletSeed(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.Len(t, seed, 64) // 32-byte key, hex-encoded

	// Deterministic: the same email always derives the same seed.
	seed2, err := svc.DeriveWalletSeed(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.Equal(t, seed, seed2)

	// Unique per email: the email participates in the salt.
	seedOther, err := svc.DeriveWalletSeed(context.Background(), "other@example.com")
	require.NoError(t, err)
	assert.NotEqual(t, seed, seedOther)
}

func TestDeriveWalletSeed_DifferentPepperChangesSeed(t *testing.T) {
	svcA := newTestService(t, Config{WalletPepper: "pepper-a", Argon2Time: 1})
	svcB := newTestService(t, Config{WalletPepper: "pepper-b", Argon2Time: 1})

	seedA, err := svcA.DeriveWalletSeed(context.Background(), "user@example.com")
	require.NoError(t, err)
	seedB, err := svcB.DeriveWalletSeed(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.NotEqual(t, seedA, seedB)
}

func TestDeriveWalletSeed_DefaultsWhenParamsZero(t *testing.T) {
	// Zero argon2 params fall back to the conservative defaults rather than
	// failing or producing degenerate output.
	svc := newTestService(t, Config{WalletPepper: "pepper"})

	seed, err := svc.DeriveWalletSeed(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.Len(t, seed, 64)
}
