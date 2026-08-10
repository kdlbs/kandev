package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

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
