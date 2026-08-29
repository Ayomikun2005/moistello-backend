package admin

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type pgRepo struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &pgRepo{db: db}
}

func (r *pgRepo) Metrics(ctx context.Context, days int) (*Metrics, error) {
	if days < 1 {
		days = 30
	}

	m := &Metrics{}

	var totalUsers, totalCircles, activeCircles, totalContributions, totalPayouts, activeUsers, newUsers int
	if err := r.db.QueryRowxContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM circles),
			(SELECT COUNT(*) FROM circles WHERE status = 'active'),
			(SELECT COUNT(*) FROM contributions),
			(SELECT COUNT(*) FROM payouts),
			(SELECT COUNT(DISTINCT u.id)
				FROM users u
				WHERE u.created_at >= NOW() - make_interval(days => $1)
				   OR EXISTS (SELECT 1 FROM contributions c WHERE c.user_id = u.id AND c.submitted_at >= NOW() - make_interval(days => $1))
				   OR EXISTS (SELECT 1 FROM payouts p WHERE p.recipient_id = u.id AND p.executed_at >= NOW() - make_interval(days => $1))),
			(SELECT COUNT(*) FROM users WHERE created_at >= NOW() - make_interval(days => $1))
	`, days).Scan(&totalUsers, &totalCircles, &activeCircles, &totalContributions, &totalPayouts, &activeUsers, &newUsers); err != nil {
		return nil, fmt.Errorf("aggregating platform metrics: %w", err)
	}
	m.TotalUsers = totalUsers
	m.TotalCircles = totalCircles
	m.ActiveCircles = activeCircles
	m.TotalContributions = totalContributions
	m.TotalPayouts = totalPayouts
	m.ActiveUsers = activeUsers
	m.NewUsers30d = newUsers

	var contribVolume, payoutVolume, volume30d float64
	if err := r.db.QueryRowxContext(ctx, `
		SELECT
			COALESCE((SELECT SUM(amount) FROM contributions WHERE status = 'confirmed'), 0),
			COALESCE((SELECT SUM(amount) FROM payouts), 0),
			COALESCE((SELECT SUM(amount) FROM contributions WHERE status = 'confirmed' AND submitted_at >= NOW() - make_interval(days => $1)), 0) +
			COALESCE((SELECT SUM(amount) FROM payouts WHERE executed_at >= NOW() - make_interval(days => $1)), 0)
	`, days).Scan(&contribVolume, &payoutVolume, &volume30d); err != nil {
		return nil, fmt.Errorf("aggregating platform volume: %w", err)
	}
	m.ContributionVolume = contribVolume
	m.PayoutVolume = payoutVolume
	m.TotalVolumeUSD = contribVolume + payoutVolume
	m.VolumeUSD30d = volume30d

	daily, err := r.DailyVolume(ctx, days)
	if err != nil {
		return nil, err
	}
	m.DailyVolume = daily

	return m, nil
}

func (r *pgRepo) DailyVolume(ctx context.Context, days int) ([]DailyVolumePoint, error) {
	if days < 1 {
		days = 30
	}

	query := `
		SELECT d.day,
			COALESCE(contrib.volume, 0) AS contribution_volume,
			COALESCE(payout.volume, 0) AS payout_volume
		FROM generate_series(CURRENT_DATE - ($1::int - 1), CURRENT_DATE, '1 day'::interval) AS d(day)
		LEFT JOIN (
			SELECT submitted_at::date AS day, SUM(amount) AS volume
			FROM contributions WHERE status = 'confirmed'
			GROUP BY submitted_at::date
		) contrib ON contrib.day = d.day
		LEFT JOIN (
			SELECT executed_at::date AS day, SUM(amount) AS volume
			FROM payouts
			GROUP BY executed_at::date
		) payout ON payout.day = d.day
		ORDER BY d.day ASC
	`

	rows, err := r.db.QueryxContext(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("querying daily volume: %w", err)
	}
	defer rows.Close()

	var points []DailyVolumePoint
	for rows.Next() {
		var p DailyVolumePoint
		if err := rows.Scan(&p.Date, &p.ContributionVolume, &p.PayoutVolume); err != nil {
			return nil, fmt.Errorf("scanning daily volume: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily volume: %w", err)
	}
	if points == nil {
		points = []DailyVolumePoint{}
	}
	return points, nil
}
