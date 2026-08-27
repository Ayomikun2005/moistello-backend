package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/moistello/backend/internal/domain/deposit"
	"github.com/moistello/backend/internal/domain/withdrawal"
	"github.com/moistello/backend/pkg/response"
	"github.com/moistello/backend/webhook"
)

// YellowCardWebhookHandler receives status-change notifications from Yellow
// Card for deposits (receive) and withdrawals (send) and reconciles the
// platform's persisted state against them, so a deposit or withdrawal can
// never silently diverge from what actually happened at the provider.
type YellowCardWebhookHandler struct {
	deposits      deposit.Repository
	withdrawals   withdrawal.Repository
	webhookSecret string
}

func NewYellowCardWebhookHandler(deposits deposit.Repository, withdrawals withdrawal.Repository, webhookSecret string) *YellowCardWebhookHandler {
	return &YellowCardWebhookHandler{deposits: deposits, withdrawals: withdrawals, webhookSecret: webhookSecret}
}

// yellowCardWebhookPayload mirrors the notification Yellow Card POSTs when a
// receive (NGN deposit -> USDC) or send (USDC -> NGN withdrawal) transaction
// changes status. paymentReference is the value we generated and handed to
// Yellow Card when creating the transaction, so it is what ties the webhook
// back to our own deposit/withdrawal record.
type yellowCardWebhookPayload struct {
	Event            string `json:"event"`
	ID               string `json:"id"`
	Type             string `json:"type"` // "receive" | "send"
	Status           string `json:"status"`
	PaymentReference string `json:"paymentReference"`
	FailureReason    string `json:"failureReason,omitempty"`
}

// HandleWebhook verifies the request signature and reconciles the matching
// deposit or withdrawal record against the reported status.
// POST /webhooks/yellowcard
func (h *YellowCardWebhookHandler) HandleWebhook(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		response.BadRequest(c, "failed to read request body")
		return
	}

	signature := c.GetHeader("X-YC-Signature")
	if h.webhookSecret == "" || !webhook.VerifyWebhookSignature(body, signature, h.webhookSecret) {
		response.Unauthorized(c, "invalid webhook signature")
		return
	}

	var payload yellowCardWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		response.BadRequest(c, "invalid webhook payload")
		return
	}
	if payload.PaymentReference == "" {
		response.BadRequest(c, "missing paymentReference")
		return
	}

	ctx := c.Request.Context()

	switch payload.Type {
	case "receive":
		h.reconcileDeposit(ctx, payload)
	case "send":
		h.reconcileWithdrawal(ctx, payload)
	default:
		log.Warn().Str("type", payload.Type).Str("paymentRef", payload.PaymentReference).
			Msg("yellowcard webhook: unknown transaction type")
	}

	// Once the signature is valid and the payload parses, always 200 — even
	// if no matching local record was found (logged above/below) — so Yellow
	// Card does not keep retrying a delivery we've already understood.
	response.OK(c, gin.H{"received": true})
}

func (h *YellowCardWebhookHandler) reconcileDeposit(ctx context.Context, payload yellowCardWebhookPayload) {
	if h.deposits == nil {
		return
	}
	d, err := h.deposits.GetByPaymentRef(ctx, payload.PaymentReference)
	if err != nil || d == nil {
		log.Warn().Err(err).Str("paymentRef", payload.PaymentReference).
			Msg("yellowcard webhook: no matching deposit")
		return
	}

	status := normalizeYCStatus(payload.Status)
	var updateErr error
	switch status {
	case "completed":
		updateErr = h.deposits.MarkCompleted(ctx, d.ID, time.Now())
	case "failed", "expired":
		reason := payload.FailureReason
		if reason == "" {
			reason = "reported " + payload.Status + " by Yellow Card"
		}
		updateErr = h.deposits.MarkFailed(ctx, d.ID, reason)
	case "pending", "processing":
		updateErr = h.deposits.UpdateStatus(ctx, d.ID, deposit.DepositStatus(status))
	default:
		log.Warn().Str("status", payload.Status).Str("depositId", d.ID).
			Msg("yellowcard webhook: unrecognized deposit status")
		return
	}

	if updateErr != nil {
		log.Error().Err(updateErr).Str("depositId", d.ID).Str("status", status).
			Msg("yellowcard webhook: failed to reconcile deposit")
	}
}

func (h *YellowCardWebhookHandler) reconcileWithdrawal(ctx context.Context, payload yellowCardWebhookPayload) {
	if h.withdrawals == nil {
		return
	}
	w, err := h.withdrawals.GetByPaymentRef(ctx, payload.PaymentReference)
	if err != nil || w == nil {
		log.Warn().Err(err).Str("paymentRef", payload.PaymentReference).
			Msg("yellowcard webhook: no matching withdrawal")
		return
	}

	status := normalizeYCStatus(payload.Status)
	var updateErr error
	switch status {
	case "completed":
		updateErr = h.withdrawals.MarkCompleted(ctx, w.ID, time.Now())
	case "failed":
		reason := payload.FailureReason
		if reason == "" {
			reason = "reported failed by Yellow Card"
		}
		updateErr = h.withdrawals.MarkFailed(ctx, w.ID, reason)
	case "pending", "processing":
		updateErr = h.withdrawals.UpdateStatus(ctx, w.ID, withdrawal.WithdrawalStatus(status))
	default:
		log.Warn().Str("status", payload.Status).Str("withdrawalId", w.ID).
			Msg("yellowcard webhook: unrecognized withdrawal status")
		return
	}

	if updateErr != nil {
		log.Error().Err(updateErr).Str("withdrawalId", w.ID).Str("status", status).
			Msg("yellowcard webhook: failed to reconcile withdrawal")
	}
}

func normalizeYCStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}
