package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// lockTaskCleanupReservation serializes a prepared cleanup reservation with
// every creator that calls taskCleanupBarrierLocked. A missing task needs no
// lock: it cannot acquire new task-owned resources and recovery may still need
// to persist cleanup after an ambiguous mutation commit.
func (r *Repository) lockTaskCleanupReservation(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	var lockedTaskID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT id FROM tasks WHERE id = ? FOR UPDATE`,
	), taskID).Scan(&lockedTaskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lock task for cleanup reservation: %w", err)
	}
	return nil
}

// taskCleanupBarrierLocked serializes session/worktree creation against task
// lifecycle cleanup (ADR-2026-08-08). PostgreSQL takes a row lock on the
// owning task so a concurrent barrier reservation either commits before the
// creation and is observed, or blocks until the creation commits; SQLite's
// single-writer transaction is the serialization. Returns
// repoerrors.ErrTaskCleanupInProgress when a prepared/pending/running cleanup
// barrier exists for the task.
func (r *Repository) taskCleanupBarrierLocked(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		var lockedTaskID string
		if err := tx.QueryRowContext(ctx, r.db.Rebind(
			`SELECT id FROM tasks WHERE id = ? FOR UPDATE`,
		), taskID).Scan(&lockedTaskID); err != nil {
			return fmt.Errorf("lock task for creation barrier: %w", err)
		}
	}

	var active bool
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = ? AND state IN (?, ?, ?, ?)
		)
	`), taskID,
		models.TaskResourceCleanupStatePrepared,
		models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRunning,
		models.TaskResourceCleanupStateRetryWait,
	).Scan(&active); err != nil {
		return fmt.Errorf("check task cleanup barrier: %w", err)
	}
	if active {
		return fmt.Errorf("%w: %s", repoerrors.ErrTaskCleanupInProgress, taskID)
	}
	return nil
}

// lockTaskRowInTx takes the task row lock (Postgres SELECT FOR UPDATE; SQLite
// serializes via its single writer). It is the raw row lock behind the
// creation barrier, used by lifecycle paths that must serialize against
// session/worktree creation but should not be rejected by an in-flight
// cleanup job.
func (r *Repository) lockTaskRowInTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	var lockedTaskID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT id FROM tasks WHERE id = ? FOR UPDATE`,
	), taskID).Scan(&lockedTaskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// On PostgreSQL a missing task surfaces here (before any
			// RowsAffected check); keep the ErrTaskNotFound classification.
			return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
		return fmt.Errorf("lock task row: %w", err)
	}
	return nil
}

// lockWorkspaceRowInTx takes the workspace row lock (Postgres SELECT FOR
// UPDATE; SQLite serializes via its single writer). Task creation and the
// workspace delete cascade take it so a task created after a cascade's
// inventory cannot escape the queue purge.
func (r *Repository) lockWorkspaceRowInTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	return r.lockWorkspaceRowStdTx(ctx, tx, workspaceID)
}

// lockWorkspaceRowStdTx is the stdlib-transaction variant used by the task
// creation/admission paths (which begin database/sql transactions). An empty
// workspace id (config tasks) skips the lock. A NON-EMPTY workspace that no
// longer has a row is rejected: a creator waiting behind a workspace delete
// cascade resumes with sql.ErrNoRows after the cascade commits, and inserting
// then would orphan the task (tasks.workspace_id is not a foreign key).
func (r *Repository) lockWorkspaceRowStdTx(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, workspaceID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	if workspaceID == "" {
		return nil
	}
	var lockedWorkspaceID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT id FROM workspaces WHERE id = ? FOR UPDATE`,
	), workspaceID).Scan(&lockedWorkspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", repoerrors.ErrWorkspaceNotFound, workspaceID)
		}
		return fmt.Errorf("lock workspace row: %w", err)
	}
	return nil
}
