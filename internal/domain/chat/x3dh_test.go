package chat

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestX3DH_KeyGenerationAndHandshake(t *testing.T) {
	// Generate Alice keys
	aliceIK, err := GenerateKeyPair()
	require.NoError(t, err)
	aliceEK, err := GenerateKeyPair()
	require.NoError(t, err)

	// Generate Bob keys
	bobIK, err := GenerateKeyPair()
	require.NoError(t, err)
	bobSPK, err := GenerateKeyPair()
	require.NoError(t, err)
	bobOPK, err := GenerateKeyPair()
	require.NoError(t, err)

	// Bob Ed25519 key for signature
	bobEdPub, bobEdPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sigHex := SignPreKey(bobEdPriv, bobSPK.PublicKey.Bytes())
	assert.True(t, VerifyPreKeySignature(bobEdPub, bobSPK.PublicKey.Bytes(), sigHex))

	bobBundle := &PreKeyBundle{
		UserID:          "bob-user-id",
		IdentityKey:     hex.EncodeToString(bobIK.PublicKey.Bytes()),
		SignedPreKey:    hex.EncodeToString(bobSPK.PublicKey.Bytes()),
		SignedPreKeyID:  1,
		Signature:       sigHex,
		OneTimePreKey:   hex.EncodeToString(bobOPK.PublicKey.Bytes()),
		OneTimePreKeyID: 101,
	}

	// Initiate handshake
	session, err := InitiateHandshake(aliceIK, aliceEK, bobBundle)
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.Len(t, session.SharedMasterKey, 32)
	assert.Equal(t, 101, session.OneTimePreKeyID)

	// Derive sequential message keys
	key1 := session.DeriveMessageKey(1)
	key2 := session.DeriveMessageKey(2)
	assert.Len(t, key1, 32)
	assert.Len(t, key2, 32)
	assert.NotEqual(t, key1, key2)
}

func TestX3DH_ResponderDerivesSameMasterKeyAsInitiator(t *testing.T) {
	aliceIK, err := GenerateKeyPair()
	require.NoError(t, err)
	aliceEK, err := GenerateKeyPair()
	require.NoError(t, err)

	bobIK, err := GenerateKeyPair()
	require.NoError(t, err)
	bobSPK, err := GenerateKeyPair()
	require.NoError(t, err)
	bobOPK, err := GenerateKeyPair()
	require.NoError(t, err)

	bobEdPub, bobEdPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sigHex := SignPreKey(bobEdPriv, bobSPK.PublicKey.Bytes())
	require.True(t, VerifyPreKeySignature(bobEdPub, bobSPK.PublicKey.Bytes(), sigHex))

	bobBundle := &PreKeyBundle{
		UserID:          "bob-user-id",
		IdentityKey:     hex.EncodeToString(bobIK.PublicKey.Bytes()),
		SignedPreKey:    hex.EncodeToString(bobSPK.PublicKey.Bytes()),
		SignedPreKeyID:  1,
		Signature:       sigHex,
		OneTimePreKey:   hex.EncodeToString(bobOPK.PublicKey.Bytes()),
		OneTimePreKeyID: 101,
	}

	aliceSession, err := InitiateHandshake(aliceIK, aliceEK, bobBundle)
	require.NoError(t, err)

	bobSession, err := RespondToHandshake(bobIK, bobSPK, bobOPK,
		hex.EncodeToString(aliceIK.PublicKey.Bytes()), aliceSession.EphemeralPubKey)
	require.NoError(t, err)

	assert.Equal(t, aliceSession.SharedMasterKey, bobSession.SharedMasterKey)
	// Message keys derived from the reconstructed master key must match too.
	assert.Equal(t, aliceSession.DeriveMessageKey(1), bobSession.DeriveMessageKey(1))
}

func TestX3DH_ResponderWithoutOneTimePreKey(t *testing.T) {
	aliceIK, err := GenerateKeyPair()
	require.NoError(t, err)
	aliceEK, err := GenerateKeyPair()
	require.NoError(t, err)

	bobIK, err := GenerateKeyPair()
	require.NoError(t, err)
	bobSPK, err := GenerateKeyPair()
	require.NoError(t, err)

	bobEdPub, bobEdPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sigHex := SignPreKey(bobEdPriv, bobSPK.PublicKey.Bytes())
	require.True(t, VerifyPreKeySignature(bobEdPub, bobSPK.PublicKey.Bytes(), sigHex))

	bobBundle := &PreKeyBundle{
		UserID:         "bob-user-id",
		IdentityKey:    hex.EncodeToString(bobIK.PublicKey.Bytes()),
		SignedPreKey:   hex.EncodeToString(bobSPK.PublicKey.Bytes()),
		SignedPreKeyID: 1,
		Signature:      sigHex,
		// No OneTimePreKey set.
	}

	aliceSession, err := InitiateHandshake(aliceIK, aliceEK, bobBundle)
	require.NoError(t, err)

	bobSession, err := RespondToHandshake(bobIK, bobSPK, nil,
		hex.EncodeToString(aliceIK.PublicKey.Bytes()), aliceSession.EphemeralPubKey)
	require.NoError(t, err)

	assert.Equal(t, aliceSession.SharedMasterKey, bobSession.SharedMasterKey)
}

func TestX3DH_NilBundleError(t *testing.T) {
	aliceIK, _ := GenerateKeyPair()
	aliceEK, _ := GenerateKeyPair()

	session, err := InitiateHandshake(aliceIK, aliceEK, nil)
	assert.Error(t, err)
	assert.Nil(t, session)
}
