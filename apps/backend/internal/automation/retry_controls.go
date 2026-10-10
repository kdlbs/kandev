package automation

import (
	"context"
	"time"
)

// CancelRetryGroupsByAutomation fences every live retry chain before a
// configuration disable or deletion can race a scheduler claim.
func (s *Store) CancelRetryGroupsByAutomation(ctx context.Context, automationID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_groups SET generation = generation + 1, state = ?, updated_at = ? WHERE automation_id = ? AND state = ?`), RetryGroupCancelled, now, automationID, RetryGroupLive); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, retry_state = ?, retry_cancelled_at = ?, retry_claim_token = '', retry_claimed_at = NULL, retry_claim_expires_at = NULL WHERE automation_id = ? AND retry_group_id <> '' AND retry_state NOT IN (?, ?, ?, ?, ?)`), RunStatusFailed, RetryStateCancelled, now, automationID, RetryStateCompleted, RetryStateExhausted, RetryStateCancelled, RetryStateSuperseded, RetryStateSchedulingFailed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_run_task_intents SET state = ?, automation_deleted_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE automation_id = ? AND retry_group_id <> '') AND state != ?`), retryIntentAbandoned, now, automationID, retryIntentAbandoned); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_run_operations SET state = ?, updated_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE automation_id = ? AND retry_group_id <> '') AND state NOT IN (?, ?)`), retryOperationAbandoned, now, automationID, retryOperationCommitted, retryOperationAbandoned)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_outbox SET state = ?, updated_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE automation_id = ? AND retry_group_id <> '') AND state IN (?, ?)`), retryOutboxRevoked, now, automationID, retryOutboxPending, retryOutboxLeased); err != nil {
		return err
	}
	return tx.Commit()
}

// CancelRetryRun settles one active retry row without requiring its group to
// remain live. This is used for a bound run whose group was superseded.
func (s *Store) CancelRetryRun(ctx context.Context, runID string, generation int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_runs
		SET status = ?, retry_state = ?, retry_cancelled_at = ?,
			retry_claim_token = '', retry_claimed_at = NULL, retry_claim_expires_at = NULL
		WHERE id = ? AND retry_group_id <> '' AND retry_group_generation = ?
			AND status IN (?, ?) AND retry_state NOT IN (?, ?, ?, ?, ?)`),
		RunStatusFailed, RetryStateCancelled, now, runID, generation,
		RunStatusTriggered, RunStatusTaskCreated, RetryStateCompleted,
		RetryStateExhausted, RetryStateCancelled, RetryStateSuperseded,
		RetryStateSchedulingFailed)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrRetryGenerationMismatch
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_task_intents
		SET state = ?, automation_deleted_at = ?
		WHERE run_id = ? AND state != ?`),
		retryIntentAbandoned, now, runID, retryIntentAbandoned); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, updated_at = ?
		WHERE run_id = ? AND state NOT IN (?, ?)`),
		retryOperationAbandoned, now, runID, retryOperationCommitted,
		retryOperationAbandoned); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, updated_at = ?
		WHERE run_id = ? AND state IN (?, ?)`),
		retryOutboxRevoked, now, runID, retryOutboxPending, retryOutboxLeased); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) CancelAutomationRetries(ctx context.Context, automationID string) error {
	return s.store.CancelRetryGroupsByAutomation(ctx, automationID)
}
