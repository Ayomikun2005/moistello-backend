package deposit

import "time"

// DepositStatus tracks a Yellow Card NGN→USDC deposit (receive) through its
// lifecycle, from bank transfer instructions being issued to USDC landing in
// the user's wallet.
type DepositStatus string

const (
	DepositStatusPending    DepositStatus = "pending"    // waiting for user's NGN bank transfer
	DepositStatusProcessing DepositStatus = "processing" // Yellow Card received NGN, converting to USDC
	DepositStatusCompleted  DepositStatus = "completed"  // USDC delivered to the user's wallet
	DepositStatusFailed     DepositStatus = "failed"
	DepositStatusExpired    DepositStatus = "expired" // payment window elapsed with no transfer
)

// Deposit is the platform's persisted record of a Yellow Card receive
// request. It is the source of truth reconciled against Yellow Card webhook
// notifications so the two can never silently diverge.
type Deposit struct {
	ID                 string        `json:"id" db:"id"`
	UserID             string        `json:"userId" db:"user_id"`
	AmountNGN          float64       `json:"amountNgn" db:"amount_ngn"`
	EstimatedUSDC      float64       `json:"estimatedUsdc" db:"estimated_usdc"`
	DestinationAddress string        `json:"destinationAddress" db:"destination_address"`
	Status             DepositStatus `json:"status" db:"status"`

	// ReceiveID is Yellow Card's identifier for the receive request.
	ReceiveID string `json:"receiveId" db:"receive_id"`
	// PaymentRef is the reference we generated and handed to Yellow Card;
	// webhook deliveries echo it back so we can look up the local record.
	PaymentRef string `json:"paymentRef" db:"payment_ref"`

	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	CompletedAt   *time.Time `json:"completedAt,omitempty" db:"completed_at"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty" db:"expires_at"`
	FailureReason *string    `json:"failureReason,omitempty" db:"failure_reason"`
}
