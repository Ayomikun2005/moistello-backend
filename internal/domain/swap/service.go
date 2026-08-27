package swap

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/moistello/backend/internal/domain/circle"
	"github.com/moistello/backend/internal/domain/user"
	"github.com/moistello/backend/pkg/apperrors"
	"github.com/rs/zerolog/log"
)

// CircleService is the slice of the circle domain the swap service needs.
// Narrow interfaces keep the swap service testable without building the full
// circle.Service mock.
type CircleService interface {
	Get(ctx context.Context, id string) (*circle.Circle, error)
	IsMember(ctx context.Context, circleID, userID string) (bool, error)
}

// UserService is the slice of the user domain the swap service needs.
type UserService interface {
	GetByID(ctx context.Context, id string) (*user.User, error)
}

// EscrowSwapClient is the slice of the Soroban escrow bindings the swap
// service uses. The concrete *soroban.EscrowSwapClient satisfies it; the
// interface exists so the sweep/cancel logic can be tested with a fake.
type EscrowSwapClient interface {
	CreateSwap(ctx context.Context, circleID, offeror, offeree string, offerorAsset string, offerorAmount int64, requestedAsset string, requestedAmount int64, expiresAt uint64) (string, error)
	AcceptSwap(ctx context.Context, swapID string, acceptor string) (string, error)
	CancelSwap(ctx context.Context, swapID string, canceller string) (string, error)
	ExecuteSwap(ctx context.Context, swapID string) (string, error)
}

type Service struct {
	repo             Repository
	circleService    CircleService
	userService      UserService
	escrowSwapClient EscrowSwapClient
}

func NewService(
	repo Repository,
	circleService CircleService,
	userService UserService,
	escrowSwapClient EscrowSwapClient,
) *Service {
	return &Service{
		repo:             repo,
		circleService:    circleService,
		userService:      userService,
		escrowSwapClient: escrowSwapClient,
	}
}

func (s *Service) CreateSwapOffer(ctx context.Context, userID string, req SwapOfferRequest) (*SwapOffer, error) {
	// Verify user is a member of the circle
	_, err := s.circleService.Get(ctx, req.CircleID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid circle ID", apperrors.ErrInvalidInput)
	}

	isMember, err := s.circleService.IsMember(ctx, req.CircleID, userID)
	if err != nil || !isMember {
		return nil, fmt.Errorf("%w: user is not a member of this circle", apperrors.ErrForbidden)
	}

	// If offeree is specified, verify they are also a circle member
	if req.OffereeUserID != nil {
		isOffereeMember, err := s.circleService.IsMember(ctx, req.CircleID, *req.OffereeUserID)
		if err != nil || !isOffereeMember {
			return nil, fmt.Errorf("%w: offeree is not a member of this circle", apperrors.ErrInvalidInput)
		}
	}

	// Create swap offer in database
	offer := &SwapOffer{
		ID:              uuid.NewString(),
		CircleID:        req.CircleID,
		OfferorUserID:   userID,
		OffereeUserID:   req.OffereeUserID,
		OfferorAsset:    req.OfferorAsset,
		OfferorAmount:   req.OfferorAmount,
		RequestedAsset:  req.RequestedAsset,
		RequestedAmount: req.RequestedAmount,
		ExpiresAt:       time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour),
	}

	err = s.repo.CreateSwapOffer(ctx, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap offer: %w", err)
	}

	// Create swap on the escrow contract
	expiresAtUnix := uint64(offer.ExpiresAt.Unix())
	contractUserID, err := s.getUserContractID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var contractOffereeID string
	if req.OffereeUserID != nil {
		contractOffereeID, err = s.getUserContractID(ctx, *req.OffereeUserID)
		if err != nil {
			return nil, err
		}
	}

	txHash, err := s.escrowSwapClient.CreateSwap(
		ctx,
		req.CircleID,
		contractUserID,
		contractOffereeID,
		req.OfferorAsset,
		req.OfferorAmount,
		req.RequestedAsset,
		req.RequestedAmount,
		expiresAtUnix,
	)
	if err != nil {
		// Mark offer as failed if contract creation fails
		_ = s.repo.UpdateSwapOfferStatus(ctx, offer.ID, SwapOfferStatusCancelled, nil)
		return nil, fmt.Errorf("failed to create swap on chain: %w", err)
	}

	// Update with transaction hash
	offer.TransactionHash = &txHash
	err = s.repo.UpdateSwapOfferStatus(ctx, offer.ID, offer.Status, &txHash)
	if err != nil {
		return offer, nil // Still return the offer even if DB update fails
	}

	return offer, nil
}

func (s *Service) AcceptSwapOffer(ctx context.Context, userID string, swapOfferID string) (*SwapOffer, error) {
	// Get the swap offer
	offer, err := s.repo.GetSwapOfferByID(ctx, swapOfferID)
	if err != nil {
		return nil, fmt.Errorf("%w: swap offer not found", apperrors.ErrNotFound)
	}

	// Verify the offer is in created status
	if offer.Status != SwapOfferStatusCreated {
		return nil, fmt.Errorf("%w: swap offer is not available for acceptance", apperrors.ErrInvalidInput)
	}

	// Reject offers that have expired but have not yet been swept. The sweep
	// worker owns the created→expired transition (it must release escrow
	// on-chain first), so an expired offer can still read as created for up to
	// one sweep interval — acceptance must not slip through that window.
	if time.Now().After(offer.ExpiresAt) {
		return nil, fmt.Errorf("%w: swap offer has expired", apperrors.ErrInvalidInput)
	}

	// Verify the acceptor is the intended offeree (or any member if no offeree specified)
	if offer.OffereeUserID != nil && *offer.OffereeUserID != userID {
		return nil, fmt.Errorf("%w: only the specified offeree can accept this swap", apperrors.ErrForbidden)
	}

	// Verify acceptor is a circle member
	isMember, err := s.circleService.IsMember(ctx, offer.CircleID, userID)
	if err != nil || !isMember {
		return nil, fmt.Errorf("%w: user is not a member of this circle", apperrors.ErrForbidden)
	}

	// Verify the acceptor is not the offeror
	if offer.OfferorUserID == userID {
		return nil, fmt.Errorf("%w: cannot accept your own swap offer", apperrors.ErrInvalidInput)
	}

	// Update database first
	offer.OffereeUserID = &userID
	offer.Status = SwapOfferStatusAccepted
	err = s.repo.UpdateSwapOfferStatus(ctx, swapOfferID, SwapOfferStatusAccepted, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to update swap offer: %w", err)
	}

	// Accept swap on chain
	contractUserID, err := s.getUserContractID(ctx, userID)
	if err != nil {
		return nil, err
	}

	txHash, err := s.escrowSwapClient.AcceptSwap(ctx, swapOfferID, contractUserID)
	if err != nil {
		// Revert status if contract call fails
		_ = s.repo.UpdateSwapOfferStatus(ctx, swapOfferID, SwapOfferStatusCreated, nil)
		return nil, fmt.Errorf("failed to accept swap on chain: %w", err)
	}

	// Execute the swap automatically after acceptance (zero spread, atomic swap)
	executeTxHash, err := s.escrowSwapClient.ExecuteSwap(ctx, swapOfferID)
	if err != nil {
		// If execution fails, still mark as accepted but log the error
		offer.TransactionHash = &txHash
		return offer, fmt.Errorf("swap accepted but execution failed: %w", err)
	}

	// Mark as completed
	finalTxHash := executeTxHash
	offer.TransactionHash = &finalTxHash
	offer.Status = SwapOfferStatusCompleted
	err = s.repo.UpdateSwapOfferStatus(ctx, swapOfferID, SwapOfferStatusCompleted, &finalTxHash)
	if err != nil {
		return offer, nil
	}

	return offer, nil
}

// CancelSwapOffer lets the offeror cancel a created offer before it is
// accepted. The escrow is released on-chain first, then the offer is marked
// cancelled — an offer that cannot be cancelled on-chain stays created so the
// caller sees the failure rather than a DB row that lies about the escrow.
func (s *Service) CancelSwapOffer(ctx context.Context, userID string, swapOfferID string) (*SwapOffer, error) {
	offer, err := s.repo.GetSwapOfferByID(ctx, swapOfferID)
	if err != nil {
		return nil, fmt.Errorf("%w: swap offer not found", apperrors.ErrNotFound)
	}

	if offer.OfferorUserID != userID {
		return nil, fmt.Errorf("%w: only the offeror can cancel a swap offer", apperrors.ErrForbidden)
	}

	if offer.Status != SwapOfferStatusCreated {
		return nil, fmt.Errorf("%w: swap offer is not available for cancellation", apperrors.ErrInvalidInput)
	}

	offerorContractID, err := s.getUserContractID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if _, err := s.escrowSwapClient.CancelSwap(ctx, swapOfferID, offerorContractID); err != nil {
		return nil, fmt.Errorf("failed to cancel swap on chain: %w", err)
	}

	if err := s.repo.UpdateSwapOfferStatus(ctx, swapOfferID, SwapOfferStatusCancelled, nil); err != nil {
		return offer, nil // escrow is released; a failed row update must not report failure
	}

	offer.Status = SwapOfferStatusCancelled
	return offer, nil
}

// SweepExpiredOffers is the expiry half of the sweep worker (#243). For every
// created offer past its expires_at it cancels the escrow on-chain (releasing
// the offeror's funds) and then marks the offer expired. An offer that cannot
// be cancelled on-chain is left created so the next sweep retries it — a DB
// transition without the chain release would orphan the escrow.
func (s *Service) SweepExpiredOffers(ctx context.Context) (int, error) {
	offers, err := s.repo.ListExpiredCreatedOffers(ctx, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to list expired swap offers: %w", err)
	}

	swept := 0
	for _, offer := range offers {
		offerContractID, err := s.getUserContractID(ctx, offer.OfferorUserID)
		if err != nil {
			log.Warn().Err(err).Str("offer", offer.ID).Msg("swap sweep: cannot resolve offeror contract ID, deferring")
			continue
		}

		if _, err := s.escrowSwapClient.CancelSwap(ctx, offer.ID, offerContractID); err != nil {
			log.Warn().Err(err).Str("offer", offer.ID).Msg("swap sweep: on-chain cancel failed, retrying next sweep")
			continue
		}

		if err := s.repo.UpdateSwapOfferStatus(ctx, offer.ID, SwapOfferStatusExpired, nil); err != nil {
			log.Warn().Err(err).Str("offer", offer.ID).Msg("swap sweep: failed to mark offer expired")
			continue
		}
		swept++
	}

	return swept, nil
}

func (s *Service) GetSwapHistory(ctx context.Context, userID string, filter SwapHistoryFilter) (*SwapHistoryResponse, error) {
	swaps, total, err := s.repo.ListUserSwapOffers(ctx, userID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch swap history: %w", err)
	}

	return &SwapHistoryResponse{
		Swaps:  swaps,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}, nil
}

func (s *Service) getUserContractID(ctx context.Context, userID string) (string, error) {
	user, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	// Return user's wallet public key as the contract identifier
	if user.WalletAddress != "" {
		return user.WalletAddress, nil
	}
	return "", fmt.Errorf("%w: user has no wallet", apperrors.ErrInvalidInput)
}
