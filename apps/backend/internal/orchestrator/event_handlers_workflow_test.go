package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// mockEventBus captures published events for assertion.
type mockEventBus struct {
	mu     sync.Mutex
	events []publishedEvent
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
