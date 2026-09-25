package scheduler

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestReactToAssigneeChange_AgentActorPayloadCarriesActorTypeAgent is the
// positive counterpart to the 4 out-of-scope producer tests
// (internal/office/service, internal/office/onboarding,
// internal/orchestrator): reactToAssigneeChange is the only
// task_assigned producer whose payload-building type (RunContext) has an
// ActorType field, and this confirms the field actually reaches the
// JSON payload end to end through encodeRunContext — the exact shape
// classifyAssignmentWake's predicate looks for.
func TestReactToAssigneeChange_AgentActorPayloadCarriesActorTypeAgent(t *testing.T) {
	ss := &SchedulerService{logger: logger.Default()}
	task := &TaskSnapshot{
		ID:                     "task-inscope",
		WorkspaceID:            "ws-1",
		State:                  "TODO",
		AssigneeAgentProfileID: "",
	}
	change := TaskMutation{
		ActorID:   "acting-agent-1",
		ActorType: "agent",
	}

	var calls []recordedQueueCall
	queue := func(agentID string, c RunContext) {
		calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
	}

	ss.reactToAssigneeChange(task, "new-assignee", change, queue, &ApplyTaskMutationResult{})

	if len(calls) != 1 {
		t.Fatalf("got %d queue calls, want 1", len(calls))
	}
	if calls[0].ctx.ActorType != "agent" {
		t.Fatalf("RunContext.ActorType = %q, want %q", calls[0].ctx.ActorType, "agent")
	}

	payload, err := encodeRunContext(calls[0].ctx)
	if err != nil {
		t.Fatalf("encodeRunContext: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded["actor_type"] != "agent" {
		t.Fatalf("payload actor_type = %v, want \"agent\": %s", decoded["actor_type"], payload)
	}

	pred := classifyAssignmentWake(RunReasonTaskAssigned, payload)
	if !pred.inScope || !pred.taskAttributed {
		t.Fatalf("classifyAssignmentWake(%s) = %+v, want in-scope and attributed", payload, pred)
	}
	if pred.taskID != task.ID {
		t.Fatalf("predicate taskID = %q, want %q", pred.taskID, task.ID)
	}
}

// TestReactToAssigneeChange_UserActorPayloadNeverCarriesAgent pins the
// negative half: a user-initiated assignment produces a payload the
// predicate classifies out of scope.
func TestReactToAssigneeChange_UserActorPayloadNeverCarriesAgent(t *testing.T) {
	ss := &SchedulerService{logger: logger.Default()}
	task := &TaskSnapshot{ID: "task-inscope-2", WorkspaceID: "ws-1", State: "TODO"}
	change := TaskMutation{ActorID: "", ActorType: "user"}

	var calls []recordedQueueCall
	queue := func(agentID string, c RunContext) {
		calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
	}

	ss.reactToAssigneeChange(task, "new-assignee", change, queue, &ApplyTaskMutationResult{})

	if len(calls) != 1 {
		t.Fatalf("got %d queue calls, want 1", len(calls))
	}
	payload, err := encodeRunContext(calls[0].ctx)
	if err != nil {
		t.Fatalf("encodeRunContext: %v", err)
	}
	pred := classifyAssignmentWake(RunReasonTaskAssigned, payload)
	if pred.inScope {
		t.Fatalf("classifyAssignmentWake(%s) = %+v, want out of scope for a user actor", payload, pred)
	}
}
