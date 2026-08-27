with open("internal/domain/circle/repository_pg.go", "r") as f:
    content = f.read()

impls = """
func (r *pgRepo) CreatePenalty(ctx context.Context, p *Penalty) error {
	query := `
		INSERT INTO penalties (id, circle_id, user_id, round_number, penalty_type, amount, strikes_applied, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query, p.ID, p.CircleID, p.UserID, p.RoundNumber, p.PenaltyType, p.Amount, p.StrikesApplied, p.Reason, p.CreatedAt)
	return err
}

func (r *pgRepo) GetPenaltiesByCircle(ctx context.Context, circleID uuid.UUID) ([]Penalty, error) {
	query := `SELECT id, circle_id, user_id, round_number, penalty_type, amount, strikes_applied, reason, created_at FROM penalties WHERE circle_id = $1 ORDER BY created_at DESC`
	var penalties []Penalty
	err := r.db.SelectContext(ctx, &penalties, query, circleID)
	return penalties, err
}

func (r *pgRepo) GetPenaltiesByUser(ctx context.Context, userID uuid.UUID) ([]Penalty, error) {
	query := `SELECT id, circle_id, user_id, round_number, penalty_type, amount, strikes_applied, reason, created_at FROM penalties WHERE user_id = $1 ORDER BY created_at DESC`
	var penalties []Penalty
	err := r.db.SelectContext(ctx, &penalties, query, userID)
	return penalties, err
}

func (r *pgRepo) GetContributionsByCircleAndRound(ctx context.Context, circleID uuid.UUID, roundNumber int) ([]uuid.UUID, error) {
	query := `SELECT user_id FROM contributions WHERE circle_id = $1 AND round_number = $2`
	var userIDs []uuid.UUID
	err := r.db.SelectContext(ctx, &userIDs, query, circleID, roundNumber)
	return userIDs, err
}
"""

with open("internal/domain/circle/repository_pg.go", "a") as f:
    f.write("\n" + impls)

with open("internal/domain/circle/mocks/repository.go", "r") as f:
    mock_content = f.read()

mock_impls = """
func (m *Repository) CreatePenalty(ctx context.Context, p *circle.Penalty) error {
	return m.Called(ctx, p).Error(0)
}

func (m *Repository) GetPenaltiesByCircle(ctx context.Context, circleID uuid.UUID) ([]circle.Penalty, error) {
	args := m.Called(ctx, circleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.Penalty), args.Error(1)
}

func (m *Repository) GetPenaltiesByUser(ctx context.Context, userID uuid.UUID) ([]circle.Penalty, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.Penalty), args.Error(1)
}

func (m *Repository) GetContributionsByCircleAndRound(ctx context.Context, circleID uuid.UUID, roundNumber int) ([]uuid.UUID, error) {
	args := m.Called(ctx, circleID, roundNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uuid.UUID), args.Error(1)
}
"""

with open("internal/domain/circle/mocks/repository.go", "a") as f:
    f.write("\n" + mock_impls)
