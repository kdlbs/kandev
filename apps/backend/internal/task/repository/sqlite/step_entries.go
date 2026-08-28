package sqlite

// workflow_step_entries / workflow_step_entry_markers back the step-entry
// allocation and per-action CAS markers described in
// docs/specs/workflow-on-enter-action-dispatch/spec.md. This Build round
// wires allocation into exactly one chokepoint (updateTaskTx, shared by
// UpdateTask and UpdateTaskWithWorkflowStepAdmission(AndState)) via the
// context-carried stepentry.PendingAllocation/AllocationResult contract —
// see internal/workflow/stepentry for why a context value was chosen over
// changing ApplyTransition's or the admission methods' signatures.
//
// Scope note: this is a deliberately simplified, single-writer-per-entry CAS
// model, not the spec's full multi-process epoch/staleness reconciliation.
// A marker only ever has one row; claiming is "insert from absent", and a
// UNIQUE-constraint conflict on that insert means "already claimed or
// handled — skip", not "steal the marker from a stale epoch". Full
// epoch/staleness reconciliation under real contention and the AC-B9
// startup recovery scan are explicitly deferred to a later Build round.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/stepentry"
)

// StepEntryMarkerState is a marker's terminal (or in-progress) state. It is
// an alias for stepentry.MarkerState so orchestrator-side dispatch callers
// can pass the same constants without importing this repository package.
type StepEntryMarkerState = stepentry.MarkerState

const (
	// StepEntryMarkerInProgress is set the moment a claim succeeds.
	StepEntryMarkerInProgress = stepentry.MarkerInProgress
	// StepEntryMarkerDone marks a successfully executed action.
	StepEntryMarkerDone = stepentry.MarkerDone
	// StepEntryMarkerFailed marks an action whose callback returned an
	// error; the error column records the cause.
	StepEntryMarkerFailed = stepentry.MarkerFailed
)

// stepEntryMarkerUniqueMessage is the substring go-sqlite3 puts in a
// UNIQUE-constraint error for the (entry_id, position) index — see
// isExternalIDUniqueViolation in task_external_id.go for the same
// cross-dialect approach applied to a different index.
const stepEntryMarkerUniqueMessage = "UNIQUE constraint failed: workflow_step_entry_markers.entry_id, workflow_step_entry_markers.position"

const stepEntryMarkerIndexName = "uniq_workflow_step_entry_markers_entry_position"

func isStepEntryMarkerUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == stepEntryMarkerIndexName
	}
	return strings.Contains(err.Error(), stepEntryMarkerUniqueMessage)
}

// initStepEntriesSchema creates workflow_step_entries and
// workflow_step_entry_markers. New tables, not columns added to existing
// ones, so CREATE TABLE IF NOT EXISTS in the init block is correct — see
// initStepTransitionsSchema's comment for why that rule doesn't apply here.
func (r *Repository) initStepEntriesSchema() error {
	idCol := dialect.AutoIncrementIDColumn(r.db.DriverName())
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS workflow_step_entries (
		` + idCol + `,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		step_id TEXT NOT NULL,
		entry_seq INTEGER NOT NULL,
		digest TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		UNIQUE(task_id, step_id, entry_seq)
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_entries_task_step
		ON workflow_step_entries(task_id, step_id);

	CREATE TABLE IF NOT EXISTS workflow_step_entry_markers (
		` + idCol + `,
		entry_id INTEGER NOT NULL REFERENCES workflow_step_entries(id) ON DELETE CASCADE,
		position INTEGER NOT NULL,
		kind TEXT NOT NULL,
		state TEXT NOT NULL,
		operation_id TEXT,
		error TEXT,
		claimed_at TIMESTAMP,
		completed_at TIMESTAMP,
		CONSTRAINT uniq_workflow_step_entry_markers_entry_position UNIQUE(entry_id, position)
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_entry_markers_entry
		ON workflow_step_entry_markers(entry_id);
	`)
	if err != nil {
		return fmt.Errorf("init step entries schema: %w", err)
	}
	return nil
}

// allocateStepEntryIfPending reads a PendingAllocation off ctx (attached by
// the caller that resolved this transition's target step — see
// workflow_store.applyTransition) and, when present, allocates a new
// workflow_step_entries row inside tx. entry_seq is the 1-based count of
// prior entries into (task_id, step_id) for this task, so a task
// re-entering the same step after a rejection round gets a fresh entry
// rather than reusing the first one. When a ResultHolder is also attached,
// the allocated (entry_id, entry_seq) is written back through it so a
// caller several calls further up the stack can read it after commit.
//
// No-op (returns nil) when no PendingAllocation is attached — the common
// case for write paths that haven't opted into step-entry allocation this
// round (e.g. manual moves, still using the legacy processOnEnter path).
func (r *Repository) allocateStepEntryIfPending(ctx context.Context, tx stepTransitionTx, taskID string, occurredAt time.Time) error {
	pending, ok := stepentry.FromContext(ctx)
	if !ok {
		return nil
	}

	var seqCount int64
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT COUNT(*) FROM workflow_step_entries WHERE task_id = ? AND step_id = ?
	`), taskID, pending.StepID).Scan(&seqCount)
	if err != nil {
		return fmt.Errorf("count prior step entries: %w", err)
	}
	entrySeq := seqCount + 1

	var entryID int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_step_entries (task_id, step_id, entry_seq, digest, created_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`), taskID, pending.StepID, entrySeq, pending.Digest, occurredAt).Scan(&entryID)
	if err != nil {
		return fmt.Errorf("allocate step entry: %w", err)
	}

	if holder, ok := stepentry.ResultHolderFromContext(ctx); ok {
		holder.EntryID = entryID
		holder.EntrySeq = entrySeq
	}
	return nil
}

// ClaimStepEntryMarker attempts to claim (entryID, position) for execution,
// transitioning it from absent (no row) to in_progress. claimed is false —
// not an error — when the marker already has a row (already claimed by a
// concurrent or prior attempt, or already terminal): the caller must treat
// that as "do not execute the action again", per AC-D3/AC-D4's at-most-once
// requirement for a given step-entry.
func (r *Repository) ClaimStepEntryMarker(ctx context.Context, entryID int64, position int, kind, operationID string, claimedAt time.Time) (bool, error) {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_step_entry_markers (entry_id, position, kind, state, operation_id, claimed_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), entryID, position, kind, string(StepEntryMarkerInProgress), operationID, claimedAt)
	if err != nil {
		if isStepEntryMarkerUniqueViolation(err) {
			return false, nil
		}
		return false, fmt.Errorf("claim step entry marker: %w", err)
	}
	return true, nil
}

// CompleteStepEntryMarker transitions a claimed marker to a terminal state
// (done/failed). It is a no-op — not an error — when the marker was never
// claimed (no row at all): there is nothing to complete, and fabricating a
// terminal row here would let a later, legitimate claim believe the action
// already ran.
func (r *Repository) CompleteStepEntryMarker(ctx context.Context, entryID int64, position int, state StepEntryMarkerState, cause string, completedAt time.Time) error {
	var errCol sql.NullString
	if cause != "" {
		errCol = sql.NullString{String: cause, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_step_entry_markers
		SET state = ?, error = ?, completed_at = ?
		WHERE entry_id = ? AND position = ? AND state = ?
	`), string(state), errCol, completedAt, entryID, position, string(StepEntryMarkerInProgress))
	if err != nil {
		return fmt.Errorf("complete step entry marker: %w", err)
	}
	return nil
}

// GetStepEntryMarkerState reads back a marker's current state and stored
// failure cause (if any). found is false when no marker row exists for
// (entryID, position) — the marker was never claimed. Used by the CAS-loser
// path in dispatchEngineOwnedOnEnterAction: losing the claim to an existing
// row means "already claimed or handled," but that covers three different
// outcomes (still in progress elsewhere, already done, or already failed
// terminally), and only this read can tell them apart.
func (r *Repository) GetStepEntryMarkerState(ctx context.Context, entryID int64, position int) (state StepEntryMarkerState, cause string, found bool, err error) {
	var row struct {
		State string         `db:"state"`
		Error sql.NullString `db:"error"`
	}
	queryErr := r.db.GetContext(ctx, &row, r.db.Rebind(`
		SELECT state, error FROM workflow_step_entry_markers WHERE entry_id = ? AND position = ?
	`), entryID, position)
	if errors.Is(queryErr, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if queryErr != nil {
		return "", "", false, fmt.Errorf("get step entry marker state: %w", queryErr)
	}
	return StepEntryMarkerState(row.State), row.Error.String, true, nil
}

// ClearStepDecisionsAndCompleteMarker satisfies AC-B6: it deletes every
// decision for (taskID, stepID) and marks the clear_decisions position
// done, in one database transaction. A crash between the delete and the
// marker update is therefore impossible to observe as "delete committed,
// marker still in_progress" — the transaction either commits both or
// neither, which is the invariant a future AC-B9 recovery scan needs to
// safely decide whether re-running clear_decisions for a non-terminal
// marker is necessary.
//
// This duplicates the DELETE FROM workflow_step_decisions statement in
// workflow/repository.Repository.ClearStepDecisions rather than depending
// on that package: AC-B6 and AC-B7 together require the decisions table
// and the step-entry tables to be written through the same transaction,
// and workflow/repository.Repository exposes no way to join a
// caller-supplied *sql.Tx into its own writes. Both tables live in the same
// database reached through the same writer *sqlx.DB (see
// internal/backendapp/storage.go), so running both statements here, against
// this repository's own r.db, is safe — see
// orchestrator.dispatchClearDecisionsAtomic for the call site and the
// capability check that keeps this path off whenever the DecisionStore
// isn't actually backed by this database (e.g. every existing test double).
func (r *Repository) ClearStepDecisionsAndCompleteMarker(
	ctx context.Context, taskID, stepID string, entryID int64, position int, now time.Time,
) (int64, error) {
	if taskID == "" || stepID == "" {
		return 0, errors.New("task_id and step_id are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin clear decisions transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM workflow_step_decisions WHERE task_id = ? AND step_id = ?`,
	), taskID, stepID)
	if err != nil {
		return 0, fmt.Errorf("clear step decisions: %w", err)
	}
	rows, _ := res.RowsAffected()

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_step_entry_markers
		SET state = ?, error = NULL, completed_at = ?
		WHERE entry_id = ? AND position = ? AND state = ?
	`), string(StepEntryMarkerDone), now, entryID, position, string(StepEntryMarkerInProgress))
	if err != nil {
		return 0, fmt.Errorf("complete step entry marker: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit clear decisions transaction: %w", err)
	}
	return rows, nil
}
