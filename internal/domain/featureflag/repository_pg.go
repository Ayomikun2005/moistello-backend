package featureflag

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

func (r *pgRepository) Get(ctx context.Context, flag string) (*FeatureFlag, error) {
	query := `SELECT id, flag, enabled, description, updated_at FROM feature_flags WHERE flag = $1`
	var f FeatureFlag
	err := r.db.GetContext(ctx, &f, query, flag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting feature flag %s: %w", flag, err)
	}
	return &f, nil
}

func (r *pgRepository) List(ctx context.Context) ([]FeatureFlag, error) {
	query := `SELECT id, flag, enabled, description, updated_at FROM feature_flags ORDER BY flag`
	var flags []FeatureFlag
	err := r.db.SelectContext(ctx, &flags, query)
	if err != nil {
		return nil, fmt.Errorf("listing feature flags: %w", err)
	}
	if flags == nil {
		flags = []FeatureFlag{}
	}
	return flags, nil
}

func (r *pgRepository) Upsert(ctx context.Context, flag string, enabled bool, description string) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO feature_flags (flag, enabled, description, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (flag) DO UPDATE SET enabled = $2, description = $3, updated_at = $4
	`
	_, err := r.db.ExecContext(ctx, query, flag, enabled, description, now)
	if err != nil {
		return fmt.Errorf("upserting feature flag %s: %w", flag, err)
	}
	return nil
}

func (r *pgRepository) Delete(ctx context.Context, flag string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM feature_flags WHERE flag = $1`, flag)
	if err != nil {
		return fmt.Errorf("deleting feature flag %s: %w", flag, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking delete result for feature flag %s: %w", flag, err)
	}
	if rows == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}
