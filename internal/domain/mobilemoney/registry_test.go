package mobilemoney_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/mobilemoney"
)

func TestRegistry_ForReturnsRegisteredProvider(t *testing.T) {
	r := mobilemoney.NewRegistry()
	mpesa := newFakeProvider("mpesa", "KES")
	r.Register(mpesa)

	got, err := r.For("KES")
	require.NoError(t, err)
	assert.Equal(t, mpesa, got)
}

func TestRegistry_ForUnregisteredCurrency(t *testing.T) {
	r := mobilemoney.NewRegistry()
	_, err := r.For("UGX")
	assert.Error(t, err)
}

func TestRegistry_LaterRegistrationReplacesEarlier(t *testing.T) {
	r := mobilemoney.NewRegistry()
	first := newFakeProvider("first", "KES")
	second := newFakeProvider("second", "KES")
	r.Register(first)
	r.Register(second)

	got, err := r.For("KES")
	require.NoError(t, err)
	assert.Equal(t, second, got)
}

func TestRegistry_SupportedCurrencies(t *testing.T) {
	r := mobilemoney.NewRegistry()
	r.Register(newFakeProvider("mpesa", "KES"))
	r.Register(newFakeProvider("mtn", "UGX"))

	currencies := r.SupportedCurrencies()
	assert.ElementsMatch(t, []string{"KES", "UGX"}, currencies)
}
