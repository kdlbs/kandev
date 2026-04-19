package github

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestListActivePRWatches_ExcludesArchivedTasks(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t-active", false)
	seedTask(t, store, "t-archived", true)

	mustCreateWatch(t, store, "s-active", "t-active")
	mustCreateWatch(t, store, "s-archived", "t-archived")

	watches, err := svc.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("expected 1 watch (active only), got %d", len(watches))
	}
	if watches[0].TaskID != "t-active" {
		t.Errorf("expected watch for t-active, got %q", watches[0].TaskID)
	}
}

func TestListActivePRWatches_ExcludesOrphanedWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Orphan: watch exists but no matching task row (task was hard-deleted).
	mustCreateWatch(t, store, "s-orphan", "t-gone")

	watches, err := svc.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 0 {
		t.Fatalf("expected orphaned watch to be excluded, got %d watches", len(watches))
	}
}

func TestHandleTaskUpdated_ArchiveDeletesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Simulate task-service publishing task.updated with archived_at set.
	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := svc.handleTaskUpdated(ctx, event); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted after archive event, got %+v", got)
	}
}

func TestHandleTaskUpdated_NonArchiveUpdateLeavesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Regular edit: no archived_at in payload.
	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id": "t1",
		"title":   "Edited title",
	})
	if err := svc.handleTaskUpdated(ctx, event); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got == nil {
		t.Error("expected watch to persist after non-archive update")
	}
}

func TestHandleTaskDeleted_DeletesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{
		"task_id": "t1",
	})
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted after delete event, got %+v", got)
	}
}

func TestHandleTaskEvents_MalformedPayloadIsNoop(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Wrong payload type — should be ignored, not crash.
	bad := bus.NewEvent(events.TaskUpdated, "task-service", "not-a-map")
	if err := svc.handleTaskUpdated(ctx, bad); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	// Missing task_id — should be ignored.
	empty := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{})
	if err := svc.handleTaskDeleted(ctx, empty); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got == nil {
		t.Error("expected watch to persist when payload is malformed")
	}
}

func TestSubscribeTaskEvents_EndToEnd(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	svc.subscribeTaskEvents()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := svc.eventBus.Publish(ctx, events.TaskUpdated, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// MemoryEventBus delivers synchronously, so the watch should already be gone.
	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted by subscribed handler, got %+v", got)
	}
}

// mustCreateWatch is a test helper for creating a PR watch with minimal boilerplate.
func mustCreateWatch(t *testing.T, store *Store, sessionID, taskID string) {
	t.Helper()
	w := &PRWatch{
		SessionID: sessionID,
		TaskID:    taskID,
		Owner:     "owner",
		Repo:      "repo",
		PRNumber:  0,
		Branch:    "main",
	}
	if err := store.CreatePRWatch(context.Background(), w); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
}
