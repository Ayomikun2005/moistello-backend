package mobilemoney

import "context"

// Repository persists mobile-money bridge transactions.
type Repository interface {
	Create(ctx context.Context, t *Transaction) error
	GetByID(ctx context.Context, id string) (*Transaction, error)
	// GetByIdempotencyKey returns nil, nil (not an error) when no record
	// exists for the key, so callers can distinguish "safe to create" from
	// a real lookup failure.
	GetByIdempotencyKey(ctx context.Context, userID, idempotencyKey string) (*Transaction, error)
	UpdateStatus(ctx context.Context, id string, status Status, failureReason *string) error
	// ListPending returns transactions still awaiting settlement that were
	// created at least olderThanSeconds ago (so a transaction isn't polled
	// before the provider has had any chance to settle it), for the
	// background reconciler to poll each provider's GetStatus.
	ListPending(ctx context.Context, olderThanSeconds int) ([]Transaction, error)
}
