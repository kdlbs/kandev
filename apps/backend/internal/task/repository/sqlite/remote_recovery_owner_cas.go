package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// RepairExecutorRunningDeadIfOwnerCurrent locks the pinned task owner before
// validating it and repairing the exact executor row in the same transaction.
func (r *Repository) RepairExecutorRunningDeadIfOwnerCurrent(
	ctx context.Context,
	sessionID, expectedExecID string,
	expectedUpdatedAt time.Time,
	owner models.RemoteRecoveryOwnerLockIDs,
	validateOwner func(context.Context) error,
) error {
	if err := validateRemoteRecoveryOwnerCAS(sessionID, expectedExecID, expectedUpdatedAt, owner, validateOwner); err != nil {
		return err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockRemoteRecoveryOwnerRows(ctx, tx, owner); err != nil {
		return err
	}
	if err := r.lockRemoteRecoveryDispositionStateTx(ctx, tx, sessionID); err != nil {
		return err
	}
	if err := validateOwner(ctx); err != nil {
		return err
	}
	return r.repairExecutorRunningDeadTx(ctx, tx, sessionID, expectedExecID, expectedUpdatedAt)
}

// SettleStoppedRemoteExecutionIfOwnerCurrent atomically stops the exact runtime,
// settles the captured open turn, and moves an active session to WAITING_FOR_INPUT
// while the retained task and environment owner locks are held.
func (r *Repository) SettleStoppedRemoteExecutionIfOwnerCurrent(
	ctx context.Context,
	sessionID, expectedExecID string,
	expectedUpdatedAt time.Time,
	expectedSessionState models.TaskSessionState,
	interruptedTurnID string,
	terminalOutcome models.DeliverySubmissionState,
	workflowOutcome *models.AgentDeliveryEffect,
	owner models.RemoteRecoveryOwnerLockIDs,
	validateOwner func(context.Context) error,
) (bool, time.Time, error) {
	if err := validateRemoteRecoveryOwnerCAS(sessionID, expectedExecID, expectedUpdatedAt, owner, validateOwner); err != nil {
		return false, time.Time{}, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockRemoteRecoveryOwnerRows(ctx, tx, owner); err != nil {
		return false, time.Time{}, err
	}
	if err := r.lockRemoteRecoveryDispositionStateTx(ctx, tx, sessionID); err != nil {
		return false, time.Time{}, err
	}
	if err := validateOwner(ctx); err != nil {
		return false, time.Time{}, err
	}
	currentSessionState, err := r.remoteRecoverySessionStateTx(ctx, tx, sessionID)
	if err != nil {
		return false, time.Time{}, err
	}
	if currentSessionState != expectedSessionState {
		return false, time.Time{}, models.ErrExecutionRotated
	}
	if err := r.recordStoppedRemoteWorkflowOutcomeTx(ctx, tx, owner.TaskID, workflowOutcome); err != nil {
		return false, time.Time{}, fmt.Errorf("record stopped remote workflow outcome: %w", err)
	}
	now := time.Now().UTC()
	if err := r.settleStoppedRemoteTurnTx(ctx, tx, now, sessionID, interruptedTurnID, terminalOutcome); err != nil {
		return false, time.Time{}, err
	}
	if err := r.stopRemoteExecutionTx(ctx, tx, now, sessionID, expectedExecID, expectedUpdatedAt); err != nil {
		return false, time.Time{}, err
	}
	stateChanged, err := r.waitStoppedRemoteSessionTx(ctx, tx, now, sessionID, currentSessionState)
	if err != nil {
		return false, time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return false, time.Time{}, err
	}
	return stateChanged, now, nil
}

func (r *Repository) recordStoppedRemoteWorkflowOutcomeTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	effect *models.AgentDeliveryEffect,
) error {
	if effect == nil {
		return nil
	}
	if !validAgentDeliveryWorkflowNoOp(taskID, effect) {
		return errors.New("exact stopped remote workflow outcome identity is required")
	}
	var existing models.AgentDeliveryEffect
	var completedAt sql.NullTime
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT effect_key, stream_id, sequence, effect_type, state, completed_at,
		       session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome
		FROM agent_delivery_effects WHERE effect_key = ?`), effect.EffectKey).Scan(
		&existing.EffectKey, &existing.StreamID, &existing.Sequence, &existing.EffectType,
		&existing.State, &completedAt, &existing.SessionID, &existing.IncarnationID,
		&existing.HarnessGeneration, &existing.SubmissionID, &existing.TurnID, &existing.Outcome)
	if err == nil {
		if completedAt.Valid && stoppedRemoteWorkflowOutcomeMatchesTx(&existing, effect) {
			return nil
		}
		return errors.New("stopped remote workflow outcome conflicts with the retained terminal")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := r.requireWorkflowOutcomeSourceTx(ctx, tx, taskID, effect); err != nil {
		return err
	}
	valid, err := r.agentDeliveryWorkflowTerminalMatchesTx(ctx, tx, effect)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("stopped remote workflow outcome no longer matches its terminal")
	}
	_, err = insertDeliveryEffectTx(ctx, tx, r.db.Rebind, effect)
	return err
}

func stoppedRemoteWorkflowOutcomeMatchesTx(existing, expected *models.AgentDeliveryEffect) bool {
	if existing == nil || expected == nil || existing.EffectKey != expected.EffectKey ||
		existing.EffectType != expected.EffectType || existing.State != models.DeliveryEffectCompleted ||
		existing.StreamID != expected.StreamID || existing.Sequence != expected.Sequence ||
		existing.SessionID != expected.SessionID || existing.IncarnationID != expected.IncarnationID ||
		existing.HarnessGeneration != expected.HarnessGeneration || existing.SubmissionID != expected.SubmissionID ||
		existing.TurnID != expected.TurnID {
		return false
	}
	switch existing.Outcome {
	case models.DeliveryWorkflowOutcomeApplied, models.DeliveryWorkflowOutcomeNoAutomaticTransition,
		models.DeliveryWorkflowOutcomeNotRequired, models.DeliveryWorkflowOutcomeStoppedRecovery:
		return true
	default:
		return false
	}
}

func (r *Repository) remoteRecoverySessionStateTx(ctx context.Context, tx *sqlx.Tx, sessionID string) (models.TaskSessionState, error) {
	var state models.TaskSessionState
	err := tx.GetContext(ctx, &state, r.db.Rebind(`SELECT state FROM task_sessions WHERE id = ?`), sessionID)
	return state, err
}

func (r *Repository) settleStoppedRemoteTurnTx(
	ctx context.Context,
	tx *sqlx.Tx,
	now time.Time,
	sessionID, turnID string,
	terminalOutcome models.DeliverySubmissionState,
) error {
	if turnID == "" {
		return nil
	}
	if _, err := r.completePendingToolCallsForTurnTx(ctx, tx, turnID); err != nil {
		return fmt.Errorf("settle tool calls for interrupted remote turn %s: %w", turnID, err)
	}
	if terminalOutcome != "" && !validDeliveryTerminalOutcome(terminalOutcome) {
		return errors.New("remote recovery turn outcome is unsupported")
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_turns
		   SET completed_at = CASE WHEN ? = '' THEN started_at ELSE ? END, updated_at = ?
		 WHERE id = ? AND task_session_id = ? AND completed_at IS NULL
	`), terminalOutcome, now, now, turnID, sessionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("remote recovery interrupted turn changed")
	}
	return nil
}

func (r *Repository) stopRemoteExecutionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	now time.Time,
	sessionID, expectedExecID string,
	expectedUpdatedAt time.Time,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE executors_running
		   SET status = ?, local_pid = 0, last_seen_at = ?, updated_at = ?
		 WHERE session_id = ? AND agent_execution_id = ? AND updated_at = ?
	`), models.ExecutorRunningStatusStopped, now, now, sessionID, expectedExecID, expectedUpdatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 0 {
		return err
	}
	exists, err := r.executorRunningRowExistsTx(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w for session: %s", models.ErrExecutorRunningNotFound, sessionID)
	}
	return models.ErrExecutionRotated
}

func (r *Repository) waitStoppedRemoteSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	now time.Time,
	sessionID string,
	state models.TaskSessionState,
) (bool, error) {
	if state != models.TaskSessionStateStarting && state != models.TaskSessionStateRunning {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		   SET state = ?, error_message = ?, completed_at = ?, updated_at = ?
		 WHERE id = ? AND state = ?
	`), models.TaskSessionStateWaitingForInput, "", nil, now, sessionID, state)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows != 1 {
		return false, models.ErrExecutionRotated
	}
	return true, nil
}

func validateRemoteRecoveryOwnerCAS(
	sessionID, expectedExecID string,
	expectedUpdatedAt time.Time,
	owner models.RemoteRecoveryOwnerLockIDs,
	validateOwner func(context.Context) error,
) error {
	if sessionID == "" || sessionID != owner.SessionID || expectedExecID == "" || expectedUpdatedAt.IsZero() {
		return errors.New("remote recovery compare-and-set identity is incomplete")
	}
	if validateOwner == nil {
		return errors.New("remote recovery owner validator is required")
	}
	return nil
}

func (r *Repository) repairExecutorRunningDeadTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, expectedExecID string,
	expectedUpdatedAt time.Time,
) error {
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE executors_running
		   SET status = ?, local_pid = 0, last_seen_at = ?, updated_at = ?
		 WHERE session_id = ? AND agent_execution_id = ? AND updated_at = ?
	`), models.ExecutorRunningStatusStopped, now, now, sessionID, expectedExecID, expectedUpdatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		exists, err := r.executorRunningRowExistsTx(ctx, tx, sessionID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w for session: %s", models.ErrExecutorRunningNotFound, sessionID)
		}
		return models.ErrExecutionRotated
	}
	return tx.Commit()
}

func (r *Repository) lockRemoteRecoveryDispositionStateTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if err := r.ensureExecutorRunningAvailableTx(ctx, tx, sessionID); err != nil {
		return err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{sessionID}, nil); err != nil {
		return err
	}
	locked, err := lockTaskSessionRow(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("remote recovery session %q no longer exists", sessionID)
	}
	return nil
}

// ConfirmExecutorRunningAbsentIfOwnerCurrent fences a no-row disposition with
// the same owner locks used by executor insertion and owner changes.
func (r *Repository) ConfirmExecutorRunningAbsentIfOwnerCurrent(
	ctx context.Context,
	sessionID string,
	owner models.RemoteRecoveryOwnerLockIDs,
	validateOwner func(context.Context) error,
) error {
	if sessionID == "" || sessionID != owner.SessionID {
		return errors.New("remote recovery absence identity is incomplete")
	}
	if validateOwner == nil {
		return errors.New("remote recovery owner validator is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockRemoteRecoveryOwnerRows(ctx, tx, owner); err != nil {
		return err
	}
	if err := r.lockRemoteRecoveryDispositionStateTx(ctx, tx, sessionID); err != nil {
		return err
	}
	exists, err := r.executorRunningRowExistsTx(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if exists {
		return models.ErrExecutionRotated
	}
	if err := validateOwner(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) lockRemoteRecoveryOwnerRows(
	ctx context.Context,
	tx *sqlx.Tx,
	owner models.RemoteRecoveryOwnerLockIDs,
) error {
	if owner.TaskID == "" || owner.SessionID == "" || owner.WorkspaceID == "" ||
		owner.EnvironmentID == "" || owner.EnvironmentOwnerTaskID == "" {
		return errors.New("remote recovery owner lock identity is incomplete")
	}
	taskIDs := []string{owner.TaskID, owner.EnvironmentOwnerTaskID}
	sort.Strings(taskIDs)
	for i, taskID := range taskIDs {
		if i > 0 && taskID == taskIDs[i-1] {
			continue
		}
		if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), taskID); err != nil {
			return fmt.Errorf("lock remote recovery task %q: %w", taskID, err)
		}
	}
	for _, row := range []struct{ table, id string }{
		{table: "workspaces", id: owner.WorkspaceID},
		{table: "task_environments", id: owner.EnvironmentID},
	} {
		if err := r.lockRemoteRecoveryRow(ctx, tx, row.table, row.id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) lockRemoteRecoveryRow(ctx context.Context, tx *sqlx.Tx, table, id string) error {
	query := "SELECT id FROM " + table + " WHERE id = ?"
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR UPDATE"
	}
	var locked string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), id).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("remote recovery owner row %s/%s no longer exists", table, id)
	}
	if err != nil {
		return fmt.Errorf("lock remote recovery owner row %s/%s: %w", table, id, err)
	}
	return nil
}
