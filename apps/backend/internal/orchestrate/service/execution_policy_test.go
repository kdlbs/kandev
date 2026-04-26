package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func newTestPolicy(stages ...service.ExecutionStage) service.ExecutionPolicy {
	return service.ExecutionPolicy{Stages: stages}
}

func reviewStage(id string, participants []service.ExecutionParticipant, needed int) service.ExecutionStage {
	return service.ExecutionStage{
		ID:              id,
		Type:            "review",
		Participants:    participants,
		ApprovalsNeeded: needed,
	}
}

func approvalStage(id string, participants []service.ExecutionParticipant, needed int) service.ExecutionStage {
	return service.ExecutionStage{
		ID:              id,
		Type:            "approval",
		Participants:    participants,
		ApprovalsNeeded: needed,
	}
}

func agentParticipant(id string) service.ExecutionParticipant {
	return service.ExecutionParticipant{Type: "agent", AgentID: id}
}

// createOrchestrateTask creates a task row and sets the assignee.
func createOrchestrateTask(t *testing.T, svc *service.Service, wsID, assigneeID string) string {
	t.Helper()
	ctx := context.Background()

	// Create an assistant agent for channel setup (channels create tasks).
	agent := &models.AgentInstance{
		WorkspaceID: wsID,
		Name:        "ch-" + t.Name(),
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create channel agent: %v", err)
	}

	channel := &models.Channel{
		WorkspaceID:     wsID,
		AgentInstanceID: agent.ID,
		Platform:        "webhook",
		Config:          `{}`,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	// Set assignee on the newly created task.
	if err := svc.SetTaskAssignee(ctx, channel.TaskID, assigneeID); err != nil {
		t.Fatalf("set assignee: %v", err)
	}
	return channel.TaskID
}

// setPolicyOnTask writes the execution_policy JSON to the task row.
func setPolicyOnTask(t *testing.T, svc *service.Service, taskID string, policy service.ExecutionPolicy) {
	t.Helper()
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	if err := svc.SetTaskExecutionPolicy(context.Background(), taskID, string(raw)); err != nil {
		t.Fatalf("set execution policy: %v", err)
	}
}

// --- Parse / Serialize unit tests ---

func TestParseExecutionPolicy_Empty(t *testing.T) {
	policy, err := service.ParseExecutionPolicy("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy != nil {
		t.Error("expected nil policy for empty string")
	}
}

func TestParseExecutionPolicy_EmptyObject(t *testing.T) {
	policy, err := service.ParseExecutionPolicy("{}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy != nil {
		t.Error("expected nil policy for empty object")
	}
}

func TestParseExecutionPolicy_Valid(t *testing.T) {
	raw := `{"stages":[{"id":"s1","type":"review","participants":[{"type":"agent","agent_id":"a1"}],"approvals_needed":1}]}`
	policy, err := service.ParseExecutionPolicy(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}
	if len(policy.Stages) != 1 {
		t.Errorf("stages = %d, want 1", len(policy.Stages))
	}
	if policy.Stages[0].Type != "review" {
		t.Errorf("type = %q, want review", policy.Stages[0].Type)
	}
}

func TestSerializeExecutionState_Roundtrip(t *testing.T) {
	state := &service.ExecutionState{
		CurrentStageIndex: 0,
		Responses:         map[string]*service.StageResponse{},
		Status:            "pending",
	}
	s, err := service.SerializeExecutionState(state)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if s == "" {
		t.Error("expected non-empty serialized state")
	}

	var decoded service.ExecutionState
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Status != "pending" {
		t.Errorf("status = %q, want pending", decoded.Status)
	}
}

// --- Integration tests ---

func TestEnterReviewStage_WakesReviewers(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "sec-agent")
	createTestAgent(t, svc, "ws-1", "qa-agent")

	taskID := createOrchestrateTask(t, svc, "ws-1", "sec-agent")

	policy := newTestPolicy(
		reviewStage("s1", []service.ExecutionParticipant{
			agentParticipant("sec-agent"),
			agentParticipant("qa-agent"),
		}, 2),
	)

	if err := svc.EnterReviewStage(ctx, taskID, policy); err != nil {
		t.Fatalf("EnterReviewStage: %v", err)
	}

	wakeups, err := svc.ListWakeupRequests(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) < 2 {
		t.Errorf("expected at least 2 wakeups for reviewers, got %d", len(wakeups))
	}
}

func TestRecordResponse_AllApprove_AdvancesStage(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "sec-agent")
	createTestAgent(t, svc, "ws-1", "qa-agent")
	createTestAgent(t, svc, "ws-1", "approver-agent")

	taskID := createOrchestrateTask(t, svc, "ws-1", "sec-agent")

	policy := newTestPolicy(
		reviewStage("s1", []service.ExecutionParticipant{
			agentParticipant("sec-agent"),
			agentParticipant("qa-agent"),
		}, 2),
		approvalStage("s2", []service.ExecutionParticipant{
			agentParticipant("approver-agent"),
		}, 1),
	)
	setPolicyOnTask(t, svc, taskID, policy)
	if err := svc.EnterReviewStage(ctx, taskID, policy); err != nil {
		t.Fatalf("EnterReviewStage: %v", err)
	}

	if err := svc.RecordParticipantResponse(ctx, taskID, "sec-agent", "approve", "LGTM"); err != nil {
		t.Fatalf("record sec-agent: %v", err)
	}
	if err := svc.RecordParticipantResponse(ctx, taskID, "qa-agent", "approve", "Good to go"); err != nil {
		t.Fatalf("record qa-agent: %v", err)
	}

	// approver-agent should be woken after review stage passes.
	wakeups, _ := svc.ListWakeupRequests(ctx, "ws-1")
	found := false
	for _, w := range wakeups {
		if w.AgentInstanceID == "approver-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected approver-agent wakeup after review stage passed")
	}
}

func TestRecordResponse_OneRejects_WaitsForAll_AggregatesFeedback(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "sec-agent")
	createTestAgent(t, svc, "ws-1", "qa-agent")
	createTestAgent(t, svc, "ws-1", "assignee-agent")

	taskID := createOrchestrateTask(t, svc, "ws-1", "assignee-agent")

	policy := newTestPolicy(
		reviewStage("s1", []service.ExecutionParticipant{
			agentParticipant("sec-agent"),
			agentParticipant("qa-agent"),
		}, 2),
	)
	setPolicyOnTask(t, svc, taskID, policy)
	if err := svc.EnterReviewStage(ctx, taskID, policy); err != nil {
		t.Fatalf("EnterReviewStage: %v", err)
	}

	// sec-agent rejects, qa-agent approves.
	if err := svc.RecordParticipantResponse(ctx, taskID, "sec-agent", "reject", "SQL injection risk"); err != nil {
		t.Fatalf("record sec-agent: %v", err)
	}
	if err := svc.RecordParticipantResponse(ctx, taskID, "qa-agent", "approve", "Tests pass"); err != nil {
		t.Fatalf("record qa-agent: %v", err)
	}

	// Assignee should be woken with aggregated feedback.
	wakeups, _ := svc.ListWakeupRequests(ctx, "ws-1")
	foundAssignee := false
	for _, w := range wakeups {
		if w.AgentInstanceID == "assignee-agent" && w.Reason == "task_assigned" {
			foundAssignee = true
		}
	}
	if !foundAssignee {
		t.Error("expected assignee-agent wakeup with aggregated feedback")
	}
}

func TestSequentialStages_ReviewThenApprovalThenDone(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "reviewer")
	createTestAgent(t, svc, "ws-1", "approver")
	createTestAgent(t, svc, "ws-1", "worker")

	taskID := createOrchestrateTask(t, svc, "ws-1", "worker")

	policy := newTestPolicy(
		reviewStage("review", []service.ExecutionParticipant{
			agentParticipant("reviewer"),
		}, 1),
		approvalStage("approval", []service.ExecutionParticipant{
			agentParticipant("approver"),
		}, 1),
	)
	setPolicyOnTask(t, svc, taskID, policy)
	if err := svc.EnterReviewStage(ctx, taskID, policy); err != nil {
		t.Fatalf("EnterReviewStage: %v", err)
	}

	// Reviewer approves -> advances to approval stage.
	if err := svc.RecordParticipantResponse(ctx, taskID, "reviewer", "approve", "OK"); err != nil {
		t.Fatalf("record reviewer: %v", err)
	}

	// Approver approves -> task completes.
	if err := svc.RecordParticipantResponse(ctx, taskID, "approver", "approve", "Approved"); err != nil {
		t.Fatalf("record approver: %v", err)
	}

	// Activity should show task completion.
	entries, _ := svc.ListActivity(ctx, "ws-1", 50)
	foundDone := false
	for _, e := range entries {
		if e.Action == "task.status_changed" && e.TargetID == taskID {
			foundDone = true
		}
	}
	if !foundDone {
		t.Error("expected task.status_changed activity entry for completion")
	}
}

func TestNoExecutionPolicy_NilReturned(t *testing.T) {
	for _, input := range []string{"", "{}"} {
		policy, err := service.ParseExecutionPolicy(input)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if policy != nil {
			t.Errorf("expected nil policy for %q", input)
		}
	}
}
