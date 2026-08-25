package featureflag

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
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