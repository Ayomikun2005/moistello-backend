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

func TestPostgresRepository_CreateDispute(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	repo := NewRepository(db)

	d := &Dispute{
		ID:             uuid.New(),
		CircleID:       uuid.New(),
		RaiserID:       uuid.New(),
		Reason:         "Missed contribution",
		Status:         DisputeStatusPending,
		IdempotencyKey: sql.NullString{String: "idem1", Valid: true},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	mock.ExpectExec("INSERT INTO disputes").
		WithArgs(d.ID, d.CircleID, d.RaiserID, d.Reason, d.Evidence, d.Status, d.IdempotencyKey, d.CreatedAt, d.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.CreateDispute(context.Background(), d)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepository_CreateVote(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	repo := NewRepository(db)

	v := &CircleVote{
		ID:             uuid.New(),
		CircleID:       uuid.New(),
		VoterID:        uuid.New(),
		VoteForID:      uuid.New(),
		RoundNumber:    1,
		IdempotencyKey: sql.NullString{String: "idem2", Valid: true},
		CreatedAt:      time.Now(),
	}

	mock.ExpectExec("INSERT INTO circle_votes").
		WithArgs(v.ID, v.CircleID, v.VoterID, v.VoteForID, v.RoundNumber, v.IdempotencyKey, v.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.CreateVote(context.Background(), v)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresRepository_CreateAuctionBid(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "sqlmock")
	repo := NewRepository(db)

	b := &CircleAuctionBid{
		ID:             uuid.New(),
		CircleID:       uuid.New(),
		BidderID:       uuid.New(),
		RoundNumber:    1,
		DiscountBips:   500,
		IdempotencyKey: sql.NullString{String: "idem3", Valid: true},
		CreatedAt:      time.Now(),
	}

	mock.ExpectExec("INSERT INTO circle_auction_bids").
		WithArgs(b.ID, b.CircleID, b.BidderID, b.RoundNumber, b.DiscountBips, b.IdempotencyKey, b.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.CreateAuctionBid(context.Background(), b)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
