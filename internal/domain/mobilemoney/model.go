package mobilemoney

import "time"

type Direction string

const (
	DirectionOnramp  Direction = "onramp"  // mobile money -> USDC
	DirectionOfframp Direction = "offramp" // USDC -> mobile money
)

// Transaction is the persisted record of one mobile-money bridge operation,
// mirroring the deposit/withdrawal pattern used for the YellowCard (NGN)
// bridge so reconciliation and idempotency work the same way across both.
type Transaction struct {
	ID                 string     `json:"id" db:"id"`
	UserID             string     `json:"userId" db:"user_id"`
	Provider           string     `json:"provider" db:"provider"`
	Direction          Direction  `json:"direction" db:"direction"`
	Currency           string     `json:"currency" db:"currency"`
	Amount             float64    `json:"amount" db:"amount"`
	USDCAmount         float64    `json:"usdcAmount" db:"usdc_amount"`
	PhoneNumber        string     `json:"phoneNumber" db:"phone_number"`
	DestinationAddress string     `json:"destinationAddress,omitempty" db:"destination_address"`
	Status             Status     `json:"status" db:"status"`
	ProviderRef        string     `json:"providerRef,omitempty" db:"provider_ref"`
	IdempotencyKey     string     `json:"idempotencyKey" db:"idempotency_key"`
	FailureReason      *string    `json:"failureReason,omitempty" db:"failure_reason"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	CompletedAt        *time.Time `json:"completedAt,omitempty" db:"completed_at"`
}
