package savings_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/savings"
	savingsMocks "github.com/moistello/backend/internal/domain/savings/mocks"
)

func ctx() context.Context { return context.Background() }

func newGoal(id, userID string) *savings.SavingsGoal {
	return &savings.SavingsGoal{
		ID: id, UserID: userID, Name: "Emergency Fund",
		TargetAmount: 1000, CurrentAmount: 0, Status: "active",
	}
}

func TestSavingsService_CreateGoal(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	repo.On("Create", mock.Anything, mock.AnythingOfType("*savings.SavingsGoal")).Return(nil).Run(func(args mock.Arguments) {
		g := args.Get(1).(*savings.SavingsGoal)
		g.ID = "goal-1"
	})

	goal, err := svc.CreateGoal(ctx(), uid, savings.CreateGoalRequest{
		Name:         "Emergency Fund",
		Description:  "6 months of expenses",
		TargetAmount: 5000,
		AutoReserve:  true,
	})

	assert.NoError(t, err)
	assert.NotNil(t, goal)
	assert.Equal(t, uid, goal.UserID)
	assert.Equal(t, "Emergency Fund", goal.Name)
	assert.Equal(t, 0.0, goal.CurrentAmount)
	assert.Equal(t, "active", goal.Status)
	assert.Equal(t, "goal-1", goal.ID)
	repo.AssertExpectations(t)
}

func TestSavingsService_CreateGoal_RepoError(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*savings.SavingsGoal")).Return(assert.AnError)

	goal, err := svc.CreateGoal(ctx(), "user-1", savings.CreateGoalRequest{Name: "X", TargetAmount: 10})

	assert.Error(t, err)
	assert.Nil(t, goal)
	repo.AssertExpectations(t)
}

func TestSavingsService_GetGoal_Owned(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", uid), nil)

	goal, err := svc.GetGoal(ctx(), uid, "goal-1")

	assert.NoError(t, err)
	assert.Equal(t, "goal-1", goal.ID)
	repo.AssertExpectations(t)
}

func TestSavingsService_GetGoal_NotOwner(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", "user-2"), nil)

	goal, err := svc.GetGoal(ctx(), "user-1", "goal-1")

	assert.Error(t, err)
	assert.Nil(t, goal)
	repo.AssertExpectations(t)
}

func TestSavingsService_GetGoal_NotFound(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(nil, assert.AnError)

	goal, err := svc.GetGoal(ctx(), "user-1", "goal-1")

	assert.Error(t, err)
	assert.Nil(t, goal)
	repo.AssertExpectations(t)
}

func TestSavingsService_ListGoals(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	repo.On("FindByUserID", mock.Anything, uid).Return([]savings.SavingsGoal{*newGoal("a", uid), *newGoal("b", uid)}, nil)

	goals, err := svc.ListGoals(ctx(), uid)

	assert.NoError(t, err)
	assert.Len(t, goals, 2)
	repo.AssertExpectations(t)
}

func TestSavingsService_ListActiveGoals(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	repo.On("FindActiveByUserID", mock.Anything, uid).Return([]savings.SavingsGoal{*newGoal("a", uid)}, nil)

	goals, err := svc.ListActiveGoals(ctx(), uid)

	assert.NoError(t, err)
	assert.Len(t, goals, 1)
	repo.AssertExpectations(t)
}

func TestSavingsService_UpdateGoal_Owned(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", uid), nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*savings.SavingsGoal")).Return(nil)

	newName := "New Home"
	goal, err := svc.UpdateGoal(ctx(), uid, "goal-1", savings.UpdateGoalRequest{Name: &newName})

	assert.NoError(t, err)
	assert.Equal(t, "New Home", goal.Name)
	repo.AssertExpectations(t)
}

func TestSavingsService_UpdateGoal_NotOwner(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", "user-2"), nil)

	goal, err := svc.UpdateGoal(ctx(), "user-1", "goal-1", savings.UpdateGoalRequest{})

	assert.Error(t, err)
	assert.Nil(t, goal)
	repo.AssertExpectations(t)
}

func TestSavingsService_DeleteGoal_Owned(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", uid), nil)
	repo.On("Delete", mock.Anything, "goal-1").Return(nil)

	err := svc.DeleteGoal(ctx(), uid, "goal-1")

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestSavingsService_DeleteGoal_NotOwner(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", "user-2"), nil)

	err := svc.DeleteGoal(ctx(), "user-1", "goal-1")

	assert.Error(t, err)
	repo.AssertExpectations(t)
}

func TestSavingsService_CompleteGoal_Owned(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	g := newGoal("goal-1", uid)
	g.CurrentAmount = 400
	repo.On("FindByID", mock.Anything, "goal-1").Return(g, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*savings.SavingsGoal")).Return(nil)

	goal, err := svc.CompleteGoal(ctx(), uid, "goal-1")

	assert.NoError(t, err)
	assert.Equal(t, "completed", goal.Status)
	assert.Equal(t, 1000.0, goal.CurrentAmount)
	repo.AssertExpectations(t)
}

func TestSavingsService_CompleteGoal_NotOwner(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(newGoal("goal-1", "user-2"), nil)

	goal, err := svc.CompleteGoal(ctx(), "user-1", "goal-1")

	assert.Error(t, err)
	assert.Nil(t, goal)
	repo.AssertExpectations(t)
}

func TestSavingsService_GetSummary(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	summary := &savings.GoalSummary{TotalGoals: 3, ActiveGoals: 2, CompletedGoals: 1}
	repo.On("GetSummary", mock.Anything, uid).Return(summary, nil)

	got, err := svc.GetSummary(ctx(), uid)

	assert.NoError(t, err)
	assert.Equal(t, 3, got.TotalGoals)
	assert.Equal(t, 2, got.ActiveGoals)
	repo.AssertExpectations(t)
}

func TestSavingsService_GetUpcomingObligations(t *testing.T) {
	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	uid := "user-1"

	future := "2026-12-31"
	obligations := []savings.SavingsGoal{*newGoal("a", uid)}
	obligations[0].TargetDate = &future
	obligations[0].AutoReserve = true

	repo.On("GetUpcomingObligations", mock.Anything, uid, 5).Return(obligations, nil)

	goals, err := svc.GetUpcomingObligations(ctx(), uid)

	assert.NoError(t, err)
	assert.Len(t, goals, 1)
	assert.True(t, goals[0].AutoReserve)
	repo.AssertExpectations(t)
}

var _ = time.Now
