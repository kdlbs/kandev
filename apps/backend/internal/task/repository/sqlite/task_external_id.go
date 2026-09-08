package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kandev/kandev/internal/task/models"
)

// externalIDIndexName is the partial unique index enforcing at most one task
// per (workspace_id, external_id). Naming it explicitly here keeps
// isExternalIDUniqueViolation attributable to this constraint specifically —
// see docs/specs/tasks/system-design/external-id-idempotency.md, "Unique-violation
// classification across dialects".
const externalIDIndexName = "uniq_tasks_external_id"

// sqliteExternalIDViolationMessage is the substring go-sqlite3 puts in a
// UNIQUE-constraint error for this specific index. SQLite's driver exposes no
// typed access to the violated index's name, only the column list of the
// failing index ("UNIQUE constraint failed: tasks.workspace_id,
// tasks.external_id"); that column pair is unique to uniq_tasks_external_id
// in this table, so matching it attributes the violation correctly — a bare
// "UNIQUE constraint failed" match would also fire on the tasks primary key.
const sqliteExternalIDViolationMessage = "UNIQUE constraint failed: tasks.workspace_id, tasks.external_id"

// isExternalIDUniqueViolation reports whether err is a violation of
// uniq_tasks_external_id specifically, not any unique violation. On
// PostgreSQL it inspects the typed pgconn.PgError's constraint name; on
// SQLite (no typed access to the constraint name) it matches the
// column-list message documented above.
func isExternalIDUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == externalIDIndexName
	}
	return strings.Contains(err.Error(), sqliteExternalIDViolationMessage)
}

// GetTaskByExternalID returns the task holding (workspaceID, externalID),
// including archived and unsettled tasks, or ErrTaskNotFound if none does.
// Read-only; used both by the REST lookup route and by the create sequence's
// step-3 lookup and its post-miss re-read paths.
//
// Queries r.ro (the read-only pool). If a Postgres read replica is ever
// wired here, the recovery re-read in recoverFoundTaskAfterInsertFailure
// must route through the writer (or a session-consistent replica handle) to
// avoid a replication-lag miss surfacing the original pre-insert error
// instead of the Found outcome the TOCTOU backstop relies on. Not an issue
// today: no read-replica DSN is wired anywhere in this codebase, so r.ro and
// r.db always resolve to the same connection.
func (r *Repository) GetTaskByExternalID(ctx context.Context, workspaceID, externalID string) (*models.Task, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.workspace_id = ? AND t.external_id = ?`),
		workspaceID, externalID)
	task, err := r.scanSingleTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: external_id %s in workspace %s", ErrTaskNotFound, externalID, workspaceID)
	}
	return task, err
}

// SettleTaskExternalID stamps external_id_settled_at on taskID if it still
// holds externalID and has not already been settled. The predicate includes
// external_id — not just id and "settled_at IS NULL" — because release
// clears both columns together; guarding on id alone would wrongly stamp a
// row that has since been released to a different (or no) identity. Returns
// whether a row was updated; zero rows means the task was deleted or its
// identity released while this create was running, and callers MUST NOT
// dispatch asynchronous work in that case (see spec "Settlement").
func (r *Repository) SettleTaskExternalID(ctx context.Context, taskID, externalID string, settledAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks
		   SET external_id_settled_at = ?
		 WHERE id = ?
		   AND external_id = ?
		   AND external_id_settled_at IS NULL
	`), settledAt, taskID, externalID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// ReleaseTaskExternalID clears external_id and external_id_settled_at on the
// task holding (workspaceID, externalID), without deleting or otherwise
// modifying the task, and bumps updated_at like any other task mutation
// (apps/backend/AGENTS.md, "Task lifecycle events" — a mutation must be
// visible to the caller so it can publish task.updated). Returns the task as
// it exists immediately after the update, or nil if no task held the
// identity. Runs in a transaction: the identity-scoped SELECT and the
// id-scoped UPDATE must observe the same row, or a concurrent release/reuse
// between them could silently update a different task than the one just
// identified.
func (r *Repository) ReleaseTaskExternalID(ctx context.Context, workspaceID, externalID string) (*models.Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var taskID string
	err = tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT id FROM tasks WHERE workspace_id = ? AND external_id = ?`),
		workspaceID, externalID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks
		   SET external_id = NULL, external_id_settled_at = NULL, updated_at = ?
		 WHERE id = ?
	`), time.Now().UTC(), taskID); err != nil {
		return nil, err
	}

	row := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.id = ?`), taskID)
	task, err := r.scanSingleTask(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}
