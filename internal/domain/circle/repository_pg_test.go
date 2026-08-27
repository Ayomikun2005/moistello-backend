package circle

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_CreatePenalty(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	repo := NewRepository(db)

	p := &Penalty{
		ID:             uuid.New(),
		CircleID:       uuid.New(),
		UserID:         uuid.New(),
		RoundNumber:    1,
		PenaltyType:    PenaltyTypeLate,
		Amount:         10.5,
		StrikesApplied: 1,
		Reason:         sql.NullString{String: "missed round", Valid: true},
		CreatedAt:      time.Now(),
	}

	mock.ExpectExec("INSERT INTO penalties").
		WithArgs(p.ID, p.CircleID, p.UserID, p.RoundNumber, p.PenaltyType, p.Amount, p.StrikesApplied, p.Reason, p.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.CreatePenalty(context.Background(), p)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
