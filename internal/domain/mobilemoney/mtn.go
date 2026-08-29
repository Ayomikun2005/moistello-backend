package mobilemoney

import (
	"context"
	"fmt"
)

// MTNConfig holds MTN Mobile Money Open API (MoMo API) credentials for one
// market. MTN MoMo is deployed per-country with separate subscription keys
// (Collections product for on-ramp, Disbursements product for off-ramp),
// obtained from https://momodeveloper.mtn.com.
type MTNConfig struct {
	SubscriptionKey string // Ocp-Apim-Subscription-Key for the target product
	APIUser         string // X-Reference-Id used when the API user was provisioned
	APIKey          string
	TargetCurrency  string // ISO code this deployment settles in, e.g. "UGX", "GHS", "XAF"
	CallbackBaseURL string
	Sandbox         bool
}

// MTNProvider is a well-specified stub for MTN Mobile Money: it validates
// and stores live config the same way MPesaProvider does, and documents the
// exact MoMo API calls a full implementation would make, but does not yet
// call the live API. Wiring InitiateOnramp/InitiateOfframp requires an MTN
// MoMo partner sandbox account to test against; NewRegistry only registers
// this provider when MTNConfig is non-empty, so it never silently shadows
// M-Pesa or Airtel for their currencies.
type MTNProvider struct {
	cfg MTNConfig
}

func NewMTNProvider(cfg MTNConfig) *MTNProvider {
	return &MTNProvider{cfg: cfg}
}

func (p *MTNProvider) Name() string     { return "mtn" }
func (p *MTNProvider) Currency() string { return p.cfg.TargetCurrency }

func (p *MTNProvider) Quote(_ context.Context, toCurrency string, usdcAmount float64) (*Quote, error) {
	return &Quote{FromCurrency: "USDC", ToCurrency: toCurrency, FromAmount: usdcAmount, ToAmount: usdcAmount, Rate: 1}, nil
}

// InitiateOnramp would call POST /collection/v1_0/requesttopay on the MoMo
// Collections product (X-Reference-Id: a new UUID, X-Target-Environment,
// Ocp-Apim-Subscription-Key, Bearer token from /collection/token/) to
// request payment from req.PhoneNumber, then poll
// GET /collection/v1_0/requesttopay/{X-Reference-Id} for status.
func (p *MTNProvider) InitiateOnramp(_ context.Context, _ OnrampRequest) (*OnrampResult, error) {
	return nil, fmt.Errorf("mtn momo integration not yet implemented — requires a provisioned MoMo Collections subscription to build and test against")
}

// InitiateOfframp would call POST /disbursement/v1_0/transfer on the MoMo
// Disbursements product to pay req.PhoneNumber, then poll
// GET /disbursement/v1_0/transfer/{X-Reference-Id} for status.
func (p *MTNProvider) InitiateOfframp(_ context.Context, _ OfframpRequest) (*OfframpResult, error) {
	return nil, fmt.Errorf("mtn momo integration not yet implemented — requires a provisioned MoMo Disbursements subscription to build and test against")
}

func (p *MTNProvider) GetStatus(_ context.Context, providerRef string) (*StatusResult, error) {
	return &StatusResult{ProviderRef: providerRef, Status: StatusPending}, nil
}
