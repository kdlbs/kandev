package automation

import (
	"context"
	"errors"
)

// PromoteClaimedRetry consumes one scheduler lease. Duplicate events and stale
// claims cannot transition an attempt twice.
func (s *Store) PromoteClaimedRetry(ctx context.Context, runID, token string, generation int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET id = id WHERE id = (SELECT automation_id FROM automation_runs WHERE id = ?)`), runID); err != nil {
		return err
	}
	var run AutomationRun
	if err := tx.GetContext(ctx, &run, tx.Rebind(`SELECT * FROM automation_runs WHERE id = ?`), runID); err != nil {
		return err
	}
	if err := checkRetryAdmissionCapacity(ctx, tx, run.AutomationID); err != nil {
		return err
	}
	snapshot, err := DecodeRetryLaunchConfigSnapshot(run.RetryLaunchConfigSnapshot, run.RetryLaunchConfigVersion)
	if err != nil {
		return err
	}
	if err := checkRetryCapacityLimit(ctx, tx, run.AutomationID, snapshot.MaxConcurrentRuns); err != nil {
		return err
	}

	if err := retryLaunchLiveForClaim(ctx, tx, runID, generation); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET retry_state = ?, status = ? WHERE id = ? AND retry_state = ? AND status = ? AND retry_claim_token = ? AND retry_group_generation = ? AND retry_claim_expires_at > CURRENT_TIMESTAMP`), RetryStateTriggered, RunStatusTriggered, runID, RetryStateClaimed, RunStatusScheduledRetry, token, generation)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.Join(ErrRetryGenerationMismatch, ErrAutomationRunNotDispatchable)
	}
	return tx.Commit()
}

func (s *Service) PromoteClaimedRetry(ctx context.Context, runID, token string, generation int64) error {
	return s.store.PromoteClaimedRetry(ctx, runID, token, generation)
}
