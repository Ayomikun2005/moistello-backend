package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func generateTestRSAKeys(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privDER := x509.MarshalPKCS1PrivateKey(privateKey)
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	})

	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	return string(privPEM), string(pubPEM)
}

func TestCreateSession_AtomicWrites(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	privPEM, pubPEM := generateTestRSAKeys(t)
	svc, err := auth.NewService(rdb, 5*time.Minute, 15*time.Minute, 7*24*time.Hour, privPEM, pubPEM)
	require.NoError(t, err)

	userID := uuid.New()
	ctx := context.Background()

	tp, err := svc.CreateSession(ctx, userID, 15*time.Minute, "mobile-app")
	require.NoError(t, err)
	require.NotNil(t, tp)
	assert.NotEmpty(t, tp.AccessToken)
	assert.NotEmpty(t, tp.RefreshToken)
	assert.NotEmpty(t, tp.CSRFToken)

	// Verify session was written
	tokenHash := sha256Hex(tp.RefreshToken)
	sessionKey := "session:" + tokenHash
	sessionVal, err := rdb.Get(ctx, sessionKey).Result()
	require.NoError(t, err)
	assert.Contains(t, sessionVal, userID.String())
	assert.Contains(t, sessionVal, "mobile-app")

	// Verify CSRF was written
	csrfHash := sha256.Sum256([]byte(tp.AccessToken))
	csrfKey := fmt.Sprintf("csrf:%x", csrfHash)
	csrfVal, err := rdb.Get(ctx, csrfKey).Result()
	require.NoError(t, err)
	assert.Equal(t, tp.CSRFToken, csrfVal)

	// Verify user session set was indexed
	userSessionsKey := "user:sessions:" + userID.String()
	isMember, err := rdb.SIsMember(ctx, userSessionsKey, tokenHash).Result()
	require.NoError(t, err)
	assert.True(t, isMember)
}

func TestCreateSession_RollbackOnFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	privPEM, pubPEM := generateTestRSAKeys(t)
	svc, err := auth.NewService(rdb, 5*time.Minute, 15*time.Minute, 7*24*time.Hour, privPEM, pubPEM)
	require.NoError(t, err)

	// Close miniredis to induce pipeline execution failure
	mr.Close()
	rdb.Close()

	userID := uuid.New()
	ctx := context.Background()

	tp, err := svc.CreateSession(ctx, userID, 15*time.Minute, "mobile-app")
	assert.Error(t, err)
	assert.Nil(t, tp)
	assert.Contains(t, err.Error(), "storing session and CSRF in redis")
}
