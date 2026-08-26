package wallet_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/wallet"
)

func TestParseEncryptionKey(t *testing.T) {
	// 32-byte hex key (64 hex characters)
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	parsed, err := wallet.ParseEncryptionKey(hexKey)
	require.NoError(t, err)
	assert.Len(t, parsed, 32)
	assert.Equal(t, byte(0x01), parsed[0])
	assert.Equal(t, byte(0xef), parsed[31])

	// 32-byte raw string
	raw32 := "12345678901234567890123456789012"
	parsedRaw, err := wallet.ParseEncryptionKey(raw32)
	require.NoError(t, err)
	assert.Len(t, parsedRaw, 32)
	assert.Equal(t, []byte(raw32), parsedRaw)

	// Short string derived via SHA-256
	short := "my-secret-passphrase"
	parsedShort, err := wallet.ParseEncryptionKey(short)
	require.NoError(t, err)
	expectedHash := sha256.Sum256([]byte(short))
	assert.Equal(t, expectedHash[:], parsedShort)

	// Empty key error
	_, err = wallet.ParseEncryptionKey("")
	assert.Error(t, err)
}

func TestWallet_EncryptionAndDecryption_WithConfiguredKey(t *testing.T) {
	stellarSecret := "SDJFKSDLJFKSLDJFKSLDJFKSLDJFKSLDJFKSLDJFKSLDJFKSLDJFKSLD"

	encKeyHex := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	encKey, err := hex.DecodeString(encKeyHex)
	require.NoError(t, err)

	w := &wallet.Wallet{
		ID:        "w-123",
		UserID:    "u-456",
		PublicKey: "GDABC123...",
	}

	passkeySeed := []byte("deterministic-passkey-seed-1234567890")

	// Encrypted with encKey (new standard):
	enc, nonce, err := encryptForTest(stellarSecret, encKey)
	require.NoError(t, err)
	w.EncryptedSecretKey = enc
	w.EncryptionNonce = nonce

	// Decrypt using encKey should succeed
	decrypted, err := w.DecryptSecret(encKey)
	require.NoError(t, err)
	assert.Equal(t, stellarSecret, decrypted)

	// Decrypt using encKey as primary with passkeySeed fallback should also succeed
	decryptedFallback, err := w.DecryptSecret(encKey, passkeySeed)
	require.NoError(t, err)
	assert.Equal(t, stellarSecret, decryptedFallback)

	// Decrypt with wrong key should fail
	wrongKey := []byte("wrong-key-0000000000000000000000")
	_, err = w.DecryptSecret(wrongKey)
	assert.Error(t, err)
}

func TestWallet_LegacyDecryptionFallback(t *testing.T) {
	stellarSecret := "SBGWSV24D33F7B7TYX7L6YV3K7B7TYX7L6YV3K7B7TYX7L6YV3K7B7TY"
	legacyPasskeySeed := []byte("old-passkey-seed-derived-from-email")

	// Simulate a legacy wallet encrypted with SHA256(legacyPasskeySeed)
	legacyKey := sha256.Sum256(legacyPasskeySeed)
	enc, nonce, err := encryptForTest(stellarSecret, legacyKey[:])
	require.NoError(t, err)

	legacyWallet := &wallet.Wallet{
		ID:                 "legacy-w",
		UserID:             "legacy-u",
		PublicKey:          "GLEGACY...",
		EncryptedSecretKey: enc,
		EncryptionNonce:    nonce,
	}

	newConfiguredKey := []byte("new-system-encryption-key-32byte")

	// Decrypt with only new key fails:
	_, err = legacyWallet.DecryptSecret(newConfiguredKey)
	assert.Error(t, err)

	// Decrypt with (newConfiguredKey, legacyPasskeySeed) succeeds via fallback!
	decrypted, err := legacyWallet.DecryptSecret(newConfiguredKey, legacyPasskeySeed)
	require.NoError(t, err)
	assert.Equal(t, stellarSecret, decrypted)
}

func TestWallet_DecryptSecret_NoKey(t *testing.T) {
	w := &wallet.Wallet{
		EncryptedSecretKey: []byte("some-encrypted-data"),
		EncryptionNonce:    []byte("123456789012"),
	}
	_, err := w.DecryptSecret()
	assert.Error(t, err)
}

func TestWallet_DeriveEncryptionKey(t *testing.T) {
	seed := []byte("seed-test-123")
	derived := wallet.DeriveEncryptionKey(seed)
	expectedHash := sha256.Sum256(seed)
	assert.Equal(t, hex.EncodeToString(expectedHash[:]), derived)
}

func encryptForTest(plaintext string, key []byte) ([]byte, []byte, error) {
	var aesKey [32]byte
	if len(key) == 32 {
		copy(aesKey[:], key)
	} else {
		aesKey = sha256.Sum256(key)
	}

	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	copy(nonce, []byte("123456789012"))
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}
