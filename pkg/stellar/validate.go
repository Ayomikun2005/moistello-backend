package stellar

import (
	"fmt"
	"regexp"
)

// AddressRegexp matches valid Stellar public key format:
// Starts with G, exactly 56 characters, Base32 alphabet (A-Z, 2-7).
var AddressRegexp = regexp.MustCompile(`^G[A-Z2-7]{55}$`)

// ValidateAddress performs a quick format check on a Stellar address.
// It verifies length, prefix, and character validity.
func ValidateAddress(address string) error {
	if len(address) != 56 {
		return fmt.Errorf("stellar address must be 56 characters, got %d", len(address))
	}
	if address[0] != 'G' {
		return fmt.Errorf("stellar address must start with G, got %c", address[0])
	}
	if !AddressRegexp.MatchString(address) {
		return fmt.Errorf("stellar address contains invalid characters")
	}
	return nil
}
