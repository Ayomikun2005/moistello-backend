package handler

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/internal/domain/auth"
	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/pkg/response"
	"github.com/moistello/backend/pkg/stellar"
)

type AuthHandler struct {
	authService auth.Service
	userService user.Service
	redisClient *redis.Client
}

func NewAuthHandler(authSvc auth.Service, userSvc user.Service, redisClient *redis.Client) *AuthHandler {
	return &AuthHandler{
		authService: authSvc,
		userService: userSvc,
		redisClient: redisClient,
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

// @Summary Login with existing wallet
// @Description Verifies a signed nonce to prove wallet ownership. Requires the user to already exist (register first).
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body object true "Signature payload" { "walletAddress": "G...", "signature": "base64_signed_nonce" }
// @Success 200 {object} response.Envelope{data=object{token=string,refreshToken=string,user=object}}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /auth/verify [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		WalletAddress string `json:"walletAddress" binding:"required"`
		Signature     string `json:"signature" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := stellar.ValidateAddress(req.WalletAddress); err != nil {
		response.BadRequest(c, "invalid wallet address: "+err.Error())
		return
	}

	valid, err := h.authService.VerifySignature(c.Request.Context(), req.WalletAddress, req.Signature)
	if err != nil || !valid {
		response.Unauthorized(c, "signature verification failed")
		return
	}
	u, err := h.userService.GetByWallet(c.Request.Context(), req.WalletAddress)
	if err != nil {
		if err == user.ErrUserNotFound {
			response.NotFound(c, "account not found. please register first.")
			return
		}
		response.InternalError(c, "failed to find user")
		return
	}
	tokenPair, err := h.authService.CreateSession(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "failed to create session")
		return
	}
	pepper := getPasskeyPepper()
	response.OK(c, gin.H{
		"token": tokenPair.AccessToken, "refreshToken": tokenPair.RefreshToken,
		"user": u, "pepper": pepper,
	})
}

// @Summary Register new user with profile
// @Description Creates a new user account. Returns 409 if the wallet is already registered.
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body object true "Registration payload"
// @Success 200 {object} response.Envelope{data=object{token=string,refreshToken=string,user=object}}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		WalletAddress string  `json:"walletAddress" binding:"required"`
		Signature     string  `json:"signature" binding:"required"`
		DisplayName    *string `json:"displayName"`
		Email          *string `json:"email"`
		CountryCode    *string `json:"countryCode"`
		Language       *string `json:"language"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := stellar.ValidateAddress(req.WalletAddress); err != nil {
		response.BadRequest(c, "invalid wallet address: "+err.Error())
		return
	}

	valid, err := h.authService.VerifySignature(c.Request.Context(), req.WalletAddress, req.Signature)
	if err != nil || !valid {
		response.Unauthorized(c, "signature verification failed")
		return
	}

	// Check for existing user before creating
	existing, err := h.userService.GetByWallet(c.Request.Context(), req.WalletAddress)
	if err == nil && existing != nil {
		response.Conflict(c, "account already exists. please log in.")
		return
	}

	u, err := h.userService.Create(c.Request.Context(), req.WalletAddress)
	if err != nil {
		response.InternalError(c, "failed to create user")
		return
	}
	updates := user.UpdateProfileInput{
		DisplayName:       req.DisplayName,
		Email:             req.Email,
		CountryCode:       req.CountryCode,
		PreferredLanguage: req.Language,
	}
	u, err = h.userService.UpdateProfile(c.Request.Context(), u.ID.String(), updates)
	if err != nil {
		response.InternalError(c, "failed to update profile")
		return
	}
	tokenPair, err := h.authService.CreateSession(c.Request.Context(), u.ID)
	if err != nil {
		log.Printf("CreateSession error for user %s: %v", u.ID.String(), err)
		response.InternalError(c, "failed to create session")
		return
	}
	pepper := getPasskeyPepper()
	response.OK(c, gin.H{"token": tokenPair.AccessToken, "refreshToken": tokenPair.RefreshToken, "user": u, "pepper": pepper})
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
