package statussummary

import (
	"context"
	"fmt"
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

func TestProjectorQueueEventTracksUserPromptActivity(t *testing.T) {
	projector, store, eventBus, updates, _ := newProjectorTest(t)
	const taskID = "task-queued-activity"
	queuedAt := time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC)

	projector.countQueuedPrompts = func(context.Context, string) (int, error) {
		return 1, nil
	}
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":   taskID,
		"queued_by": "user-1",
		"queued_at": queuedAt,
	})

	summary := store.summary(taskID)
	if summary == nil || summary.LastActivityAt == nil || !summary.LastActivityAt.Equal(queuedAt) {
		t.Fatalf("queued user activity = %+v, want %s", summary, queuedAt)
	}
	if got := updates.Load(); got != 1 {
		t.Fatalf("queued user admission published %d summary updates, want 1", got)
	}

	// Queue status events without a user-owned admission must remain count-only
	// bookkeeping, even when they carry a newer timestamp-shaped value.
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":   taskID,
		"queued_by": "agent",
		"queued_at": queuedAt.Add(time.Hour),
	})
	if got := store.summary(taskID).LastActivityAt; got == nil || !got.Equal(queuedAt) {
		t.Fatalf("agent queue activity changed last activity to %v, want %s", got, queuedAt)
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
	base         *projectorTestStore
	competingGit *GitSummary
	rejected     bool
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
			competing.Git = s.competingGit
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
	store := &competingWriterStore{
		base:         newProjectorTestStore(),
		competingGit: &GitSummary{ChangedFiles: 9},
	}
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
	if summary.Git == nil || summary.Git.ChangedFiles != 9 {
		t.Fatalf("Git summary = %+v, want competing writer's observation preserved", summary.Git)
	}
}

func TestProjectorQueueEventForMissingTaskIsNoop(t *testing.T) {
	const taskID = "task-deleted-queue"
	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	updates := new(atomic.Int64)
	if _, err := eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(_ context.Context, event *bus.Event) error {
		updates.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var counterCalls atomic.Int32
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		// Simulate DeleteTask: the task row is already gone when purge
		// publishes message.queue.status_changed.
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "", fmt.Errorf("task %q not found", taskID)
		},
		CountQueuedPrompts: func(context.Context, string) (int, error) {
			counterCalls.Add(1)
			return 0, nil
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

	// Direct handleEvent call: Publish swallows subscriber errors, so we need
	// the return value to prove the missing-task path is a quiet no-op.
	err := projector.handleEvent(ctx, bus.NewEvent(events.MessageQueueStatusChanged, "test", map[string]interface{}{
		"task_id": taskID,
	}))
	if err != nil {
		t.Fatalf("queue status for missing task returned error: %v", err)
	}
	if counterCalls.Load() != 0 {
		t.Fatalf("counter calls = %d, want 0 (resolve fails before count)", counterCalls.Load())
	}
	if got := updates.Load(); got != 0 {
		t.Fatalf("summary publishes = %d, want 0", got)
	}
	if len(store.rows) != 0 {
		t.Fatalf("missing-task queue event wrote summaries: %d rows", len(store.rows))
	}
	// ensureState may insert a placeholder before resolve fails; drop it so
	// deleted tasks do not retain projectionState for the process lifetime.
	projector.mu.Lock()
	_, retained := projector.state[taskID]
	projector.mu.Unlock()
	if retained {
		t.Fatal("missing-task queue event retained projection state")
	}
}

func TestProjectorQueueEventPropagatesTransientResolveFailure(t *testing.T) {
	const taskID = "task-resolve-transient"
	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	ctx, cancel := context.WithCancel(context.Background())
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "", fmt.Errorf("database is locked")
		},
		CountQueuedPrompts: func(context.Context, string) (int, error) {
			return 0, nil
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

	err := projector.handleEvent(ctx, bus.NewEvent(events.MessageQueueStatusChanged, "test", map[string]interface{}{
		"task_id": taskID,
	}))
	if err == nil {
		t.Fatal("expected transient resolve failure to propagate")
	}
}

// failOnInsertStore rejects every write so a warm projector cannot recreate a
// summary after the task/summary FK cascade on delete. err overrides the
// default FK failure when testing transient persist errors.
type failOnInsertStore struct {
	base *projectorTestStore
	err  error
}

func (s *failOnInsertStore) LoadTaskStatusSummaries(
	ctx context.Context,
	taskIDs []string,
) (map[string]*TaskStatusSummary, error) {
	return s.base.LoadTaskStatusSummaries(ctx, taskIDs)
}

func (s *failOnInsertStore) CompareAndUpdateTaskStatusSummary(
	_ context.Context,
	_ *StoredTaskStatusSummary,
) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return false, fmt.Errorf("FOREIGN KEY constraint failed")
}

func TestProjectorQueueEventZeroCountToleratesGoneTaskPersistFailure(t *testing.T) {
	const taskID = "task-warm-deleted-queue"
	// Warm in-process state: a prior projection already knows the workspace and
	// a stale non-zero count. After delete the FK cascade removes the row; the
	// next queue-status recount sees pending=0 and must not ERROR on persist.
	store := &failOnInsertStore{base: newProjectorTestStore()}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	ctx, cancel := context.WithCancel(context.Background())
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		CountQueuedPrompts: func(context.Context, string) (int, error) {
			return 0, nil
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

	unlock := projector.lockTask(taskID)
	state, err := projector.ensureState(ctx, taskID)
	if err != nil {
		unlock()
		t.Fatalf("ensureState: %v", err)
	}
	state.workspaceID = "workspace-1"
	state.queuedCount = 11
	state.revision = 4
	unlock()

	err = projector.handleEvent(ctx, bus.NewEvent(events.MessageQueueStatusChanged, "test", map[string]interface{}{
		"task_id": taskID,
	}))
	if err != nil {
		t.Fatalf("zero-count queue status with gone-task persist failure returned error: %v", err)
	}
}

func TestProjectorQueueEventZeroCountDoesNotPoisonStateOnTransientPersistFailure(t *testing.T) {
	const taskID = "task-zero-transient"
	store := &failOnInsertStore{
		base: newProjectorTestStore(),
		err:  fmt.Errorf("database is locked"),
	}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	ctx, cancel := context.WithCancel(context.Background())
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		CountQueuedPrompts: func(context.Context, string) (int, error) {
			return 0, nil
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

	unlock := projector.lockTask(taskID)
	state, err := projector.ensureState(ctx, taskID)
	if err != nil {
		unlock()
		t.Fatalf("ensureState: %v", err)
	}
	state.workspaceID = "workspace-1"
	state.queuedCount = 11
	state.revision = 4
	unlock()

	err = projector.handleEvent(ctx, bus.NewEvent(events.MessageQueueStatusChanged, "test", map[string]interface{}{
		"task_id": taskID,
	}))
	if err == nil {
		t.Fatal("expected transient persist error to propagate")
	}

	unlock = projector.lockTask(taskID)
	if state.queuedCount != 11 {
		unlock()
		t.Fatalf("queuedCount poisoned to %d, want previous 11 so a later event can retry", state.queuedCount)
	}
	unlock()
}
