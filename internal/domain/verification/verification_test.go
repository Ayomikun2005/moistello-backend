package verification_test

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/verification"
)

func TestVerificationService_WithEmailSender(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skip("Redis is not running, skipping live verification service test")
	}

	ctx := context.Background()
	svc := verification.NewService(rdb)

	var sentOTPEmail, sentOTPCode string
	var sentRecoveryEmail, sentRecoveryCode string

	svc.WithEmailSender(
		func(email, code string) error {
			sentOTPEmail = email
			sentOTPCode = code
			return nil
		},
		func(email, code string) error {
			sentRecoveryEmail = email
			sentRecoveryCode = code
			return nil
		},
	)

	testEmail := "verify-test@example.com"
	defer func() {
		rdb.Del(ctx, "otp:code:"+testEmail)
		rdb.Del(ctx, "otp:resend:"+testEmail)
		rdb.Del(ctx, "recovery:code:"+testEmail)
	}()

	// 1. Test SendOTP delivers via injected sender
	err := svc.SendOTP(ctx, testEmail)
	require.NoError(t, err)
	assert.Equal(t, testEmail, sentOTPEmail)
	assert.Len(t, sentOTPCode, 6)

	// Verify the delivered OTP code validates against Redis hash
	ok, err := svc.VerifyOTP(ctx, testEmail, sentOTPCode)
	require.NoError(t, err)
	assert.True(t, ok)

	// 2. Test SendRecoveryCode delivers via injected sender
	err = svc.SendRecoveryCode(ctx, testEmail)
	require.NoError(t, err)
	assert.Equal(t, testEmail, sentRecoveryEmail)
	assert.Len(t, sentRecoveryCode, 8)

	// Verify the delivered recovery code validates against Redis hash
	ok, err = svc.VerifyRecoveryCode(ctx, testEmail, sentRecoveryCode)
	require.NoError(t, err)
	assert.True(t, ok)
}
