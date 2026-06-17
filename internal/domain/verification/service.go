package verification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	otpTTL       = 5 * time.Minute
	resendTTL    = 60 * time.Second
	maxAttempts  = 5
	recoveryTTL  = 15 * time.Minute
)

// Service handles email verification OTP codes stored in Redis.
type Service struct {
	rdb *redis.Client
}

func NewService(rdb *redis.Client) *Service {
	return &Service{rdb: rdb}
}

// SendOTP generates a 6-digit code and stores it in Redis.
// Returns the code on success (for logging/development; in production the code is emailed).
func (s *Service) SendOTP(ctx context.Context, email string) (string, error) {
	// Rate limit: check if a code was sent recently
	resendKey := fmt.Sprintf("otp:resend:%s", email)
	exists, err := s.rdb.Exists(ctx, resendKey).Result()
	if err != nil {
		return "", fmt.Errorf("checking resend rate limit: %w", err)
	}
	if exists > 0 {
		return "", fmt.Errorf("please wait before requesting another code")
	}

	// Generate a 6-digit code
	code, err := generateNumericCode(6)
	if err != nil {
		return "", fmt.Errorf("generating OTP code: %w", err)
	}

	// Store hashed code in Redis
	codeHash := sha256.Sum256([]byte(code))
	hashedCode := hex.EncodeToString(codeHash[:])
	otpKey := fmt.Sprintf("otp:code:%s", email)
	otpValue := fmt.Sprintf("%s:0", hashedCode) // hash:attempts
	if err := s.rdb.Set(ctx, otpKey, otpValue, otpTTL).Err(); err != nil {
		return "", fmt.Errorf("storing OTP: %w", err)
	}

	// Set resend rate limit
	s.rdb.Set(ctx, resendKey, "1", resendTTL)

	return code, nil
}

// VerifyOTP checks a 6-digit code against the stored hash.
// Returns true and marks email as verified, or false if invalid/expired.
func (s *Service) VerifyOTP(ctx context.Context, email, code string) (bool, error) {
	otpKey := fmt.Sprintf("otp:code:%s", email)
	stored, err := s.rdb.Get(ctx, otpKey).Result()
	if err == redis.Nil {
		return false, fmt.Errorf("OTP code expired or not found")
	}
	if err != nil {
		return false, fmt.Errorf("reading OTP: %w", err)
	}

	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		s.rdb.Del(ctx, otpKey)
		return false, fmt.Errorf("invalid OTP format")
	}

	attempts, _ := strconv.Atoi(parts[1])
	if attempts >= maxAttempts {
		s.rdb.Del(ctx, otpKey)
		return false, fmt.Errorf("too many incorrect attempts. request a new code")
	}

	codeHash := sha256.Sum256([]byte(code))
	hashedCode := hex.EncodeToString(codeHash[:])

	if hashedCode == parts[0] {
		s.rdb.Del(ctx, otpKey)
		return true, nil
	}

	// Increment attempts
	newValue := fmt.Sprintf("%s:%d", parts[0], attempts+1)
	s.rdb.Set(ctx, otpKey, newValue, otpTTL)
	return false, nil
}

// SendRecoveryCode generates a one-time recovery code for backup code flow.
func (s *Service) SendRecoveryCode(ctx context.Context, email string) (string, error) {
	code, err := generateNumericCode(8)
	if err != nil {
		return "", err
	}

	codeHash := sha256.Sum256([]byte(code))
	recoveryKey := fmt.Sprintf("recovery:code:%s", email)
	if err := s.rdb.Set(ctx, recoveryKey, hex.EncodeToString(codeHash[:]), recoveryTTL).Err(); err != nil {
		return "", fmt.Errorf("storing recovery code: %w", err)
	}

	return code, nil
}

// VerifyRecoveryCode checks an 8-digit recovery code.
func (s *Service) VerifyRecoveryCode(ctx context.Context, email, code string) (bool, error) {
	recoveryKey := fmt.Sprintf("recovery:code:%s", email)
	stored, err := s.rdb.Get(ctx, recoveryKey).Result()
	if err == redis.Nil {
		return false, fmt.Errorf("recovery code expired or not found")
	}
	if err != nil {
		return false, fmt.Errorf("reading recovery code: %w", err)
	}

	codeHash := sha256.Sum256([]byte(code))
	if hex.EncodeToString(codeHash[:]) == stored {
		s.rdb.Del(ctx, recoveryKey)
		return true, nil
	}
	return false, nil
}

func generateNumericCode(length int) (string, error) {
	code := make([]byte, length)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generating random digit: %w", err)
		}
		code[i] = byte('0') + byte(n.Int64())
	}
	return string(code), nil
}
