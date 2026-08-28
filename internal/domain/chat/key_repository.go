package chat

import "context"

// OneTimePreKeyInput is a single one-time prekey a client publishes as part
// of replenishing its pool.
type OneTimePreKeyInput struct {
	KeyID  int
	PubKey string // X25519 public key, hex-encoded
}

// KeyRepository persists the X3DH key material described in
// specs/chat-encryption.md: a user's long-term identity key, their current
// signed prekey, and their pool of one-time prekeys.
type KeyRepository interface {
	UpsertIdentityKey(ctx context.Context, userID, identityKeyPub string) error
	UpsertSignedPreKey(ctx context.Context, userID string, keyID int, signedPreKeyPub, signature string) error
	AddOneTimePreKeys(ctx context.Context, userID string, keys []OneTimePreKeyInput) error
	// CountUnusedOneTimePreKeys lets a client know when its published pool
	// is running low and it should replenish.
	CountUnusedOneTimePreKeys(ctx context.Context, userID string) (int, error)
	// GetBundle assembles a PreKeyBundle for userID and, if an unused
	// one-time prekey is available, atomically marks one as used and
	// includes it — so the same one-time prekey is never handed out to two
	// different handshake initiators.
	GetBundle(ctx context.Context, userID string) (*PreKeyBundle, error)
}
