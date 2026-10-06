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
// On PostgreSQL this is a real SELECT FOR UPDATE held until tx ends. On
// SQLite this helper does not reserve a writer. The caller must reserve it
// before gated reads when independent database handles can compete; task
// hierarchy admission does so through LockTaskHierarchy. A single-connection
// writer pool serializes only callers using that particular pool.
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
