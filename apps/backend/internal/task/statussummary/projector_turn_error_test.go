package statussummary

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type mutableTurnErrorSource struct {
	mu       sync.Mutex
	value    *TurnErrorSummary
	observed bool
	err      error
}

func (s *mutableTurnErrorSource) load(context.Context, string) (TaskTurnErrorObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return TaskTurnErrorObservation{}, s.err
	}
	if !s.observed {
		return TaskTurnErrorObservation{}, nil
	}
	if s.value == nil {
		return TaskTurnErrorObservation{Observed: true}, nil
	}
	copy := *s.value
	return TaskTurnErrorObservation{Observed: true, Error: &copy}, nil
}

func (s *mutableTurnErrorSource) set(value *TurnErrorSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observed = true
	s.value = value
}

func turnErrorSummary(stamp, sessionID string, occurredAt time.Time) *TurnErrorSummary {
	return &TurnErrorSummary{
		SessionID: sessionID, TurnID: "turn-" + stamp, MessageID: stamp,
		Stamp: stamp, OccurredAt: occurredAt, Preview: "Failed to execute statement",
	}
}

func TestTaskTurnErrorAttentionProjection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	t.Cleanup(eventBus.Close)
	source := &mutableTurnErrorSource{}
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadTaskTurnError: source.load,
		Now:               func() time.Time { return time.Date(2026, 10, 29, 20, 33, 0, 0, time.UTC) },
	})
	if err := projector.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(projector.Close)

	occurredAt := time.Date(2026, 10, 29, 20, 32, 55, 0, time.UTC)
	source.set(turnErrorSummary("error-1", "session-a", occurredAt))
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": "turn-error-task", "workspace_id": "workspace-1",
	})
	summary := store.summary("turn-error-task")
	if summary == nil || summary.TurnError == nil {
		t.Fatalf("summary = %+v, want turn_error attention", summary)
	}
	if summary.TurnError.Stamp != "error-1" || summary.TurnError.SessionID != "session-a" {
		t.Fatalf("turn error = %+v, want the observed attention", summary.TurnError)
	}
	if summary.ActiveError != nil {
		t.Fatalf("active_error = %+v, want turn error to stay out of blocking recovery", summary.ActiveError)
	}
	firstRevision := summary.Revision

	source.set(turnErrorSummary("error-2", "session-b", occurredAt.Add(time.Second)))
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": "turn-error-task", "workspace_id": "workspace-1",
	})
	summary = store.summary("turn-error-task")
	if summary.TurnError == nil || summary.TurnError.Stamp != "error-2" {
		t.Fatalf("turn error = %+v, want the newer sibling failure", summary.TurnError)
	}
	if summary.Revision <= firstRevision {
		t.Fatalf("revision = %d, want > %d after a semantic change", summary.Revision, firstRevision)
	}

	source.set(nil)
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": "turn-error-task", "workspace_id": "workspace-1",
	})
	summary = store.summary("turn-error-task")
	if summary.TurnError != nil {
		t.Fatalf("turn error = %+v, want the acknowledged pointer cleared", summary.TurnError)
	}

	// A restarted projector derives the same empty attention from the durable
	// source instead of trusting a stale in-memory copy.
	projector.Close()
	restarted := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadTaskTurnError: source.load,
	})
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": "turn-error-task", "workspace_id": "workspace-1",
	})
	summary = store.summary("turn-error-task")
	if summary.TurnError != nil {
		t.Fatalf("restarted projector turn error = %+v, want still empty", summary.TurnError)
	}

	source.set(turnErrorSummary("error-3", "session-a", occurredAt.Add(time.Minute)))
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": "turn-error-task", "workspace_id": "workspace-1",
	})
	summary = store.summary("turn-error-task")
	if summary.TurnError == nil || summary.TurnError.Stamp != "error-3" {
		t.Fatalf("restarted projector turn error = %+v, want the new failure", summary.TurnError)
	}
}

func TestTaskTurnErrorSummaryIsNotBlockingRecovery(t *testing.T) {
	value := turnErrorSummary("error-1", "session-a", time.Date(2026, 10, 29, 20, 32, 0, 0, time.UTC))
	summary := TaskStatusSummary{TurnError: value}
	if err := summary.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if summary.ActiveError != nil {
		t.Fatal("turn attention must not populate active_error")
	}
	withoutTurnError := TaskStatusSummary{}
	if summary.SemanticEqual(withoutTurnError) {
		t.Fatal("turn attention must participate in semantic equality")
	}
}

type rejectingOnceTurnErrorStore struct {
	*projectorTestStore
	rejected bool
}

func (s *rejectingOnceTurnErrorStore) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *StoredTaskStatusSummary,
) (bool, error) {
	if !s.rejected {
		s.rejected = true
		return false, nil
	}
	return s.projectorTestStore.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

func TestTaskTurnErrorAttentionSurvivesRejectedWriteRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	baseStore := newProjectorTestStore()
	store := &rejectingOnceTurnErrorStore{projectorTestStore: baseStore}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	t.Cleanup(eventBus.Close)
	source := &mutableTurnErrorSource{}
	source.set(turnErrorSummary("error-rejected", "session-a",
		time.Date(2026, 10, 29, 20, 32, 55, 0, time.UTC)))
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadTaskTurnError: source.load,
		RetryBackoff:      func(context.Context, int) error { return nil },
		Now:               func() time.Time { return time.Date(2026, 10, 29, 20, 33, 0, 0, time.UTC) },
	})
	if err := projector.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(projector.Close)

	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": "turn-error-rejected", "workspace_id": "workspace-1",
	})
	summary := baseStore.summary("turn-error-rejected")
	if summary == nil || summary.TurnError == nil || summary.TurnError.Stamp != "error-rejected" {
		t.Fatalf("summary turn error = %+v, want attention after a rejected write retry", summary)
	}
}
