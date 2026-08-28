package featureflag

import (
	"context"
)

// Repository defines persistence operations for feature flags.
type Repository interface {
	Get(ctx context.Context, flag string) (*FeatureFlag, error)
	List(ctx context.Context) ([]FeatureFlag, error)
	Upsert(ctx context.Context, flag string, enabled bool, description string) error
	Delete(ctx context.Context, flag string) error
}
