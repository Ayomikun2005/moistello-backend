package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type pgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &pgRepository{db: db}
}

func (r *pgRepository) CreateProposal(ctx context.Context, p *Proposal) error {
	query := `
		INSERT INTO governance_proposals (id, title, description, proposal_type, creator_id, status, for_votes, against_votes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		p.ID, p.Title, p.Description, p.ProposalType, p.CreatorID,
		string(p.Status), p.ForVotes, p.AgainstVotes, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting governance proposal: %w", err)
	}
	return nil
}

func (r *pgRepository) GetProposal(ctx context.Context, id uuid.UUID) (*Proposal, error) {
	query := `
		SELECT id, title, description, proposal_type, creator_id, status, for_votes, against_votes, executed_at, created_at, updated_at
		FROM governance_proposals
		WHERE id = $1
	`
	var p Proposal
	err := r.db.GetContext(ctx, &p, query, id)
	if err != nil {
		return nil, fmt.Errorf("getting governance proposal: %w", err)
	}
	return &p, nil
}

func (r *pgRepository) ListProposals(ctx context.Context) ([]Proposal, error) {
	query := `
		SELECT id, title, description, proposal_type, creator_id, status, for_votes, against_votes, executed_at, created_at, updated_at
		FROM governance_proposals
		ORDER BY created_at DESC
	`
	var proposals []Proposal
	err := r.db.SelectContext(ctx, &proposals, query)
	if err != nil {
		return nil, fmt.Errorf("listing governance proposals: %w", err)
	}
	if proposals == nil {
		proposals = []Proposal{}
	}
	return proposals, nil
}

func (r *pgRepository) RecordVote(ctx context.Context, proposalID, voterID uuid.UUID, vote bool) error {
	query := `
		INSERT INTO governance_votes (proposal_id, voter_id, vote, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (proposal_id, voter_id) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, proposalID, voterID, vote, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("recording governance vote: %w", err)
	}

	// Update the vote count on the proposal
	if vote {
		_, err = r.db.ExecContext(ctx, `UPDATE governance_proposals SET for_votes = for_votes + 1, updated_at = $1 WHERE id = $2`, time.Now().UTC(), proposalID)
	} else {
		_, err = r.db.ExecContext(ctx, `UPDATE governance_proposals SET against_votes = against_votes + 1, updated_at = $1 WHERE id = $2`, time.Now().UTC(), proposalID)
	}
	if err != nil {
		return fmt.Errorf("updating proposal vote count: %w", err)
	}
	return nil
}

func (r *pgRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status ProposalStatus, executedAt *string) error {
	now := time.Now().UTC()
	if status == ProposalStatusExecuted && executedAt != nil {
		_, err := r.db.ExecContext(ctx, `UPDATE governance_proposals SET status = $1, executed_at = $2, updated_at = $3 WHERE id = $4`,
			string(status), *executedAt, now, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE governance_proposals SET status = $1, updated_at = $2 WHERE id = $3`,
		string(status), now, id)
	return err
}