package payout

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/pkg/apperrors"
	"github.com/moistello/backend/pkg/stellar"
)

type Service interface {
	Record(ctx context.Context, input RecordInput) (*Payout, error)
	UpdateVerification(ctx context.Context, id string, verifiedOnchain bool, status VerificationStatus) error
	GetUserHistory(ctx context.Context, userID string, page, limit int) ([]Payout, int, error)
	GetCircleHistory(ctx context.Context, circleID string, page, limit int) ([]Payout, int, error)
}

type RecordInput struct {
	CircleID           string              `json:"circleId" validate:"required"`
	RecipientID        string              `json:"recipientId" validate:"required"`
	RoundNumber        int                 `json:"roundNumber" validate:"required,gte=1"`
	Amount             float64             `json:"amount" validate:"required,gt=0"`
	FeeAmount          float64             `json:"feeAmount" validate:"gte=0"`
	TxnHash            string              `json:"txnHash"`
	PayoutType         PayoutType          `json:"payoutType" validate:"required,oneof=random fixed auction vote"`
	VerifiedOnchain    *bool               `json:"verifiedOnchain,omitempty"`
	VerificationStatus *VerificationStatus `json:"verificationStatus,omitempty"`
}

type payoutService struct {
	repo          Repository
	stellarClient *stellar.Client
	userRepo      user.Repository
}

// NewService creates a payout service. The last two parameters are optional
// and may be nil. When a Stellar client and a user repository are provided,
// payouts with a TxnHash will be verified on-chain before recording.
func NewService(repo Repository, stellarClient *stellar.Client, userRepo user.Repository) Service {
	return &payoutService{repo: repo, stellarClient: stellarClient, userRepo: userRepo}
}

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID: %w", err)
	}
	return id, nil
}

func (s *payoutService) Record(ctx context.Context, input RecordInput) (*Payout, error) {
	circleID, err := parseUUID(input.CircleID)
	if err != nil {
		return nil, err
	}
	recipientID, err := parseUUID(input.RecipientID)
	if err != nil {
		return nil, err
	}

	var txnHash sql.NullString
	if input.TxnHash != "" {
		txnHash = sql.NullString{String: input.TxnHash, Valid: true}
	}

	verifiedOnchain := false
	if input.VerifiedOnchain != nil {
		verifiedOnchain = *input.VerifiedOnchain
	}
	verificationStatus := VerificationStatusUnverified
	if input.VerificationStatus != nil {
		verificationStatus = *input.VerificationStatus
	} else if verifiedOnchain {
		verificationStatus = VerificationStatusVerified
	}

	p := &Payout{
		ID:                 uuid.New(),
		CircleID:           circleID,
		RecipientID:        recipientID,
		RoundNumber:        input.RoundNumber,
		Amount:             input.Amount,
		FeeAmount:          input.FeeAmount,
		TxnHash:            txnHash,
		PayoutType:         input.PayoutType,
		VerifiedOnchain:    verifiedOnchain,
		VerificationStatus: verificationStatus,
		CreatedAt:          time.Now().UTC(),
	}

	// If transaction hash is provided and a Stellar client is configured,
	// perform on-chain verification and make the operation idempotent by
	// returning an existing payout with the same txn hash if present.
	if input.TxnHash != "" {
		// Search for existing payouts in this circle to avoid duplicates
		existing, _, err := s.repo.ListByCircle(ctx, circleID, 1, 100)
		if err == nil {
			for _, ex := range existing {
				if ex.TxnHash.Valid && ex.TxnHash.String == input.TxnHash {
					return &ex, nil
				}
			}
		}

		if s.stellarClient != nil && s.userRepo != nil {
			// Resolve recipient wallet address
			uid, err := uuid.Parse(input.RecipientID)
			if err == nil {
				usr, uerr := s.userRepo.FindByID(ctx, uid)
				if uerr == nil && usr != nil {
					amtStr := strconv.FormatFloat(input.Amount, 'f', 7, 64)
					ok, verr := s.stellarClient.VerifyTransaction(ctx, input.TxnHash, usr.WalletAddress, amtStr)
					if verr != nil {
						return nil, fmt.Errorf("failed to verify transaction: %w", verr)
					}
					if !ok {
						return nil, fmt.Errorf("on-chain verification failed")
					}
					p.VerifiedOnchain = true
					p.VerificationStatus = VerificationStatusVerified
				}
			}
		}

	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("recording payout: %w", err)
	}
	return p, nil
}

func (s *payoutService) UpdateVerification(ctx context.Context, id string, verifiedOnchain bool, status VerificationStatus) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.repo.UpdateVerificationStatus(ctx, uid, verifiedOnchain, status)
}

func (s *payoutService) GetUserHistory(ctx context.Context, userID string, page, limit int) ([]Payout, int, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	payouts, total, err := s.repo.ListByUser(ctx, uid, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("getting user payout history: %w", err)
	}
	return payouts, total, nil
}

func (s *payoutService) GetCircleHistory(ctx context.Context, circleID string, page, limit int) ([]Payout, int, error) {
	cid, err := parseUUID(circleID)
	if err != nil {
		return nil, 0, err
	}
	payouts, total, err := s.repo.ListByCircle(ctx, cid, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("getting circle payout history: %w", err)
	}
	return payouts, total, nil
}
