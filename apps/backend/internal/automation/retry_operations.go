package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRetryOperationUndispatchable     = errors.New("retry task operation is no longer dispatchable")
	ErrRetryContinuationCommitAmbiguous = errors.New("retry continuation identity commit is ambiguous")
)

const retryTaskOperationKind = "create_task"

// RetryTaskExternalID is the stable provider identity for one retry attempt.
// Task creation retries use it as the provider's idempotency key.
func RetryTaskExternalID(runID string, generation int64) string {
	return fmt.Sprintf("automation-retry-task:%s:%d", runID, generation)
}

func (s *Store) GetRetryTaskOperation(ctx context.Context, runID string, generation int64) (*RetryOperation, error) {
	var operation RetryOperation
	err := s.ro.GetContext(ctx, &operation, s.ro.Rebind(`
		SELECT * FROM automation_run_operations
		WHERE run_id = ? AND group_generation = ? AND operation_kind = ?`),
		runID, generation, retryTaskOperationKind)
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

// BeginRetryTaskOperation leases the task-create operation after checking the
// live group generation. Existing leases and committed operations are returned
// so recovery can safely reuse the same provider identity.
func (s *Store) BeginRetryTaskOperation(ctx context.Context, runID string, generation int64) (*RetryOperation, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var operation RetryOperation
	if err := tx.GetContext(ctx, &operation, tx.Rebind(`
		SELECT * FROM automation_run_operations
		WHERE run_id = ? AND group_generation = ? AND operation_kind = ?`),
		runID, generation, retryTaskOperationKind); err != nil {
		return nil, err
	}
	if operation.State == retryOperationCommitted {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &operation, nil
	}
	if operation.State == retryOperationAbandoned {
		return nil, ErrRetryGenerationMismatch
	}

	var live int
	if err := tx.GetContext(ctx, &live, tx.Rebind(`
		SELECT COUNT(*) FROM automation_runs ar
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
		JOIN automations a ON a.id = ar.automation_id
		WHERE ar.id = ? AND ar.retry_group_generation = ?
			AND ar.status = ? AND ar.retry_state = ?
			AND rg.generation = ? AND rg.state = ? AND a.enabled = TRUE`),
		runID, generation, RunStatusTriggered, RetryStateTriggered, generation, RetryGroupLive); err != nil {
		return nil, err
	}
	if live != 1 {
		return nil, ErrRetryGenerationMismatch
	}

	now := time.Now().UTC()
	if operation.State == retryOperationLeased && operation.LeaseExpiresAt != nil && operation.LeaseExpiresAt.After(now) {
		return nil, ErrRetryOperationUndispatchable
	}
	token := uuid.NewString()
	expires := now.Add(time.Minute)
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, lease_token = ?, lease_expires_at = ?, updated_at = ?
		WHERE operation_id = ? AND state IN (?, ?)
			AND (state = ? OR lease_expires_at IS NULL OR lease_expires_at <= ?)`),
		retryOperationLeased, token, expires, now, operation.ID,
		retryOperationRequested, retryOperationLeased, retryOperationRequested, now)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, ErrRetryGenerationMismatch
	}
	operation.State, operation.LeaseToken, operation.LeaseExpiresAt, operation.UpdatedAt = retryOperationLeased, token, &expires, now
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &operation, nil
}

// VerifyRetryTaskOperation is the final durable admission fence immediately
// before provider task creation.
func (s *Store) VerifyRetryTaskOperation(ctx context.Context, runID string, generation int64, leaseToken string) error {
	var live int
	err := s.ro.GetContext(ctx, &live, s.ro.Rebind(`
		SELECT COUNT(*) FROM automation_run_operations o
		JOIN automation_runs ar ON ar.id = o.run_id
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
		JOIN automations a ON a.id = ar.automation_id
		WHERE o.run_id = ? AND o.group_generation = ? AND o.operation_kind = ?
			AND o.state = ? AND o.lease_token = ?
			AND ar.status = ? AND ar.retry_state = ?
			AND ar.retry_group_generation = ? AND rg.generation = ?
			AND rg.state = ? AND a.enabled = TRUE`),
		runID, generation, retryTaskOperationKind, retryOperationLeased, leaseToken,
		RunStatusTriggered, RetryStateTriggered, generation, generation, RetryGroupLive)
	if err != nil {
		return err
	}
	if live != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

// CommitRetryTaskOperation records the provider task identity before the run
// is bound. The identity remains durable if the process crashes afterward.
func (s *Store) CommitRetryTaskOperation(ctx context.Context, runID string, generation int64, leaseToken, taskID string) error {
	if taskID == "" {
		return errors.New("retry task identity is required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, external_task_id = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE run_id = ? AND group_generation = ? AND operation_kind = ?
			AND state = ? AND lease_token = ?`),
		retryOperationCommitted, taskID, now, runID, generation, retryTaskOperationKind,
		retryOperationLeased, leaseToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	operation, getErr := s.GetRetryTaskOperation(ctx, runID, generation)
	if getErr == nil && operation.State == retryOperationCommitted && operation.ExternalTaskID == taskID {
		return nil
	}
	if errors.Is(getErr, sql.ErrNoRows) {
		return getErr
	}
	return ErrRetryGenerationMismatch
}

// CommitRetryContinuationOperation records an exact accepted continuation turn
// or upgrades a previously committed task identity with its exact launch
// session and turn before run binding completes.
func (s *Store) CommitRetryContinuationOperation(
	ctx context.Context,
	runID string,
	generation int64,
	leaseToken string,
	dispatch RunDispatch,
) error {
	if dispatch.TaskID == "" || dispatch.SessionID == "" || dispatch.TurnID == "" {
		return errors.New("retry continuation identity is required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, external_task_id = ?, external_session_id = ?,
			external_turn_id = ?, lease_token = '', lease_expires_at = NULL,
			updated_at = ?
		WHERE run_id = ? AND group_generation = ? AND operation_kind = ?
			AND (
				(state = ? AND lease_token = ?)
				OR (
					state = ? AND external_task_id = ? AND
					(
						(external_session_id = '' AND external_turn_id = '')
						OR (external_session_id = ? AND external_turn_id = ?)
					)
				)
			)`),
		retryOperationCommitted, dispatch.TaskID, dispatch.SessionID,
		dispatch.TurnID, now, runID, generation, retryTaskOperationKind,
		retryOperationLeased, leaseToken, retryOperationCommitted, dispatch.TaskID,
		dispatch.SessionID, dispatch.TurnID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	operation, getErr := s.GetRetryTaskOperation(ctx, runID, generation)
	if getErr == nil && operation.State == retryOperationCommitted &&
		operation.ExternalTaskID == dispatch.TaskID &&
		operation.ExternalSessionID == dispatch.SessionID &&
		operation.ExternalTurnID == dispatch.TurnID {
		return nil
	}
	if errors.Is(getErr, sql.ErrNoRows) {
		return getErr
	}
	return ErrRetryGenerationMismatch
}

// MarkRetryOperationAmbiguous preserves an accepted provider turn when the
// exact identity commit could not be confirmed.
func (s *Store) MarkRetryOperationAmbiguous(
	ctx context.Context,
	runID string,
	generation int64,
	leaseToken string,
	dispatch RunDispatch,
) error {
	if dispatch.TaskID == "" || dispatch.SessionID == "" || dispatch.TurnID == "" {
		return errors.New("retry ambiguous identity is required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, external_task_id = ?, external_session_id = ?,
			external_turn_id = ?, lease_token = '', lease_expires_at = NULL,
			updated_at = ?
		WHERE run_id = ? AND group_generation = ? AND operation_kind = ?
			AND state = ? AND lease_token = ?`),
		retryOperationAmbiguous, dispatch.TaskID, dispatch.SessionID,
		dispatch.TurnID, now, runID, generation, retryTaskOperationKind,
		retryOperationLeased, leaseToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	operation, getErr := s.GetRetryTaskOperation(ctx, runID, generation)
	if getErr == nil && operation.State == retryOperationAmbiguous &&
		operation.ExternalTaskID == dispatch.TaskID &&
		operation.ExternalSessionID == dispatch.SessionID &&
		operation.ExternalTurnID == dispatch.TurnID {
		return nil
	}
	if errors.Is(getErr, sql.ErrNoRows) {
		return getErr
	}
	return ErrRetryGenerationMismatch
}

func (s *Service) GetRetryTaskOperation(ctx context.Context, runID string, generation int64) (*RetryOperation, error) {
	return s.store.GetRetryTaskOperation(ctx, runID, generation)
}

func (s *Service) BeginRetryTaskOperation(ctx context.Context, runID string, generation int64) (*RetryOperation, error) {
	return s.store.BeginRetryTaskOperation(ctx, runID, generation)
}

func (s *Service) VerifyRetryTaskOperation(ctx context.Context, runID string, generation int64, leaseToken string) error {
	return s.store.VerifyRetryTaskOperation(ctx, runID, generation, leaseToken)
}

func (s *Service) CommitRetryTaskOperation(ctx context.Context, runID string, generation int64, leaseToken, taskID string) error {
	return s.store.CommitRetryTaskOperation(ctx, runID, generation, leaseToken, taskID)
}

func (s *Service) CommitRetryContinuationOperation(
	ctx context.Context,
	runID string,
	generation int64,
	leaseToken string,
	dispatch RunDispatch,
) error {
	return s.store.CommitRetryContinuationOperation(ctx, runID, generation, leaseToken, dispatch)
}

func (s *Service) MarkRetryOperationAmbiguous(
	ctx context.Context,
	runID string,
	generation int64,
	leaseToken string,
	dispatch RunDispatch,
) error {
	return s.store.MarkRetryOperationAmbiguous(ctx, runID, generation, leaseToken, dispatch)
}
