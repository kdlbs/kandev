package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestUpdateTaskStateIfSessionState_RequiresCurrentOwningSession(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-session-cas")
	insertSession(t, repo, "session-cas", "task-session-cas", string(models.TaskSessionStateWaitingForInput))

	if err := repo.UpdateTaskState(ctx, "task-session-cas", v1.TaskStateReview); err != nil {
		t.Fatalf("seed REVIEW: %v", err)
	}
	oldState, updated, err := repo.UpdateTaskStateIfSessionState(
		ctx,
		"task-session-cas",
		"session-cas",
		models.TaskSessionStateStarting,
		v1.TaskStateInProgress,
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfSessionState: %v", err)
	}
	if updated {
		t.Fatal("expected stale STARTING writer to be rejected after clarification paused the session")
	}
	if oldState != v1.TaskStateReview {
		t.Fatalf("old state = %q, want %q", oldState, v1.TaskStateReview)
	}
	task, err := repo.GetTask(ctx, "task-session-cas")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Fatalf("task state = %q, want %q", task.State, v1.TaskStateReview)
	}
}

func TestUpdateTaskStateIfSessionState_SkipsArchivedTask(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-session-archived")
	insertSession(t, repo, "session-archived", "task-session-archived", string(models.TaskSessionStateStarting))

	if err := repo.UpdateTaskState(ctx, "task-session-archived", v1.TaskStateReview); err != nil {
		t.Fatalf("seed REVIEW: %v", err)
	}
	if err := repo.ArchiveTask(ctx, "task-session-archived"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	oldState, updated, err := repo.UpdateTaskStateIfSessionState(
		ctx,
		"task-session-archived",
		"session-archived",
		models.TaskSessionStateStarting,
		v1.TaskStateInProgress,
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfSessionState: %v", err)
	}
	if updated {
		t.Fatal("expected archived task write to be rejected")
	}
	if oldState != v1.TaskStateReview {
		t.Fatalf("old state = %q, want %q", oldState, v1.TaskStateReview)
	}
}

func TestUpdateTaskStateIfSessionState_UpdatesMatchingSession(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-session-match")
	insertSession(t, repo, "session-match", "task-session-match", string(models.TaskSessionStateStarting))

	if err := repo.UpdateTaskState(ctx, "task-session-match", v1.TaskStateReview); err != nil {
		t.Fatalf("seed REVIEW: %v", err)
	}
	oldState, updated, err := repo.UpdateTaskStateIfSessionState(
		ctx,
		"task-session-match",
		"session-match",
		models.TaskSessionStateStarting,
		v1.TaskStateInProgress,
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfSessionState: %v", err)
	}
	if !updated {
		t.Fatal("expected matching session to permit task update")
	}
	if oldState != v1.TaskStateReview {
		t.Fatalf("old state = %q, want %q", oldState, v1.TaskStateReview)
	}
	task, err := repo.GetTask(ctx, "task-session-match")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Fatalf("task state = %q, want %q", task.State, v1.TaskStateInProgress)
	}
}

func TestRestoreTaskMessageRollbackIfSessionState_RejectionDoesNotMutateCandidate(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-rollback-rejected")
	insertSession(t, repo, "session-rollback-rejected", "task-rollback-rejected", string(models.TaskSessionStateWaitingForInput))
	if _, err := repo.db.Exec(`
		UPDATE tasks SET state = ?, workflow_step_id = ? WHERE id = ?
	`, v1.TaskStateInProgress, "current-step", "task-rollback-rejected"); err != nil {
		t.Fatalf("seed current task fields: %v", err)
	}

	originalUpdatedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	candidate := &models.Task{
		ID:             "task-rollback-rejected",
		State:          v1.TaskStateReview,
		WorkflowStepID: "restored-step",
		UpdatedAt:      originalUpdatedAt,
	}
	updated, err := repo.RestoreTaskMessageRollbackIfSessionState(
		ctx,
		candidate,
		"session-rollback-rejected",
		models.TaskSessionStateRunning,
	)
	if err != nil {
		t.Fatalf("RestoreTaskMessageRollbackIfSessionState: %v", err)
	}
	if updated {
		t.Fatal("expected session-state mismatch to reject rollback")
	}
	if !candidate.UpdatedAt.Equal(originalUpdatedAt) {
		t.Fatalf("rejected candidate UpdatedAt = %v, want unchanged %v", candidate.UpdatedAt, originalUpdatedAt)
	}
	persisted, err := repo.GetTask(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if persisted.State != v1.TaskStateInProgress || persisted.WorkflowStepID != "current-step" {
		t.Fatalf("rejected rollback persisted state/step = %q/%q", persisted.State, persisted.WorkflowStepID)
	}
}

func TestRestoreTaskMessageRollbackIfSessionState_RestoresFieldsAndRunner(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-rollback-success")
	insertSession(t, repo, "session-rollback-success", "task-rollback-success", string(models.TaskSessionStateRunning))
	if _, err := repo.db.Exec(`
		UPDATE tasks SET state = ?, workflow_step_id = ? WHERE id = ?
	`, v1.TaskStateInProgress, "current-step", "task-rollback-success"); err != nil {
		t.Fatalf("seed current task fields: %v", err)
	}

	originalUpdatedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	candidate := &models.Task{
		ID:                     "task-rollback-success",
		State:                  v1.TaskStateReview,
		WorkflowStepID:         "restored-step",
		AssigneeAgentProfileID: "profile-restored",
		UpdatedAt:              originalUpdatedAt,
	}
	updated, err := repo.RestoreTaskMessageRollbackIfSessionState(
		ctx,
		candidate,
		"session-rollback-success",
		models.TaskSessionStateRunning,
	)
	if err != nil {
		t.Fatalf("RestoreTaskMessageRollbackIfSessionState: %v", err)
	}
	if !updated {
		t.Fatal("expected matching session state to restore task")
	}
	if !candidate.UpdatedAt.After(originalUpdatedAt) {
		t.Fatalf("successful candidate UpdatedAt = %v, want after %v", candidate.UpdatedAt, originalUpdatedAt)
	}
	persisted, err := repo.GetTask(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if persisted.State != v1.TaskStateReview || persisted.WorkflowStepID != "restored-step" {
		t.Fatalf("restored state/step = %q/%q", persisted.State, persisted.WorkflowStepID)
	}
	if persisted.AssigneeAgentProfileID != "profile-restored" {
		t.Fatalf("restored runner = %q, want profile-restored", persisted.AssigneeAgentProfileID)
	}
}

// TestUpdateTaskStateIfCurrentIn_SkipsArchivedTask reproduces the TOCTOU race
// flagged in PR #1706 review: a caller's earlier archived-state guard (a
// plain GetTask read) can observe archived_at == NULL, then have ArchiveTask
// commit before the caller's CAS write lands. Because state is untouched by
// ArchiveTask, the old `WHERE id = ? AND state = ?` clause alone would still
// match and resurrect the task to REVIEW. The archived_at IS NULL clause
// added to the UPDATE closes that window: the write becomes a no-op even
// though currentState (read moments earlier, inside the same transaction)
// was still in the allowed set.
func TestUpdateTaskStateIfCurrentIn_SkipsArchivedTask(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-archived-race")

	if err := repo.UpdateTaskState(ctx, "task-archived-race", v1.TaskStateInProgress); err != nil {
		t.Fatalf("seed IN_PROGRESS: %v", err)
	}

	// Simulates the archive committing in the race window between the
	// caller's taskArchived() guard read and this CAS call.
	if err := repo.ArchiveTask(ctx, "task-archived-race"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	gotState, updated, err := repo.UpdateTaskStateIfCurrentIn(
		ctx, "task-archived-race", v1.TaskStateReview,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if updated {
		t.Fatal("expected archived task's state to be left untouched, got updated=true")
	}
	if gotState != v1.TaskStateInProgress {
		t.Errorf("returned currentState = %q, want %q (pre-CAS read)", gotState, v1.TaskStateInProgress)
	}

	task, err := repo.GetTask(ctx, "task-archived-race")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Errorf("persisted state = %q, want %q (archived task must not resurrect to REVIEW)", task.State, v1.TaskStateInProgress)
	}
	if task.ArchivedAt == nil {
		t.Error("expected task to remain archived")
	}
}

// TestUpdateTaskStateIfCurrentIn_UpdatesWhenNotArchived is the CAS positive
// path sanity check: a non-archived task whose state is in the allowed set
// still transitions normally. Guards against a too-broad archived_at fix
// (e.g. accidentally scoping the WHERE clause to always require archived_at
// IS NULL on every row, including ones with no archive concept at all) that
// would silently break every ordinary REVIEW/IN_PROGRESS transition.
func TestUpdateTaskStateIfCurrentIn_UpdatesWhenNotArchived(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-normal")

	if err := repo.UpdateTaskState(ctx, "task-normal", v1.TaskStateInProgress); err != nil {
		t.Fatalf("seed IN_PROGRESS: %v", err)
	}

	gotState, updated, err := repo.UpdateTaskStateIfCurrentIn(
		ctx, "task-normal", v1.TaskStateReview,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if !updated {
		t.Fatal("expected non-archived task in the allowed set to transition, got updated=false")
	}
	if gotState != v1.TaskStateInProgress {
		t.Errorf("returned currentState = %q, want %q", gotState, v1.TaskStateInProgress)
	}

	task, err := repo.GetTask(ctx, "task-normal")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Errorf("persisted state = %q, want %q", task.State, v1.TaskStateReview)
	}
}

// TestUpdateTaskStateIfNotArchived_SkipsArchivedTask is the
// UpdateTaskStateIfNotArchived analog of TestUpdateTaskStateIfCurrentIn_SkipsArchivedTask
// for the IN_PROGRESS-reconciliation writers (Service/Executor
// writeTaskInProgressForRuntime, review comment on PR #1706): those callers
// have no "allowed" prior-state set to check, only an archived guard, so
// they route through this CAS instead. Same race: ArchiveTask commits
// between the caller's earlier archived-state read and this call; the
// archived_at IS NULL clause inside the UPDATE must still make it a no-op.
func TestUpdateTaskStateIfNotArchived_SkipsArchivedTask(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-archived-race-2")

	if err := repo.UpdateTaskState(ctx, "task-archived-race-2", v1.TaskStateWaitingForInput); err != nil {
		t.Fatalf("seed WAITING_FOR_INPUT: %v", err)
	}

	// Simulates the archive committing in the race window between the
	// caller's taskArchived() guard read and this CAS call.
	if err := repo.ArchiveTask(ctx, "task-archived-race-2"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	gotState, updated, err := repo.UpdateTaskStateIfNotArchived(ctx, "task-archived-race-2", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfNotArchived: %v", err)
	}
	if updated {
		t.Fatal("expected archived task's state to be left untouched, got updated=true")
	}
	if gotState != v1.TaskStateWaitingForInput {
		t.Errorf("returned currentState = %q, want %q (pre-CAS read)", gotState, v1.TaskStateWaitingForInput)
	}

	task, err := repo.GetTask(ctx, "task-archived-race-2")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateWaitingForInput {
		t.Errorf("persisted state = %q, want %q (archived task must not resurrect to IN_PROGRESS)", task.State, v1.TaskStateWaitingForInput)
	}
	if task.ArchivedAt == nil {
		t.Error("expected task to remain archived")
	}
}

// TestUpdateTaskStateIfNotArchived_UpdatesWhenNotArchived is the CAS
// positive-path sanity check: a non-archived task transitions normally
// regardless of its prior state, since UpdateTaskStateIfNotArchived has no
// "allowed" constraint.
func TestUpdateTaskStateIfNotArchived_UpdatesWhenNotArchived(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-normal-2")

	if err := repo.UpdateTaskState(ctx, "task-normal-2", v1.TaskStateWaitingForInput); err != nil {
		t.Fatalf("seed WAITING_FOR_INPUT: %v", err)
	}

	gotState, updated, err := repo.UpdateTaskStateIfNotArchived(ctx, "task-normal-2", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfNotArchived: %v", err)
	}
	if !updated {
		t.Fatal("expected non-archived task to transition, got updated=false")
	}
	if gotState != v1.TaskStateWaitingForInput {
		t.Errorf("returned currentState = %q, want %q", gotState, v1.TaskStateWaitingForInput)
	}

	task, err := repo.GetTask(ctx, "task-normal-2")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Errorf("persisted state = %q, want %q", task.State, v1.TaskStateInProgress)
	}
}

// TestUpdateTaskStateIfNotArchived_RetriesOnConcurrentStateChange reproduces
// the atomicity gap CodeRabbit flagged on PR #1706: the original
// implementation read currentState, then updated any row with
// archived_at IS NULL — with no predicate pinning the row to the state it
// just read. A concurrent writer that changes state (not archived_at)
// between that SELECT and this method's UPDATE would still match, silently
// overwriting the newer state while this method kept reporting the stale
// first-read value as the "pre-update state" — which the service layer
// publishes as the task.state_changed event's old_state.
//
// Reproduces the actual race (not just sequential ordering) using two
// independent SQLite connections to the same database file under WAL mode.
// A single shared *Repository can't exercise this: db.OpenSQLite caps its
// pool to one Go-level connection (single-writer-connection app design), so
// two calls through the SAME repo would be fully serialized by
// database/sql's connection pool before either reached SQLite's own
// locking — never opening the race window. A second, independent
// connection (as a genuinely concurrent writer — another connection or
// process — would be) models that correctly: the racer opens its own write
// transaction, changes the task's state, and holds it open (uncommitted).
// WAL readers don't block on an uncommitted writer, so the main call's
// SELECT (via the first connection) observes the pre-racer state and
// proceeds to its own UPDATE, which blocks on SQLite's single-writer lock
// until the racer commits. Once it does, the UPDATE's WHERE state = <stale
// value> matches zero rows — this is the exact window the fix's
// optimistic-retry loop exists to close: it must re-read and retry rather
// than reporting the stale value or silently clobbering the racer's write.
func TestUpdateTaskStateIfNotArchived_RetriesOnConcurrentStateChange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mainConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open main connection: %v", err)
	}
	mainDB := sqlx.NewDb(mainConn, "sqlite3")
	t.Cleanup(func() { _ = mainDB.Close() })
	repo, err := NewWithDB(mainDB, mainDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	// racerConn is a second, independent connection to the same file —
	// standing in for a genuinely concurrent writer (another connection or
	// process), which is what the atomicity gap this test targets actually
	// requires.
	racerConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open racer connection: %v", err)
	}
	racerDB := sqlx.NewDb(racerConn, "sqlite3")
	t.Cleanup(func() { _ = racerDB.Close() })

	ctx := context.Background()
	insertTask(t, repo.db, "task-state-race")
	if err := repo.UpdateTaskState(ctx, "task-state-race", v1.TaskStateWaitingForInput); err != nil {
		t.Fatalf("seed WAITING_FOR_INPUT: %v", err)
	}

	racerWrote := make(chan struct{})
	releaseRacer := make(chan struct{})
	racerDone := make(chan error, 1)
	go func() {
		tx, err := racerDB.BeginTx(ctx, nil)
		if err != nil {
			racerDone <- err
			return
		}
		if _, err := tx.ExecContext(ctx, racerDB.Rebind(
			`UPDATE tasks SET state = ? WHERE id = ?`,
		), v1.TaskStateBlocked, "task-state-race"); err != nil {
			_ = tx.Rollback()
			racerDone <- err
			return
		}
		close(racerWrote)
		<-releaseRacer
		racerDone <- tx.Commit()
	}()

	select {
	case <-racerWrote:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for racer to write")
	}

	type callResult struct {
		state   v1.TaskState
		updated bool
		err     error
	}
	done := make(chan callResult, 1)
	go func() {
		state, updated, err := repo.UpdateTaskStateIfNotArchived(ctx, "task-state-race", v1.TaskStateInProgress)
		done <- callResult{state, updated, err}
	}()

	// Give the main call's UPDATE time to reach SQLite's write-lock queue
	// (it blocks behind the racer's open transaction) before releasing the
	// racer — otherwise the racer could commit before the main call even
	// starts, and the race window this test targets would never open.
	time.Sleep(50 * time.Millisecond)
	close(releaseRacer)

	if err := <-racerDone; err != nil {
		t.Fatalf("racer transaction failed: %v", err)
	}

	var result callResult
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for UpdateTaskStateIfNotArchived")
	}
	if result.err != nil {
		t.Fatalf("UpdateTaskStateIfNotArchived: %v", result.err)
	}
	if !result.updated {
		t.Fatal("expected the retry to succeed once the racer's write committed, got updated=false")
	}
	if result.state != v1.TaskStateBlocked {
		t.Errorf("returned pre-update state = %q, want %q (the racer's committed value, not the stale first read)",
			result.state, v1.TaskStateBlocked)
	}

	task, err := repo.GetTask(ctx, "task-state-race")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Errorf("persisted state = %q, want %q", task.State, v1.TaskStateInProgress)
	}
}
