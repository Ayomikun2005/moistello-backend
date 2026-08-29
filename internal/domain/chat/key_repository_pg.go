package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/moistello/backend/pkg/apperrors"
)

type pgKeyRepository struct {
	db *sqlx.DB
}

func NewKeyRepository(db *sqlx.DB) KeyRepository {
	return &pgKeyRepository{db: db}
}

func (r *pgKeyRepository) UpsertIdentityKey(ctx context.Context, userID, identityKeyPub string) error {
	query := `
		INSERT INTO x3dh_identity_keys (user_id, identity_key_pub)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET identity_key_pub = $2
	`
	_, err := r.db.ExecContext(ctx, query, userID, identityKeyPub)
	if err != nil {
		return fmt.Errorf("upserting identity key: %w", err)
	}
	return nil
}

func (r *pgKeyRepository) UpsertSignedPreKey(ctx context.Context, userID string, keyID int, signedPreKeyPub, signature string) error {
	query := `
		INSERT INTO x3dh_signed_prekeys (user_id, key_id, signed_prekey_pub, signature)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, key_id) DO UPDATE SET signed_prekey_pub = $3, signature = $4
	`
	_, err := r.db.ExecContext(ctx, query, userID, keyID, signedPreKeyPub, signature)
	if err != nil {
		return fmt.Errorf("upserting signed prekey: %w", err)
	}
	return nil
}

func (r *pgKeyRepository) AddOneTimePreKeys(ctx context.Context, userID string, keys []OneTimePreKeyInput) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning one-time prekey transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO x3dh_one_time_prekeys (user_id, key_id, one_time_prekey_pub)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key_id) DO NOTHING
	`
	for _, k := range keys {
		if _, err := tx.ExecContext(ctx, query, userID, k.KeyID, k.PubKey); err != nil {
			return fmt.Errorf("inserting one-time prekey %d: %w", k.KeyID, err)
		}
	}
	return tx.Commit()
}

func (r *pgKeyRepository) CountUnusedOneTimePreKeys(ctx context.Context, userID string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM x3dh_one_time_prekeys WHERE user_id = $1 AND used = FALSE`
	if err := r.db.GetContext(ctx, &count, query, userID); err != nil {
		return 0, fmt.Errorf("counting unused one-time prekeys: %w", err)
	}
	return count, nil
}

func (r *pgKeyRepository) GetBundle(ctx context.Context, userID string) (*PreKeyBundle, error) {
	var identityKeyPub string
	err := r.db.GetContext(ctx, &identityKeyPub,
		`SELECT identity_key_pub FROM x3dh_identity_keys WHERE user_id = $1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting identity key: %w", err)
	}

	var signedPreKey struct {
		KeyID           int    `db:"key_id"`
		SignedPreKeyPub string `db:"signed_prekey_pub"`
		Signature       string `db:"signature"`
	}
	err = r.db.GetContext(ctx, &signedPreKey,
		`SELECT key_id, signed_prekey_pub, signature FROM x3dh_signed_prekeys
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting signed prekey: %w", err)
	}

	bundle := &PreKeyBundle{
		UserID:         userID,
		IdentityKey:    identityKeyPub,
		SignedPreKey:   signedPreKey.SignedPreKeyPub,
		SignedPreKeyID: signedPreKey.KeyID,
		Signature:      signedPreKey.Signature,
	}

	// Atomically claim one unused one-time prekey, if any remain, so it is
	// never handed out to two concurrent handshake initiators. Absence of
	// a one-time prekey is not an error — X3DH degrades gracefully to
	// DH1-DH3 only.
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning one-time prekey claim transaction: %w", err)
	}
	defer tx.Rollback()

	var opk struct {
		KeyID            int    `db:"key_id"`
		OneTimePreKeyPub string `db:"one_time_prekey_pub"`
	}
	claimQuery := `
		UPDATE x3dh_one_time_prekeys
		SET used = TRUE
		WHERE id = (
			SELECT id FROM x3dh_one_time_prekeys
			WHERE user_id = $1 AND used = FALSE
			ORDER BY key_id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING key_id, one_time_prekey_pub
	`
	err = tx.GetContext(ctx, &opk, claimQuery, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("claiming one-time prekey: %w", err)
	}
	if err == nil {
		bundle.OneTimePreKey = opk.OneTimePreKeyPub
		bundle.OneTimePreKeyID = opk.KeyID
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing one-time prekey claim: %w", err)
	}

	return bundle, nil
}
