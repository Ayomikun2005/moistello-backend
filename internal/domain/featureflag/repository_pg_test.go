package featureflag

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/pkg/apperrors"
)

func newTestRepo(t *testing.T) (Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db := sqlx.NewDb(mockDB, "sqlmock")
	return NewRepository(db), mock, func() { mockDB.Close() }
}

func TestRepository_Get(t *testing.T) {
	repo, mock, cleanup := newTestRepo(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"id", "flag", "enabled", "description", "updated_at"}).
		AddRow(1, "kyc_required", true, "Require KYC for withdrawals", time.Now())
	mock.ExpectQuery("SELECT (.+) FROM feature_flags WHERE flag").
		WithArgs("kyc_required").
		WillReturnRows(rows)

	f, err := repo.Get(context.Background(), "kyc_required")
	require.NoError(t, err)
	assert.Equal(t, "kyc_required", f.Flag)
	assert.True(t, f.Enabled)
}

func TestRepository_Get_NotFound(t *testing.T) {
	repo, mock, cleanup := newTestRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT (.+) FROM feature_flags WHERE flag").
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag", "enabled", "description", "updated_at"}))

	_, err := repo.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRepository_List(t *testing.T) {
	repo, mock, cleanup := newTestRepo(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"id", "flag", "enabled", "description", "updated_at"}).
		AddRow(1, "auction_payouts", false, "Enable auction-based payout type", time.Now()).
		AddRow(2, "kyc_required", true, "Require KYC for withdrawals", time.Now())
	mock.ExpectQuery("SELECT (.+) FROM feature_flags ORDER BY flag").WillReturnRows(rows)

	flags, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, flags, 2)
}

func TestRepository_Upsert(t *testing.T) {
	repo, mock, cleanup := newTestRepo(t)
	defer cleanup()

	mock.ExpectExec("INSERT INTO feature_flags").
		WithArgs("premium_circles", true, "Enable premium circle type", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Upsert(context.Background(), "premium_circles", true, "Enable premium circle type")
	require.NoError(t, err)
}

func TestRepository_Delete(t *testing.T) {
	repo, mock, cleanup := newTestRepo(t)
	defer cleanup()

	mock.ExpectExec("DELETE FROM feature_flags WHERE flag").
		WithArgs("premium_circles").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), "premium_circles")
	require.NoError(t, err)
}

func TestRepository_Delete_NotFound(t *testing.T) {
	repo, mock, cleanup := newTestRepo(t)
	defer cleanup()

	mock.ExpectExec("DELETE FROM feature_flags WHERE flag").
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}
