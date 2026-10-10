package automation

import (
	"context"
	"errors"
)

// PromoteClaimedRetry consumes one scheduler lease. Duplicate events and stale
// claims cannot transition an attempt twice.
func (s *Store) PromoteClaimedRetry(ctx context.Context, runID, token string, generation int64) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET retry_state = ?, status = ? WHERE id = ? AND retry_state = ? AND status = ? AND retry_claim_token = ? AND retry_group_generation = ? AND retry_claim_expires_at > CURRENT_TIMESTAMP`), RetryStateTriggered, RunStatusTriggered, runID, RetryStateClaimed, RunStatusScheduledRetry, token, generation)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.Join(ErrRetryGenerationMismatch, ErrAutomationRunNotDispatchable)
	}
	return nil
}

func (s *Service) PromoteClaimedRetry(ctx context.Context, runID, token string, generation int64) error {
	return s.store.PromoteClaimedRetry(ctx, runID, token, generation)
}
