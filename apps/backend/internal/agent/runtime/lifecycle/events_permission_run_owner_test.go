package lifecycle

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// A run-owned execution has no task session ID, so its permission request
// must be addressed by run session; an empty subject token matches no wildcard
// subscriber and leaves the agent waiting forever (AC-OFFICE-TASKLESS-001.9).
func TestPublishPermissionRequest_RunOwnedReachesWildcardSubscriber(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	publisher := NewEventPublisher(eventBus, newTestLogger())
	received := make(chan *bus.Event, 1)
	if _, err := eventBus.Subscribe(events.BuildPermissionRequestWildcardSubject(), func(_ context.Context, e *bus.Event) error {
		received <- e
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	execution := &AgentExecution{
		ID:                   "exec-1",
		WorkspaceID:          "ws-1",
		RunID:                "run-1",
		RunSessionID:         "run-session-1",
		RunAttempt:           2,
		OfficeAgentProfileID: "office-agent-1",
		AgentProfileID:       "execution-profile-1",
		Owner: ExecutionOwner{
			Kind: ExecutionOwnerRun, WorkspaceID: "ws-1", RunID: "run-1",
			RunSessionID: "run-session-1", Attempt: 2, AgentProfileID: "office-agent-1",
		},
	}
	if err := publisher.PublishPermissionRequest(execution, agentctl.AgentEvent{
		Type: "permission_request", RequestID: "req-1", PendingID: "pending-1", PermissionTitle: "Run ls",
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	var event *bus.Event
	select {
	case event = <-received:
	case <-time.After(time.Second):
		t.Fatalf("run-owned permission request reached no wildcard subscriber (subject %q)",
			events.BuildPermissionRequestSubject(execution.SessionID))
	}
	payload, ok := event.Data.(PermissionRequestEventPayload)
	if !ok {
		t.Fatalf("payload type = %T, want PermissionRequestEventPayload", event.Data)
	}
	type owner struct {
		kind                                      ExecutionOwnerKind
		workspace, run, runSession, agent, execID string
		attempt                                   int
	}
	got := owner{payload.OwnerKind, payload.WorkspaceID, payload.RunID, payload.RunSessionID,
		payload.AgentProfileID, payload.AgentExecutionID, payload.RunAttempt}
	want := owner{ExecutionOwnerRun, "ws-1", "run-1", "run-session-1", "office-agent-1", "exec-1", 2}
	if got != want {
		t.Fatalf("owner fields = %+v, want %+v", got, want)
	}
}

func TestPublishPermissionRequest_TaskOwnedKeepsSessionSubject(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	publisher := NewEventPublisher(eventBus, newTestLogger())
	received := make(chan *bus.Event, 1)
	if _, err := eventBus.Subscribe(events.BuildPermissionRequestSubject("session-1"), func(_ context.Context, e *bus.Event) error {
		received <- e
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		Owner: ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: "task-1", SessionID: "session-1"},
	}
	if err := publisher.PublishPermissionRequest(execution, agentctl.AgentEvent{
		Type: "permission_request", RequestID: "req-1", PendingID: "pending-1",
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case event := <-received:
		payload := event.Data.(PermissionRequestEventPayload)
		if payload.OwnerKind != ExecutionOwnerTask || payload.RunSessionID != "" {
			t.Fatalf("task-owned payload owner = %q run_session = %q", payload.OwnerKind, payload.RunSessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("task-owned permission request was not published on its session subject")
	}
}
