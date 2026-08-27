package handler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/moistello/backend/config"
	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/internal/domain/deposit"
	"github.com/moistello/backend/internal/domain/wallet"
	"github.com/moistello/backend/internal/domain/withdrawal"
	"github.com/moistello/backend/internal/domain/yellowcard"
	"github.com/moistello/backend/pkg/response"
	"github.com/rs/zerolog/log"
)

type DepositHandler struct {
	yc          *yellowcard.Client
	wallet      wallet.Service
	rdb         *redis.Client
	cfg         config.YellowCardConfig
	deposits    deposit.Repository
	withdrawals withdrawal.Repository
}

func NewDepositHandler(yc *yellowcard.Client, walletSvc wallet.Service) *DepositHandler {
	return &DepositHandler{yc: yc, wallet: walletSvc}
}

func (h *DepositHandler) WithRedis(rdb *redis.Client) *DepositHandler {
	h.rdb = rdb
	return h
}

func (h *DepositHandler) WithConfig(cfg config.YellowCardConfig) *DepositHandler {
	h.cfg = cfg
	return h
}

// WithRepositories wires persistence for deposits and withdrawals so their
// state survives process restarts and can be reconciled against Yellow Card
// webhook notifications instead of only living in the initial API response.
func (h *DepositHandler) WithRepositories(deposits deposit.Repository, withdrawals withdrawal.Repository) *DepositHandler {
	h.deposits = deposits
	h.withdrawals = withdrawals
	return h
}

func (h *DepositHandler) maxDepositNGN() float64 {
	if h.cfg.MaxDepositNGN > 0 {
		return h.cfg.MaxDepositNGN
	}
	return 5_000_000 // 5M NGN default per-transaction cap
}

func (h *DepositHandler) maxWithdrawUSDC() float64 {
	if h.cfg.MaxWithdrawUSDC > 0 {
		return h.cfg.MaxWithdrawUSDC
	}
	return 10_000 // 10k USDC default per-transaction cap
}

func (h *DepositHandler) dailyDepositCapNGN() float64 {
	if h.cfg.DailyDepositCapNGN > 0 {
		return h.cfg.DailyDepositCapNGN
	}
	return 10_000_000 // 10M NGN default daily cap
}

func (h *DepositHandler) dailyWithdrawCapUSDC() float64 {
	if h.cfg.DailyWithdrawCapUSDC > 0 {
		return h.cfg.DailyWithdrawCapUSDC
	}
	return 20_000 // 20k USDC default daily cap
}

func getIdempotencyKey(c *gin.Context, bodyKey string) string {
	if bodyKey != "" {
		return strings.TrimSpace(bodyKey)
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
	}
	return key
}

// GetDepositQuote returns a NGN→USDC quote
// GET /v1/wallet/deposit/quote?amount=50000
func (h *DepositHandler) GetDepositQuote(c *gin.Context) {
	amountStr := c.Query("amount")
	if amountStr == "" {
		response.BadRequest(c, "amount is required")
		return
	}

	var amount float64
	if _, err := fmt.Sscanf(amountStr, "%f", &amount); err != nil || amount <= 0 {
		response.BadRequest(c, "invalid amount")
		return
	}

	quote, err := h.yc.GetQuote("NGN", "USDC", amount)
	if err != nil {
		response.InternalError(c, "failed to get quote: "+err.Error())
		return
	}

	response.OK(c, gin.H{"quote": quote})
}

// InitiateDeposit creates a deposit request (NGN → USDC)
// POST /v1/wallet/deposit
func (h *DepositHandler) InitiateDeposit(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		AmountNGN      float64 `json:"amountNgn" binding:"required,gt=0"`
		IdempotencyKey string  `json:"idempotencyKey"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "amountNgn is required")
		return
	}

	idempotencyKey := getIdempotencyKey(c, req.IdempotencyKey)
	ctx := c.Request.Context()

	// Check idempotency cache in Redis
	if idempotencyKey != "" && h.rdb != nil {
		cachedKey := fmt.Sprintf("yc:idempotency:deposit:%s:%s", userID, idempotencyKey)
		cachedData, err := h.rdb.Get(ctx, cachedKey).Bytes()
		if err == nil && len(cachedData) > 0 {
			var cachedResp gin.H
			if err := json.Unmarshal(cachedData, &cachedResp); err == nil {
				response.OK(c, cachedResp)
				return
			}
		}
	}

	// Validate per-transaction amount cap
	maxDeposit := h.maxDepositNGN()
	if req.AmountNGN > maxDeposit {
		response.BadRequest(c, fmt.Sprintf("deposit amount exceeds maximum allowed limit of %.2f NGN", maxDeposit))
		return
	}

	// Validate daily amount cap
	dailyCap := h.dailyDepositCapNGN()
	if h.rdb != nil {
		today := time.Now().UTC().Format("2006-01-02")
		dailyKey := fmt.Sprintf("yc:daily:deposit:%s:%s", userID, today)
		currentDaily, _ := h.rdb.Get(ctx, dailyKey).Float64()
		if currentDaily+req.AmountNGN > dailyCap {
			response.BadRequest(c, fmt.Sprintf("deposit amount exceeds daily limit of %.2f NGN (current total: %.2f NGN)", dailyCap, currentDaily))
			return
		}
	}

	// Get user's primary wallet
	wallets, err := h.wallet.GetWallets(ctx, userID)
	if err != nil || len(wallets) == 0 {
		response.BadRequest(c, "no wallet found. Create a wallet first.")
		return
	}
	userWallet := wallets[0]

	// Get quote
	quote, err := h.yc.GetQuote("NGN", "USDC", req.AmountNGN)
	if err != nil {
		response.InternalError(c, "failed to get quote: "+err.Error())
		return
	}

	// Create receive request
	paymentRef := fmt.Sprintf("MOIST-%d", time.Now().UnixMilli())
	receive, err := h.yc.CreateReceive(yellowcard.ReceiveRequest{
		Amount:              req.AmountNGN,
		Currency:            "NGN",
		DestinationCurrency: "USDC",
		DestinationAddress:  userWallet.PublicKey,
		PaymentReference:    paymentRef,
	})
	if err != nil {
		response.InternalError(c, "failed to create deposit: "+err.Error())
		return
	}

	// Persist the deposit so its state survives process restarts and can be
	// reconciled against Yellow Card webhook notifications. Yellow Card has
	// already accepted the receive request at this point, so a persistence
	// failure here is logged loudly for manual reconciliation rather than
	// silently discarded — the deposit still exists on Yellow Card's side.
	if h.deposits != nil {
		var expiresAt *time.Time
		if receive.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, receive.ExpiresAt); err == nil {
				expiresAt = &t
			}
		}
		d := &deposit.Deposit{
			ID:                 uuid.New().String(),
			UserID:             userID,
			AmountNGN:          req.AmountNGN,
			EstimatedUSDC:      quote.ToAmount,
			DestinationAddress: userWallet.PublicKey,
			Status:             deposit.DepositStatusPending,
			ReceiveID:          receive.ReceiveID,
			PaymentRef:         paymentRef,
			CreatedAt:          time.Now(),
			ExpiresAt:          expiresAt,
		}
		if err := h.deposits.Create(ctx, d); err != nil {
			log.Error().Err(err).
				Str("receiveId", receive.ReceiveID).
				Str("paymentRef", paymentRef).
				Str("userId", userID).
				Msg("failed to persist deposit after Yellow Card accepted it — requires manual reconciliation")
			response.InternalError(c, "failed to record deposit")
			return
		}
	}

	respData := gin.H{
		"deposit": gin.H{
			"receiveId":     receive.ReceiveID,
			"paymentRef":    paymentRef,
			"bankDetails":   receive.BankDetails,
			"estimatedUsdc": quote.ToAmount,
			"spread":        quote.FeePercentage,
			"expiresAt":     receive.ExpiresAt,
		},
	}

	// Update daily total and cache idempotency
	if h.rdb != nil {
		today := time.Now().UTC().Format("2006-01-02")
		dailyKey := fmt.Sprintf("yc:daily:deposit:%s:%s", userID, today)
		h.rdb.IncrByFloat(ctx, dailyKey, req.AmountNGN)
		h.rdb.Expire(ctx, dailyKey, 48*time.Hour)

		if idempotencyKey != "" {
			cachedKey := fmt.Sprintf("yc:idempotency:deposit:%s:%s", userID, idempotencyKey)
			if payload, err := json.Marshal(respData); err == nil {
				h.rdb.Set(ctx, cachedKey, payload, 24*time.Hour)
			}
		}
	}

	response.Created(c, respData)
}

// POST /v1/wallet/withdraw
func (h *DepositHandler) InitiateWithdraw(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		AmountUSDC     float64 `json:"amountUsdc" binding:"required,gt=0"`
		BankCode       string  `json:"bankCode" binding:"required"`
		AccountNumber  string  `json:"accountNumber" binding:"required"`
		AccountName    string  `json:"accountName" binding:"required"`
		IdempotencyKey string  `json:"idempotencyKey"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "amountUsdc, bankCode, accountNumber, and accountName are required")
		return
	}

	idempotencyKey := getIdempotencyKey(c, req.IdempotencyKey)
	ctx := c.Request.Context()

	// Check idempotency cache in Redis
	if idempotencyKey != "" && h.rdb != nil {
		cachedKey := fmt.Sprintf("yc:idempotency:withdraw:%s:%s", userID, idempotencyKey)
		cachedData, err := h.rdb.Get(ctx, cachedKey).Bytes()
		if err == nil && len(cachedData) > 0 {
			var cachedResp gin.H
			if err := json.Unmarshal(cachedData, &cachedResp); err == nil {
				response.OK(c, cachedResp)
				return
			}
		}
	}

	// Validate per-transaction amount cap
	maxWithdraw := h.maxWithdrawUSDC()
	if req.AmountUSDC > maxWithdraw {
		response.BadRequest(c, fmt.Sprintf("withdrawal amount exceeds maximum allowed limit of %.2f USDC", maxWithdraw))
		return
	}

	// Validate daily amount cap
	dailyCap := h.dailyWithdrawCapUSDC()
	if h.rdb != nil {
		today := time.Now().UTC().Format("2006-01-02")
		dailyKey := fmt.Sprintf("yc:daily:withdraw:%s:%s", userID, today)
		currentDaily, _ := h.rdb.Get(ctx, dailyKey).Float64()
		if currentDaily+req.AmountUSDC > dailyCap {
			response.BadRequest(c, fmt.Sprintf("withdrawal amount exceeds daily limit of %.2f USDC (current total: %.2f USDC)", dailyCap, currentDaily))
			return
		}
	}

	// Get user's primary wallet
	wallets, err := h.wallet.GetWallets(ctx, userID)
	if err != nil || len(wallets) == 0 {
		response.BadRequest(c, "no wallet found. Create a wallet first.")
		return
	}
	userWallet := wallets[0]

	// Get quote
	quote, err := h.yc.GetQuote("USDC", "NGN", req.AmountUSDC)
	if err != nil {
		response.InternalError(c, "failed to get quote: "+err.Error())
		return
	}

	// Create send request
	paymentRef := fmt.Sprintf("MOIST-%d", time.Now().UnixMilli())
	sendResp, err := h.yc.CreateSend(yellowcard.SendRequest{
		Amount:         req.AmountUSDC,
		Currency:       "USDC",
		TargetCurrency: "NGN",
		BankCode:       req.BankCode,
		AccountNumber:  req.AccountNumber,
		AccountName:    req.AccountName,
		PaymentRef:     paymentRef,
	})
	if err != nil {
		response.InternalError(c, "failed to create withdrawal: "+err.Error())
		return
	}

	// Return Yellow Card's configured Stellar address for the user to send USDC
	// to. The address is provided at startup from config rather than hard-coded.
	ycAddress := h.yc.StellarAddress()

	// Persist the withdrawal so its state survives process restarts and can
	// be reconciled against Yellow Card webhook notifications. Yellow Card
	// has already accepted the send request at this point, so a persistence
	// failure here is logged loudly for manual reconciliation rather than
	// silently discarded.
	if h.withdrawals != nil {
		wd := &withdrawal.Withdrawal{
			ID:              uuid.New().String(),
			UserID:          userID,
			AmountUSDC:      req.AmountUSDC,
			EstimatedNGN:    quote.ToAmount,
			BankCode:        req.BankCode,
			AccountNumber:   req.AccountNumber,
			AccountName:     req.AccountName,
			Status:          withdrawal.WithdrawalStatusPending,
			PlatformAddress: ycAddress,
			PaymentRef:      paymentRef,
			CreatedAt:       time.Now(),
		}
		if err := h.withdrawals.Create(ctx, wd); err != nil {
			log.Error().Err(err).
				Str("sendId", sendResp.SendID).
				Str("paymentRef", paymentRef).
				Str("userId", userID).
				Msg("failed to persist withdrawal after Yellow Card accepted it — requires manual reconciliation")
			response.InternalError(c, "failed to record withdrawal")
			return
		}
		if err := h.withdrawals.UpdateYellowCardTxID(ctx, wd.ID, sendResp.SendID); err != nil {
			log.Error().Err(err).Str("withdrawalId", wd.ID).Msg("failed to record yellow card send id")
		}
	}

	respData := gin.H{
		"withdraw": gin.H{
			"sendId":            sendResp.SendID,
			"status":            sendResp.Status,
			"paymentRef":        paymentRef,
			"estimatedNgn":      quote.ToAmount,
			"spread":            quote.FeePercentage,
			"yellowCardAddress": ycAddress,
			"usdcAmount":        req.AmountUSDC,
			"userWallet":        userWallet.PublicKey,
		},
	}

	// Update daily total and cache idempotency
	if h.rdb != nil {
		today := time.Now().UTC().Format("2006-01-02")
		dailyKey := fmt.Sprintf("yc:daily:withdraw:%s:%s", userID, today)
		h.rdb.IncrByFloat(ctx, dailyKey, req.AmountUSDC)
		h.rdb.Expire(ctx, dailyKey, 48*time.Hour)

		if idempotencyKey != "" {
			cachedKey := fmt.Sprintf("yc:idempotency:withdraw:%s:%s", userID, idempotencyKey)
			if payload, err := json.Marshal(respData); err == nil {
				h.rdb.Set(ctx, cachedKey, payload, 24*time.Hour)
			}
		}
	}

	response.OK(c, respData)
}

// GET /v1/wallet/transactions/:yellowCardId
func (h *DepositHandler) GetTransactionStatus(c *gin.Context) {
	txnID := c.Param("yellowCardId")
	status, err := h.yc.GetTransactionStatus(txnID)
	if err != nil {
		response.InternalError(c, "failed to get status: "+err.Error())
		return
	}
	response.OK(c, gin.H{"transaction": status})
}
