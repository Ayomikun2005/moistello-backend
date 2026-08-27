package deposit

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository persists Yellow Card deposits (NGN→USDC receives) so their
// state survives process restarts and can be reconciled against webhook
// notifications instead of only living in an in-memory response.
type Repository interface {
	Create(ctx context.Context, d *Deposit) error
	GetByID(ctx context.Context, id string) (*Deposit, error)
	GetByReceiveID(ctx context.Context, receiveID string) (*Deposit, error)
	GetByPaymentRef(ctx context.Context, paymentRef string) (*Deposit, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]Deposit, error)
	UpdateStatus(ctx context.Context, id string, status DepositStatus) error
	MarkCompleted(ctx context.Context, id string, completedAt time.Time) error
	MarkFailed(ctx context.Context, id string, reason string) error
}

type pgRepo struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed deposit Repository.
func NewRepository(db *sqlx.DB) Repository {
	return &pgRepo{db: db}
}

const selectColumns = `
	id, user_id, amount_ngn, estimated_usdc, destination_address, status,
	receive_id, payment_ref, created_at, completed_at, expires_at, failure_reason
`

func (r *pgRepo) Create(ctx context.Context, d *Deposit) error {
	query := `
		INSERT INTO deposits (
			id, user_id, amount_ngn, estimated_usdc, destination_address, status,
			receive_id, payment_ref, created_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		d.ID, d.UserID, d.AmountNGN, d.EstimatedUSDC, d.DestinationAddress, d.Status,
		d.ReceiveID, d.PaymentRef, d.CreatedAt, d.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating deposit: %w", err)
	}
	return nil
}

func (r *pgRepo) GetByID(ctx context.Context, id string) (*Deposit, error) {
	var d Deposit
	query := `SELECT ` + selectColumns + ` FROM deposits WHERE id = $1`
	if err := r.db.GetContext(ctx, &d, query, id); err != nil {
		return nil, fmt.Errorf("getting deposit by id: %w", err)
	}
	return &d, nil
}

func (r *pgRepo) GetByReceiveID(ctx context.Context, receiveID string) (*Deposit, error) {
	var d Deposit
	query := `SELECT ` + selectColumns + ` FROM deposits WHERE receive_id = $1`
	if err := r.db.GetContext(ctx, &d, query, receiveID); err != nil {
		return nil, fmt.Errorf("getting deposit by receive id: %w", err)
	}
	return &d, nil
}

func (r *pgRepo) GetByPaymentRef(ctx context.Context, paymentRef string) (*Deposit, error) {
	var d Deposit
	query := `SELECT ` + selectColumns + ` FROM deposits WHERE payment_ref = $1`
	if err := r.db.GetContext(ctx, &d, query, paymentRef); err != nil {
		return nil, fmt.Errorf("getting deposit by payment ref: %w", err)
	}
	return &d, nil
}

func (r *pgRepo) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]Deposit, error) {
	var deposits []Deposit
	query := `SELECT ` + selectColumns + ` FROM deposits WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &deposits, query, userID, limit, offset); err != nil {
		return nil, fmt.Errorf("getting deposits by user: %w", err)
	}
	return deposits, nil
}

func (r *pgRepo) UpdateStatus(ctx context.Context, id string, status DepositStatus) error {
	_, err := r.db.ExecContext(ctx, `UPDATE deposits SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating deposit status: %w", err)
	}
	return nil
}

func (r *pgRepo) MarkCompleted(ctx context.Context, id string, completedAt time.Time) error {
	query := `UPDATE deposits SET status = $1, completed_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, DepositStatusCompleted, completedAt, id)
	if err != nil {
		return fmt.Errorf("marking deposit completed: %w", err)
	}
	return nil
}

func (r *pgRepo) MarkFailed(ctx context.Context, id string, reason string) error {
	query := `UPDATE deposits SET status = $1, failure_reason = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, DepositStatusFailed, reason, id)
	if err != nil {
		return fmt.Errorf("marking deposit failed: %w", err)
	}
	return nil
}
