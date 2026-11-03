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
