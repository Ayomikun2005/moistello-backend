package circle

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type CircleType string

const (
	CircleTypePublic  CircleType = "public"
	CircleTypePrivate CircleType = "private"
	CircleTypePremium CircleType = "premium"
)

type PayoutType string

const (
	PayoutTypeRandom  PayoutType = "random"
	PayoutTypeFixed   PayoutType = "fixed"
	PayoutTypeAuction PayoutType = "auction"
	PayoutTypeVote    PayoutType = "vote"
)

type CircleFrequency string

const (
	FrequencyDaily    CircleFrequency = "daily"
	FrequencyWeekly   CircleFrequency = "weekly"
	FrequencyBiweekly CircleFrequency = "biweekly"
	FrequencyMonthly  CircleFrequency = "monthly"
)

type CircleCurrency string

const (
	CurrencyUSDC CircleCurrency = "USDC"
	CurrencyXLM  CircleCurrency = "XLM"
)

type CircleStatus string

const (
	CircleStatusPending   CircleStatus = "pending"
	CircleStatusActive    CircleStatus = "active"
	CircleStatusCompleted CircleStatus = "completed"
	CircleStatusCancelled CircleStatus = "cancelled"
)

type MemberStatus string

const (
	MemberStatusActive  MemberStatus = "active"
	MemberStatusExited  MemberStatus = "exited"
	MemberStatusRemoved MemberStatus = "removed"
)

type Circle struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	ContractID         sql.NullString  `json:"contractId,omitempty" db:"contract_id"`
	CommunityID        *uuid.UUID      `json:"communityId,omitempty" db:"community_id"`
	Name               string          `json:"name" db:"name"`
	Description        sql.NullString  `json:"description,omitempty" db:"description"`
	CircleType         CircleType      `json:"circleType" db:"circle_type"`
	PayoutType         PayoutType      `json:"payoutType" db:"payout_type"`
	ContributionAmount float64         `json:"contributionAmount" db:"contribution_amount"`
	Currency           CircleCurrency  `json:"currency" db:"currency"`
	Frequency          CircleFrequency `json:"frequency" db:"frequency"`
	MaxMembers         int             `json:"maxMembers" db:"max_members"`
	MinMoiScore        int             `json:"minMoiScore" db:"min_moi_score"`
	CollateralPercent  float64         `json:"collateralPercent" db:"collateral_percent"`
	LateFeePercent     float64         `json:"lateFeePercent" db:"late_fee_percent"`
	GracePeriodHours   int             `json:"gracePeriodHours" db:"grace_period_hours"`
	MaxStrikes         int             `json:"maxStrikes" db:"max_strikes"`
	MemberCount        int             `json:"memberCount" db:"member_count"`
	RequiresInvite     bool            `json:"requiresInvite" db:"requires_invite"`
	StartDate          sql.NullTime    `json:"startDate,omitempty" db:"start_date"`
	EndDate            sql.NullTime    `json:"endDate,omitempty" db:"end_date"`
	Status             CircleStatus    `json:"status" db:"status"`
	CurrentRound       int             `json:"currentRound" db:"current_round"`
	TotalContributions float64         `json:"totalContributions" db:"total_contributions"`
	OrganizerID        uuid.UUID       `json:"organizerId" db:"organizer_id"`
	CreatedAt          time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time       `json:"updatedAt" db:"updated_at"`
	DeletedAt          *time.Time      `json:"deletedAt,omitempty" db:"deleted_at"`
}

type CircleMember struct {
	CircleID uuid.UUID    `json:"circleId" db:"circle_id"`
	UserID   uuid.UUID    `json:"userId" db:"user_id"`
	Position int          `json:"position" db:"position"`
	Status   MemberStatus `json:"status" db:"status"`
	JoinedAt time.Time    `json:"joinedAt" db:"joined_at"`
}

type PenaltyType string

const (
	PenaltyTypeLate      PenaltyType = "late"
	PenaltyTypeDefault   PenaltyType = "default"
	PenaltyTypeEarlyExit PenaltyType = "early_exit"
)

type Penalty struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	CircleID       uuid.UUID      `json:"circleId" db:"circle_id"`
	UserID         uuid.UUID      `json:"userId" db:"user_id"`
	RoundNumber    int            `json:"roundNumber" db:"round_number"`
	PenaltyType    PenaltyType    `json:"penaltyType" db:"penalty_type"`
	Amount         float64        `json:"amount" db:"amount"`
	StrikesApplied int            `json:"strikesApplied" db:"strikes_applied"`
	Reason         sql.NullString `json:"reason,omitempty" db:"reason"`
	CreatedAt      time.Time      `json:"createdAt" db:"created_at"`
}
