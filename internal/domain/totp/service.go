package totp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Service handles TOTP (Time-based One-Time Password) operations
// using the standard TOTP algorithm (RFC 6238).
type Service struct{}

func NewService() *Service {
	return &Service{}
}

// GenerateSecret creates a new TOTP secret and returns it along with
// the standard otpauth:// URI for QR code generation.
// email is used to identify the account in the authenticator app.
func (s *Service) GenerateSecret(email string) (secret string, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Moistello",
		AccountName: email,
		Period:      30,
		SecretSize:  20,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating TOTP secret: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// ValidateCode checks if the provided TOTP code is valid for the given secret.
// It allows a 30-second skew (one period before and after) to account for
// clock drift between the server and the authenticator app.
func (s *Service) ValidateCode(secret, code string) bool {
	valid, err := 	totp.ValidateCustom(
		code,
		secret,
		time.Now().UTC(),
		totp.ValidateOpts{
			Period:    30,
			Skew:      1,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		},
	)
	return err == nil && valid
}

// BackupCode represents a single backup/recovery code.
type BackupCode struct {
	Plain string // shown to user once
	Hash  string // stored as SHA-256
}

// GenerateBackupCodes creates 10 backup codes, each 12 alphanumeric characters.
// Returns both the plain versions (to show the user) and hashed versions (to store).
func (s *Service) GenerateBackupCodes() ([]BackupCode, error) {
	codes := make([]BackupCode, 10)
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I,O,0,1 to avoid confusion
	for i := range codes {
		plain := make([]byte, 12)
		for j := range plain {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return nil, fmt.Errorf("generating backup code: %w", err)
			}
			plain[j] = charset[n.Int64()]
			if j == 3 || j == 7 {
				plain[j] = '-'
			}
		}
		code := string(plain)
		hash := sha256.Sum256([]byte(code))
		codes[i] = BackupCode{
			Plain: code,
			Hash:  hex.EncodeToString(hash[:]),
		}
	}
	return codes, nil
}

// ValidateBackupCode checks a plain backup code against the stored hashed codes.
// Returns the updated list of hashed codes (with the used code removed) and true
// if the code was valid, or nil and false if invalid.
func (s *Service) ValidateBackupCode(plainCode string, hashedCodes []string) ([]string, bool) {
	hash := sha256.Sum256([]byte(plainCode))
	hashStr := hex.EncodeToString(hash[:])
	for i, stored := range hashedCodes {
		if stored == hashStr {
			// Remove the used code
			remaining := make([]string, 0, len(hashedCodes)-1)
			remaining = append(remaining, hashedCodes[:i]...)
			remaining = append(remaining, hashedCodes[i+1:]...)
			return remaining, true
		}
	}
	return nil, false
}

// HashBackupCodes returns only the hashed versions of backup codes for storage.
func (s *Service) HashBackupCodes(codes []BackupCode) []string {
	hashed := make([]string, len(codes))
	for i, c := range codes {
		hashed[i] = c.Hash
	}
	return hashed
}
