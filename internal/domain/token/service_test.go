package token

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/domain/wallet"
)

type mockWalletRepoForToken struct {
	wallets map[string]*wallet.Wallet
}

func newMockWalletRepoForToken() *mockWalletRepoForToken {
	return &mockWalletRepoForToken{
		wallets: make(map[string]*wallet.Wallet),
	}
}

func (m *mockWalletRepoForToken) Create(ctx context.Context, w *wallet.Wallet) error {
	m.wallets[w.ID] = w
	return nil
}

func (m *mockWalletRepoForToken) FindByID(ctx context.Context, id string) (*wallet.Wallet, error) {
	w, ok := m.wallets[id]
	if !ok {
		return nil, errors.New("wallet not found")
	}
	return w, nil
}

func (m *mockWalletRepoForToken) FindByUserID(ctx context.Context, userID string) ([]wallet.Wallet, error) {
	var result []wallet.Wallet
	for _, w := range m.wallets {
		if w.UserID == userID {
			result = append(result, *w)
		}
	}
	return result, nil
}

func (m *mockWalletRepoForToken) FindByPublicKey(ctx context.Context, publicKey string) (*wallet.Wallet, error) {
	for _, w := range m.wallets {
		if w.PublicKey == publicKey {
			return w, nil
		}
	}
	return nil, errors.New("wallet not found")
}

func (m *mockWalletRepoForToken) Delete(ctx context.Context, id string) error { return nil }
func (m *mockWalletRepoForToken) DeleteByOwner(ctx context.Context, walletID, userID string) error {
	return nil
}
func (m *mockWalletRepoForToken) CheckRateLimit(ctx context.Context, userID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockWalletRepoForToken) IncrementRateLimit(ctx context.Context, userID uuid.UUID) error {
	return nil
}
func (m *mockWalletRepoForToken) GetDailySpending(ctx context.Context, userID uuid.UUID) (float64, error) {
	return 0, nil
}
func (m *mockWalletRepoForToken) RecordWithdrawalAudit(ctx context.Context, r *wallet.WithdrawalRecord) error {
	return nil
}

// ─── Token Service Unit Tests ─────────────────────────────────────────────────

func TestTokenService_NewService_MissingContractID(t *testing.T) {
	repo := newMockWalletRepoForToken()
	_, err := NewService(repo, Config{
		GovernanceTokenContractID: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "governance token contract ID is required")
}

func TestTokenService_Stake_WalletNotFound(t *testing.T) {
	repo := newMockWalletRepoForToken()
	svc, err := NewService(repo, Config{
		GovernanceTokenContractID: "CCONTACT...",
		SorobanRPCURL:             "http://localhost:8000",
	})
	require.NoError(t, err)

	_, err = svc.Stake(context.Background(), uuid.NewString(), []byte("seed"), 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user wallet not found")
}

func TestTokenService_Unstake_WalletNotFound(t *testing.T) {
	repo := newMockWalletRepoForToken()
	svc, err := NewService(repo, Config{
		GovernanceTokenContractID: "CCONTACT...",
		SorobanRPCURL:             "http://localhost:8000",
	})
	require.NoError(t, err)

	_, err = svc.Unstake(context.Background(), uuid.NewString(), []byte("seed"), 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user wallet not found")
}

func TestTokenService_Stake_DecryptSecretFailure(t *testing.T) {
	repo := newMockWalletRepoForToken()
	userID := uuid.NewString()
	repo.wallets["w1"] = &wallet.Wallet{
		ID:                 "w1",
		UserID:             userID,
		EncryptedSecretKey: []byte("corrupted-secret"),
		EncryptionNonce:    []byte("bad-nonce"),
	}

	svc, err := NewService(repo, Config{
		GovernanceTokenContractID: "CCONTACT...",
		SorobanRPCURL:             "http://localhost:8000",
	})
	require.NoError(t, err)

	_, err = svc.Stake(context.Background(), userID, []byte("seed"), 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decrypting wallet secret")
}

// ─── scvalToUint64 Conversion Tests ──────────────────────────────────────────

func TestSCValToUint64(t *testing.T) {
	// 1. u64
	val, err := scvalToUint64(map[string]any{"u64": "1234567890"})
	require.NoError(t, err)
	assert.Equal(t, uint64(1234567890), val)

	// 2. u32
	val, err = scvalToUint64(map[string]any{"u32": "42"})
	require.NoError(t, err)
	assert.Equal(t, uint64(42), val)

	// 3. i64
	val, err = scvalToUint64(map[string]any{"i64": "999"})
	require.NoError(t, err)
	assert.Equal(t, uint64(999), val)

	// 4. u128 with hi=0
	val, err = scvalToUint64(map[string]any{"u128": map[string]any{"hi": "0", "lo": "500"}})
	require.NoError(t, err)
	assert.Equal(t, uint64(500), val)

	// 5. u128 with hi > 0 (overflows uint64)
	_, err = scvalToUint64(map[string]any{"u128": map[string]any{"hi": "1", "lo": "500"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds uint64")

	// 6. Invalid type
	_, err = scvalToUint64("not a map")
	assert.Error(t, err)

	// 7. Missing numeric field
	_, err = scvalToUint64(map[string]any{"str": "hello"})
	assert.Error(t, err)
}
