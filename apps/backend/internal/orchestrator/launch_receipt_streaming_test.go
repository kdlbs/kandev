package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
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

// TestHandleSessionMCPAttachmentEventRejectsDelayedSameExecutionAttachment
// proves that an attachment queued by a replaced startup cannot bind to the
// current receipt merely because its execution was reused.
func TestHandleSessionMCPAttachmentEventRejectsDelayedSameExecutionAttachment(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-attachment-generation", "session-attachment-generation", "step-attachment-generation")
	history := LaunchReceiptHistory{Current: LaunchReceipt{Identity: LaunchAttemptIdentity{
		SessionID: "session-attachment-generation", Incarnation: "execution-reused", Generation: 2,
	}}}
	if err := repo.SetSessionMetadataKey(ctx, "session-attachment-generation", models.SessionMetaKeyLaunchReceiptState, history); err != nil {
		t.Fatalf("seed launch receipt: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.handleSessionMCPAttachmentEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task-attachment-generation", SessionID: "session-attachment-generation", ExecutionID: "execution-reused",
		Data: &lifecycle.AgentStreamEventData{
			MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{AttemptID: "catalog-delayed", ExecutionID: "execution-reused", StartupGeneration: 1},
			MCPAttachment:        &streams.MCPAttachmentEvidence{AttemptID: "catalog-delayed", StartupGeneration: 1, ServerName: "kandev", Kind: streams.MCPAttachmentEvidenceConfigured},
		},
	})
	session, err := repo.GetTaskSession(ctx, "session-attachment-generation")
	if err != nil {
		t.Fatalf("get delayed receipt: %v", err)
	}
	delayedCurrent := session.Metadata[models.SessionMetaKeyLaunchReceiptState].(map[string]interface{})["current"].(map[string]interface{})
	if _, bound := delayedCurrent["catalog_attachment_attempt_id"]; bound {
		t.Fatalf("delayed attachment bound replacement receipt: %#v", delayedCurrent)
	}
	svc.handleSessionMCPAttachmentEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task-attachment-generation", SessionID: "session-attachment-generation", ExecutionID: "execution-reused",
		Data: &lifecycle.AgentStreamEventData{
			MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{AttemptID: "catalog-current", ExecutionID: "execution-reused", StartupGeneration: 2},
			MCPAttachment:        &streams.MCPAttachmentEvidence{AttemptID: "catalog-current", StartupGeneration: 2, ServerName: "kandev", Kind: streams.MCPAttachmentEvidenceConfigured},
		},
	})

	session, err = repo.GetTaskSession(ctx, "session-attachment-generation")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	current := session.Metadata[models.SessionMetaKeyLaunchReceiptState].(map[string]interface{})["current"].(map[string]interface{})
	if current["catalog_attachment_attempt_id"] != "catalog-current" {
		t.Fatalf("receipt = %#v, want only current attachment", current)
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

func TestHandleAgentStreamEventRejectsDelayedSameExecutionStartupFact(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-replaced", "session-replaced", "step-replaced")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	for _, event := range []struct {
		generation uint64
		fact       string
	}{{1, "started"}, {2, "started"}, {1, "process_started"}} {
		svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID: "task-replaced", SessionID: "session-replaced", ExecutionID: "execution-reused",
			Data: &lifecycle.AgentStreamEventData{Type: "launch_receipt", Data: event.fact, StartupGeneration: event.generation},
		})
	}
	session, err := repo.GetTaskSession(ctx, "session-replaced")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stored := session.Metadata["launch_receipt_state"].(map[string]interface{})
	current := stored["current"].(map[string]interface{})
	identity := current["identity"].(map[string]interface{})
	if identity["generation"] != float64(2) || current["process_created"] != string(LaunchTriStateUnknown) {
		t.Fatalf("current receipt = %#v, want replacement generation with no stale process fact", current)
	}
}

func TestSyntheticCopilotFreshAndResumeReceiptsReachFirstActivity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	seedSession(t, repo, "copilot-task", "copilot-session", "copilot-step")
	for _, tc := range []struct {
		executionID string
		generation  uint64
	}{{"copilot-fresh-exec", 1}, {"copilot-resume-exec", 2}} {
		for _, fact := range []string{"started", "process_started"} {
			svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{TaskID: "copilot-task", SessionID: "copilot-session", ExecutionID: tc.executionID,
				Data: &lifecycle.AgentStreamEventData{Type: "launch_receipt", Data: fact, StartupGeneration: tc.generation}})
		}
		svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{TaskID: "copilot-task", SessionID: "copilot-session", ExecutionID: tc.executionID,
			Data: &lifecycle.AgentStreamEventData{Type: agentEventToolCall, ToolCallID: "copilot-activity", ToolStatus: "running"}})
		session, err := repo.GetTaskSession(ctx, "copilot-session")
		if err != nil {
			t.Fatalf("get receipt: %v", err)
		}
		current := session.Metadata["launch_receipt_state"].(map[string]interface{})["current"].(map[string]interface{})
		if current["process_created"] != string(LaunchTriStateTrue) || current["inference_started"] != string(LaunchTriStateTrue) {
			t.Fatalf("receipt = %#v", current)
		}
	}
}

func TestCallerCatalogRemainsActiveWhenTargetBootstrapFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "message-task", "caller-session", "message-step")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "target-session", TaskID: "message-task", State: models.TaskSessionStateRunning}); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	caller := &lifecycle.AgentStreamEventPayload{TaskID: "message-task", SessionID: "caller-session", ExecutionID: "caller-exec", Data: &lifecycle.AgentStreamEventData{
		MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{AttemptID: "caller-catalog", ExecutionID: "caller-exec"},
		MCPAttachment:        &streams.MCPAttachmentEvidence{AttemptID: "caller-catalog", ServerName: "kandev", Kind: streams.MCPAttachmentEvidenceToolsListObserved, ToolCount: 1, Tools: []streams.MCPToolSummary{{Name: "message_task_kandev"}}},
	}}
	svc.handleSessionMCPAttachmentEvent(ctx, caller)
	for _, fact := range []string{"started", "terminal_preflight_failure"} {
		svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{TaskID: "message-task", SessionID: "target-session", ExecutionID: "target-exec", Data: &lifecycle.AgentStreamEventData{Type: "launch_receipt", Data: fact, StartupGeneration: 1}})
	}
	callerSession, _ := repo.GetTaskSession(ctx, "caller-session")
	targetSession, _ := repo.GetTaskSession(ctx, "target-session")
	callerState := callerSession.Metadata[models.SessionMetaKeyMCPAttachmentState].(map[string]interface{})
	callerServer := callerState["current"].(map[string]interface{})["servers"].([]interface{})[0].(map[string]interface{})
	targetReceipt := targetSession.Metadata[models.SessionMetaKeyLaunchReceiptState].(map[string]interface{})["current"].(map[string]interface{})
	if callerServer["status"] != string(streams.MCPAttachmentStatusActive) || targetReceipt["process_created"] != string(LaunchTriStateFalse) || targetReceipt["inference_started"] != string(LaunchTriStateFalse) {
		t.Fatalf("caller catalog=%#v target receipt=%#v", callerServer, targetReceipt)
	}
}
