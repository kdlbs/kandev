package streamjson

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/pkg/claudecode"
)

// --- drainExitPlanContent tests ---

func TestDrainExitPlanContent_ReturnsAndClears(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	a.mu.Lock()
	a.exitPlanContent = "my plan"
	a.mu.Unlock()

	content := a.drainExitPlanContent()
	if content != "my plan" {
		t.Errorf("drainExitPlanContent() = %q, want %q", content, "my plan")
	}

	// Second drain should return empty
	content2 := a.drainExitPlanContent()
	if content2 != "" {
		t.Errorf("second drainExitPlanContent() = %q, want empty", content2)
	}
}

func TestDrainExitPlanContent_EmptyReturnsEmpty(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	content := a.drainExitPlanContent()
	if content != "" {
		t.Errorf("drainExitPlanContent() = %q, want empty", content)
	}
}

// --- processToolUseBlock ExitPlanMode tests ---

func TestProcessToolUseBlock_ExitPlanMode_CapturesPlanAndEmitsMode(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"

	block := claudecode.ContentBlock{
		Type:  "tool_use",
		ID:    "tool-1",
		Name:  "ExitPlanMode",
		Input: map[string]any{"plan": "## My Plan\n- step 1\n- step 2"},
	}

	a.processToolUseBlock(block, "sess-1", "op-1", "")

	events := drainEvents(a)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// First event: session_mode with empty mode (plan mode exited)
	if events[0].Type != "session_mode" {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, "session_mode")
	}
	if events[0].CurrentModeID != "" {
		t.Errorf("events[0].CurrentModeID = %q, want empty", events[0].CurrentModeID)
	}

	// Verify plan content was captured
	a.mu.RLock()
	planContent := a.exitPlanContent
	a.mu.RUnlock()
	if planContent != "## My Plan\n- step 1\n- step 2" {
		t.Errorf("exitPlanContent = %q, want plan text", planContent)
	}
}

func TestProcessToolUseBlock_NonExitPlanMode_NoPlanCapture(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"

	block := claudecode.ContentBlock{
		Type:  "tool_use",
		ID:    "tool-1",
		Name:  "Bash",
		Input: map[string]any{"command": "ls"},
	}

	a.processToolUseBlock(block, "sess-1", "op-1", "")

	events := drainEvents(a)
	for _, ev := range events {
		if ev.Type == "session_mode" {
			t.Error("unexpected session_mode event for non-ExitPlanMode tool")
		}
	}

	a.mu.RLock()
	planContent := a.exitPlanContent
	a.mu.RUnlock()
	if planContent != "" {
		t.Errorf("exitPlanContent = %q, want empty", planContent)
	}
}

// --- handleResultMessage plan content tests ---

func TestHandleResultMessage_ClearsPendingAndDrainsPlan(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"

	a.mu.Lock()
	a.exitPlanContent = "the plan"
	a.mu.Unlock()
	a.exitPlanPending.Store(true)

	msg := &claudecode.CLIMessage{
		Type:   claudecode.MessageTypeResult,
		Result: json.RawMessage(`"done"`),
	}

	a.handleResultMessage(msg, "sess-1", "op-1")

	if a.exitPlanPending.Load() {
		t.Error("exitPlanPending should be false after result message")
	}

	a.mu.RLock()
	remaining := a.exitPlanContent
	a.mu.RUnlock()
	if remaining != "" {
		t.Errorf("exitPlanContent = %q, want empty after drain", remaining)
	}

	events := drainEvents(a)
	var completeEvent *AgentEvent
	for i := range events {
		if events[i].Type == "complete" {
			completeEvent = &events[i]
			break
		}
	}
	if completeEvent == nil {
		t.Fatal("no complete event found")
	}
	if completeEvent.Data == nil {
		t.Fatal("complete data is nil")
	}
	if completeEvent.Data["plan_content"] != "the plan" {
		t.Errorf("plan_content = %v, want %q", completeEvent.Data["plan_content"], "the plan")
	}
}

// --- scheduleExitPlanModeCompletion tests ---

func TestScheduleExitPlanModeCompletion_EmitsCompleteAfterDelay(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"
	a.resultCh = make(chan resultComplete, 1)

	a.mu.Lock()
	a.exitPlanContent = "delayed plan"
	a.mu.Unlock()

	a.scheduleExitPlanModeCompletion()

	select {
	case result := <-a.resultCh:
		if !result.success {
			t.Error("expected success=true")
		}
	case <-time.After(exitPlanModeCompletionDelay + time.Second):
		t.Fatal("timed out waiting for completion signal")
	}

	events := drainEvents(a)
	var completeEvent *AgentEvent
	for i := range events {
		if events[i].Type == "complete" {
			completeEvent = &events[i]
			break
		}
	}
	if completeEvent == nil {
		t.Fatal("no complete event emitted")
	}
	if completeEvent.Data == nil {
		t.Fatal("complete data is nil")
	}
	if completeEvent.Data["plan_content"] != "delayed plan" {
		t.Errorf("plan_content = %v, want %q", completeEvent.Data["plan_content"], "delayed plan")
	}

	remaining := a.drainExitPlanContent()
	if remaining != "" {
		t.Errorf("exitPlanContent not drained: %q", remaining)
	}
}

func TestScheduleExitPlanModeCompletion_CancelledByResult(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"

	a.scheduleExitPlanModeCompletion()

	// Simulate a result message arriving before the delay
	a.exitPlanPending.Store(false)

	time.Sleep(exitPlanModeCompletionDelay + 500*time.Millisecond)

	events := drainEvents(a)
	for _, ev := range events {
		if ev.Type == "complete" {
			t.Error("unexpected complete event — should have been cancelled by result")
		}
	}
}

func TestScheduleExitPlanModeCompletion_CancelledByContext(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"

	a.scheduleExitPlanModeCompletion()

	// Cancel the adapter context immediately
	a.cancel()

	time.Sleep(500 * time.Millisecond)

	if a.exitPlanPending.Load() {
		t.Error("exitPlanPending should be false after context cancel")
	}

	events := drainEvents(a)
	for _, ev := range events {
		if ev.Type == "complete" {
			t.Error("unexpected complete event after context cancellation")
		}
	}
}

// --- handleSystemMessage status/permission mode tests ---

func TestHandleSystemMessage_StatusPermissionMode(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantModeID string
	}{
		{"plan mode", "plan", "plan"},
		{"bypass mapped to empty", "bypassPermissions", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
			a.sessionID = "sess-1"
			a.operationID = "op-1"

			msg := &claudecode.CLIMessage{
				Type:           claudecode.MessageTypeSystem,
				Subtype:        "status",
				PermissionMode: tt.mode,
			}

			a.handleSystemMessage(msg)

			events := drainEvents(a)
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].Type != "session_mode" {
				t.Errorf("event type = %q, want %q", events[0].Type, "session_mode")
			}
			if events[0].CurrentModeID != tt.wantModeID {
				t.Errorf("CurrentModeID = %q, want %q", events[0].CurrentModeID, tt.wantModeID)
			}
		})
	}
}

// --- hook callback and permission handler ExitPlanMode tests ---

func TestHandleHookCallback_AutoApprove_ExitPlanMode(t *testing.T) {
	var buf syncBuf
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.client = claudecode.NewClient(&buf, &emptyReader{}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"
	a.resultCh = make(chan resultComplete, 1)

	req := &claudecode.ControlRequest{
		Subtype:    claudecode.SubtypeHookCallback,
		CallbackID: "auto_approve",
		Input: map[string]any{
			"tool_name":       "ExitPlanMode",
			"hook_event_name": "PreToolUse",
		},
	}

	a.handleHookCallback("req-1", req)

	if !a.exitPlanPending.Load() {
		t.Error("exitPlanPending should be true after ExitPlanMode hook approval")
	}

	a.cancel()
}

func TestHandleToolPermission_ExitPlanMode_NoHandler(t *testing.T) {
	var buf syncBuf
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.client = claudecode.NewClient(&buf, &emptyReader{}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"
	a.resultCh = make(chan resultComplete, 1)

	req := &claudecode.ControlRequest{
		Subtype:  claudecode.SubtypeCanUseTool,
		ToolName: "ExitPlanMode",
		Input:    map[string]any{"plan": "test plan"},
	}

	a.handleToolPermission("req-1", req)

	if !a.exitPlanPending.Load() {
		t.Error("exitPlanPending should be true after ExitPlanMode auto-allow")
	}

	a.cancel()
}
