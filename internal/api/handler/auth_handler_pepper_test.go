package handler

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/config"
)

func TestDeriveWalletSeed_MissingPepper(t *testing.T) {
	h := NewAuthHandler(nil, nil, nil, nil, nil, nil, nil, nil)

	seed, err := h.deriveWalletSeed("user@example.com")
	assert.Error(t, err)
	assert.Empty(t, seed)
	assert.Contains(t, err.Error(), "wallet pepper is not configured")
}

func TestDeriveWalletSeed_Success(t *testing.T) {
	h := NewAuthHandler(nil, nil, nil, nil, nil, nil, nil, nil, config.SecurityConfig{
		WalletPepper: "test-secret-pepper-123",
	})

	seed, err := h.deriveWalletSeed("user@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, seed)

	// Deterministic test
	seed2, err := h.deriveWalletSeed("user@example.com")
	require.NoError(t, err)
	assert.Equal(t, seed, seed2)

	// Unique per email test
	seedOther, err := h.deriveWalletSeed("other@example.com")
	require.NoError(t, err)
	assert.NotEqual(t, seed, seedOther)
}

func TestGetPasskeyPepper_MissingPepper(t *testing.T) {
	origPepper := os.Getenv("MOISTELLO_PASSKEY_PEPPER")
	os.Unsetenv("MOISTELLO_PASSKEY_PEPPER")
	defer func() {
		if origPepper != "" {
			os.Setenv("MOISTELLO_PASSKEY_PEPPER", origPepper)
		} else {
			os.Unsetenv("MOISTELLO_PASSKEY_PEPPER")
		}
	}()

	pepper, err := getPasskeyPepper()
	assert.Error(t, err)
	assert.Empty(t, pepper)
	assert.Contains(t, err.Error(), "MOISTELLO_PASSKEY_PEPPER environment variable is not set")
}

func TestGetPasskeyPepper_Success(t *testing.T) {
	origPepper := os.Getenv("MOISTELLO_PASSKEY_PEPPER")
	expectedPepper := "test-passkey-pepper-456"
	os.Setenv("MOISTELLO_PASSKEY_PEPPER", expectedPepper)
	defer func() {
		if origPepper != "" {
			os.Setenv("MOISTELLO_PASSKEY_PEPPER", origPepper)
		} else {
			os.Unsetenv("MOISTELLO_PASSKEY_PEPPER")
		}
	}()

	pepper, err := getPasskeyPepper()
	require.NoError(t, err)
	assert.Equal(t, expectedPepper, pepper)
}
