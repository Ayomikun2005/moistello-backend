package admin

import "context"

// Repository computes platform-wide aggregate metrics from the primary
// database.
type Repository interface {
	// Metrics returns the aggregate platform snapshot. The time-bucketed
	// daily volume for the trailing `days` days is included.
	Metrics(ctx context.Context, days int) (*Metrics, error)
	// DailyVolume returns per-day contribution and payout volume for the
	// trailing `days` calendar days, oldest first.
	DailyVolume(ctx context.Context, days int) ([]DailyVolumePoint, error)
}
