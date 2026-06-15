package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/moistello/backend/pkg/apperrors"
)

type Service interface {
	GenerateNonce(ctx context.Context, walletAddress string) (*Nonce, error)
	VerifySignature(ctx context.Context, walletAddress, signature string) (bool, error)
	CreateSession(ctx context.Context, userID uuid.UUID) (*TokenPair, error)
	ValidateSession(ctx context.Context, refreshToken string) (*uuid.UUID, error)
	GenerateJWT(userID uuid.UUID, walletAddress, role string) (string, error)
	ValidateJWT(tokenString string) (*JWTCustomClaims, error)
	RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
}

type authService struct {
	redis         *redis.Client
	nonceTTL      time.Duration
	accessTTL     time.Duration
	refreshTTL    time.Duration
	jwtPrivateKey []byte
	jwtPublicKey  []byte
}

func NewService(redisClient *redis.Client, nonceTTL, accessTTL, refreshTTL time.Duration, jwtPrivateKeyPath, jwtPublicKeyPath string) (Service, error) {
	privateKeyPEM, err := os.ReadFile(jwtPrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading JWT private key: %w", err)
	}
	publicKeyPEM, err := os.ReadFile(jwtPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading JWT public key: %w", err)
	}
	return &authService{
		redis:         redisClient,
		nonceTTL:      nonceTTL,
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		jwtPrivateKey: privateKeyPEM,
		jwtPublicKey:  publicKeyPEM,
	}, nil
}

func (s *authService) GenerateNonce(ctx context.Context, walletAddress string) (*Nonce, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generating random nonce: %w", err)
	}
	nonceStr := hex.EncodeToString(b)

	// Store nonce with creation timestamp for clock skew tolerance
	now := time.Now().Unix()
	storedValue := fmt.Sprintf("%s:%d", nonceStr, now)
	key := fmt.Sprintf("nonce:%s", walletAddress)

	// Add 30s clock skew tolerance to the TTL
	ttl := s.nonceTTL + 30*time.Second
	if err := s.redis.Set(ctx, key, storedValue, ttl).Err(); err != nil {
		return nil, fmt.Errorf("storing nonce in redis: %w", err)
	}

	return &Nonce{
		WalletAddress: walletAddress,
		Nonce:         nonceStr,
		ExpiresAt:     time.Now().UTC().Add(s.nonceTTL),
	}, nil
}

func (s *authService) VerifySignature(ctx context.Context, walletAddress, signature string) (bool, error) {
	key := fmt.Sprintf("nonce:%s", walletAddress)
	stored, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return false, apperrors.ErrNonceExpired
		}
		return false, fmt.Errorf("retrieving nonce from redis: %w", err)
	}

	// Delete nonce immediately to prevent any replay
	s.redis.Del(ctx, key)

	// Parse nonce value and creation timestamp
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid nonce format")
	}
	nonceStr := parts[0]
	createdAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid nonce timestamp: %w", err)
	}

	// Check expiry with 30-second clock skew tolerance
	now := time.Now().Unix()
	skewTolerance := int64(30)
	if now > createdAt+int64(s.nonceTTL.Seconds())+skewTolerance {
		return false, apperrors.ErrNonceExpired
	}
	if now < createdAt-skewTolerance {
		return false, fmt.Errorf("nonce from the future — clock skew detected")
	}

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false, fmt.Errorf("decoding signature hex: %w", err)
	}

	publicKey, err := decodeStellarPublicKey(walletAddress)
	if err != nil {
		return false, fmt.Errorf("decoding public key: %w", err)
	}

	message := sha256.Sum256([]byte(nonceStr))
	valid := ed25519.Verify(publicKey, message[:], sigBytes)

	return valid, nil
}

func (s *authService) CreateSession(ctx context.Context, userID uuid.UUID) (*TokenPair, error) {
	accessToken, err := s.GenerateJWT(userID, "", "user")
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	refreshBytes := make([]byte, 64)
	if _, err := rand.Read(refreshBytes); err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}
	refreshToken := hex.EncodeToString(refreshBytes)
	tokenHash := sha256Hash(refreshToken)

	userIDStr := userID.String()

	// Store session
	sessionKey := fmt.Sprintf("session:%s", tokenHash)
	if err := s.redis.Set(ctx, sessionKey, userIDStr, s.refreshTTL).Err(); err != nil {
		return nil, fmt.Errorf("storing session in redis: %w", err)
	}

	// Index session by user for bulk operations (logout, force-invalidate)
	userSessionsKey := fmt.Sprintf("user:sessions:%s", userIDStr)
	if err := s.redis.SAdd(ctx, userSessionsKey, tokenHash).Err(); err != nil {
		log.Warn().Err(err).Msg("failed to index user session — non-fatal")
	}
	s.redis.Expire(ctx, userSessionsKey, s.refreshTTL)

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *authService) ValidateSession(ctx context.Context, refreshToken string) (*uuid.UUID, error) {
	tokenHash := sha256Hash(refreshToken)
	key := fmt.Sprintf("session:%s", tokenHash)

	userIDStr, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apperrors.ErrTokenExpired
		}
		return nil, fmt.Errorf("retrieving session from redis: %w", err)
	}

	// Check if the user's refresh tokens have been blocklisted
	blocklistKey := fmt.Sprintf("refresh:blocklist:%s", userIDStr)
	blocklisted, err := s.redis.Exists(ctx, blocklistKey).Result()
	if err != nil {
		log.Warn().Err(err).Str("userID", userIDStr).Msg("failed to check refresh blocklist")
		return nil, fmt.Errorf("session validation error")
	}
	if blocklisted > 0 {
		// Session revoked — delete it immediately
		s.redis.Del(ctx, key)
		return nil, fmt.Errorf("session revoked")
	}

	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("parsing session user ID: %w", err)
	}

	return &uid, nil
}

func (s *authService) GenerateJWT(userID uuid.UUID, walletAddress, role string) (string, error) {
	block, _ := pem.Decode(s.jwtPrivateKey)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block for private key")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		privateKey2, err2 := x509.ParseECPrivateKey(block.Bytes)
		if err2 != nil {
			rsaKey, err3 := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err3 != nil {
				return "", fmt.Errorf("parsing private key: %w (also tried EC: %v, RSA: %v)", err, err2, err3)
			}
			privateKey = rsaKey
		} else {
			privateKey = privateKey2
		}
	}

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub":    userID.String(),
		"wallet": walletAddress,
		"role":   role,
		"iat":    now.Unix(),
		"exp":    now.Add(s.accessTTL).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	return signed, nil
}

func (s *authService) ValidateJWT(tokenString string) (*JWTCustomClaims, error) {
	block, _ := pem.Decode(s.jwtPublicKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block for public key")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing public key: %w", err)
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing JWT: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, _ := claims["sub"].(string)
		wallet, _ := claims["wallet"].(string)
		role, _ := claims["role"].(string)
		return &JWTCustomClaims{
			UserID: userID,
			Wallet: wallet,
			Role:   role,
		}, nil
	}

	return nil, apperrors.ErrUnauthorized
}

func (s *authService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	uid, err := s.ValidateSession(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	// Create the NEW session first so that if this fails, the old one remains valid
	newPair, err := s.CreateSession(ctx, *uid)
	if err != nil {
		return nil, fmt.Errorf("creating new session: %w", err)
	}

	// Grace period: keep the old session alive for 60 seconds so that
	// in-flight requests using the old refresh token can still complete.
	oldTokenHash := sha256Hash(refreshToken)
	oldKey := fmt.Sprintf("session:%s", oldTokenHash)
	graceTTL := 60 * time.Second
	if err := s.redis.Expire(ctx, oldKey, graceTTL).Err(); err != nil {
		log.Warn().Err(err).Msg("failed to set old session grace period — non-fatal")
	}

	return newPair, nil
}

// stellarBase32Alphabet is the RFC 4648 Base32 alphabet used by Stellar StrKey.
const stellarBase32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

var stellarBase32Decode [256]byte

func init() {
	for i := range stellarBase32Decode {
		stellarBase32Decode[i] = 0xFF
	}
	for i, c := range stellarBase32Alphabet {
		stellarBase32Decode[c] = byte(i)
	}
}

// decodeStellarPublicKey decodes a Stellar G... address to an Ed25519 public key.
// Stellar addresses use StrKey encoding: Base32 + 1 version byte + 2 CRC16 checksum.
func decodeStellarPublicKey(address string) (ed25519.PublicKey, error) {
	if len(address) != 56 {
		return nil, fmt.Errorf("invalid stellar address length: got %d, want 56", len(address))
	}
	if address[0] != 'G' {
		return nil, fmt.Errorf("invalid stellar address prefix: got %c, want G", address[0])
	}

	// Base32 decode: 56 chars → 35 bytes (1 version + 32 key + 2 checksum)
	decoded := make([]byte, 35)
	for i := 0; i < 56; i++ {
		c := address[i]
		val := stellarBase32Decode[c]
		if val == 0xFF {
			return nil, fmt.Errorf("invalid character %c at position %d", c, i)
		}
		bitPos := uint(i * 5)
		byteIdx := bitPos / 8
		bitOffset := bitPos % 8
		decoded[byteIdx] |= val << (3 - bitOffset)
		if bitOffset > 3 {
			decoded[byteIdx+1] |= val >> (bitOffset - 3)
		}
	}

	// Verify XDR CRC-16 checksum
	payload := decoded[:33]
	checksum := decoded[33:35]
	crc := xdrCRC16(payload)
	if checksum[0] != byte(crc>>8) || checksum[1] != byte(crc) {
		return nil, fmt.Errorf("stellar address checksum mismatch")
	}

	// Strip version byte (index 0), return 32-byte public key
	return ed25519.PublicKey(decoded[1:33]), nil
}

// xdrCRC16 computes the XDR CRC-16 used by Stellar for address checksums.
func xdrCRC16(data []byte) uint16 {
	const poly uint16 = 0x8005
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
