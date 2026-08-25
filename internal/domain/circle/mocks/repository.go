package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/circle"
)

type Repository struct {
	mock.Mock
}

func (m *Repository) FindByID(ctx context.Context, id uuid.UUID) (*circle.Circle, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*circle.Circle), args.Error(1)
}

func (m *Repository) FindByContractID(ctx context.Context, contractID string) (*circle.Circle, error) {
	args := m.Called(ctx, contractID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*circle.Circle), args.Error(1)
}

func (m *Repository) List(ctx context.Context, filter circle.CircleFilter) ([]circle.Circle, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.Circle), args.Error(1)
}

func (m *Repository) Count(ctx context.Context, filter circle.CircleFilter) (int, error) {
	args := m.Called(ctx, filter)
	return args.Int(0), args.Error(1)
}

func (m *Repository) Create(ctx context.Context, c *circle.Circle) error {
	return m.Called(ctx, c).Error(0)
}

func (m *Repository) Update(ctx context.Context, c *circle.Circle) error {
	return m.Called(ctx, c).Error(0)
}

func (m *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *Repository) CreateMember(ctx context.Context, cm *circle.CircleMember) error {
	return m.Called(ctx, cm).Error(0)
}

func (m *Repository) GetMembers(ctx context.Context, circleID uuid.UUID) ([]circle.CircleMember, error) {
	args := m.Called(ctx, circleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.CircleMember), args.Error(1)
}

func (m *Repository) GetMemberCount(ctx context.Context, circleID uuid.UUID) (int, error) {
	args := m.Called(ctx, circleID)
	return args.Int(0), args.Error(1)
}

func (m *Repository) UpdateMemberStatus(ctx context.Context, circleID, userID uuid.UUID, status circle.MemberStatus) error {
	return m.Called(ctx, circleID, userID, status).Error(0)
}

func (m *Repository) FindMemberByCircleAndUser(ctx context.Context, circleID, userID uuid.UUID) (*circle.CircleMember, error) {
	args := m.Called(ctx, circleID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*circle.CircleMember), args.Error(1)
}

func (m *Repository) FindCirclesByUserID(ctx context.Context, userID uuid.UUID) ([]circle.Circle, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]circle.Circle), args.Error(1)
}


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
