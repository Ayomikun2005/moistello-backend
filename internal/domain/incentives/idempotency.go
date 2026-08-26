package incentives

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ensureNoIncentive is the shared idempotency guard used by every grant flow.
// It returns an error when the user already has an incentive of the given type
// that satisfies the conflict predicate, so a retried call (duplicate webhook,
// double submit) can never mint a second incentive. A nil predicate conflicts
// on any existing incentive of the type.
func ensureNoIncentive(ctx context.Context, repo Repository, userID uuid.UUID, typ IncentiveType, conflict func(Incentive) bool, conflictMsg string) error {
	existing, err := repo.FindByUserIDAndType(ctx, userID, typ)
	if err != nil {
		// Surface repository failures instead of silently granting twice.
		return fmt.Errorf("checking existing %s incentives: %w", typ, err)
	}
	for _, inc := range existing {
		if conflict == nil || conflict(inc) {
			return errors.New(conflictMsg)
		}
	}
	return nil
}
