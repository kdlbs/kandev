package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrArchivedTaskEligibilityBudget means no eligibility decision was made. The
// caller skips the task and marks the pass partial rather than guessing.
var ErrArchivedTaskEligibilityBudget = errors.New("archived task eligibility budget exceeded")

// ErrCleanupSnapshotEligibilityBudget means a cleanup job's terminal state could
// not be decided within the statement budget.
var ErrCleanupSnapshotEligibilityBudget = errors.New("cleanup snapshot eligibility budget exceeded")

const archivedEligibilityBudget = 200 * time.Millisecond

// ArchivedTaskEligible reports whether a task may have its archived transcript
// reduced. The archived timestamp is the age basis, and the task must still be
// archived at the moment of the call, so an unarchive before the write protects
// the row.
func ArchivedTaskEligible(ctx context.Context, q sqlx.QueryerContext, driverName, taskID string, cutoff time.Time) (bool, error) {
	if err := requireSQLite(driverName); err != nil {
		return false, err
	}
	var eligible bool
	stmt, cancel := context.WithTimeout(ctx, archivedEligibilityBudget)
	defer cancel()
	err := sqlx.GetContext(stmt, q, &eligible, archivedTaskEligibilitySQL, taskID, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, fmt.Errorf("%w: %w", ErrArchivedTaskEligibilityBudget, err)
		}
		return false, err
	}
	return eligible, nil
}

// HistoryTaskEligible reports whether a task may have its message metadata
// reduced. A task is eligible when it is archived at or before the cutoff, or
// when it is finished under the existing inactivity contract at the cutoff.
func HistoryTaskEligible(ctx context.Context, q sqlx.QueryerContext, driverName, taskID string, cutoff time.Time) (bool, error) {
	archived, err := ArchivedTaskEligible(ctx, q, driverName, taskID, cutoff)
	if err != nil || archived {
		return archived, err
	}
	return FinishedTaskEligible(ctx, q, taskID, cutoff)
}

// FinishedTaskEligible reports whether a task is finished: its state is terminal
// (COMPLETED, FAILED, or CANCELLED) and it satisfies the existing inactivity
// contract at the cutoff. A nonterminal task is never finished-eligible.
func FinishedTaskEligible(ctx context.Context, q sqlx.QueryerContext, taskID string, cutoff time.Time) (bool, error) {
	terminal, err := terminalTaskState(ctx, q, taskID)
	if err != nil || !terminal {
		return false, err
	}
	return PayloadTaskEligible(ctx, q, taskID, cutoff)
}

func terminalTaskState(ctx context.Context, q sqlx.QueryerContext, taskID string) (bool, error) {
	var terminal bool
	stmt, cancel := context.WithTimeout(ctx, archivedEligibilityBudget)
	defer cancel()
	err := sqlx.GetContext(stmt, q, &terminal, terminalTaskStateSQL, taskID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, fmt.Errorf("%w: %w", ErrArchivedTaskEligibilityBudget, err)
		}
		return false, err
	}
	return terminal, nil
}

// terminalTaskStateSQL is the finished-task gate. Only a terminal task state
// qualifies for the finished path, so a quiescent but nonterminal task is not
// reduced through it.
var terminalTaskStateSQL = `SELECT EXISTS(SELECT 1 FROM tasks
 WHERE id=? AND state IN ('COMPLETED','FAILED','CANCELLED'))`

// SucceededCleanupJobEligible reports whether a cleanup job may have its
// snapshot reduced. Only a terminal success qualifies; every other state,
// including a missing or unreadable completion timestamp, is retained.
func SucceededCleanupJobEligible(ctx context.Context, q sqlx.QueryerContext, driverName, id string, cutoff time.Time) (bool, error) {
	if err := requireSQLite(driverName); err != nil {
		return false, err
	}
	var eligible bool
	stmt, cancel := context.WithTimeout(ctx, archivedEligibilityBudget)
	defer cancel()
	err := sqlx.GetContext(stmt, q, &eligible, succeededCleanupJobEligibilitySQL, id, models.TaskResourceCleanupStateSucceeded, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, fmt.Errorf("%w: %w", ErrCleanupSnapshotEligibilityBudget, err)
		}
		return false, err
	}
	return eligible, nil
}

// archivedRetentionSQLiteDriver is the only engine archived data retention measures.
const archivedRetentionSQLiteDriver = "sqlite3"

// requireSQLite keeps the SQLite-only date comparisons below unreachable on any
// other engine, so a Postgres install is refused rather than mis-measured.
func requireSQLite(driverName string) error {
	if driverName != archivedRetentionSQLiteDriver {
		return errors.New("archived data retention requires SQLite")
	}
	return nil
}

// archivedTaskEligibilitySQL requires the task to still carry an archived_at at
// or before the cutoff. Timestamps are compared through SQLite date functions
// because legacy rows can carry textual UTC offsets that break lexical ordering.
var archivedTaskEligibilitySQL = `SELECT EXISTS(SELECT 1 FROM tasks t
 WHERE t.id=? AND t.archived_at IS NOT NULL
 AND julianday(t.archived_at) IS NOT NULL
 AND julianday(t.archived_at)>julianday('0001-01-01') AND julianday(t.archived_at)<=julianday(?))`

// succeededCleanupJobEligibilitySQL pins both the terminal state and a readable
// completion timestamp, so an in-flight or failed job is never reduced.
var succeededCleanupJobEligibilitySQL = `SELECT EXISTS(SELECT 1 FROM task_resource_cleanup_jobs j
 WHERE j.id=? AND j.state=?
 AND j.completed_at IS NOT NULL
 AND julianday(j.completed_at) IS NOT NULL
 AND julianday(j.completed_at)>julianday('0001-01-01') AND julianday(j.completed_at)<=julianday(?))`

// ArchivedMessageRow is one candidate transcript row with its stored metadata
// length, so a batch can be bounded before any body is fetched.
type ArchivedMessageRow struct {
	RowID int64  `db:"row_id"`
	ID    string `db:"id"`
	Type  string `db:"type"`
	Bytes int64  `db:"bytes"`
}

// NextHistoryTaskID returns the next candidate task after the cursor, bounded by
// the captured upper cursor. A task is a candidate when it is archived or its
// state is terminal, so the message target walks finished and archived tasks in
// one keyset.
func NextHistoryTaskID(ctx context.Context, q sqlx.QueryerContext, after, upper string) (string, error) {
	var id string
	err := sqlx.GetContext(ctx, q, &id, `SELECT id FROM tasks
	 WHERE (archived_at IS NOT NULL OR state IN ('COMPLETED','FAILED','CANCELLED'))
	 AND id>? AND id<=? ORDER BY id LIMIT 1`, after, upper)
	return id, err
}

// HistoryUpperTaskID captures the highest candidate task id for this pass.
func HistoryUpperTaskID(ctx context.Context, q sqlx.QueryerContext) (string, error) {
	var id string
	err := sqlx.GetContext(ctx, q, &id, `SELECT COALESCE(MAX(id),'') FROM tasks
	 WHERE archived_at IS NOT NULL OR state IN ('COMPLETED','FAILED','CANCELLED')`)
	return id, err
}

// NextSessionID returns the next session of a task for the archived scan.
func NextSessionID(ctx context.Context, q sqlx.QueryerContext, taskID, after string) (string, error) {
	var id string
	err := sqlx.GetContext(ctx, q, &id, `SELECT id FROM task_sessions WHERE task_id=? AND id>? ORDER BY id LIMIT 1`, taskID, after)
	return id, err
}

// SessionUpperMessageRowID captures a session's highest message rowid so a pass
// never walks rows appended after it started.
func SessionUpperMessageRowID(ctx context.Context, q sqlx.QueryerContext, sessionID string) (int64, error) {
	var rowID int64
	err := sqlx.GetContext(ctx, q, &rowID, `SELECT COALESCE(MAX(rowid),0) FROM task_session_messages WHERE task_session_id=?`, sessionID)
	return rowID, err
}

// NextArchivedMessageRows lists the next page of one archived session's messages.
func NextArchivedMessageRows(ctx context.Context, q sqlx.QueryerContext, sessionID string, afterRowID, upperRowID int64, limit int) ([]ArchivedMessageRow, error) {
	rows := []ArchivedMessageRow{}
	err := sqlx.SelectContext(ctx, q, &rows, `SELECT rowid AS row_id,id,type,
	 COALESCE(LENGTH(CAST(metadata AS BLOB)),0) AS bytes
	 FROM task_session_messages
	 WHERE task_session_id=? AND rowid>? AND rowid<=? ORDER BY rowid LIMIT ?`,
		sessionID, afterRowID, upperRowID, limit)
	return rows, err
}

// ArchivedMessageMetadata reads one candidate message's metadata body.
func ArchivedMessageMetadata(ctx context.Context, q sqlx.QueryerContext, rowID int64, id string) (string, error) {
	var raw string
	err := sqlx.GetContext(ctx, q, &raw, `SELECT COALESCE(metadata,'') FROM task_session_messages WHERE rowid=? AND id=?`, rowID, id)
	return raw, err
}

// WriteReducedMessageMetadata replaces a message's metadata only while it still
// holds the value the reduction was computed from, so a concurrent write wins
// instead of being overwritten.
func WriteReducedMessageMetadata(ctx context.Context, tx *sqlx.Tx, id, expected, replacement string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_messages SET metadata=?
	 WHERE id=? AND metadata=?`), replacement, id, expected)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("message_conflict")
	}
	return nil
}

// NextSucceededCleanupJobID returns the next succeeded job completed at or
// before the cutoff. The window is part of the cursor query so an out-of-window
// job is never counted as a scanned row or charged to the batch byte budget.
func NextSucceededCleanupJobID(ctx context.Context, q sqlx.QueryerContext, after, upper string, cutoff time.Time) (string, error) {
	var id string
	err := sqlx.GetContext(ctx, q, &id, `SELECT id FROM task_resource_cleanup_jobs
	 WHERE state=? AND id>? AND id<=? AND completed_at IS NOT NULL
	 AND julianday(completed_at)<=julianday(?) ORDER BY id LIMIT 1`,
		models.TaskResourceCleanupStateSucceeded, after, upper, cutoff.UTC().Format(time.RFC3339Nano))
	return id, err
}

// SucceededCleanupUpperJobID captures the highest succeeded job id for a pass.
func SucceededCleanupUpperJobID(ctx context.Context, q sqlx.QueryerContext) (string, error) {
	var id string
	err := sqlx.GetContext(ctx, q, &id, `SELECT COALESCE(MAX(id),'') FROM task_resource_cleanup_jobs WHERE state=?`,
		models.TaskResourceCleanupStateSucceeded)
	return id, err
}

// CleanupJobSnapshotRow reads a job's stored snapshot size and value.
func CleanupJobSnapshotRow(ctx context.Context, q sqlx.QueryerContext, id string) (int64, string, error) {
	var bytes int64
	var snapshot string
	if err := sqlx.GetContext(ctx, q, &bytes, `SELECT COALESCE(LENGTH(CAST(resource_snapshot AS BLOB)),0)
	 FROM task_resource_cleanup_jobs WHERE id=?`, id); err != nil {
		return 0, "", err
	}
	if err := sqlx.GetContext(ctx, q, &snapshot, `SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id=?`, id); err != nil {
		return 0, "", err
	}
	return bytes, snapshot, nil
}

// WriteReducedCleanupSnapshot replaces a succeeded job's snapshot only while it
// still holds the value the reduction was computed from, so a job that left the
// succeeded state keeps its snapshot.
func WriteReducedCleanupSnapshot(ctx context.Context, tx *sqlx.Tx, id, expected, replacement string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_resource_cleanup_jobs SET resource_snapshot=?
	 WHERE id=? AND state=? AND resource_snapshot=?`),
		replacement, id, models.TaskResourceCleanupStateSucceeded, expected)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("cleanup_snapshot_conflict")
	}
	return nil
}
