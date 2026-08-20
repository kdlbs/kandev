package sqlite

// Coverage for workflow_step_entries / workflow_step_entry_markers: the
// step-entry allocation hook inside updateTaskTx (via stepentry's
// context-carried PendingAllocation/AllocationResult), entry_seq
// incrementing across repeated entries into the same step, and the
// CAS claim/complete semantics markers use to guarantee at-most-once
// dispatch of an engine-owned on_enter action.
//
// See docs/specs/workflow-on-enter-action-dispatch/spec.md and this Build
// round's task plan scope note: this file exercises only the write path this
// round wires (plain UpdateTask through updateTaskTx), not the full
// multi-process epoch/staleness reconciliation the spec describes.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
)

func newStepEntriesTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "step-entries.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo
}

func createStepEntriesTestTask(t *testing.T, repo *Repository, taskID, workflowID, stepID string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-1",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Test Task",
		Priority:       "medium",
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

type stepEntryRow struct {
	id        int64
	taskID    string
	stepID    string
	entrySeq  int64
	digest    string
	createdAt time.Time
}

func stepEntryRowsForTask(t *testing.T, repo *Repository, taskID string) []stepEntryRow {
	t.Helper()
	rows, err := repo.db.QueryContext(context.Background(), repo.db.Rebind(`
		SELECT id, task_id, step_id, entry_seq, digest, created_at
		FROM workflow_step_entries WHERE task_id = ? ORDER BY entry_seq ASC
	`), taskID)
	if err != nil {
		t.Fatalf("query workflow_step_entries: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []stepEntryRow
	for rows.Next() {
		var r stepEntryRow
		if err := rows.Scan(&r.id, &r.taskID, &r.stepID, &r.entrySeq, &r.digest, &r.createdAt); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		out = append(out, r)
	}
	return out
}

func TestUpdateTaskAllocatesStepEntryWhenPendingAllocationPresent(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	task := createStepEntriesTestTask(t, repo, "task-1", "wf-1", "step-a")

	task.WorkflowStepID = "step-review"
	holder := &stepentry.AllocationResult{}
	ctx := stepentry.WithResultHolder(context.Background(), holder)
	ctx = stepentry.WithPendingAllocation(ctx, stepentry.PendingAllocation{
		StepID: "step-review",
		Digest: "digest-1",
		Positions: []stepentry.EnginePosition{
			{Position: 0, Kind: "clear_decisions"},
			{Position: 1, Kind: "queue_run_for_each_participant"},
		},
	})

	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if holder.EntryID == 0 {
		t.Fatalf("expected holder.EntryID to be populated, got 0")
	}
	if holder.EntrySeq != 1 {
		t.Fatalf("EntrySeq = %d, want 1", holder.EntrySeq)
	}

	rows := stepEntryRowsForTask(t, repo, "task-1")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].stepID != "step-review" {
		t.Fatalf("stepID = %q, want step-review", rows[0].stepID)
	}
	if rows[0].digest != "digest-1" {
		t.Fatalf("digest = %q, want digest-1", rows[0].digest)
	}
	if rows[0].id != holder.EntryID {
		t.Fatalf("row id = %d, want holder.EntryID = %d", rows[0].id, holder.EntryID)
	}
}

func TestUpdateTaskWithoutPendingAllocationAllocatesNothing(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	task := createStepEntriesTestTask(t, repo, "task-2", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	rows := stepEntryRowsForTask(t, repo, "task-2")
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 (no pending allocation attached to ctx)", len(rows))
	}
}

func TestUpdateTaskStepEntrySeqIncrementsAcrossRepeatedEntries(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	task := createStepEntriesTestTask(t, repo, "task-3", "wf-1", "step-a")

	enter := func(stepID string) *stepentry.AllocationResult {
		task.WorkflowStepID = stepID
		holder := &stepentry.AllocationResult{}
		ctx := stepentry.WithResultHolder(context.Background(), holder)
		ctx = stepentry.WithPendingAllocation(ctx, stepentry.PendingAllocation{
			StepID: stepID,
			Digest: "digest",
			Positions: []stepentry.EnginePosition{
				{Position: 0, Kind: "clear_decisions"},
			},
		})
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("UpdateTask: %v", err)
		}
		return holder
	}

	first := enter("step-review")
	if first.EntrySeq != 1 {
		t.Fatalf("first EntrySeq = %d, want 1", first.EntrySeq)
	}

	// Round-trip through a different step and back into step-review — a
	// second entry into the same step (e.g. after a rejection round).
	enter("step-work")
	second := enter("step-review")
	if second.EntrySeq != 2 {
		t.Fatalf("second EntrySeq = %d, want 2", second.EntrySeq)
	}
	if second.EntryID == first.EntryID {
		t.Fatalf("expected a distinct entry row for the second entry, got the same id %d", second.EntryID)
	}

	rows := stepEntryRowsForTask(t, repo, "task-3")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (step-review, step-work, step-review)", len(rows))
	}
}

func allocateOneStepEntry(t *testing.T, repo *Repository, taskID, stepID string) int64 {
	t.Helper()
	task := createStepEntriesTestTask(t, repo, taskID, "wf-1", "step-a")
	task.WorkflowStepID = stepID
	holder := &stepentry.AllocationResult{}
	ctx := stepentry.WithResultHolder(context.Background(), holder)
	ctx = stepentry.WithPendingAllocation(ctx, stepentry.PendingAllocation{
		StepID: stepID,
		Digest: "digest",
		Positions: []stepentry.EnginePosition{
			{Position: 0, Kind: "clear_decisions"},
			{Position: 1, Kind: "queue_run_for_each_participant"},
		},
	})
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	return holder.EntryID
}

func TestClaimStepEntryMarkerFirstClaimSucceeds(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-4", "step-review")

	claimed, err := repo.ClaimStepEntryMarker(context.Background(), entryID, 0, "clear_decisions", "op-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}
	if !claimed {
		t.Fatalf("expected the first claim on an absent marker to succeed")
	}
}

func TestClaimStepEntryMarkerDoubleClaimIsRejected(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-5", "step-review")

	ctx := context.Background()
	claimed, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimStepEntryMarker (first): %v", err)
	}
	if !claimed {
		t.Fatalf("expected the first claim to succeed")
	}

	claimedAgain, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-2", time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimStepEntryMarker (second): %v", err)
	}
	if claimedAgain {
		t.Fatalf("expected a second claim on an already-claimed marker to be rejected, not silently succeed")
	}
}

func TestClaimStepEntryMarkerDistinctPositionsClaimIndependently(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-6", "step-review")

	ctx := context.Background()
	claimedA, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimStepEntryMarker (position 0): %v", err)
	}
	claimedB, err := repo.ClaimStepEntryMarker(ctx, entryID, 1, "queue_run_for_each_participant", "op-2", time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimStepEntryMarker (position 1): %v", err)
	}
	if !claimedA || !claimedB {
		t.Fatalf("expected both distinct positions to claim independently, got (%v, %v)", claimedA, claimedB)
	}
}

func TestCompleteStepEntryMarkerDoneAfterClaim(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-7", "step-review")
	ctx := context.Background()

	if _, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}

	if err := repo.CompleteStepEntryMarker(ctx, entryID, 0, StepEntryMarkerDone, "", time.Now().UTC()); err != nil {
		t.Fatalf("CompleteStepEntryMarker: %v", err)
	}

	state, markerErr := stepEntryMarkerState(t, repo, entryID, 0)
	if state != string(StepEntryMarkerDone) {
		t.Fatalf("state = %q, want %q", state, StepEntryMarkerDone)
	}
	if markerErr != "" {
		t.Fatalf("marker error = %q, want empty", markerErr)
	}
}

func TestCompleteStepEntryMarkerFailedRecordsCause(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-8", "step-review")
	ctx := context.Background()

	if _, err := repo.ClaimStepEntryMarker(ctx, entryID, 1, "queue_run_for_each_participant", "op-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}

	if err := repo.CompleteStepEntryMarker(ctx, entryID, 1, StepEntryMarkerFailed, "queue_run for participant p1: boom", time.Now().UTC()); err != nil {
		t.Fatalf("CompleteStepEntryMarker: %v", err)
	}

	state, markerErr := stepEntryMarkerState(t, repo, entryID, 1)
	if state != string(StepEntryMarkerFailed) {
		t.Fatalf("state = %q, want %q", state, StepEntryMarkerFailed)
	}
	if markerErr != "queue_run for participant p1: boom" {
		t.Fatalf("marker error = %q, want the recorded failure cause", markerErr)
	}
}

func TestCompleteStepEntryMarkerWithoutClaimIsNoop(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-9", "step-review")
	ctx := context.Background()

	// Completing a marker that was never claimed (state is still absent — no
	// row at all) must not fabricate a terminal row: there is nothing to
	// transition, and a spurious "done" row would let a later claim think
	// the action already ran when it never did.
	if err := repo.CompleteStepEntryMarker(ctx, entryID, 0, StepEntryMarkerDone, "", time.Now().UTC()); err != nil {
		t.Fatalf("CompleteStepEntryMarker: %v", err)
	}

	state, _ := stepEntryMarkerState(t, repo, entryID, 0)
	if state != "" {
		t.Fatalf("state = %q, want empty (no row, since no claim ever happened)", state)
	}
}

func TestGetStepEntryMarkerStateNoRowReturnsNotFound(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-10", "step-review")
	ctx := context.Background()

	_, _, found, err := repo.GetStepEntryMarkerState(ctx, entryID, 0)
	if err != nil {
		t.Fatalf("GetStepEntryMarkerState: %v", err)
	}
	if found {
		t.Fatalf("found = true, want false (no claim ever happened)")
	}
}

func TestGetStepEntryMarkerStateReportsDone(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-11", "step-review")
	ctx := context.Background()

	if _, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}
	if err := repo.CompleteStepEntryMarker(ctx, entryID, 0, StepEntryMarkerDone, "", time.Now().UTC()); err != nil {
		t.Fatalf("CompleteStepEntryMarker: %v", err)
	}

	state, cause, found, err := repo.GetStepEntryMarkerState(ctx, entryID, 0)
	if err != nil {
		t.Fatalf("GetStepEntryMarkerState: %v", err)
	}
	if !found {
		t.Fatalf("found = false, want true")
	}
	if state != StepEntryMarkerDone {
		t.Fatalf("state = %q, want %q", state, StepEntryMarkerDone)
	}
	if cause != "" {
		t.Fatalf("cause = %q, want empty", cause)
	}
}

func TestGetStepEntryMarkerStateReportsFailedWithCause(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	entryID := allocateOneStepEntry(t, repo, "task-12", "step-review")
	ctx := context.Background()

	if _, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}
	if err := repo.CompleteStepEntryMarker(ctx, entryID, 0, StepEntryMarkerFailed, "boom", time.Now().UTC()); err != nil {
		t.Fatalf("CompleteStepEntryMarker: %v", err)
	}

	state, cause, found, err := repo.GetStepEntryMarkerState(ctx, entryID, 0)
	if err != nil {
		t.Fatalf("GetStepEntryMarkerState: %v", err)
	}
	if !found {
		t.Fatalf("found = false, want true")
	}
	if state != StepEntryMarkerFailed {
		t.Fatalf("state = %q, want %q", state, StepEntryMarkerFailed)
	}
	if cause != "boom" {
		t.Fatalf("cause = %q, want %q", cause, "boom")
	}
}

// seedWorkflowStepDecisionsTable creates workflow_step_decisions directly
// (rather than importing internal/workflow/repository, which owns it in
// production) so ClearStepDecisionsAndCompleteMarker has something to
// delete against — see phase2_sqlite.go's decisionsSchema for the DDL this
// mirrors.
func seedWorkflowStepDecisionsTable(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := repo.db.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_step_decisions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			step_id TEXT NOT NULL,
			participant_id TEXT NOT NULL,
			decision TEXT NOT NULL,
			decided_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create workflow_step_decisions: %v", err)
	}
}

func insertStepDecision(t *testing.T, repo *Repository, id, taskID, stepID string) {
	t.Helper()
	_, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		INSERT INTO workflow_step_decisions (id, task_id, step_id, participant_id, decision, decided_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), id, taskID, stepID, "participant-1", "approve", time.Now().UTC())
	if err != nil {
		t.Fatalf("insert workflow_step_decisions row: %v", err)
	}
}

func countStepDecisions(t *testing.T, repo *Repository, taskID, stepID string) int {
	t.Helper()
	var n int
	err := repo.db.QueryRowContext(context.Background(), repo.db.Rebind(
		`SELECT COUNT(*) FROM workflow_step_decisions WHERE task_id = ? AND step_id = ?`,
	), taskID, stepID).Scan(&n)
	if err != nil {
		t.Fatalf("count workflow_step_decisions: %v", err)
	}
	return n
}

func TestClearStepDecisionsAndCompleteMarkerDeletesDecisionsAndCompletesMarker(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	seedWorkflowStepDecisionsTable(t, repo)
	entryID := allocateOneStepEntry(t, repo, "task-10", "step-review")
	ctx := context.Background()

	insertStepDecision(t, repo, "dec-1", "task-10", "step-review")
	insertStepDecision(t, repo, "dec-2", "task-10", "step-review")

	if _, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}

	rows, err := repo.ClearStepDecisionsAndCompleteMarker(ctx, "task-10", "step-review", entryID, 0, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClearStepDecisionsAndCompleteMarker: %v", err)
	}
	if rows != 2 {
		t.Fatalf("rows = %d, want 2", rows)
	}

	if n := countStepDecisions(t, repo, "task-10", "step-review"); n != 0 {
		t.Fatalf("remaining decisions = %d, want 0", n)
	}

	state, markerErr := stepEntryMarkerState(t, repo, entryID, 0)
	if state != string(StepEntryMarkerDone) {
		t.Fatalf("marker state = %q, want %q", state, StepEntryMarkerDone)
	}
	if markerErr != "" {
		t.Fatalf("marker error = %q, want empty", markerErr)
	}
}

func TestClearStepDecisionsAndCompleteMarkerRejectsMissingIDs(t *testing.T) {
	repo := newStepEntriesTestRepo(t)
	seedWorkflowStepDecisionsTable(t, repo)
	entryID := allocateOneStepEntry(t, repo, "task-11", "step-review")
	ctx := context.Background()

	insertStepDecision(t, repo, "dec-1", "task-11", "step-review")
	if _, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimStepEntryMarker: %v", err)
	}

	if _, err := repo.ClearStepDecisionsAndCompleteMarker(ctx, "", "step-review", entryID, 0, time.Now().UTC()); err == nil {
		t.Fatalf("expected an error for an empty task_id")
	}

	// Nothing should have changed: the validation guard runs before BeginTx.
	if n := countStepDecisions(t, repo, "task-11", "step-review"); n != 1 {
		t.Fatalf("remaining decisions = %d, want 1 (untouched)", n)
	}
	state, _ := stepEntryMarkerState(t, repo, entryID, 0)
	if state != string(StepEntryMarkerInProgress) {
		t.Fatalf("marker state = %q, want %q (untouched)", state, StepEntryMarkerInProgress)
	}
}

func stepEntryMarkerState(t *testing.T, repo *Repository, entryID int64, position int) (state, markerErr string) {
	t.Helper()
	var s, e *string
	err := repo.db.QueryRowContext(context.Background(), repo.db.Rebind(`
		SELECT state, error FROM workflow_step_entry_markers WHERE entry_id = ? AND position = ?
	`), entryID, position).Scan(&s, &e)
	if err != nil {
		return "", ""
	}
	if s != nil {
		state = *s
	}
	if e != nil {
		markerErr = *e
	}
	return state, markerErr
}
