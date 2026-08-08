package statussummary

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestProjectorQueueEventUpdatesQueuedPromptCount(t *testing.T) {
	projector, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-queued-count"

	publishSessionState(t, eventBus, taskID, "session-1", nil)

	projector.countQueuedPrompts = func(_ context.Context, id string) (int, error) {
		if id != taskID {
			return 0, nil
		}
		return 3, nil
	}

	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})

	summary := store.summary(taskID)
	if summary == nil {
		t.Fatal("expected a summary after the queue event")
	}
	if summary.QueuedPromptCount != 3 {
		t.Fatalf("queued prompt count = %d, want 3", summary.QueuedPromptCount)
	}
}

func TestProjectorQueueEventWithUnchangedCountDoesNotRepublish(t *testing.T) {
	projector, store, eventBus, updates, _ := newProjectorTest(t)
	const taskID = "task-queued-unchanged"

	publishSessionState(t, eventBus, taskID, "session-1", nil)
	baseline := updates.Load()

	projector.countQueuedPrompts = func(_ context.Context, id string) (int, error) {
		return 2, nil
	}
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})
	first := updates.Load()
	if first == baseline {
		t.Fatalf("first queue event should have published a summary update (baseline %d)", baseline)
	}

	// Same count again: the projector must not bump the revision or republish.
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})
	if got := updates.Load(); got != first {
		t.Fatalf("unchanged queued count republished: updates %d -> %d", first, got)
	}
	if summary := store.summary(taskID); summary.QueuedPromptCount != 2 {
		t.Fatalf("queued prompt count = %d, want 2", summary.QueuedPromptCount)
	}
}

func TestProjectorRestoresQueuedPromptCountFromPersistedSummary(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-queued-restore"

	// Seed a persisted summary carrying a queued count from a previous run.
	store.rows[taskID] = &StoredTaskStatusSummary{
		TaskID:      taskID,
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:          7,
			QueuedPromptCount: 5,
		},
	}

	publishSessionState(t, eventBus, taskID, "session-1", nil)

	summary := store.summary(taskID)
	if summary == nil {
		t.Fatal("expected a summary")
	}
	if summary.QueuedPromptCount != 5 {
		t.Fatalf("restored queued prompt count = %d, want 5", summary.QueuedPromptCount)
	}
}

func TestProjectorQueueEventWithoutTaskIDIsIgnored(t *testing.T) {
	projector, store, eventBus, updates, _ := newProjectorTest(t)

	calls := 0
	projector.countQueuedPrompts = func(_ context.Context, id string) (int, error) {
		calls++
		return 1, nil
	}

	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"session_id": "session-1",
	})

	if calls != 0 {
		t.Fatalf("counter calls = %d, want 0 for an event without task_id", calls)
	}
	if got := updates.Load(); got != 0 {
		t.Fatalf("summary publishes = %d, want 0", got)
	}
	if len(store.rows) != 0 {
		t.Fatalf("queue event without task_id persisted summaries: %d rows", len(store.rows))
	}
}

// competingWriterStore rejects the projector's first compare-and-update by
// landing a higher revision carrying the STALE queued count (simulating
// another writer persisting between the projector's count query and its
// write), then accepts subsequent writes.
type competingWriterStore struct {
	base     *projectorTestStore
	rejected bool
}

func (s *competingWriterStore) LoadTaskStatusSummaries(
	ctx context.Context,
	taskIDs []string,
) (map[string]*TaskStatusSummary, error) {
	return s.base.LoadTaskStatusSummaries(ctx, taskIDs)
}

func (s *competingWriterStore) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *StoredTaskStatusSummary,
) (bool, error) {
	if !s.rejected {
		s.rejected = true
		rows, _ := s.base.LoadTaskStatusSummaries(ctx, []string{stored.TaskID})
		if row := rows[stored.TaskID]; row != nil {
			competing := *row
			competing.Revision = stored.Summary.Revision + 1 // beat the projector's attempt
			_, _ = s.base.CompareAndUpdateTaskStatusSummary(ctx, &StoredTaskStatusSummary{
				TaskID:      stored.TaskID,
				WorkspaceID: stored.WorkspaceID,
				Summary:     competing,
			})
		}
	}
	return s.base.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

func TestProjectorQueueEventRetriesAfterRejectedWrite(t *testing.T) {
	const taskID = "task-queued-retry"
	store := &competingWriterStore{base: newProjectorTestStore()}
	store.base.rows[taskID] = &StoredTaskStatusSummary{
		TaskID:      taskID,
		WorkspaceID: "workspace-1",
		Summary:     TaskStatusSummary{Revision: 5, QueuedPromptCount: 0},
	}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	updates := new(atomic.Int64)
	if _, err := eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(_ context.Context, event *bus.Event) error {
		updates.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		CountQueuedPrompts: func(context.Context, string) (int, error) {
			return 3, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC) },
	})
	if err := projector.Start(ctx); err != nil {
		cancel()
		eventBus.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		projector.Close()
		eventBus.Close()
	})

	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})

	summary := store.base.summary(taskID)
	if summary == nil || summary.QueuedPromptCount != 3 {
		t.Fatalf("stored queued prompt count = %+v, want 3 after rejected-write retry", summary)
	}
	if summary.Revision != 8 {
		t.Fatalf("revision = %d, want 8 (5 -> competing 7 -> accepted 8)", summary.Revision)
	}
	if got := updates.Load(); got != 1 {
		t.Fatalf("publishes = %d, want exactly 1 (the retried write)", got)
	}
}
