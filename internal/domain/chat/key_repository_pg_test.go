package chat

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/pkg/apperrors"
)

func newTestKeyRepo(t *testing.T) (KeyRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db := sqlx.NewDb(mockDB, "sqlmock")
	return NewKeyRepository(db), mock, func() { mockDB.Close() }
}

func TestKeyRepository_UpsertIdentityKey(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectExec("INSERT INTO x3dh_identity_keys").
		WithArgs("user-1", "ik-pub").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.UpsertIdentityKey(context.Background(), "user-1", "ik-pub"))
}

func TestKeyRepository_UpsertSignedPreKey(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectExec("INSERT INTO x3dh_signed_prekeys").
		WithArgs("user-1", 1, "spk-pub", "sig").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.UpsertSignedPreKey(context.Background(), "user-1", 1, "spk-pub", "sig"))
}

func TestKeyRepository_AddOneTimePreKeys(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO x3dh_one_time_prekeys").WithArgs("user-1", 1, "opk-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO x3dh_one_time_prekeys").WithArgs("user-1", 2, "opk-2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.AddOneTimePreKeys(context.Background(), "user-1", []OneTimePreKeyInput{
		{KeyID: 1, PubKey: "opk-1"},
		{KeyID: 2, PubKey: "opk-2"},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestKeyRepository_GetBundle_NoIdentityKey(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT identity_key_pub FROM x3dh_identity_keys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"identity_key_pub"}))

	_, err := repo.GetBundle(context.Background(), "user-1")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestKeyRepository_GetBundle_WithOneTimePreKey(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT identity_key_pub FROM x3dh_identity_keys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"identity_key_pub"}).AddRow("ik-pub"))

	mock.ExpectQuery("SELECT key_id, signed_prekey_pub, signature FROM x3dh_signed_prekeys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id", "signed_prekey_pub", "signature"}).AddRow(1, "spk-pub", "sig"))

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE x3dh_one_time_prekeys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id", "one_time_prekey_pub"}).AddRow(5, "opk-5"))
	mock.ExpectCommit()

	bundle, err := repo.GetBundle(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, "ik-pub", bundle.IdentityKey)
	assert.Equal(t, "spk-pub", bundle.SignedPreKey)
	assert.Equal(t, "opk-5", bundle.OneTimePreKey)
	assert.Equal(t, 5, bundle.OneTimePreKeyID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestKeyRepository_GetBundle_WithoutOneTimePreKeyRemaining(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT identity_key_pub FROM x3dh_identity_keys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"identity_key_pub"}).AddRow("ik-pub"))

	mock.ExpectQuery("SELECT key_id, signed_prekey_pub, signature FROM x3dh_signed_prekeys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id", "signed_prekey_pub", "signature"}).AddRow(1, "spk-pub", "sig"))

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE x3dh_one_time_prekeys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id", "one_time_prekey_pub"}))
	mock.ExpectCommit()

	bundle, err := repo.GetBundle(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Empty(t, bundle.OneTimePreKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestKeyRepository_CountUnusedOneTimePreKeys(t *testing.T) {
	repo, mock, cleanup := newTestKeyRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM x3dh_one_time_prekeys").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountUnusedOneTimePreKeys(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}
