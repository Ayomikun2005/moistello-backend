package config_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/config"
)

// validTestKeypair returns a freshly generated, checksum-valid Stellar keypair
// so that the format/length validation added in #227 passes.
func validTestKeypair(t *testing.T) (secret, public string) {
	t.Helper()
	kp, err := keypair.Random()
	require.NoError(t, err)
	return kp.Seed(), kp.Address()
}

const (
	longWalletPepper  = "wallet-pepper-0123456789abcdef0123456789"
	longPasskeyPepper = "passkey-pepper-0123456789abcdef0123456789"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	secret, public := validTestKeypair(t)
	t.Setenv("MOISTELLO_DATABASE_URL", "postgres://localhost:5432/db")
	t.Setenv("MOISTELLO_STELLAR_MASTER_SECRET_KEY", secret)
	t.Setenv("MOISTELLO_STELLAR_MASTER_PUBLIC_KEY", public)
	t.Setenv("MOISTELLO_WALLET_PEPPER", longWalletPepper)
	t.Setenv("MOISTELLO_PASSKEY_PEPPER", longPasskeyPepper)
	t.Setenv("ENCRYPTION_KEY", hex.EncodeToString([]byte("12345678901234567890123456789012")))
	t.Setenv("JWT_PRIVATE_KEY", "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBALs=\n-----END RSA PRIVATE KEY-----")
	t.Setenv("JWT_PUBLIC_KEY", "-----BEGIN RSA PUBLIC KEY-----\nMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBALs=\n-----END RSA PUBLIC KEY-----")
}

// loadPanicsWith asserts that config.Load panics with a message containing substr.
func loadPanicsWith(t *testing.T, substr string) {
	t.Helper()
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected config.Load to panic")
		require.Contains(t, fmt.Sprint(r), substr)
	}()
	config.Load("")
}

func TestLoad_SucceedsWithRequiredConfig(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, "postgres://localhost:5432/db", cfg.Database.URL)
	require.Equal(t, longWalletPepper, cfg.Security.WalletPepper)
	require.Equal(t, longPasskeyPepper, cfg.Security.PasskeyPepper)
	require.NotEmpty(t, cfg.Auth.JWTPrivateKeyPEM)
	require.NotEmpty(t, cfg.Auth.JWTPublicKeyPEM)
}

func TestLoad_PanicsWithoutCriticalConfig(t *testing.T) {
	t.Setenv("MOISTELLO_DATABASE_URL", "")
	t.Setenv("MOISTELLO_STELLAR_MASTER_SECRET_KEY", "")
	t.Setenv("MOISTELLO_STELLAR_MASTER_PUBLIC_KEY", "")
	t.Setenv("MOISTELLO_WALLET_PEPPER", "")
	t.Setenv("MOISTELLO_PASSKEY_PEPPER", "")
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("JWT_PRIVATE_KEY", "")
	t.Setenv("JWT_PUBLIC_KEY", "")

	require.Panics(t, func() {
		config.Load("")
	})
}

func TestLoad_PanicsOnMalformedMasterSecretKey(t *testing.T) {
	setRequiredEnv(t)
	// Not a valid S-prefixed strkey (bad length + bad checksum).
	t.Setenv("MOISTELLO_STELLAR_MASTER_SECRET_KEY", "SAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	loadPanicsWith(t, "stellar.master_secret_key must be a valid Stellar secret key")
}

func TestLoad_PanicsOnMalformedMasterPublicKey(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MOISTELLO_STELLAR_MASTER_PUBLIC_KEY", "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	loadPanicsWith(t, "stellar.master_public_key must be a valid Stellar public key")
}

func TestLoad_PanicsOnShortWalletPepper(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MOISTELLO_WALLET_PEPPER", "short")

	loadPanicsWith(t, "security.wallet_pepper must be at least")
}

func TestLoad_PanicsOnShortPasskeyPepper(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MOISTELLO_PASSKEY_PEPPER", "short")

	loadPanicsWith(t, "security.passkey_pepper must be at least")
}
