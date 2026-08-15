package gitlab

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestTaskDeletedEventRemovesTaskMRsAndWatches(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	for _, taskID := range []string{"deleted", "retained"} {
		seedTask(t, store, taskID, "ws")
		if err := store.UpsertTaskMR(ctx, newTestMR(taskID, "", "group/project", 1)); err != nil {
			t.Fatalf("create task MR: %v", err)
		}
		if err := store.CreateMRWatch(ctx, &MRWatch{SessionID: "session-" + taskID, TaskID: taskID, ProjectPath: "group/project", Branch: "main"}); err != nil {
			t.Fatalf("create MR watch: %v", err)
		}
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eventBus := bus.NewMemoryEventBus(log)
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc.SetEventBus(eventBus)
	svc.SetStore(store)
	t.Cleanup(svc.unsubscribeTaskEvents)
	if err := eventBus.Publish(ctx, events.TaskDeleted, bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{"task_id": "deleted"})); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got, _ := store.ListTaskMRsByTask(ctx, "deleted"); len(got) != 0 {
		t.Fatalf("deleted task MRs = %d, want 0", len(got))
	}
	if got, _ := store.ListTaskMRsByTask(ctx, "retained"); len(got) != 1 {
		t.Fatalf("retained task MRs = %d, want 1", len(got))
	}
	if got, _ := store.GetMRWatchBySession(ctx, "session-deleted"); got != nil {
		t.Fatal("deleted task watch survived")
	}
	if got, _ := store.GetMRWatchBySession(ctx, "session-retained"); got == nil {
		t.Fatal("unrelated watch was removed")
	}
}

// TestHandleTaskDeletedEvent_MalformedPayloadIsNoop covers AC3: a nil event, a
// non-map payload, and a payload without a non-empty task_id all leave
// gitlab_task_mrs/gitlab_mr_watches rows untouched.
func TestHandleTaskDeletedEvent_MalformedPayloadIsNoop(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	seedTask(t, store, "t1", "ws")
	if err := store.UpsertTaskMR(ctx, newTestMR("t1", "", "group/project", 1)); err != nil {
		t.Fatalf("create task MR: %v", err)
	}
	if err := store.CreateMRWatch(ctx, &MRWatch{SessionID: "s1", TaskID: "t1", ProjectPath: "group/project", Branch: "main"}); err != nil {
		t.Fatalf("create MR watch: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc.SetStore(store)

	if err := svc.handleTaskDeleted(ctx, nil); err != nil {
		t.Fatalf("nil event: %v", err)
	}
	if err := svc.handleTaskDeleted(ctx, bus.NewEvent(events.TaskDeleted, "task-service", "not-a-map")); err != nil {
		t.Fatalf("non-map payload: %v", err)
	}
	if err := svc.handleTaskDeleted(ctx, bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{})); err != nil {
		t.Fatalf("missing task_id: %v", err)
	}

	if got, _ := store.ListTaskMRsByTask(ctx, "t1"); len(got) != 1 {
		t.Fatalf("task MRs after malformed events = %d, want 1", len(got))
	}
	if got, _ := store.GetMRWatchBySession(ctx, "s1"); got == nil {
		t.Fatal("watch was removed by a malformed event")
	}
}

// TestHandleTaskUpdated_ArchiveRetainsTaskMRs covers AC4: GitLab does not
// subscribe to task.updated, so an archive event never touches task/MR links.
func TestHandleTaskUpdated_ArchiveRetainsTaskMRs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	seedTask(t, store, "t1", "ws")
	if err := store.UpsertTaskMR(ctx, newTestMR("t1", "", "group/project", 1)); err != nil {
		t.Fatalf("create task MR: %v", err)
	}
	if err := store.CreateMRWatch(ctx, &MRWatch{SessionID: "s1", TaskID: "t1", ProjectPath: "group/project", Branch: "main"}); err != nil {
		t.Fatalf("create MR watch: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eventBus := bus.NewMemoryEventBus(log)
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc.SetEventBus(eventBus)
	svc.SetStore(store)
	t.Cleanup(svc.unsubscribeTaskEvents)

	// GitLab only ever subscribes to events.TaskDeleted (see
	// subscribeTaskEvents), so this assertion pins that set: the retention
	// below is a no-op because nothing handles task.updated, not because a
	// handler inspects the event and chooses to retain. If GitLab ever grows
	// a task.updated subscriber, this count changes and the assumption below
	// must be revisited alongside it.
	if got := len(svc.taskEventSubs); got != 1 {
		t.Fatalf("task event subscriptions = %d, want 1 (task.deleted only)", got)
	}

	archiveEvent := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := eventBus.Publish(ctx, events.TaskUpdated, archiveEvent); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got, _ := store.ListTaskMRsByTask(ctx, "t1"); len(got) != 1 {
		t.Fatalf("task MRs after archive event = %d, want 1", len(got))
	}
	if got, _ := store.GetMRWatchBySession(ctx, "s1"); got == nil {
		t.Fatal("watch was removed by an archive event")
	}
}

// TestSubscribeTaskEventsIsIdempotentRegardlessOfSetOrder covers the
// duplicate-subscription risk: SetEventBus/SetStore may be called more than
// once, and in either order, but must only ever register one task.deleted
// subscription.
func TestSubscribeTaskEventsIsIdempotentRegardlessOfSetOrder(t *testing.T) {
	store := newTestStore(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eventBus := bus.NewMemoryEventBus(log)

	svc := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc.SetStore(store)
	svc.SetEventBus(eventBus)
	t.Cleanup(svc.unsubscribeTaskEvents)
	if got := len(svc.taskEventSubs); got != 1 {
		t.Fatalf("subscriptions after store-then-bus = %d, want 1", got)
	}

	svc.SetStore(store)
	svc.SetEventBus(eventBus)
	if got := len(svc.taskEventSubs); got != 1 {
		t.Fatalf("subscriptions after repeat Set calls = %d, want 1", got)
	}

	// Repeat with the opposite setter order: SetEventBus before SetStore.
	svc2 := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc2.SetEventBus(eventBus)
	svc2.SetStore(store)
	t.Cleanup(svc2.unsubscribeTaskEvents)
	if got := len(svc2.taskEventSubs); got != 1 {
		t.Fatalf("subscriptions after bus-then-store = %d, want 1", got)
	}

	svc2.SetEventBus(eventBus)
	svc2.SetStore(store)
	if got := len(svc2.taskEventSubs); got != 1 {
		t.Fatalf("subscriptions after repeat Set calls (bus-then-store) = %d, want 1", got)
	}
}
