package auth_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/moistello/backend/internal/domain/auth"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestTokenPairStructure(t *testing.T) {
	tp := auth.TokenPair{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-def",
	}
	assert.NotEmpty(t, tp.AccessToken)
	assert.NotEmpty(t, tp.RefreshToken)
}

func TestNonceStructure(t *testing.T) {
	now := time.Now().UTC()
	n := auth.Nonce{
		WalletAddress: "GABC...",
		Nonce:         "abc123",
		ExpiresAt:     now.Add(5 * time.Minute),
	}
	assert.Equal(t, "GABC...", n.WalletAddress)
	assert.Equal(t, "abc123", n.Nonce)
	assert.True(t, n.ExpiresAt.After(now))
}

func TestJWTCustomClaimsStructure(t *testing.T) {
	claims := auth.JWTCustomClaims{
		UserID: uuid.New().String(),
		Wallet: "GABC...",
		Role:   "user",
	}
	assert.NotEmpty(t, claims.UserID)
	assert.Equal(t, "user", claims.Role)
}

func TestJWTCustomClaims_AdminRole(t *testing.T) {
	claims := auth.JWTCustomClaims{
		UserID: uuid.New().String(),
		Wallet: "GADMIN...",
		Role:   "admin",
	}
	assert.NotEmpty(t, claims.UserID)
	assert.Equal(t, "admin", claims.Role)
}

func TestSessionRole_ParsesRoleFromLastField(t *testing.T) {
	// Session data format: userID|deviceInfo|timestamp|role — deviceInfo itself
	// contains '|' (userAgent|ip), so the role must come from the LAST field.
	assert.Equal(t, "admin", auth.SessionRole("3f9d...|Mozilla/5.0 (Linux)|192.168.1.1|1724687762|admin"))
	assert.Equal(t, "user", auth.SessionRole("3f9d...|Mozilla/5.0 (Linux)|192.168.1.1|1724687762|user"))
}

func TestSessionRole_LegacySessionWithoutRole_DefaultsToUser(t *testing.T) {
	// Pre-role sessions only stored userID|deviceInfo|timestamp (last field is
	// the timestamp), so they must fall back to "user".
	assert.Equal(t, "user", auth.SessionRole("3f9d...|Mozilla/5.0 (Linux)|192.168.1.1|1724687762"))
	assert.Equal(t, "user", auth.SessionRole(""))
}

func TestSessionStructure(t *testing.T) {
	s := auth.Session{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: sha256Hex("some-token"),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	assert.NotEqual(t, uuid.Nil, s.ID)
	assert.NotEmpty(t, s.TokenHash)
}
