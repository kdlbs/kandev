package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// mockEventBus captures published events for assertion.
type mockEventBus struct {
	mu     sync.Mutex
	events []publishedEvent
}

func TestUpdateTransitionTaskWithCapacity_RejectsFullLimitedStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "occupant",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step2",
		Title:          "Occupant",
		State:          "TODO",
		Priority:       "medium",
	}); err != nil {
		t.Fatalf("create occupant: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step2"
	target := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", WIPLimit: 1}

	err = svc.updateTransitionTaskWithCapacity(ctx, task, target)
	if !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("error=%v, want typed WIP rejection", err)
	}
	stored, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if stored.WorkflowStepID != "step1" {
		t.Fatalf("task moved despite full target, got %q", stored.WorkflowStepID)
	}
}

func TestExecuteStepTransition_FullTargetLeavesOnExitStateIntact(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.SetSessionMetadataKey(ctx, "s1", "plan_mode", true); err != nil {
		t.Fatalf("enable plan mode: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "occupant",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step2",
		Title:          "Occupant",
		State:          "TODO",
		Priority:       "medium",
	}); err != nil {
		t.Fatalf("create occupant: %v", err)
	}

	steps := newMockStepGetter()
	fromStep := &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{
			Type: wfmodels.OnExitDisablePlanMode,
		}}},
	}
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Limited", WIPLimit: 1,
	}
	svc := createTestService(repo, steps, newMockTaskRepo())

	svc.executeStepTransition(ctx, "t1", "s1", fromStep, "step2", false)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("task moved despite full target, got %q", task.WorkflowStepID)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if enabled, _ := session.Metadata["plan_mode"].(bool); !enabled {
		t.Fatal("capacity rejection must not run source step's on_exit actions")
	}
}

type publishedEvent struct {
	Subject string
	Event   *bus.Event
}

func (m *mockEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, publishedEvent{Subject: subject, Event: event})
	return nil
}

func (m *mockEventBus) Subscribe(_ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) QueueSubscribe(_, _ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) Request(_ context.Context, _ string, _ *bus.Event, _ time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (m *mockEventBus) Close()            {}
func (m *mockEventBus) IsConnected() bool { return true }

func (m *mockEventBus) published() []publishedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]publishedEvent, len(m.events))
	copy(out, m.events)
	return out
}

func TestPublishSessionWaitingEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("includes agent_profile_id and metadata", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Update session with agent profile and metadata.
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		session.AgentProfileID = "profile-auggie"
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]any{"plan_mode": true})
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("failed to update session: %v", err)
		}

		eb := &mockEventBus{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.eventBus = eb

		svc.publishSessionWaitingEvent(ctx, "t1", "s1", "step1")

		published := eb.published()
		if len(published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(published))
		}
		if published[0].Subject != events.TaskSessionStateChanged {
			t.Errorf("expected subject %q, got %q", events.TaskSessionStateChanged, published[0].Subject)
		}

		data, ok := published[0].Event.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected event data to be map[string]any, got %T", published[0].Event.Data)
		}
		if data["task_id"] != "t1" {
			t.Errorf("expected task_id %q, got %q", "t1", data["task_id"])
		}
		if data["session_id"] != "s1" {
			t.Errorf("expected session_id %q, got %q", "s1", data["session_id"])
		}
		if data["new_state"] != string(models.TaskSessionStateWaitingForInput) {
			t.Errorf("expected new_state %q, got %q", models.TaskSessionStateWaitingForInput, data["new_state"])
		}
		session, err = repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if data["updated_at"] != session.UpdatedAt.UTC().Format(time.RFC3339Nano) {
			t.Errorf("expected updated_at %q, got %q", session.UpdatedAt.UTC().Format(time.RFC3339Nano), data["updated_at"])
		}
		if data["agent_profile_id"] != "profile-auggie" {
			t.Errorf("expected agent_profile_id %q, got %v", "profile-auggie", data["agent_profile_id"])
		}
		if data["session_metadata"] == nil {
			t.Error("expected session_metadata to be set")
		}
	})

	t.Run("omits agent_profile_id when empty", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		eb := &mockEventBus{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.eventBus = eb

		svc.publishSessionWaitingEvent(ctx, "t1", "s1", "step1")

		published := eb.published()
		if len(published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(published))
		}

		data := published[0].Event.Data.(map[string]any)
		if _, exists := data["agent_profile_id"]; exists {
			t.Errorf("expected agent_profile_id to be absent, got %v", data["agent_profile_id"])
		}
		if _, exists := data["session_metadata"]; exists {
			t.Errorf("expected session_metadata to be absent, got %v", data["session_metadata"])
		}
	})

	t.Run("no-op when eventBus is nil", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// eventBus is nil by default

		// Should not panic.
		svc.publishSessionWaitingEvent(ctx, "t1", "s1", "step1")
	})
}

func TestPublishSessionCreatedEventIncludesUpdatedAt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	eb := &mockEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	svc.publishSessionCreatedEvent(ctx, "t1", "s1", "step1")

	published := eb.published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(published))
	}
	data, ok := published[0].Event.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected event data to be map[string]any, got %T", published[0].Event.Data)
	}
	if data["new_state"] != string(models.TaskSessionStateCreated) {
		t.Errorf("expected new_state %q, got %q", models.TaskSessionStateCreated, data["new_state"])
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if data["updated_at"] != session.UpdatedAt.UTC().Format(time.RFC3339Nano) {
		t.Errorf("expected updated_at %q, got %q", session.UpdatedAt.UTC().Format(time.RFC3339Nano), data["updated_at"])
	}
}
