with open("internal/domain/circle/mocks/repository.go", "r") as f:
    content = f.read()

impls = """
func (m *Repository) CreateDispute(ctx context.Context, d *circle.Dispute) error {
	return m.Called(ctx, d).Error(0)
}

func (m *Repository) GetDisputeByID(ctx context.Context, id uuid.UUID) (*circle.Dispute, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*circle.Dispute), args.Error(1)
}

func (m *Repository) UpdateDisputeStatus(ctx context.Context, id uuid.UUID, status circle.DisputeStatus) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *Repository) ListDisputesByCircle(ctx context.Context, circleID uuid.UUID) ([]circle.Dispute, error) {
	args := m.Called(ctx, circleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.Dispute), args.Error(1)
}

func (m *Repository) CreateVote(ctx context.Context, v *circle.CircleVote) error {
	return m.Called(ctx, v).Error(0)
}

func (m *Repository) GetVotesByCircleAndRound(ctx context.Context, circleID uuid.UUID, roundNumber int) ([]circle.CircleVote, error) {
	args := m.Called(ctx, circleID, roundNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.CircleVote), args.Error(1)
}

func (m *Repository) GetVoteByVoterAndRound(ctx context.Context, circleID, voterID uuid.UUID, roundNumber int) (*circle.CircleVote, error) {
	args := m.Called(ctx, circleID, voterID, roundNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*circle.CircleVote), args.Error(1)
}

func (m *Repository) CreateAuctionBid(ctx context.Context, b *circle.CircleAuctionBid) error {
	return m.Called(ctx, b).Error(0)
}

func (m *Repository) GetAuctionBidsByCircleAndRound(ctx context.Context, circleID uuid.UUID, roundNumber int) ([]circle.CircleAuctionBid, error) {
	args := m.Called(ctx, circleID, roundNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.CircleAuctionBid), args.Error(1)
}

func (m *Repository) GetAuctionBidByBidderAndRound(ctx context.Context, circleID, bidderID uuid.UUID, roundNumber int) (*circle.CircleAuctionBid, error) {
	args := m.Called(ctx, circleID, bidderID, roundNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*circle.CircleAuctionBid), args.Error(1)
}
"""

with open("internal/domain/circle/mocks/repository.go", "a") as f:
    f.write("\n" + impls)
