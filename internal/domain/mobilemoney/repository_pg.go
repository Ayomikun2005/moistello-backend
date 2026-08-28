package mobilemoney

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/moistello/backend/pkg/apperrors"
)

type pgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &pgRepository{db: db}
}

const selectColumns = `
	id, user_id, provider, direction, currency, amount, usdc_amount,
	phone_number, destination_address, status, provider_ref,
	idempotency_key, failure_reason, created_at, completed_at
`

func (r *pgRepository) Create(ctx context.Context, t *Transaction) error {
	query := `
		INSERT INTO mobile_money_transactions (
			id, user_id, provider, direction, currency, amount, usdc_amount,
			phone_number, destination_address, status, provider_ref,
			idempotency_key, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		t.ID, t.UserID, t.Provider, t.Direction, t.Currency, t.Amount, t.USDCAmount,
		t.PhoneNumber, t.DestinationAddress, t.Status, t.ProviderRef,
		t.IdempotencyKey, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating mobile money transaction: %w", err)
	}
	return nil
}

func (r *pgRepository) GetByID(ctx context.Context, id string) (*Transaction, error) {
	var t Transaction
	query := `SELECT ` + selectColumns + ` FROM mobile_money_transactions WHERE id = $1`
	err := r.db.GetContext(ctx, &t, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting mobile money transaction: %w", err)
	}
	return &t, nil
}

func (r *pgRepository) GetByIdempotencyKey(ctx context.Context, userID, idempotencyKey string) (*Transaction, error) {
	var t Transaction
	query := `SELECT ` + selectColumns + ` FROM mobile_money_transactions WHERE user_id = $1 AND idempotency_key = $2`
	err := r.db.GetContext(ctx, &t, query, userID, idempotencyKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting mobile money transaction by idempotency key: %w", err)
	}
	return &t, nil
}

func (r *pgRepository) UpdateStatus(ctx context.Context, id string, status Status, failureReason *string) error {
	var completedAt *time.Time
	if status == StatusCompleted || status == StatusFailed {
		now := time.Now().UTC()
		completedAt = &now
	}
	query := `
		UPDATE mobile_money_transactions
		SET status = $1, failure_reason = $2, completed_at = $3
		WHERE id = $4
	`
	_, err := r.db.ExecContext(ctx, query, status, failureReason, completedAt, id)
	if err != nil {
		return fmt.Errorf("updating mobile money transaction status: %w", err)
	}
	return nil
}

func (r *pgRepository) ListPending(ctx context.Context, olderThanSeconds int) ([]Transaction, error) {
	var txns []Transaction
	query := `
		SELECT ` + selectColumns + `
		FROM mobile_money_transactions
		WHERE status = $1 AND created_at <= NOW() - ($2 * INTERVAL '1 second')
		ORDER BY created_at ASC
	`
	err := r.db.SelectContext(ctx, &txns, query, StatusPending, olderThanSeconds)
	if err != nil {
		return nil, fmt.Errorf("listing pending mobile money transactions: %w", err)
	}
	if txns == nil {
		txns = []Transaction{}
	}
	return txns, nil
}
