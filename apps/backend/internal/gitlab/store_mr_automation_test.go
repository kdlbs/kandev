package gitlab

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func boolPtr(b bool) *bool { return &b }

func TestStore_GetTaskMRAutomationOptions_ImplicitDefault(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if opts.TaskID != "task-1" || opts.PromptOnReviewRequested || opts.PromptOnMerged ||
		opts.PromptOnClosed || opts.ReviewReviewerUsername != "" {
		t.Fatalf("expected all-false implicit default, got %+v", opts)
	}
}

func TestStore_UpdateTaskMRAutomationOptions_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	updated, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	}, nil)
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptions: %v", err)
	}
	if !updated.PromptOnMerged || updated.PromptOnReviewRequested || updated.PromptOnClosed {
		t.Fatalf("unexpected options after first patch: %+v", updated)
	}

	username := "alice"
	updated, err = store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}, &username)
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptions second patch: %v", err)
	}
	if !updated.PromptOnMerged || !updated.PromptOnReviewRequested || updated.ReviewReviewerUsername != "alice" {
		t.Fatalf("expected merged switch preserved and reviewer set, got %+v", updated)
	}

	got, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if !got.PromptOnMerged || !got.PromptOnReviewRequested || got.ReviewReviewerUsername != "alice" {
		t.Fatalf("persisted options mismatch: %+v", got)
	}
}

func TestStore_TaskMRLifecycleState_CheckpointIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "open"); err != nil {
		t.Fatalf("SetTaskMRObservedState a: %v", err)
	}
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/b", 2, "merged"); err != nil {
		t.Fatalf("SetTaskMRObservedState b: %v", err)
	}

	a, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || a == nil || a.LastObservedState != "open" {
		t.Fatalf("checkpoint a leaked or wrong: %+v err=%v", a, err)
	}
	b, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/b", 2)
	if err != nil || b == nil || b.LastObservedState != "merged" {
		t.Fatalf("checkpoint b leaked or wrong: %+v err=%v", b, err)
	}

	states, err := store.ListTaskMRLifecycleStates(ctx, "task-1")
	if err != nil || len(states) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d err=%v", len(states), err)
	}
}

func TestStore_GetTaskMRLifecycleState_NilWhenAbsent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil {
		t.Fatalf("GetTaskMRLifecycleState: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil checkpoint, got %+v", got)
	}
}

func TestStore_SetTaskMRReviewRequestState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/a", 1, true); err != nil {
		t.Fatalf("SetTaskMRReviewRequestState: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || !got.ReviewRequestInitialized || !got.LastReviewRequested {
		t.Fatalf("unexpected checkpoint: %+v err=%v", got, err)
	}
}

func TestStore_RecordTaskMRLifecyclePrompt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Event: mrLifecycleEventMerged, SessionID: "sess-1",
		PromptedAt: time.Now().UTC(), ObservedState: "merged",
	})
	if err != nil {
		t.Fatalf("RecordTaskMRLifecyclePrompt: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastLifecycleEvent != mrLifecycleEventMerged || got.LastObservedState != "merged" ||
		got.LastLifecyclePromptAt == nil || got.LastLifecycleSessionID == nil || *got.LastLifecycleSessionID != "sess-1" {
		t.Fatalf("unexpected checkpoint after prompt: %+v", got)
	}
}

func TestStore_RecordAndClearTaskMRAutomationError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "boom"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastError == nil || *got.LastError != "boom" {
		t.Fatalf("unexpected checkpoint after error: %+v err=%v", got, err)
	}
	if err := store.ClearTaskMRAutomationError(ctx, "task-1", "", "group/a", 1); err != nil {
		t.Fatalf("ClearTaskMRAutomationError: %v", err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastError != nil {
		t.Fatalf("expected cleared error, got %+v err=%v", got, err)
	}
}

// TestStore_SyncErrorAndDeliveryErrorDoNotClobberEachOther proves the fix for
// a review finding: last_error (lifecycle delivery) and last_sync_error
// (poller sync) are separate columns, so a poller sync success clearing
// last_sync_error cannot erase an unresolved delivery error recorded by the
// orchestrator's evaluation pass, and vice versa.
func TestStore_SyncErrorAndDeliveryErrorDoNotClobberEachOther(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "delivery failed"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/a", 1, "sync failed"); err != nil {
		t.Fatalf("RecordTaskMRSyncError: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil ||
		got.LastError == nil || *got.LastError != "delivery failed" ||
		got.LastSyncError == nil || *got.LastSyncError != "sync failed" {
		t.Fatalf("expected both errors recorded independently, got %+v err=%v", got, err)
	}

	// A recovered sync must not erase the still-unresolved delivery error.
	if err := store.ClearTaskMRSyncError(ctx, "task-1", "", "group/a", 1); err != nil {
		t.Fatalf("ClearTaskMRSyncError: %v", err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastSyncError != nil {
		t.Fatalf("expected sync error cleared, got %+v err=%v", got, err)
	}
	if got.LastError == nil || *got.LastError != "delivery failed" {
		t.Fatalf("expected delivery error to survive a sync-error clear, got %+v", got)
	}

	// A recovered delivery must not erase a still-unresolved sync error.
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/a", 1, "sync failed again"); err != nil {
		t.Fatalf("RecordTaskMRSyncError: %v", err)
	}
	if err := store.ClearTaskMRAutomationError(ctx, "task-1", "", "group/a", 1); err != nil {
		t.Fatalf("ClearTaskMRAutomationError: %v", err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil || got.LastError != nil {
		t.Fatalf("expected delivery error cleared, got %+v err=%v", got, err)
	}
	if got.LastSyncError == nil || *got.LastSyncError != "sync failed again" {
		t.Fatalf("expected sync error to survive a delivery-error clear, got %+v", got)
	}
}

func TestStore_RebindTaskMRReviewer_ClearsBaselines(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	alice := "alice"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}, &alice); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/a", 1, true); err != nil {
		t.Fatalf("SetTaskMRReviewRequestState: %v", err)
	}

	changed, err := store.RebindTaskMRReviewer(ctx, "task-1", "alice")
	if err != nil {
		t.Fatalf("RebindTaskMRReviewer (unchanged): %v", err)
	}
	if changed {
		t.Fatalf("expected no change when username is identical")
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if !got.LastReviewRequested {
		t.Fatalf("baseline should survive an unchanged rebind: %+v", got)
	}

	changed, err = store.RebindTaskMRReviewer(ctx, "task-1", "bob")
	if err != nil {
		t.Fatalf("RebindTaskMRReviewer (changed): %v", err)
	}
	if !changed {
		t.Fatalf("expected change when username differs")
	}
	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil || opts.ReviewReviewerUsername != "bob" {
		t.Fatalf("reviewer not updated: %+v err=%v", opts, err)
	}
	got, err = store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.ReviewRequestInitialized || got.LastReviewRequested {
		t.Fatalf("expected review baseline reset after reviewer change: %+v", got)
	}
}

// TestStore_UpdateTaskMRAutomationOptions_ResendingSameSwitchResetsBaselineOnIdentityChange
// pins a gap in the boolean-only reviewChanged check: a PATCH that resends
// prompt_on_review_requested=true (already true) still re-resolves the
// authenticated username, which can differ after the workspace's connected
// GitLab account changes. The baseline must reset even though the boolean
// itself never flips, otherwise a request already recorded for the old
// identity can suppress the next prompt evaluated against the new one.
func TestStore_UpdateTaskMRAutomationOptions_ResendingSameSwitchResetsBaselineOnIdentityChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	alice := "alice"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}, &alice); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/a", 1, true); err != nil {
		t.Fatalf("SetTaskMRReviewRequestState: %v", err)
	}

	bob := "bob"
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true), // unchanged: was already true
	}, &bob); err != nil {
		t.Fatalf("resend patch with new identity: %v", err)
	}

	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil || opts.ReviewReviewerUsername != "bob" {
		t.Fatalf("reviewer not updated: %+v err=%v", opts, err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.ReviewRequestInitialized || got.LastReviewRequested {
		t.Fatalf("expected review baseline reset after identity change even with an unchanged switch: %+v", got)
	}
}

// TestStore_UpdateTaskMRAutomationOptions_ReenablingMergedSwitchResetsCheckpoint
// is the P1 finding: an MR that reached the merged state while the switch was
// off must not stay permanently suppressed once the switch is re-enabled.
func TestStore_UpdateTaskMRAutomationOptions_ReenablingMergedSwitchResetsCheckpoint(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("enable switch: %v", err)
	}
	if err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID: "task-1", ProjectPath: "group/a", MRIID: 1,
		Event: gitlabStateMerged, PromptedAt: time.Now().UTC(), ObservedState: gitlabStateMerged,
	}); err != nil {
		t.Fatalf("record merged prompt: %v", err)
	}

	// Disable, then re-enable — the checkpoint from the still-merged MR must
	// not survive, or the re-enabled switch would never fire for it again.
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(false),
	}, nil); err != nil {
		t.Fatalf("disable switch: %v", err)
	}
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("re-enable switch: %v", err)
	}

	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastObservedState != "" || got.LastLifecycleEvent != "" {
		t.Fatalf("expected terminal checkpoint reset after re-enabling the switch, got %+v", got)
	}
}

// TestStore_ObservedStateAndErrorMutatorsDoNotClobberEachOther pins the
// data-integrity fix directly: SetTaskMRObservedState and
// RecordTaskMRAutomationError each write only their own column now, so
// calling one after the other must not erase the first one's write (the
// previous full-row-upsert implementation would have lost this on a
// concurrent — or here, simply interleaved — pair of calls).
func TestStore_ObservedStateAndErrorMutatorsDoNotClobberEachOther(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "opened"); err != nil {
		t.Fatalf("SetTaskMRObservedState: %v", err)
	}
	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "transient failure"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}

	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil {
		t.Fatalf("GetTaskMRLifecycleState: %+v err=%v", got, err)
	}
	if got.LastObservedState != "opened" {
		t.Fatalf("RecordTaskMRAutomationError clobbered last_observed_state: %+v", got)
	}
	if got.LastError == nil || *got.LastError != "transient failure" {
		t.Fatalf("expected last_error set: %+v", got)
	}
}

// TestStore_TaskDeleteCascadesMRAutomationRows is the FK-cascade finding: a
// hard-deleted task must not leave its MR automation options or lifecycle
// checkpoints behind — both tables declare ON DELETE CASCADE against tasks.
func TestStore_TaskDeleteCascadesMRAutomationRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("seed options: %v", err)
	}
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/a", 1, "opened"); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, "task-1"); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	opts, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptions: %v", err)
	}
	if opts.PromptOnMerged {
		t.Fatalf("expected options row cascaded away, got %+v", opts)
	}
	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil {
		t.Fatalf("GetTaskMRLifecycleState: %v", err)
	}
	if state != nil {
		t.Fatalf("expected lifecycle state row cascaded away, got %+v", state)
	}
}

func TestStore_ListLifecycleSubscribedTaskMRs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	seedTask(t, store, "task-2", "ws-1")

	subscribed := newTestMR("task-1", "", "group/subscribed", 1)
	if err := store.UpsertTaskMR(ctx, subscribed); err != nil {
		t.Fatalf("upsert subscribed MR: %v", err)
	}
	unsubscribed := newTestMR("task-2", "", "group/unsubscribed", 2)
	if err := store.UpsertTaskMR(ctx, unsubscribed); err != nil {
		t.Fatalf("upsert unsubscribed MR: %v", err)
	}
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("enable switch for task-1: %v", err)
	}

	rows, err := store.ListLifecycleSubscribedTaskMRs(ctx)
	if err != nil {
		t.Fatalf("ListLifecycleSubscribedTaskMRs: %v", err)
	}
	if len(rows) != 1 || rows[0].TaskID != "task-1" || rows[0].ProjectPath != "group/subscribed" {
		t.Fatalf("expected only task-1's MR, got %+v", rows)
	}
}

// TestStore_MRAutomationTables_FreshDBAndReplay covers ADR 0027: schema
// creation must succeed both against a brand new database file and when run
// a second time against an existing one (idempotent CREATE TABLE IF NOT
// EXISTS — no ALTER TABLE migration is involved since both tables are new).
func TestStore_MRAutomationTables_FreshDBAndReplay(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gitlab-replay.db")

	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("fresh-DB NewStore: %v", err)
	}
	assertMRAutomationTablesExist(t, sqlxDB)

	// Same-DB replay: open a second Store against the same file/handle and
	// confirm createTables (and specifically createMRAutomationTables) is a
	// no-op rather than an error on an existing database.
	if _, err := NewStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("same-DB replay NewStore: %v", err)
	}
	assertMRAutomationTablesExist(t, sqlxDB)

	// A nil error from CREATE TABLE IF NOT EXISTS doesn't prove the columns
	// this package's mutators depend on actually exist — exercise a
	// representative write/read round trip through every column added this
	// session (including the last_sync_error split) as a functional check.
	ctx := context.Background()
	if _, err := sqlxDB.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', '')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := store.RecordTaskMRAutomationError(ctx, "task-1", "", "group/a", 1, "delivery"); err != nil {
		t.Fatalf("RecordTaskMRAutomationError: %v", err)
	}
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/a", 1, "sync"); err != nil {
		t.Fatalf("RecordTaskMRSyncError: %v", err)
	}
	got, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
	if err != nil || got == nil ||
		got.LastError == nil || *got.LastError != "delivery" ||
		got.LastSyncError == nil || *got.LastSyncError != "sync" {
		t.Fatalf("expected both error columns to round-trip, got %+v err=%v", got, err)
	}
}

// assertMRAutomationTablesExist queries sqlite_master directly so a nil error
// from CREATE TABLE IF NOT EXISTS can't mask a table that was never actually
// created (e.g. a typo in the DDL that SQLite silently no-ops on replay).
func assertMRAutomationTablesExist(t *testing.T, sqlxDB *sqlx.DB) {
	t.Helper()
	for _, table := range []string{"gitlab_task_mr_options", "gitlab_task_mr_state"} {
		var name string
		if err := sqlxDB.Get(&name, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}
}
