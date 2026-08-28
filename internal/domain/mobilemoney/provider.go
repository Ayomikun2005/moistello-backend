// Package mobilemoney implements the mobile-money bridge described in
// product-spec.md §7 (Phase 2): off-ramp/on-ramp between USDC on Stellar and
// mobile-money wallets (M-Pesa, MTN Mobile Money, Airtel Money) for non-NGN
// African markets. NGN already has a dedicated bridge via
// internal/domain/yellowcard; this package covers everything else.
package mobilemoney

import "context"

// Quote is a provider's FX quote for converting between a mobile-money
// currency and USDC.
type Quote struct {
	FromCurrency  string
	ToCurrency    string
	FromAmount    float64
	ToAmount      float64
	Rate          float64
	FeePercentage float64
}

// OnrampRequest asks a provider to collect funds from a customer's mobile
// money wallet (e.g. an M-Pesa STK push) in exchange for USDC sent to
// DestinationAddress once the collection settles.
type OnrampRequest struct {
	Amount             float64
	Currency           string // provider's local currency, e.g. "KES", "UGX", "GHS"
	PhoneNumber        string // MSISDN in international format, e.g. "254712345678"
	DestinationAddress string // Stellar public key to receive USDC
	Reference          string // caller-supplied idempotent reference
}

// OnrampResult is returned immediately after an on-ramp collection request
// is accepted by the provider; final settlement is asynchronous and must be
// confirmed via GetStatus (polling) or the provider's callback/webhook.
type OnrampResult struct {
	ProviderRef string // provider's transaction/request ID
	Status      Status
}

// OfframpRequest asks a provider to disburse funds to a customer's mobile
// money wallet (e.g. an M-Pesa B2C payment) after USDC has been received
// from the customer's Stellar wallet.
type OfframpRequest struct {
	Amount      float64
	Currency    string
	PhoneNumber string
	AccountName string
	Reference   string
}

// OfframpResult is returned immediately after an off-ramp disbursement
// request is accepted by the provider.
type OfframpResult struct {
	ProviderRef string
	Status      Status
}

// StatusResult reports the current state of a previously-initiated
// transaction when polled via GetStatus.
type StatusResult struct {
	ProviderRef   string
	Status        Status
	FailureReason string
}

// Status is a provider-agnostic transaction status. Each concrete provider
// maps its own status vocabulary onto this set.
type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Provider is implemented by each mobile-money integration (M-Pesa, MTN
// MoMo, Airtel Money, ...). The service layer selects a Provider per
// transaction via a Registry keyed by currency, so callers never need to
// know which concrete provider handles a given market.
type Provider interface {
	// Name identifies the provider, e.g. "mpesa", "mtn", "airtel".
	Name() string
	// Currency is the ISO 4217 code this provider instance settles in.
	Currency() string
	Quote(ctx context.Context, toCurrency string, usdcAmount float64) (*Quote, error)
	InitiateOnramp(ctx context.Context, req OnrampRequest) (*OnrampResult, error)
	InitiateOfframp(ctx context.Context, req OfframpRequest) (*OfframpResult, error)
	GetStatus(ctx context.Context, providerRef string) (*StatusResult, error)
}
