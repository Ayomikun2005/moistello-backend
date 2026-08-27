package swap

import (
	"context"
	"time"
)

type Repository interface {
	CreateSwapOffer(ctx context.Context, offer *SwapOffer) error
	GetSwapOfferByID(ctx context.Context, id string) (*SwapOffer, error)
	UpdateSwapOfferStatus(ctx context.Context, id string, status SwapOfferStatus, transactionHash *string) error
	ListUserSwapOffers(ctx context.Context, userID string, filter SwapHistoryFilter) ([]SwapOffer, int, error)
	ListCircleSwapOffers(ctx context.Context, circleID string, filter SwapHistoryFilter) ([]SwapOffer, int, error)
	// ListExpiredCreatedOffers returns created offers whose expires_at has
	// passed. The sweep worker (#243) uses this to release escrow on-chain and
	// then mark each offer expired — a status-only UPDATE would orphan the
	// escrowed funds, so the transition is owned by the sweep, not the query.
	ListExpiredCreatedOffers(ctx context.Context, now time.Time) ([]SwapOffer, error)
}
