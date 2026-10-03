package automation

import (
	"context"
	"time"
)

func (s *Store) DeferRetryClaimForCapacity(ctx context.Context, runID, token string, generation int64) error {
	now := time.Now().UTC()
	due := now.Add(15 * time.Second)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_runs
		SET retry_state = ?, retry_scheduled_at = ?, retry_claimed_at = NULL,
			retry_claim_expires_at = NULL, retry_claim_token = ''
		WHERE id = ? AND retry_group_generation = ? AND retry_state = ?
			AND retry_claim_token = ?`),
		RetryStateScheduled, due, runID, generation, RetryStateClaimed, token)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}
