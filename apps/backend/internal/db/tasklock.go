package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// pgxDriverName mirrors dialect.PGX. Inlined rather than importing
// internal/db/dialect: that package's own test binary imports internal/db,
// so internal/db importing dialect back would create a build cycle for
// dialect's tests.
const pgxDriverName = "pgx"

// ErrTaskRowNotFound is returned by LockTaskRowInTx when the named task row
// does not exist.
var ErrTaskRowNotFound = errors.New("task row not found")

// LockTaskRowInTx takes the shared row lock on tasks(id) that serializes any
// writer able to affect a task-scoped gate condition (session, environment,
// running-executor, workspace-folder, workspace-group-membership, and the
// task row's own fields) against every other such writer and against the
// task row's own concurrent readers of that state.
//
// On PostgreSQL this is a real `SELECT ... FOR UPDATE`, held for the
// lifetime of tx. On SQLite it is a deliberate no-op: the writer pool
// (OpenSQLite) is capped at one connection, so a single open write
// transaction already excludes every other write against the same
// database for its whole lifetime — there is no second writer connection
// that could interleave. This mirrors the two-primitive split already used
// by the task-cleanup barrier (task/repository/sqlite/task_cleanup_barrier.go):
// a caller across a package boundary (this function is shared by both the
// task and office repositories, which write to the same tasks table using
// the same underlying writer handle) gets the identical guarantee without
// duplicating the dialect branch at every call site.
func LockTaskRowInTx(ctx context.Context, tx *sqlx.Tx, driverName, taskID string) error {
	if driverName != pgxDriverName {
		return nil
	}
	var locked string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT id FROM tasks WHERE id = ? FOR UPDATE`), taskID).Scan(&locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrTaskRowNotFound, taskID)
		}
		return fmt.Errorf("lock task row: %w", err)
	}
	return nil
}
