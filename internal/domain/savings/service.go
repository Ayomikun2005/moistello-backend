package savings

import (
	"context"
	"fmt"
)

type Service interface {
	CreateGoal(ctx context.Context, userID string, req CreateGoalRequest) (*SavingsGoal, error)
	GetGoal(ctx context.Context, userID, goalID string) (*SavingsGoal, error)
	ListGoals(ctx context.Context, userID string) ([]SavingsGoal, error)
	ListActiveGoals(ctx context.Context, userID string) ([]SavingsGoal, error)
	UpdateGoal(ctx context.Context, userID, goalID string, req UpdateGoalRequest) (*SavingsGoal, error)
	DeleteGoal(ctx context.Context, userID, goalID string) error
	CompleteGoal(ctx context.Context, userID, goalID string) (*SavingsGoal, error)
	GetSummary(ctx context.Context, userID string) (*GoalSummary, error)
	GetUpcomingObligations(ctx context.Context, userID string) ([]SavingsGoal, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreateGoal(ctx context.Context, userID string, req CreateGoalRequest) (*SavingsGoal, error) {
	g := &SavingsGoal{
		UserID:        userID,
		Name:          req.Name,
		Description:   req.Description,
		TargetAmount:  req.TargetAmount,
		CurrentAmount: 0,
		TargetDate:    req.TargetDate,
		CircleID:      req.CircleID,
		AutoReserve:   req.AutoReserve,
		Status:        "active",
	}
	if err := s.repo.Create(ctx, g); err != nil {
		return nil, fmt.Errorf("creating goal: %w", err)
	}
	return g, nil
}

func (s *service) GetGoal(ctx context.Context, userID, goalID string) (*SavingsGoal, error) {
	g, err := s.repo.FindByID(ctx, goalID)
	if err != nil {
		return nil, fmt.Errorf("finding goal: %w", err)
	}
	if g.UserID != userID {
		return nil, fmt.Errorf("goal not found")
	}
	return g, nil
}

func (s *service) ListGoals(ctx context.Context, userID string) ([]SavingsGoal, error) {
	return s.repo.FindByUserID(ctx, userID)
}

func (s *service) ListActiveGoals(ctx context.Context, userID string) ([]SavingsGoal, error) {
	return s.repo.FindActiveByUserID(ctx, userID)
}

func (s *service) UpdateGoal(ctx context.Context, userID, goalID string, req UpdateGoalRequest) (*SavingsGoal, error) {
	g, err := s.repo.FindByID(ctx, goalID)
	if err != nil {
		return nil, fmt.Errorf("finding goal: %w", err)
	}
	if g.UserID != userID {
		return nil, fmt.Errorf("goal not found")
	}

	if req.Name != nil {
		g.Name = *req.Name
	}
	if req.Description != nil {
		g.Description = *req.Description
	}
	if req.TargetAmount != nil {
		g.TargetAmount = *req.TargetAmount
	}
	if req.CurrentAmount != nil {
		g.CurrentAmount = *req.CurrentAmount
	}
	if req.TargetDate != nil {
		g.TargetDate = req.TargetDate
	}
	if req.CircleID != nil {
		g.CircleID = req.CircleID
	}
	if req.AutoReserve != nil {
		g.AutoReserve = *req.AutoReserve
	}
	if req.Status != nil {
		g.Status = *req.Status
	}

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, fmt.Errorf("updating goal: %w", err)
	}
	return g, nil
}

func (s *service) DeleteGoal(ctx context.Context, userID, goalID string) error {
	g, err := s.repo.FindByID(ctx, goalID)
	if err != nil {
		return fmt.Errorf("finding goal: %w", err)
	}
	if g.UserID != userID {
		return fmt.Errorf("goal not found")
	}
	return s.repo.Delete(ctx, goalID)
}

func (s *service) CompleteGoal(ctx context.Context, userID, goalID string) (*SavingsGoal, error) {
	g, err := s.repo.FindByID(ctx, goalID)
	if err != nil {
		return nil, fmt.Errorf("finding goal: %w", err)
	}
	if g.UserID != userID {
		return nil, fmt.Errorf("goal not found")
	}
	g.Status = "completed"
	g.CurrentAmount = g.TargetAmount
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, fmt.Errorf("completing goal: %w", err)
	}
	return g, nil
}

func (s *service) GetSummary(ctx context.Context, userID string) (*GoalSummary, error) {
	return s.repo.GetSummary(ctx, userID)
}

func (s *service) GetUpcomingObligations(ctx context.Context, userID string) ([]SavingsGoal, error) {
	return s.repo.GetUpcomingObligations(ctx, userID, 5)
}
