package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

func TestStoreRunTransitionsSurviveRecreation(t *testing.T) {
	conn := newSQLite(t)
	pool := db.NewPool(conn, conn)
	store := newStorageStore(t, pool)
	ctx := context.Background()
	run := &MaintenanceRun{
		ID:               "run-1",
		Trigger:          RunTriggerManual,
		State:            RunStateQueued,
		SettingsSnapshot: json.RawMessage(`{"enabled":true}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.TransitionRun(ctx, run.ID, RunStateRunning, nil, ""); err != nil {
		t.Fatalf("transition running: %v", err)
	}

	store = newStorageStore(t, pool)
	got, err := store.TransitionRun(ctx, run.ID, RunStateSucceeded, json.RawMessage(`{"bytes":42}`), "done")
	if err != nil {
		t.Fatalf("transition succeeded after recreation: %v", err)
	}
	if got.State != RunStateSucceeded || got.CompletedAt == nil || got.Message != "done" || string(got.Result) != `{"bytes":42}` {
		t.Fatalf("completed run = %#v", got)
	}
}

func TestStoreRejectsInvalidRunTransitions(t *testing.T) {
	conn := newSQLite(t)
	store := newStorageStore(t, db.NewPool(conn, conn))
	ctx := context.Background()
	if err := store.CreateRun(ctx, &MaintenanceRun{ID: "run-1", Trigger: RunTriggerScheduled, State: RunStateQueued}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.TransitionRun(ctx, "run-1", RunStateSucceeded, nil, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("queued -> succeeded error = %v, want ErrInvalidTransition", err)
	}
}

func TestStoreAllowsEveryRunStateTransition(t *testing.T) {
	tests := []struct {
		name  string
		steps []RunState
	}{
		{name: "busy before running", steps: []RunState{RunStateSkippedBusy}},
		{name: "succeeded", steps: []RunState{RunStateRunning, RunStateSucceeded}},
		{name: "failed", steps: []RunState{RunStateRunning, RunStateFailed}},
		{name: "cancelled", steps: []RunState{RunStateRunning, RunStateCancelled}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newSQLite(t)
			store := newStorageStore(t, db.NewPool(conn, conn))
			id := fmt.Sprintf("run-%d", i)
			if err := store.CreateRun(context.Background(), &MaintenanceRun{
				ID: id, Trigger: RunTriggerScheduled, State: RunStateQueued,
			}); err != nil {
				t.Fatalf("CreateRun: %v", err)
			}
			for _, state := range tt.steps {
				if _, err := store.TransitionRun(context.Background(), id, state, nil, ""); err != nil {
					t.Fatalf("transition to %s: %v", state, err)
				}
			}
		})
	}
}

func TestStoreRunRetentionKeepsNewestTwentyTerminalAndAllNonTerminal(t *testing.T) {
	conn := newSQLite(t)
	store := newStorageStore(t, db.NewPool(conn, conn))
	ctx := context.Background()
	base := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	for i := range 25 {
		run := &MaintenanceRun{
			ID:        fmt.Sprintf("run-%02d", i),
			Trigger:   RunTriggerManual,
			State:     RunStateQueued,
			StartedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun %d: %v", i, err)
		}
		if _, err := store.TransitionRun(ctx, run.ID, RunStateRunning, nil, ""); err != nil {
			t.Fatalf("transition running %d: %v", i, err)
		}
		if _, err := store.TransitionRun(ctx, run.ID, RunStateSucceeded, nil, ""); err != nil {
			t.Fatalf("transition succeeded %d: %v", i, err)
		}
	}
	for _, state := range []RunState{RunStateQueued, RunStateRunning} {
		id := "active-" + string(state)
		if err := store.CreateRun(ctx, &MaintenanceRun{ID: id, Trigger: RunTriggerAnalysis, State: RunStateQueued}); err != nil {
			t.Fatalf("CreateRun %s: %v", id, err)
		}
		if state == RunStateRunning {
			if _, err := store.TransitionRun(ctx, id, state, nil, ""); err != nil {
				t.Fatalf("transition %s: %v", id, err)
			}
		}
	}

	var terminal, nonTerminal int
	if err := conn.Get(&terminal, `SELECT COUNT(*) FROM storage_maintenance_runs WHERE state IN ('succeeded','failed','cancelled','skipped_busy')`); err != nil {
		t.Fatalf("count terminal: %v", err)
	}
	if err := conn.Get(&nonTerminal, `SELECT COUNT(*) FROM storage_maintenance_runs WHERE state IN ('queued','running')`); err != nil {
		t.Fatalf("count non-terminal: %v", err)
	}
	if terminal != 20 || nonTerminal != 2 {
		t.Fatalf("retained terminal=%d non-terminal=%d, want 20 and 2", terminal, nonTerminal)
	}
	if _, err := store.GetRun(ctx, "run-00"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest run error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetRun(ctx, "run-24"); err != nil {
		t.Fatalf("newest run missing: %v", err)
	}
}

func TestStoreQuarantineTransitionsSurviveRecreation(t *testing.T) {
	conn := newSQLite(t)
	pool := db.NewPool(conn, conn)
	store := newStorageStore(t, pool)
	ctx := context.Background()
	entry := &QuarantineEntry{
		ID:             "entry-1",
		ResourceType:   ResourceTypeTaskWorkspace,
		TaskID:         "task-1",
		WorkspaceID:    "workspace-1",
		OriginalPath:   "/tmp/tasks/task-1",
		QuarantinePath: "/tmp/trash/tasks/task-1",
		SizeBytes:      42,
		State:          QuarantineStateQuarantined,
		QuarantinedAt:  time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC),
		DeleteAfter:    time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC),
		Metadata:       json.RawMessage(`{"layout":1}`),
	}
	if err := store.CreateQuarantineEntry(ctx, entry); err != nil {
		t.Fatalf("CreateQuarantineEntry: %v", err)
	}
	failed, err := store.TransitionQuarantineEntry(ctx, entry.ID, QuarantineStateFailed, "permission denied")
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if failed.LastError != "permission denied" {
		t.Fatalf("failed entry last_error = %q", failed.LastError)
	}

	store = newStorageStore(t, pool)
	restored, err := store.TransitionQuarantineEntry(ctx, entry.ID, QuarantineStateRestored, "")
	if err != nil {
		t.Fatalf("transition restored after recreation: %v", err)
	}
	if restored.State != QuarantineStateRestored || restored.RestoredAt == nil || restored.LastError != "" {
		t.Fatalf("restored entry = %#v", restored)
	}
	if _, err := store.TransitionQuarantineEntry(ctx, entry.ID, QuarantineStateDeleted, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("restored -> deleted error = %v, want ErrInvalidTransition", err)
	}
}

func TestStoreQuarantineOriginalPathUniqueWhileActive(t *testing.T) {
	conn := newSQLite(t)
	store := newStorageStore(t, db.NewPool(conn, conn))
	ctx := context.Background()
	first := &QuarantineEntry{
		ID:             "entry-1",
		ResourceType:   ResourceTypeGoCache,
		OriginalPath:   "/tmp/go-build",
		QuarantinePath: "/tmp/trash/go-build-1",
		State:          QuarantineStateQuarantined,
		QuarantinedAt:  time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC),
		DeleteAfter:    time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC),
	}
	if err := store.CreateQuarantineEntry(ctx, first); err != nil {
		t.Fatalf("create first entry: %v", err)
	}
	duplicate := *first
	duplicate.ID = "entry-2"
	duplicate.QuarantinePath = "/tmp/trash/go-build-2"
	if err := store.CreateQuarantineEntry(ctx, &duplicate); err == nil {
		t.Fatal("expected duplicate active original path to fail")
	}
	if _, err := store.TransitionQuarantineEntry(ctx, first.ID, QuarantineStateDeleted, ""); err != nil {
		t.Fatalf("delete first entry: %v", err)
	}
	if err := store.CreateQuarantineEntry(ctx, &duplicate); err != nil {
		t.Fatalf("reuse terminal original path: %v", err)
	}
}

func TestStoreAllowsEveryQuarantineStateTransition(t *testing.T) {
	tests := []struct {
		name  string
		steps []QuarantineState
	}{
		{name: "restored", steps: []QuarantineState{QuarantineStateRestored}},
		{name: "deleted", steps: []QuarantineState{QuarantineStateDeleted}},
		{name: "failed then restored", steps: []QuarantineState{QuarantineStateFailed, QuarantineStateRestored}},
		{name: "failed then deleted", steps: []QuarantineState{QuarantineStateFailed, QuarantineStateDeleted}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newSQLite(t)
			store := newStorageStore(t, db.NewPool(conn, conn))
			entry := testQuarantineEntry(fmt.Sprintf("entry-%d", i))
			if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
				t.Fatalf("CreateQuarantineEntry: %v", err)
			}
			for _, state := range tt.steps {
				if _, err := store.TransitionQuarantineEntry(context.Background(), entry.ID, state, ""); err != nil {
					t.Fatalf("transition to %s: %v", state, err)
				}
			}
		})
	}
}

func TestStoreSummarizeQuarantineCountsOnlyActiveEntries(t *testing.T) {
	conn := newSQLite(t)
	store := newStorageStore(t, db.NewPool(conn, conn))
	ctx := context.Background()

	entries := []QuarantineEntry{
		testQuarantineEntry("quarantined"),
		testQuarantineEntry("failed"),
		testQuarantineEntry("restored"),
		testQuarantineEntry("deleted"),
	}
	for index := range entries {
		entries[index].SizeBytes = int64((index + 1) * 10)
		if err := store.CreateQuarantineEntry(ctx, &entries[index]); err != nil {
			t.Fatalf("CreateQuarantineEntry(%s): %v", entries[index].ID, err)
		}
	}
	if _, err := store.TransitionQuarantineEntry(ctx, "failed", QuarantineStateFailed, "retryable"); err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if _, err := store.TransitionQuarantineEntry(ctx, "restored", QuarantineStateRestored, ""); err != nil {
		t.Fatalf("transition restored: %v", err)
	}
	if _, err := store.TransitionQuarantineEntry(ctx, "deleted", QuarantineStateDeleted, ""); err != nil {
		t.Fatalf("transition deleted: %v", err)
	}

	summary, err := store.SummarizeQuarantine(ctx)
	if err != nil {
		t.Fatalf("SummarizeQuarantine: %v", err)
	}
	if summary.Count != 2 || summary.SizeBytes != 30 {
		t.Fatalf("summary = %#v, want count=2 size_bytes=30", summary)
	}
}

func TestStoreSummarizeQuarantineReturnsZeroWhenEmpty(t *testing.T) {
	conn := newSQLite(t)
	store := newStorageStore(t, db.NewPool(conn, conn))

	summary, err := store.SummarizeQuarantine(context.Background())
	if err != nil {
		t.Fatalf("SummarizeQuarantine: %v", err)
	}
	if summary.Count != 0 || summary.SizeBytes != 0 {
		t.Fatalf("empty summary = %#v", summary)
	}
}

func TestStoreSchemaReplaySQLite(t *testing.T) {
	conn := newSQLite(t)
	pool := db.NewPool(conn, conn)
	newStorageStore(t, pool)
	newStorageStore(t, pool)
}

func TestStoreSchemaReplayPostgres(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	pool := db.NewPool(conn, conn)
	store := newStorageStore(t, pool)
	ctx := context.Background()
	if err := store.CreateRun(ctx, &MaintenanceRun{
		ID: "postgres-run", Trigger: RunTriggerManual, State: RunStateQueued,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.TransitionRun(ctx, "postgres-run", RunStateRunning, nil, ""); err != nil {
		t.Fatalf("transition running: %v", err)
	}
	entry := testQuarantineEntry("postgres-entry")
	if err := store.CreateQuarantineEntry(ctx, &entry); err != nil {
		t.Fatalf("CreateQuarantineEntry: %v", err)
	}

	store = newStorageStore(t, pool)
	if _, err := store.TransitionRun(ctx, "postgres-run", RunStateSucceeded, nil, ""); err != nil {
		t.Fatalf("transition succeeded after replay: %v", err)
	}
	if _, err := store.TransitionQuarantineEntry(ctx, entry.ID, QuarantineStateRestored, ""); err != nil {
		t.Fatalf("transition restored after replay: %v", err)
	}
}

func newStorageStore(t *testing.T, pool *db.Pool) *Store {
	t.Helper()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func testQuarantineEntry(id string) QuarantineEntry {
	return QuarantineEntry{
		ID:             id,
		ResourceType:   ResourceTypeTaskWorkspace,
		OriginalPath:   "/tmp/tasks/" + id,
		QuarantinePath: "/tmp/trash/tasks/" + id,
		State:          QuarantineStateQuarantined,
		QuarantinedAt:  time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC),
		DeleteAfter:    time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC),
	}
}
