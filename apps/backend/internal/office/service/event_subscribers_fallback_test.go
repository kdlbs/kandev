package service_test

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
)

// stubTaskWorkspace is a minimal implementation of service.TaskWorkspaceService
// that returns a fixed message from GetLastAgentMessage.
type stubTaskWorkspace struct {
	lastMsg string
}

func (s *stubTaskWorkspace) GetWorkspace(_ context.Context, _ string) (*taskmodels.Workspace, error) {
	return nil, nil
}
func (s *stubTaskWorkspace) ListWorkspaces(_ context.Context) ([]*taskmodels.Workspace, error) {
	return nil, nil
}
func (s *stubTaskWorkspace) DeleteWorkspace(_ context.Context, _ string) error {
	return nil
}
func (s *stubTaskWorkspace) ListTasksByWorkspace(
	_ context.Context, _, _, _, _ string, _, _ int, _, _, _, _ bool,
) ([]*taskmodels.Task, int, error) {
	return nil, 0, nil
}
func (s *stubTaskWorkspace) DeleteTask(_ context.Context, _ string) error { return nil }
func (s *stubTaskWorkspace) GetLastAgentMessage(_ context.Context, _ string) (string, error) {
	return s.lastMsg, nil
}

// newTestServiceWithTaskWorkspace creates a service+bus like newTestServiceWithBus
// but injects the given TaskWorkspace stub.
func newTestServiceWithTaskWorkspace(t *testing.T, tw service.TaskWorkspaceService) (*service.Service, bus.EventBus) {
	t.Helper()
	svc := newTestService(t, service.ServiceOptions{TaskWorkspace: tw})
	svc.SetSyncHandlers(true)
	log := logger.Default()
	eb := bus.NewMemoryEventBus(log)
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	svc.SetWorkflowEngineDispatcher(&queueRunDispatcher{svc: svc})
	return svc, eb
}

// TestAutoPostAgentComment_FallbackPath verifies that when agent_text in the
// event payload is empty the handler falls back to GetLastAgentMessage and
// creates the session comment from the DB-stored text.
func TestAutoPostAgentComment_FallbackPath(t *testing.T) {
	stub := &stubTaskWorkspace{lastMsg: "agent reply from DB"}
	svc, eb := newTestServiceWithTaskWorkspace(t, stub)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-fallback")
	taskID := createOfficeTask(t, svc, "ws-1", "agent-fallback")

	// Publish event with empty agent_text — handler must fall back to GetLastAgentMessage.
	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-fallback",
		"agent_text": "",
		"agent_id":   "agent-fallback",
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	comments, err := svc.ListComments(ctx, taskID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}

	for _, c := range comments {
		if c.Source == "session" && c.AuthorType == "agent" && c.Body == "agent reply from DB" {
			return // fallback comment created correctly
		}
	}
	t.Fatalf("expected session comment with body from GetLastAgentMessage, got %+v", comments)
}
