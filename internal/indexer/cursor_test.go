package indexer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursorTracker_GetCurrent(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	tracker := NewCursorTracker(db)

	lastProcessed := time.Now().Add(-30 * time.Second)
	rows := sqlmock.NewRows([]string{"chain", "last_ledger", "last_processed_at"}).
		AddRow("stellar", int64(1000), lastProcessed)
	mock.ExpectQuery("SELECT chain, last_ledger, last_processed_at FROM indexer_cursor").
		WillReturnRows(rows)

	cursor, err := tracker.GetCurrent(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "stellar", cursor.Chain)
	assert.Equal(t, int64(1000), cursor.LastLedger)
	assert.WithinDuration(t, lastProcessed, cursor.LastProcessedAt, time.Second)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCursorTracker_GetCurrent_Error(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	tracker := NewCursorTracker(db)

	mock.ExpectQuery("SELECT chain, last_ledger, last_processed_at FROM indexer_cursor").
		WillReturnError(errors.New("connection refused"))

	_, err = tracker.GetCurrent(context.Background())

	assert.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCursorTracker_Update(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	tracker := NewCursorTracker(db)

	mock.ExpectExec("UPDATE indexer_cursor SET last_ledger").
		WithArgs(int64(1001), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = tracker.Update(context.Background(), 1001)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCursor_Lag(t *testing.T) {
	now := time.Now()

	fresh := &Cursor{LastProcessedAt: now.Add(-5 * time.Second)}
	assert.InDelta(t, 5*time.Second, fresh.Lag(now), float64(time.Millisecond))

	stale := &Cursor{LastProcessedAt: now.Add(-10 * time.Minute)}
	assert.InDelta(t, 10*time.Minute, stale.Lag(now), float64(time.Millisecond))
}
