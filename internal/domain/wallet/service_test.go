package wallet

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWalletRepo implements wallet.Repository for unit tests
type mockWalletRepo struct {
	wallets       map[string]*Wallet
	audits        []*WithdrawalRecord
	rateLimitFail bool
	rateLimitErr  error
	dailySpending float64
	createErr     error
	deleteErr     error
}

func newMockWalletRepo() *mockWalletRepo {
	return &mockWalletRepo{
		wallets: make(map[string]*Wallet),
		audits:  make([]*WithdrawalRecord, 0),
	}
}

func (m *mockWalletRepo) Create(ctx context.Context, w *Wallet) error {
	if m.createErr != nil {
		return m.createErr
	}
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	w.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	w.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	m.wallets[w.ID] = w
	return nil
}

func (m *mockWalletRepo) FindByID(ctx context.Context, id string) (*Wallet, error) {
	w, ok := m.wallets[id]
	if !ok {
		return nil, errors.New("wallet not found")
	}
	return w, nil
}

func (m *mockWalletRepo) FindByUserID(ctx context.Context, userID string) ([]Wallet, error) {
	var result []Wallet
	for _, w := range m.wallets {
		if w.UserID == userID {
			result = append(result, *w)
		}
	}
	return result, nil
}

func (m *mockWalletRepo) FindByPublicKey(ctx context.Context, publicKey string) (*Wallet, error) {
	for _, w := range m.wallets {
		if w.PublicKey == publicKey {
			return w, nil
		}
	}
	return nil, errors.New("wallet not found")
}

func (m *mockWalletRepo) Delete(ctx context.Context, id string) error {
	delete(m.wallets, id)
	return nil
}

func (m *mockWalletRepo) DeleteByOwner(ctx context.Context, walletID, userID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	w, ok := m.wallets[walletID]
	if !ok || w.UserID != userID {
		return errors.New("wallet not found or does not belong to user")
	}
	delete(m.wallets, walletID)
	return nil
}

func (m *mockWalletRepo) CheckRateLimit(ctx context.Context, userID uuid.UUID) (bool, error) {
	if m.rateLimitErr != nil {
		return false, m.rateLimitErr
	}
	return !m.rateLimitFail, nil
}

func (m *mockWalletRepo) IncrementRateLimit(ctx context.Context, userID uuid.UUID) error {
	return nil
}

func (m *mockWalletRepo) GetDailySpending(ctx context.Context, userID uuid.UUID) (float64, error) {
	return m.dailySpending, nil
}

func (m *mockWalletRepo) RecordWithdrawalAudit(ctx context.Context, r *WithdrawalRecord) error {
	m.audits = append(m.audits, r)
	return nil
}

func setupTestService(t *testing.T) (*service, *mockWalletRepo) {
	masterKP, err := keypair.Random()
	require.NoError(t, err)

	repo := newMockWalletRepo()
	cfg := Config{
		MasterSecretKey:   masterKP.Seed(),
		MasterPublicKey:   masterKP.Address(),
		HorizonURL:        "https://horizon-testnet.stellar.org",
		USDCIssuer:        masterKP.Address(),
		NetworkPassphrase: "Test SDF Network ; September 2015",
		MinBalanceXLM:     2.0,
	}

	svc, err := NewService(repo, cfg)
	require.NoError(t, err)

	return svc.(*service), repo
}

// ─── Crypto Unit Tests ────────────────────────────────────────────────────────

func TestCrypto_EncryptDecrypt_RoundTrip(t *testing.T) {
	kp, err := keypair.Random()
	require.NoError(t, err)

	seed := make([]byte, 32)
	_, err = rand.Read(seed)
	require.NoError(t, err)

	originalSecret := kp.Seed()

	ciphertext, nonce, err := encryptSecret(originalSecret, seed)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)
	assert.NotEmpty(t, nonce)
	assert.NotEqual(t, []byte(originalSecret), ciphertext)

	decrypted, err := decryptSecret(ciphertext, nonce, seed)
	require.NoError(t, err)
	assert.Equal(t, originalSecret, decrypted)
}

func TestCrypto_Decrypt_TamperedCiphertext(t *testing.T) {
	kp, err := keypair.Random()
	require.NoError(t, err)

	seed := make([]byte, 32)
	_, err = rand.Read(seed)
	require.NoError(t, err)

	ciphertext, nonce, err := encryptSecret(kp.Seed(), seed)
	require.NoError(t, err)

	// Tamper with ciphertext byte
	tamperedCiphertext := make([]byte, len(ciphertext))
	copy(tamperedCiphertext, ciphertext)
	tamperedCiphertext[0] ^= 0xFF

	_, err = decryptSecret(tamperedCiphertext, nonce, seed)
	assert.Error(t, err, "tampered ciphertext must fail authentication tag check")
}

func TestCrypto_Decrypt_TamperedNonce(t *testing.T) {
	kp, err := keypair.Random()
	require.NoError(t, err)

	seed := make([]byte, 32)
	_, err = rand.Read(seed)
	require.NoError(t, err)

	ciphertext, nonce, err := encryptSecret(kp.Seed(), seed)
	require.NoError(t, err)

	// Tamper with nonce byte
	tamperedNonce := make([]byte, len(nonce))
	copy(tamperedNonce, nonce)
	tamperedNonce[0] ^= 0xFF

	_, err = decryptSecret(ciphertext, tamperedNonce, seed)
	assert.Error(t, err, "tampered nonce must fail decryption")
}

func TestCrypto_Decrypt_WrongSeed(t *testing.T) {
	kp, err := keypair.Random()
	require.NoError(t, err)

	correctSeed := make([]byte, 32)
	_, _ = rand.Read(correctSeed)
	wrongSeed := make([]byte, 32)
	_, _ = rand.Read(wrongSeed)

	ciphertext, nonce, err := encryptSecret(kp.Seed(), correctSeed)
	require.NoError(t, err)

	_, err = decryptSecret(ciphertext, nonce, wrongSeed)
	assert.Error(t, err, "decryption with wrong seed must fail")
}

func TestCrypto_DeriveEncryptionKey(t *testing.T) {
	seed := []byte("deterministic-test-seed-12345678")
	keyHex := DeriveEncryptionKey(seed)
	assert.Len(t, keyHex, 64) // SHA-256 hex string

	// Consistent derivation
	keyHex2 := DeriveEncryptionKey(seed)
	assert.Equal(t, keyHex, keyHex2)
}

// ─── DeleteWallet Security Tests ──────────────────────────────────────────────

func TestWalletService_DeleteWallet_Success(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	wID := uuid.NewString()
	userID := uuid.New().String()
	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    userID,
		PublicKey: "GBTEST...",
	}

	err := svc.DeleteWallet(ctx, userID, wID)
	assert.NoError(t, err)
	assert.Empty(t, repo.wallets)
}

func TestWalletService_DeleteWallet_NotFound(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	err := svc.DeleteWallet(ctx, uuid.New().String(), uuid.New().String())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wallet not found")
}

func TestWalletService_DeleteWallet_IDOR_Unauthorized(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	wID := uuid.NewString()
	ownerID := uuid.New().String()
	attackerID := uuid.New().String()

	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    ownerID,
		PublicKey: "GBTEST...",
	}

	// Attacker tries to delete owner's wallet
	err := svc.DeleteWallet(ctx, attackerID, wID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized: wallet does not belong to user")
	assert.NotEmpty(t, repo.wallets, "wallet must not be deleted by unauthorized user")
}

// ─── SendPayment Security & Policy Tests ──────────────────────────────────────

func TestWalletService_SendPayment_InvalidUserID(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	_, err := svc.SendPayment(ctx, "invalid-uuid", []byte("seed"), "GBTEST", "XLM", 10.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid user ID")
}

func TestWalletService_SendPayment_NoWallet(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	userID := uuid.New().String()
	_, err := svc.SendPayment(ctx, userID, []byte("seed"), "GBTEST", "XLM", 10.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no wallet found")
}

func TestWalletService_SendPayment_SelfSendBlocked(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userID := uuid.New()
	kp, _ := keypair.Random()
	wID := uuid.NewString()

	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    userID.String(),
		PublicKey: kp.Address(),
	}

	// Try sending to own wallet address
	_, err := svc.SendPayment(ctx, userID.String(), []byte("seed"), kp.Address(), "XLM", 10.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot send to your own wallet address")

	// Verify security audit record
	require.Len(t, repo.audits, 1)
	assert.Equal(t, "blocked_self_send", repo.audits[0].Status)
	assert.Equal(t, userID, repo.audits[0].UserID)
	assert.Equal(t, kp.Address(), repo.audits[0].Destination)
}

func TestWalletService_SendPayment_RateLimitExceeded(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userID := uuid.New()
	userKP, _ := keypair.Random()
	destKP, _ := keypair.Random()
	wID := uuid.NewString()

	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    userID.String(),
		PublicKey: userKP.Address(),
	}
	repo.rateLimitFail = true // Rate limit check returns false

	_, err := svc.SendPayment(ctx, userID.String(), []byte("seed"), destKP.Address(), "XLM", 5.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded: max 3 withdrawals per hour")

	// Verify rate limit audit record
	require.Len(t, repo.audits, 1)
	assert.Equal(t, "blocked_rate_limit", repo.audits[0].Status)
	assert.Equal(t, userID, repo.audits[0].UserID)
}

func TestWalletService_SendPayment_DailyLimitExceeded(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userID := uuid.New()
	userKP, _ := keypair.Random()
	destKP, _ := keypair.Random()
	wID := uuid.NewString()

	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    userID.String(),
		PublicKey: userKP.Address(),
	}
	repo.dailySpending = 950.0 // Already spent $950 today; daily limit is $1000

	// Sending $100 exceeds daily limit ($950 + $100 = $1050 > $1000)
	_, err := svc.SendPayment(ctx, userID.String(), []byte("seed"), destKP.Address(), "USDC", 100.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "daily spending limit exceeded")

	// Verify daily limit audit record
	require.Len(t, repo.audits, 1)
	assert.Equal(t, "blocked_daily_limit", repo.audits[0].Status)
	assert.Equal(t, userID, repo.audits[0].UserID)
}

func TestWalletService_SendPayment_InvalidStellarAddress(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userID := uuid.New()
	userKP, _ := keypair.Random()
	wID := uuid.NewString()

	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    userID.String(),
		PublicKey: userKP.Address(),
	}

	_, err := svc.SendPayment(ctx, userID.String(), []byte("seed"), "NOT_A_VALID_STELLAR_ADDRESS", "XLM", 10.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Stellar address")
}

func TestWalletService_SendPayment_DecryptionFailure(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userID := uuid.New()
	userKP, _ := keypair.Random()
	destKP, _ := keypair.Random()
	wID := uuid.NewString()

	correctSeed := []byte("correct-passkey-seed-32bytes-len")
	encSecret, nonce, err := encryptSecret(userKP.Seed(), correctSeed)
	require.NoError(t, err)

	repo.wallets[wID] = &Wallet{
		ID:                 wID,
		UserID:             userID.String(),
		PublicKey:          userKP.Address(),
		EncryptedSecretKey: encSecret,
		EncryptionNonce:    nonce,
	}

	wrongSeed := []byte("wrong-passkey-seed-32bytes-lengt")
	_, err = svc.SendPayment(ctx, userID.String(), wrongSeed, destKP.Address(), "XLM", 10.0, "", "127.0.0.1", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decrypting secret key")
}

// ─── SignTransaction Tests ────────────────────────────────────────────────────

func TestWalletService_SignTransaction_WalletNotFound(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	_, err := svc.SignTransaction(ctx, uuid.NewString(), []byte("seed"), "AAAA...")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wallet not found")
}

func TestWalletService_SignTransaction_MissingKeys(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	wID := uuid.NewString()
	repo.wallets[wID] = &Wallet{
		ID:        wID,
		UserID:    uuid.New().String(),
		PublicKey: "GBTEST...",
	}

	_, err := svc.SignTransaction(ctx, wID, []byte("seed"), "AAAA...")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wallet has no encrypted secret key")
}

func TestWalletService_SignTransaction_InvalidXDR(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userKP, _ := keypair.Random()
	seed := []byte("passkey-seed-for-signing-32bytes")
	encSecret, nonce, err := encryptSecret(userKP.Seed(), seed)
	require.NoError(t, err)

	wID := uuid.NewString()
	repo.wallets[wID] = &Wallet{
		ID:                 wID,
		UserID:             uuid.New().String(),
		PublicKey:          userKP.Address(),
		EncryptedSecretKey: encSecret,
		EncryptionNonce:    nonce,
	}

	_, err = svc.SignTransaction(ctx, wID, seed, "NOT_VALID_BASE64_XDR!!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing transaction XDR")
}

func TestWalletService_GetWallets(t *testing.T) {
	svc, repo := setupTestService(t)
	ctx := context.Background()

	userID := uuid.NewString()
	w1 := &Wallet{ID: "w1", UserID: userID, PublicKey: "G1..."}
	w2 := &Wallet{ID: "w2", UserID: userID, PublicKey: "G2..."}
	repo.wallets["w1"] = w1
	repo.wallets["w2"] = w2

	wallets, err := svc.GetWallets(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, wallets, 2)
}
