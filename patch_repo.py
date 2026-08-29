with open("internal/domain/circle/repository.go", "r") as f:
    content = f.read()

methods = """
	CreatePenalty(ctx context.Context, p *Penalty) error
	GetPenaltiesByCircle(ctx context.Context, circleID uuid.UUID) ([]Penalty, error)
	GetPenaltiesByUser(ctx context.Context, userID uuid.UUID) ([]Penalty, error)
	GetContributionsByCircleAndRound(ctx context.Context, circleID uuid.UUID, roundNumber int) ([]uuid.UUID, error)
"""

content = content.replace("FindCirclesByUserID(ctx context.Context, userID uuid.UUID) ([]Circle, error)\n}", f"FindCirclesByUserID(ctx context.Context, userID uuid.UUID) ([]Circle, error)\n{methods}\n}}")

with open("internal/domain/circle/repository.go", "w") as f:
    f.write(content)
