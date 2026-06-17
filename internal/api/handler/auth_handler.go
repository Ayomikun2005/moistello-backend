package handler

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/internal/domain/auth"
	"github.com/moistello/backend/internal/domain/email"
	"github.com/moistello/backend/internal/domain/totp"
	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/internal/domain/verification"
	"github.com/moistello/backend/pkg/response"
	"github.com/moistello/backend/pkg/stellar"
)

type AuthHandler struct {
	authService      auth.Service
	userService      user.Service
	totpService      *totp.Service
	verificationSvc  *verification.Service
	emailSvc         *email.Service
	redisClient      *redis.Client
	userRepo         user.Repository
}

func NewAuthHandler(authSvc auth.Service, userSvc user.Service, totpSvc *totp.Service,
	verificationSvc *verification.Service, emailSvc *email.Service,
	redisClient *redis.Client, userRepo user.Repository) *AuthHandler {
	return &AuthHandler{
		authService:     authSvc,
		userService:     userSvc,
		totpService:     totpSvc,
		verificationSvc: verificationSvc,
		emailSvc:        emailSvc,
		redisClient:     redisClient,
		userRepo:        userRepo,
	}
}
// @Summary Get authentication nonce
// @Description Returns a signed nonce for wallet authentication. The nonce must be signed with the wallet's private key and sent to /auth/verify.
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body object true "Wallet address" { "walletAddress": "G..." }
// @Success 200 {object} response.Envelope{data=object{nonce=string}}
// @Failure 400 {object} response.Envelope
// @Router /auth/nonce [post]
func (h *AuthHandler) Nonce(c *gin.Context) {
	var req struct {
		WalletAddress string `json:"walletAddress" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := stellar.ValidateAddress(req.WalletAddress); err != nil {
		response.BadRequest(c, "invalid wallet address: "+err.Error())
		return
	}

	nonce, err := h.authService.GenerateNonce(c.Request.Context(), req.WalletAddress)
	if err != nil {
		response.InternalError(c, "failed to generate nonce")
		return
	}
	response.OK(c, gin.H{"nonce": nonce})
}

// @Summary Refresh JWT tokens
// @Description Exchanges a valid refresh token for a new access token and refresh token pair.
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body object true "Refresh token" { "refreshToken": "string" }
// @Success 200 {object} response.Envelope{data=object{token=string,refreshToken=string}}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	tokenPair, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Unauthorized(c, "invalid refresh token")
		return
	}
	response.OK(c, gin.H{"token": tokenPair.AccessToken, "refreshToken": tokenPair.RefreshToken})
}

// @Summary Get current user
// @Description Returns the authenticated user's profile. Requires Bearer token.
// @Tags Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=object{user=object}}
// @Failure 401 {object} response.Envelope
// @Router /auth/me [post]
func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Unauthorized(c, "user not found")
		return
	}
	response.OK(c, gin.H{"user": u})
}

// @Summary Logout
// @Description Invalidates the current session and all refresh tokens.
// @Tags Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=object{success=bool}}
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		response.Unauthorized(c, "missing or invalid token")
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	if h.redisClient != nil {
		// 1. Blocklist the access token
		if expiry, err := middleware.ExtractTokenExpiry(token); err == nil {
			middleware.BlocklistToken(ctx, h.redisClient, token, expiry)
		}

		// 2. Delete all user sessions from Redis
		if userID != "" {
			userSessionsKey := fmt.Sprintf("user:sessions:%s", userID)
			sessionHashes, err := h.redisClient.SMembers(ctx, userSessionsKey).Result()
			if err == nil {
				pipe := h.redisClient.Pipeline()
				for _, hash := range sessionHashes {
					pipe.Del(ctx, fmt.Sprintf("session:%s", hash))
				}
				pipe.Del(ctx, userSessionsKey)
				pipe.Exec(ctx)
			}

			// 3. Set blocklist key for any missed sessions
			middleware.BlocklistUserRefreshTokens(ctx, h.redisClient, userID)
		}

		// 4. If refresh token was provided in body, also delete that specific session
		var req struct {
			RefreshToken string `json:"refreshToken"`
		}
		if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
			tokenHash := sha256HashForLogout(req.RefreshToken)
			sessionKey := fmt.Sprintf("session:%s", tokenHash)
			h.redisClient.Del(ctx, sessionKey)
		}
	}

	response.OK(c, gin.H{"success": true})
}

// ──────────────────────────────────────────────
// Email OTP Registration & Login
// ──────────────────────────────────────────────

// Register sends an email OTP to begin registration.
// POST /auth/register { email }
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "valid email is required")
		return
	}

	// Check if email already exists
	existing, err := h.userService.GetByEmail(c.Request.Context(), req.Email)
	if err == nil && existing != nil {
		response.Conflict(c, "email already registered. please log in.")
		return
	}

	code, err := h.verificationSvc.SendOTP(c.Request.Context(), req.Email)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if h.emailSvc != nil {
		if err := h.emailSvc.SendOTP(req.Email, code); err != nil {
			log.Printf("Failed to send OTP email: %v", err)
		}
	}

	response.OK(c, gin.H{"message": "verification code sent", "expiresIn": 300})
}

// RegisterVerify verifies the email OTP and creates the user.
// POST /auth/register/verify { email, code }
func (h *AuthHandler) RegisterVerify(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email and 6-digit code are required")
		return
	}

	valid, err := h.verificationSvc.VerifyOTP(c.Request.Context(), req.Email, req.Code)
	if err != nil || !valid {
		response.BadRequest(c, "invalid or expired verification code")
		return
	}

	existing, err := h.userService.GetByEmail(c.Request.Context(), req.Email)
	if err == nil && existing != nil {
		response.Conflict(c, "email already registered")
		return
	}

	walletAddr := fmt.Sprintf("EMAIL:%s", req.Email)
	u, err := h.userService.Create(c.Request.Context(), walletAddr)
	if err != nil {
		response.InternalError(c, "failed to create account")
		return
	}

	email := req.Email
	u.Email = &email
	u.EmailVerified = true
	h.userRepo.Update(c.Request.Context(), u)

	pair, err := h.authService.CreateSession(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "failed to create session")
		return
	}

	response.Created(c, gin.H{
		"token": pair.AccessToken, "refreshToken": pair.RefreshToken, "user": u,
	})
}

// Login sends an OTP to the user's email for login.
// POST /auth/login { email }
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "valid email is required")
		return
	}

	_, err := h.userService.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		response.NotFound(c, "account not found. please register first.")
		return
	}

	code, err := h.verificationSvc.SendOTP(c.Request.Context(), req.Email)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if h.emailSvc != nil {
		if err := h.emailSvc.SendOTP(req.Email, code); err != nil {
			log.Printf("Failed to send OTP email: %v", err)
		}
	}

	response.OK(c, gin.H{"message": "verification code sent", "expiresIn": 300})
}

// LoginVerify verifies the email OTP and returns tokens.
// POST /auth/login/verify { email, code }
func (h *AuthHandler) LoginVerify(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email and 6-digit code are required")
		return
	}

	valid, err := h.verificationSvc.VerifyOTP(c.Request.Context(), req.Email, req.Code)
	if err != nil || !valid {
		response.BadRequest(c, "invalid or expired verification code")
		return
	}

	u, err := h.userService.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		response.NotFound(c, "account not found")
		return
	}

	pair, err := h.authService.CreateSession(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "failed to create session")
		return
	}

	response.OK(c, gin.H{
		"token": pair.AccessToken, "refreshToken": pair.RefreshToken, "user": u,
	})
}

// ──────────────────────────────────────────────
// Optional TOTP 2FA (Settings only)
// ──────────────────────────────────────────────

// SetupTOTP generates a new TOTP secret for an authenticated user.
// POST /auth/totp/setup [AUTH]
func (h *AuthHandler) SetupTOTP(c *gin.Context) {
	userID := middleware.GetUserID(c)
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	email := ""
	if u.Email != nil {
		email = *u.Email
	}

	totpSecret, totpURI, err := h.totpService.GenerateSecret(email)
	if err != nil {
		response.InternalError(c, "failed to generate TOTP secret")
		return
	}

	u.TOTPSecret = totpSecretString(totpSecret)
	u.TOTPEnabled = false
	h.userRepo.Update(c.Request.Context(), u)

	response.OK(c, gin.H{"totpSecret": totpSecret, "totpUri": totpURI})
}

// VerifyTOTPSetup confirms TOTP setup and generates backup codes.
// POST /auth/totp/verify [AUTH] { totpCode }
func (h *AuthHandler) VerifyTOTPSetup(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req struct {
		TOTPCode string `json:"totpCode" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "6-digit TOTP code is required")
		return
	}

	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	if !u.TOTPSecret.Valid {
		response.BadRequest(c, "TOTP not set up. use /auth/totp/setup first.")
		return
	}
	if !h.totpService.ValidateCode(u.TOTPSecret.String, req.TOTPCode) {
		response.BadRequest(c, "invalid TOTP code")
		return
	}

	backupCodes, err := h.totpService.GenerateBackupCodes()
	if err != nil {
		response.InternalError(c, "failed to generate backup codes")
		return
	}

	u.TOTPEnabled = true
	u.BackupCodes = h.totpService.HashBackupCodes(backupCodes)
	h.userRepo.Update(c.Request.Context(), u)

	plainCodes := make([]string, len(backupCodes))
	for i, bc := range backupCodes {
		plainCodes[i] = bc.Plain
	}
	response.OK(c, gin.H{"backupCodes": plainCodes})
}

// Recovery uses a backup code to log in (bypasses TOTP).
// POST /auth/recovery { email, backupCode }
func (h *AuthHandler) Recovery(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required,email"`
		BackupCode string `json:"backupCode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email and backup code are required")
		return
	}

	u, err := h.userService.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		response.NotFound(c, "account not found")
		return
	}
	if len(u.BackupCodes) == 0 {
		response.BadRequest(c, "no backup codes remaining")
		return
	}

	remaining, valid := h.totpService.ValidateBackupCode(req.BackupCode, u.BackupCodes)
	if !valid {
		response.BadRequest(c, "invalid backup code")
		return
	}

	u.BackupCodes = remaining
	h.userRepo.Update(c.Request.Context(), u)

	pair, err := h.authService.CreateSession(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "failed to create session")
		return
	}

	response.OK(c, gin.H{
		"token": pair.AccessToken, "refreshToken": pair.RefreshToken, "user": u,
	})
}

// totpSecretString wraps a TOTP secret as sql.NullString.
func totpSecretString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// getPasskeyPepper returns the passkey pepper used for wallet seed derivation.
// In production, set MOISTELLO_PASSKEY_PEPPER environment variable.
func getPasskeyPepper() string {
	p := os.Getenv("MOISTELLO_PASSKEY_PEPPER")
	if p != "" {
		return p
	}
	return "moistello-local-dev"
}

// sha256HashForLogout computes SHA-256 for refresh token session lookup.
func sha256HashForLogout(s string) string {
	hash := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", hash)
}
