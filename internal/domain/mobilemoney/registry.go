package mobilemoney

import "fmt"

// Registry maps a mobile-money currency to the Provider that settles it, so
// the service layer can pick the right adapter (M-Pesa for KES, MTN MoMo for
// UGX/GHS/XAF, Airtel Money for other supported markets) without a big
// switch statement scattered through business logic.
type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider for its currency. A later call for the same
// currency replaces the earlier registration.
func (r *Registry) Register(p Provider) {
	r.providers[p.Currency()] = p
}

// For returns the provider configured for currency, or an error naming the
// unsupported currency if none is registered.
func (r *Registry) For(currency string) (Provider, error) {
	p, ok := r.providers[currency]
	if !ok {
		return nil, fmt.Errorf("no mobile money provider configured for currency %q", currency)
	}
	return p, nil
}

// SupportedCurrencies lists every currency with a registered provider.
func (r *Registry) SupportedCurrencies() []string {
	currencies := make([]string, 0, len(r.providers))
	for c := range r.providers {
		currencies = append(currencies, c)
	}
	return currencies
}
