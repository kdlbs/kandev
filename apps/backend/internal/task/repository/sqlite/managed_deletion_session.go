package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) managedSessionDeletionBarrierTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	// Reserve SQLite's writer before the session-to-task locator read.
	if !dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET id = id WHERE 0`); err != nil {
			return err
		}
	}
	var taskID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.managedDeletionBarrierTx(ctx, tx, taskID)
}

// Only runtime admission needs a new transaction; terminal observations retain
// the existing narrow update and cleanup-stop semantics.
func (r *Repository) managedSessionStateWriter(ctx context.Context, id string, state models.TaskSessionState) (taskSessionExecutor, *sqlx.Tx, error) {
	if state != models.TaskSessionStateStarting && state != models.TaskSessionStateRunning {
		return r.db, nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, nil, err
	}
	if err := r.managedSessionDeletionBarrierTx(ctx, tx, id); err != nil {
		_ = tx.Rollback()
		return nil, nil, err
	}
	return tx, tx, nil
}
