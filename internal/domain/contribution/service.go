package contribution

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/moistello/backend/pkg/apperrors"
)

// Broadcaster defines the interface for real-time event broadcasting
// of contribution events to WebSocket clients.
type Broadcaster interface {
	ContributionRecorded(ctx context.Context, circleID, userID string, roundNumber int, amount float64)
	PayoutExecuted(ctx context.Context, circleID, recipientID string, roundNumber int, amount float64)
}

type Service interface {
	Record(ctx context.Context, input RecordInput) (*Contribution, error)
	UpdateVerification(ctx context.Context, id string, verifiedOnchain bool, status VerificationStatus) error
	GetUserHistory(ctx context.Context, userID string, page, limit int) ([]Contribution, int, error)
	GetCircleHistory(ctx context.Context, circleID string, page, limit int) ([]Contribution, int, error)
}

type Transactor interface {
	WithTransaction(ctx context.Context, fn func(repo Repository) error) error
}

type RecordInput struct {
	CircleID           string              `json:"circleId" validate:"required"`
	UserID             string              `json:"userId" validate:"required"`
	RoundNumber        int                 `json:"roundNumber" validate:"required,gte=1"`
	Amount             float64             `json:"amount" validate:"required,gt=0"`
	TxnHash            string              `json:"txnHash" validate:"required"`
	VerifiedOnchain    *bool               `json:"verifiedOnchain,omitempty"`
	VerificationStatus *VerificationStatus `json:"verificationStatus,omitempty"`
}

type contributionService struct {
	repo        Repository
	broadcaster Broadcaster
	tx          Transactor
}

func NewService(repo Repository, broadcaster Broadcaster, tx Transactor) Service {
	return &contributionService{repo: repo, broadcaster: broadcaster, tx: tx}
}

type contribTransactor struct {
	db *sqlx.DB
}

func NewTransactor(db *sqlx.DB) Transactor {
	return &contribTransactor{db: db}
}

func (t *contribTransactor) WithTransaction(ctx context.Context, fn func(repo Repository) error) error {
	tx, err := t.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(NewRepositoryFromTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID: %w", err)
	}
	return id, nil
}

func (s *contributionService) Record(ctx context.Context, input RecordInput) (*Contribution, error) {
	userID, err := parseUUID(input.UserID)
	if err != nil {
		return nil, err
	}
	circleID, err := parseUUID(input.CircleID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	txnHash := sql.NullString{String: input.TxnHash, Valid: true}

	verifiedOnchain := false
	if input.VerifiedOnchain != nil {
		verifiedOnchain = *input.VerifiedOnchain
	}
	verificationStatus := VerificationStatusUnverified
	if input.VerificationStatus != nil {
		verificationStatus = *input.VerificationStatus
	} else if verifiedOnchain {
		verificationStatus = VerificationStatusVerified
	}

	c := &Contribution{
		ID:                 uuid.New(),
		CircleID:           circleID,
		UserID:             userID,
		RoundNumber:        input.RoundNumber,
		Amount:             input.Amount,
		TxnHash:            txnHash,
		Status:             StatusPending,
		OnTime:             true,
		VerifiedOnchain:    verifiedOnchain,
		VerificationStatus: verificationStatus,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if s.tx != nil {
		err := s.tx.WithTransaction(ctx, func(repo Repository) error {
			if err := repo.Create(ctx, c); err != nil {
				if err == apperrors.ErrConflict {
					return fmt.Errorf("duplicate contribution: %w", err)
				}
				return fmt.Errorf("recording contribution: %w", err)
			}
			return nil
		})
		if err == nil && s.broadcaster != nil {
			s.broadcaster.ContributionRecorded(ctx, input.CircleID, input.UserID, input.RoundNumber, input.Amount)
		}
		return c, err
	}

	if err := s.repo.Create(ctx, c); err != nil {
		if err == apperrors.ErrConflict {
			return nil, fmt.Errorf("duplicate contribution: %w", err)
		}
		return nil, fmt.Errorf("recording contribution: %w", err)
	}
	if s.broadcaster != nil {
		s.broadcaster.ContributionRecorded(ctx, input.CircleID, input.UserID, input.RoundNumber, input.Amount)
	}
	return c, nil
}

func (s *contributionService) UpdateVerification(ctx context.Context, id string, verifiedOnchain bool, status VerificationStatus) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.repo.UpdateVerificationStatus(ctx, uid, verifiedOnchain, status)
}

func (s *contributionService) GetUserHistory(ctx context.Context, userID string, page, limit int) ([]Contribution, int, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	contribs, total, err := s.repo.ListByUser(ctx, uid, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("getting user contribution history: %w", err)
	}
	return contribs, total, nil
}

func (s *contributionService) GetCircleHistory(ctx context.Context, circleID string, page, limit int) ([]Contribution, int, error) {
	cid, err := parseUUID(circleID)
	if err != nil {
		return nil, 0, err
	}
	contribs, total, err := s.repo.ListByCircle(ctx, cid, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("getting circle contribution history: %w", err)
	}
	return contribs, total, nil
}
