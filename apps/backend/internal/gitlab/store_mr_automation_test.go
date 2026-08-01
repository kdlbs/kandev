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

func TestStore_RebindTaskMRReviewer_ClearsBaselines(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
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
	got, _ := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/a", 1)
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
	t.Cleanup(func() {})
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
	if _, err := NewStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("fresh-DB NewStore: %v", err)
	}

	// Same-DB replay: open a second Store against the same file/handle and
	// confirm createTables (and specifically createMRAutomationTables) is a
	// no-op rather than an error on an existing database.
	if _, err := NewStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("same-DB replay NewStore: %v", err)
	}
}
