package incentives

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// IncentiveType represents the type of incentive/bonus
type IncentiveType string

const (
	IncentiveTypeReferral           IncentiveType = "referral"
	IncentiveTypeCircleCompletion   IncentiveType = "circle_completion"
	IncentiveTypeContributionMatch  IncentiveType = "contribution_match"
	IncentiveTypeSavingsStreak      IncentiveType = "savings_streak"
	IncentiveTypeFirstDeposit       IncentiveType = "first_deposit"
)

// IncentiveStatus represents the status of an incentive
type IncentiveStatus string

const (
	IncentiveStatusPending   IncentiveStatus = "pending"
	IncentiveStatusClaimed   IncentiveStatus = "claimed"
	IncentiveStatusExpired   IncentiveStatus = "expired"
	IncentiveStatusCancelled IncentiveStatus = "cancelled"
)

// Incentive represents a bonus/reward for a user
type Incentive struct {
	ID           uuid.UUID       `db:"id" json:"id"`
	UserID       uuid.UUID       `db:"user_id" json:"userId"`
	Type         IncentiveType   `db:"type" json:"type"`
	Status       IncentiveStatus `db:"status" json:"status"`
	Amount       float64         `db:"amount" json:"amount"`
	Currency     string          `db:"currency" json:"currency"`
	Metadata     sql.NullString  `db:"metadata" json:"metadata,omitempty"`
	ReferenceID  sql.NullString  `db:"reference_id" json:"referenceId,omitempty"` // Circle ID, referral code, etc.
	ExpiresAt    sql.NullTime    `db:"expires_at" json:"expiresAt,omitempty"`
	ClaimedAt    sql.NullTime    `db:"claimed_at" json:"claimedAt,omitempty"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updatedAt"`
}

// Referral represents a referral relationship
type Referral struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	ReferrerID   uuid.UUID  `db:"referrer_id" json:"referrerId"`
	ReferredID   uuid.UUID  `db:"referred_id" json:"referredId"`
	ReferralCode string     `db:"referral_code" json:"referralCode"`
	Status       string     `db:"status" json:"status"` // pending, completed, expired
	CompletedAt  sql.NullTime `db:"completed_at" json:"completedAt,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
}

// SavingsStreak represents a user's savings streak
type SavingsStreak struct {
	ID              uuid.UUID `db:"id" json:"id"`
	UserID          uuid.UUID `db:"user_id" json:"userId"`
	CurrentStreak   int       `db:"current_streak" json:"currentStreak"`
	LongestStreak   int       `db:"longest_streak" json:"longestStreak"`
	LastContributionAt sql.NullTime `db:"last_contribution_at" json:"lastContributionAt,omitempty"`
	BonusTier       int       `db:"bonus_tier" json:"bonusTier"` // 1, 2, 3 based on streak length
	CreatedAt       time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time `db:"updated_at" json:"updatedAt"`
}

// IncentiveConfig represents configurable bonus parameters
type IncentiveConfig struct {
	ID                       uuid.UUID `db:"id" json:"id"`
	ReferralBonusAmount      float64   `db:"referral_bonus_amount" json:"referralBonusAmount"`
	ReferralBonusCurrency    string    `db:"referral_bonus_currency" json:"referralBonusCurrency"`
	CircleCompletionBonus    float64   `db:"circle_completion_bonus" json:"circleCompletionBonus"`
	CircleCompletionCurrency string    `db:"circle_completion_currency" json:"circleCompletionCurrency"`
	ContributionMatchPercent float64   `db:"contribution_match_percent" json:"contributionMatchPercent"`
	ContributionMatchMax     float64   `db:"contribution_match_max" json:"contributionMatchMax"`
	StreakBonusTier1         int       `db:"streak_bonus_tier1" json:"streakBonusTier1"` // consecutive contributions
	StreakBonusTier1Amount   float64   `db:"streak_bonus_tier1_amount" json:"streakBonusTier1Amount"`
	StreakBonusTier2         int       `db:"streak_bonus_tier2" json:"streakBonusTier2"`
	StreakBonusTier2Amount   float64   `db:"streak_bonus_tier2_amount" json:"streakBonusTier2Amount"`
	StreakBonusTier3         int       `db:"streak_bonus_tier3" json:"streakBonusTier3"`
	StreakBonusTier3Amount   float64   `db:"streak_bonus_tier3_amount" json:"streakBonusTier3Amount"`
	FirstDepositBonus        float64   `db:"first_deposit_bonus" json:"firstDepositBonus"`
	FirstDepositCurrency     string    `db:"first_deposit_currency" json:"firstDepositCurrency"`
	FirstDepositMinAmount    float64   `db:"first_deposit_min_amount" json:"firstDepositMinAmount"`
	IsActive                 bool      `db:"is_active" json:"isActive"`
	CreatedAt                time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt                time.Time `db:"updated_at" json:"updatedAt"`
}

// UserIncentiveSummary represents a summary of a user's incentives
type UserIncentiveSummary struct {
	TotalEarned      float64 `json:"totalEarned"`
	TotalClaimed     float64 `json:"totalClaimed"`
	PendingAmount    float64 `json:"pendingAmount"`
	ReferralCount    int     `json:"referralCount"`
	CurrentStreak    int     `json:"currentStreak"`
	LongestStreak    int     `json:"longestStreak"`
	BonusTier        int     `json:"bonusTier"`
}
