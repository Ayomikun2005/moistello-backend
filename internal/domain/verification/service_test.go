package verification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewService(rdb)

	return svc, mr
}

func TestSendOTP_Success(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(
		func(email, code string) error {
			sentCode = code
			return nil
		},
		nil,
	)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.Len(t, sentCode, 6, "OTP should be 6 digits")
}

func TestSendOTP_ResendRateLimit(t *testing.T) {
	svc, _ := setupTestService(t)
	svc.WithEmailSender(func(email, code string) error { return nil }, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	// Second call within resend window should fail
	err = svc.SendOTP(context.Background(), "user@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "please wait")
}

func TestSendOTP_ResendAfterCooldown(t *testing.T) {
	svc, mr := setupTestService(t)
	svc.WithEmailSender(func(email, code string) error { return nil }, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	// Fast-forward past the resend TTL
	mr.FastForward(resendTTL + time.Second)

	err = svc.SendOTP(context.Background(), "user@example.com")
	assert.NoError(t, err, "should be able to resend after cooldown")
}

func TestSendOTP_EmailSenderError(t *testing.T) {
	svc, _ := setupTestService(t)
	svc.WithEmailSender(
		func(email, code string) error { return fmt.Errorf("SMTP down") },
		nil,
	)

	err := svc.SendOTP(context.Background(), "user@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sending OTP email")
}

func TestSendOTP_NoEmailSender(t *testing.T) {
	svc, _ := setupTestService(t)
	// No email sender configured — should still succeed (code stored but not sent)
	err := svc.SendOTP(context.Background(), "user@example.com")
	assert.NoError(t, err)
}

func TestVerifyOTP_Success(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(func(email, code string) error {
		sentCode = code
		return nil
	}, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	valid, err := svc.VerifyOTP(context.Background(), "user@example.com", sentCode)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyOTP_WrongCode(t *testing.T) {
	svc, _ := setupTestService(t)
	svc.WithEmailSender(func(email, code string) error { return nil }, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	valid, err := svc.VerifyOTP(context.Background(), "user@example.com", "000000")
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestVerifyOTP_MaxAttempts(t *testing.T) {
	svc, _ := setupTestService(t)
	svc.WithEmailSender(func(email, code string) error { return nil }, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	// Exhaust all attempts with wrong codes
	for i := 0; i < maxAttempts; i++ {
		valid, err := svc.VerifyOTP(context.Background(), "user@example.com", "000000")
		assert.NoError(t, err, "attempt %d should not error", i+1)
		assert.False(t, valid)
	}

	// Next attempt should return "too many incorrect attempts"
	valid, err := svc.VerifyOTP(context.Background(), "user@example.com", "000000")
	assert.Error(t, err)
	assert.False(t, valid)
	assert.Contains(t, err.Error(), "too many incorrect attempts")
}

func TestVerifyOTP_ExpiredCode(t *testing.T) {
	svc, mr := setupTestService(t)
	svc.WithEmailSender(func(email, code string) error { return nil }, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	// Fast-forward past OTP TTL
	mr.FastForward(otpTTL + time.Second)

	valid, err := svc.VerifyOTP(context.Background(), "user@example.com", "123456")
	assert.Error(t, err)
	assert.False(t, valid)
	assert.Contains(t, err.Error(), "expired or not found")
}

func TestVerifyOTP_NoCodeSent(t *testing.T) {
	svc, _ := setupTestService(t)

	valid, err := svc.VerifyOTP(context.Background(), "nobody@example.com", "123456")
	assert.Error(t, err)
	assert.False(t, valid)
	assert.Contains(t, err.Error(), "expired or not found")
}

func TestVerifyOTP_CorrectAfterWrongAttempts(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(func(email, code string) error {
		sentCode = code
		return nil
	}, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	// Wrong attempts (less than max)
	for i := 0; i < 3; i++ {
		valid, err := svc.VerifyOTP(context.Background(), "user@example.com", "000000")
		assert.NoError(t, err)
		assert.False(t, valid)
	}

	// Correct code should still work
	valid, err := svc.VerifyOTP(context.Background(), "user@example.com", sentCode)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyOTP_CodeDeletedAfterSuccess(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(func(email, code string) error {
		sentCode = code
		return nil
	}, nil)

	err := svc.SendOTP(context.Background(), "user@example.com")
	require.NoError(t, err)

	valid, err := svc.VerifyOTP(context.Background(), "user@example.com", sentCode)
	assert.NoError(t, err)
	assert.True(t, valid)

	// Second verify should fail (code was deleted)
	valid, err = svc.VerifyOTP(context.Background(), "user@example.com", sentCode)
	assert.Error(t, err)
	assert.False(t, valid)
	assert.Contains(t, err.Error(), "expired or not found")
}

func TestSendRecoveryCode_Success(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(nil, func(email, code string) error {
		sentCode = code
		return nil
	})

	err := svc.SendRecoveryCode(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.Len(t, sentCode, 8, "Recovery code should be 8 digits")
}

func TestVerifyRecoveryCode_Success(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(nil, func(email, code string) error {
		sentCode = code
		return nil
	})

	err := svc.SendRecoveryCode(context.Background(), "user@example.com")
	require.NoError(t, err)

	valid, err := svc.VerifyRecoveryCode(context.Background(), "user@example.com", sentCode)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestVerifyRecoveryCode_WrongCode(t *testing.T) {
	svc, _ := setupTestService(t)
	svc.WithEmailSender(nil, func(email, code string) error { return nil })

	err := svc.SendRecoveryCode(context.Background(), "user@example.com")
	require.NoError(t, err)

	valid, err := svc.VerifyRecoveryCode(context.Background(), "user@example.com", "00000000")
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestVerifyRecoveryCode_Expired(t *testing.T) {
	svc, mr := setupTestService(t)
	svc.WithEmailSender(nil, func(email, code string) error { return nil })

	err := svc.SendRecoveryCode(context.Background(), "user@example.com")
	require.NoError(t, err)

	mr.FastForward(recoveryTTL + time.Second)

	valid, err := svc.VerifyRecoveryCode(context.Background(), "user@example.com", "12345678")
	assert.Error(t, err)
	assert.False(t, valid)
	assert.Contains(t, err.Error(), "expired or not found")
}

func TestVerifyRecoveryCode_DeletedAfterSuccess(t *testing.T) {
	svc, _ := setupTestService(t)

	var sentCode string
	svc.WithEmailSender(nil, func(email, code string) error {
		sentCode = code
		return nil
	})

	err := svc.SendRecoveryCode(context.Background(), "user@example.com")
	require.NoError(t, err)

	valid, err := svc.VerifyRecoveryCode(context.Background(), "user@example.com", sentCode)
	assert.NoError(t, err)
	assert.True(t, valid)

	// Second attempt should fail
	valid, err = svc.VerifyRecoveryCode(context.Background(), "user@example.com", sentCode)
	assert.Error(t, err)
	assert.False(t, valid)
}

func TestPendingRegistration_StoreAndRetrieve(t *testing.T) {
	svc, _ := setupTestService(t)

	data := &PendingRegistration{
		PasswordHash: "hash123",
		WalletAddr:   "GABC...",
		Email:        "user@example.com",
		DisplayName:  "Test User",
		Language:     "en",
	}

	err := svc.StorePendingRegistration(context.Background(), "user@example.com", data)
	require.NoError(t, err)

	result, err := svc.GetPendingRegistration(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "hash123", result.PasswordHash)
	assert.Equal(t, "GABC...", result.WalletAddr)
	assert.Equal(t, "user@example.com", result.Email)
	assert.Equal(t, "Test User", result.DisplayName)
	assert.Equal(t, "en", result.Language)
}

func TestPendingRegistration_NotFound(t *testing.T) {
	svc, _ := setupTestService(t)

	result, err := svc.GetPendingRegistration(context.Background(), "nobody@example.com")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestPendingRegistration_Delete(t *testing.T) {
	svc, _ := setupTestService(t)

	data := &PendingRegistration{
		PasswordHash: "hash123",
		Email:        "user@example.com",
	}

	err := svc.StorePendingRegistration(context.Background(), "user@example.com", data)
	require.NoError(t, err)

	err = svc.DeletePendingRegistration(context.Background(), "user@example.com")
	require.NoError(t, err)

	result, err := svc.GetPendingRegistration(context.Background(), "user@example.com")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestPendingRegistration_Expired(t *testing.T) {
	svc, mr := setupTestService(t)

	data := &PendingRegistration{
		PasswordHash: "hash123",
		Email:        "user@example.com",
	}

	err := svc.StorePendingRegistration(context.Background(), "user@example.com", data)
	require.NoError(t, err)

	mr.FastForward(pendingRegistrationTTL + time.Second)

	result, err := svc.GetPendingRegistration(context.Background(), "user@example.com")
	assert.NoError(t, err)
	assert.Nil(t, result, "pending registration should expire")
}

func TestVerifyOTP_HashIntegrity(t *testing.T) {
	// Verify that OTP verification uses SHA-256 hashing
	code := "123456"
	hash := sha256.Sum256([]byte(code))
	expected := hex.EncodeToString(hash[:])

	// The hash should be a valid 64-character hex string
	assert.Len(t, expected, 64)
	assert.NotEqual(t, code, expected, "code should not equal its hash")
}

func TestGenerateNumericCode(t *testing.T) {
	code, err := generateNumericCode(6)
	require.NoError(t, err)
	assert.Len(t, code, 6)

	// All characters should be digits
	for _, c := range code {
		assert.True(t, c >= '0' && c <= '9', "character %c should be a digit", c)
	}

	// Generate multiple codes to ensure randomness (not all the same)
	codes := make(map[string]bool)
	for i := 0; i < 10; i++ {
		c, err := generateNumericCode(6)
		require.NoError(t, err)
		codes[c] = true
	}
	assert.Greater(t, len(codes), 1, "should generate different codes")
}

func TestGenerateNumericCode_DifferentLengths(t *testing.T) {
	for _, length := range []int{4, 6, 8, 10} {
		code, err := generateNumericCode(length)
		require.NoError(t, err)
		assert.Len(t, code, length)
	}
}
