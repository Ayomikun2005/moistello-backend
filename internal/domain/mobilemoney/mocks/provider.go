package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/mobilemoney"
)

type Provider struct {
	mock.Mock
	NameVal     string
	CurrencyVal string
}

func (m *Provider) Name() string     { return m.NameVal }
func (m *Provider) Currency() string { return m.CurrencyVal }

func (m *Provider) Quote(ctx context.Context, toCurrency string, usdcAmount float64) (*mobilemoney.Quote, error) {
	args := m.Called(ctx, toCurrency, usdcAmount)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mobilemoney.Quote), args.Error(1)
}

func (m *Provider) InitiateOnramp(ctx context.Context, req mobilemoney.OnrampRequest) (*mobilemoney.OnrampResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mobilemoney.OnrampResult), args.Error(1)
}

func (m *Provider) InitiateOfframp(ctx context.Context, req mobilemoney.OfframpRequest) (*mobilemoney.OfframpResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mobilemoney.OfframpResult), args.Error(1)
}

func (m *Provider) GetStatus(ctx context.Context, providerRef string) (*mobilemoney.StatusResult, error) {
	args := m.Called(ctx, providerRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mobilemoney.StatusResult), args.Error(1)
}
