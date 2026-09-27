package handlers

import (
	"context"
	"encoding/json"
	"testing"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestHandleAssignExactTaskProfileForwardsGuardedRequest(t *testing.T) {
	assigner := &recordingExactTaskProfileAssigner{}
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger(t))
	h.SetExactTaskProfileAssigner(assigner)
	payload, err := json.Marshal(map[string]interface{}{
		"task_id": "task-1", "agent_profile_id": "profile-1", "generation": 2,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	if _, err := h.handleAssignExactTaskProfile(mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{CallerTaskID: "task-1", CallerSessionID: "session-1"}), &ws.Message{
		ID: "request-1", Action: ws.ActionMCPAssignExactTaskProfile, Payload: payload,
	}); err != nil {
		t.Fatalf("handle assignment: %v", err)
	}
	if assigner.taskID != "task-1" || assigner.profileID != "profile-1" || assigner.generation != 2 {
		t.Fatalf("assignment = (%q, %q, %d)", assigner.taskID, assigner.profileID, assigner.generation)
	}
}

func TestHandleAssignExactTaskProfileForwardsTrustedExternalRequest(t *testing.T) {
	assigner := &recordingExactTaskProfileAssigner{}
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger(t))
	h.SetExactTaskProfileAssigner(assigner)
	payload, err := json.Marshal(map[string]interface{}{
		"task_id": "task-external", "agent_profile_id": "profile-1", "generation": 2,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	response, err := h.handleAssignExactTaskProfile(
		mcporigin.WithTrustedExternalTransport(context.Background()),
		&ws.Message{ID: "request-external", Action: ws.ActionMCPAssignExactTaskProfile, Payload: payload},
	)
	if err != nil {
		t.Fatalf("handle assignment: %v", err)
	}
	if response == nil || response.Type == ws.MessageTypeError {
		t.Fatalf("response = %#v, want assignment response", response)
	}
	if assigner.taskID != "task-external" || assigner.profileID != "profile-1" || assigner.generation != 2 {
		t.Fatalf("assignment = (%q, %q, %d)", assigner.taskID, assigner.profileID, assigner.generation)
	}
}

type recordingExactTaskProfileAssigner struct {
	taskID, profileID string
	generation        int64
}

func (a *recordingExactTaskProfileAssigner) AssignExactTaskProfile(
	_ context.Context, taskID, profileID string, generation int64,
) (*orchestrator.ExactProfileLaunchDecision, error) {
	a.taskID, a.profileID, a.generation = taskID, profileID, generation
	return &orchestrator.ExactProfileLaunchDecision{AgentProfileID: profileID, Generation: generation}, nil
}

func TestHandleAssignExactTaskProfileRejectsForeignCallerTask(t *testing.T) {
	assigner := &recordingExactTaskProfileAssigner{}
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger(t))
	h.SetExactTaskProfileAssigner(assigner)
	payload, err := json.Marshal(map[string]interface{}{
		"task_id": "task-foreign", "agent_profile_id": "profile-1", "generation": 1,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	response, err := h.handleAssignExactTaskProfile(
		mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{CallerTaskID: "task-1", CallerSessionID: "session-1"}),
		&ws.Message{ID: "request-foreign", Action: ws.ActionMCPAssignExactTaskProfile, Payload: payload},
	)
	if err != nil {
		t.Fatalf("handle assignment: %v", err)
	}
	if response == nil || response.Type != ws.MessageTypeError {
		t.Fatalf("response = %#v, want authorization error", response)
	}
	if assigner.taskID != "" {
		t.Fatalf("assigner called for foreign task %q", assigner.taskID)
	}
}
