package handlers

import (
	"context"
	"encoding/json"
	"testing"

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

	if _, err := h.handleAssignExactTaskProfile(context.Background(), &ws.Message{
		ID: "request-1", Action: ws.ActionMCPAssignExactTaskProfile, Payload: payload,
	}); err != nil {
		t.Fatalf("handle assignment: %v", err)
	}
	if assigner.taskID != "task-1" || assigner.profileID != "profile-1" || assigner.generation != 2 {
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
