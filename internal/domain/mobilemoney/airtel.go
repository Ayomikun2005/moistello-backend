package mobilemoney

import (
	"context"
	"fmt"
)

// AirtelConfig holds Airtel Money Open API credentials for one market,
// obtained from https://developers.airtel.africa.
type AirtelConfig struct {
	ClientID        string
	ClientSecret    string
	Country         string // two-letter market code Airtel expects, e.g. "KE", "UG", "TZ"
	TargetCurrency  string // ISO code this deployment settles in
	CallbackBaseURL string
	Sandbox         bool
}

// AirtelProvider is a well-specified stub for Airtel Money, following the
// same shape as MTNProvider: it documents the exact Airtel Open API calls a
// full implementation would make, but does not yet call the live API
// pending a provisioned partner account to test against.
type AirtelProvider struct {
	cfg AirtelConfig
}

func NewAirtelProvider(cfg AirtelConfig) *AirtelProvider {
	return &AirtelProvider{cfg: cfg}
}

func (p *AirtelProvider) Name() string     { return "airtel" }
func (p *AirtelProvider) Currency() string { return p.cfg.TargetCurrency }

func (p *AirtelProvider) Quote(_ context.Context, toCurrency string, usdcAmount float64) (*Quote, error) {
	return &Quote{FromCurrency: "USDC", ToCurrency: toCurrency, FromAmount: usdcAmount, ToAmount: usdcAmount, Rate: 1}, nil
}

// InitiateOnramp would call POST /merchant/v1/payments/ (Collections) with
// an OAuth2 client-credentials Bearer token from /auth/oauth2/token, headers
// X-Country/X-Currency set from cfg.Country/cfg.TargetCurrency, to request
// payment from req.PhoneNumber, then poll
// GET /standard/v1/payments/{transaction_id} for status.
func (p *AirtelProvider) InitiateOnramp(_ context.Context, _ OnrampRequest) (*OnrampResult, error) {
	return nil, fmt.Errorf("airtel money integration not yet implemented — requires a provisioned Airtel Open API partner account to build and test against")
}

// InitiateOfframp would call POST /standard/v1/disbursements/ to pay
// req.PhoneNumber.
func (p *AirtelProvider) InitiateOfframp(_ context.Context, _ OfframpRequest) (*OfframpResult, error) {
	return nil, fmt.Errorf("airtel money integration not yet implemented — requires a provisioned Airtel Open API partner account to build and test against")
}

func (p *AirtelProvider) GetStatus(_ context.Context, providerRef string) (*StatusResult, error) {
	return &StatusResult{ProviderRef: providerRef, Status: StatusPending}, nil
}
