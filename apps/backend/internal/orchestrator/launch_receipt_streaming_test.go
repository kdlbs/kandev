package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestHandleAgentStreamEventMarksLaunchReceiptInferenceFromActivity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-receipt", "session-receipt", "step-receipt")
	history := LaunchReceiptHistory{Current: LaunchReceipt{
		Identity:         LaunchAttemptIdentity{SessionID: "session-receipt", Incarnation: "execution-receipt"},
		ProcessCreated:   LaunchTriStateTrue,
		InferenceStarted: LaunchTriStateUnknown,
	}}
	if err := repo.SetSessionMetadataKey(ctx, "session-receipt", "launch_receipt_state", history); err != nil {
		t.Fatalf("seed launch receipt: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task-receipt", SessionID: "session-receipt", ExecutionID: "execution-receipt",
		Data: &lifecycle.AgentStreamEventData{Type: agentEventToolCall, ToolCallID: "tool-receipt", ToolStatus: "running"},
	})

	session, err := repo.GetTaskSession(ctx, "session-receipt")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stored, ok := session.Metadata["launch_receipt_state"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored receipt = %#v, want persisted receipt", session.Metadata["launch_receipt_state"])
	}
	current, ok := stored["current"].(map[string]interface{})
	if !ok || current["inference_started"] != string(LaunchTriStateTrue) {
		t.Fatalf("current receipt = %#v, want inference_started true", current)
	}
}

func TestBindLaunchReceiptToMCPAttemptKeepsMatchingExecutionOnly(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-bind", "session-bind", "step-bind")
	history := LaunchReceiptHistory{Current: LaunchReceipt{Identity: LaunchAttemptIdentity{
		SessionID: "session-bind", Incarnation: "execution-current",
	}}}
	if err := repo.SetSessionMetadataKey(ctx, "session-bind", "launch_receipt_state", history); err != nil {
		t.Fatalf("seed launch receipt: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.bindLaunchReceiptToMCPAttempt(ctx, "session-bind", "execution-stale", &lifecycle.AgentStreamEventData{
		MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{AttemptID: "catalog-stale"},
	})
	svc.bindLaunchReceiptToMCPAttempt(ctx, "session-bind", "execution-current", &lifecycle.AgentStreamEventData{
		MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{AttemptID: "catalog-current"},
	})

	session, err := repo.GetTaskSession(ctx, "session-bind")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stored, ok := session.Metadata["launch_receipt_state"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored receipt = %#v", session.Metadata["launch_receipt_state"])
	}
	current := stored["current"].(map[string]interface{})
	if current["catalog_attachment_attempt_id"] != "catalog-current" {
		t.Fatalf("current receipt = %#v, want only matching catalog attempt", current)
	}
}

func TestHandleAgentStreamEventStartsReceiptBeforeProcessConfiguration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-preflight", "session-preflight", "step-preflight")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task-preflight", SessionID: "session-preflight", ExecutionID: "execution-preflight",
		Data: &lifecycle.AgentStreamEventData{Type: "launch_receipt", Data: "started"},
	})

	session, err := repo.GetTaskSession(ctx, "session-preflight")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stored, ok := session.Metadata["launch_receipt_state"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored receipt = %#v, want persisted receipt", session.Metadata["launch_receipt_state"])
	}
	current, ok := stored["current"].(map[string]interface{})
	if !ok || current["process_created"] != string(LaunchTriStateUnknown) {
		t.Fatalf("current receipt = %#v, want unknown process state before configuration", current)
	}
}
