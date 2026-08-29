package savings

import "time"

type SavingsGoal struct {
	ID            string    `json:"id" db:"id"`
	UserID        string    `json:"userId" db:"user_id"`
	Name          string    `json:"name" db:"name"`
	Description   string    `json:"description" db:"description"`
	TargetAmount  float64   `json:"targetAmount" db:"target_amount"`
	CurrentAmount float64   `json:"currentAmount" db:"current_amount"`
	TargetDate    *string   `json:"targetDate,omitempty" db:"target_date"`
	CircleID      *string   `json:"circleId,omitempty" db:"circle_id"`
	AutoReserve   bool      `json:"autoReserve" db:"auto_reserve"`
	Status        string    `json:"status" db:"status"`
	CreatedAt     time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time `json:"updatedAt" db:"updated_at"`
}

type CreateGoalRequest struct {
	Name         string  `json:"name" binding:"required"`
	Description  string  `json:"description"`
	TargetAmount float64 `json:"targetAmount" binding:"required,gt=0"`
	TargetDate   *string `json:"targetDate"`
	CircleID     *string `json:"circleId"`
	AutoReserve  bool    `json:"autoReserve"`
}

type UpdateGoalRequest struct {
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	TargetAmount  *float64 `json:"targetAmount"`
	CurrentAmount *float64 `json:"currentAmount"`
	TargetDate    *string  `json:"targetDate"`
	CircleID      *string  `json:"circleId"`
	AutoReserve   *bool    `json:"autoReserve"`
	Status        *string  `json:"status"`
}

type GoalSummary struct {
	TotalGoals     int     `json:"totalGoals"`
	ActiveGoals    int     `json:"activeGoals"`
	CompletedGoals int     `json:"completedGoals"`
	TotalTarget    float64 `json:"totalTarget"`
	TotalSaved     float64 `json:"totalSaved"`
	OverallPercent float64 `json:"overallPercent"`
	SavingsStreak  int     `json:"savingsStreak"`
}
