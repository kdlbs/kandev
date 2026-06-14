package acp

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestCancelActiveToolCalls_PreservesSubagentTask pins the fix for the
// "main agent finished, subagent keeps working" bug. The Claude Agent SDK
// (anthropics/claude-code#47936) can return session/prompt with stop_reason
// while a Task subagent is still in flight. Cancelling that tool call here
// would mark the subagent card terminated even though a real
// tool_call_update lands seconds later.
func TestCancelActiveToolCalls_PreservesSubagentTask(t *testing.T) {
	a := newTestAdapter()

	a.activeToolCalls["shell-1"] = streams.NewShellExec("ls", "", "list files", 0, false)
	a.activeToolCalls["subagent-1"] = streams.NewSubagentTask("Investigate", "do it", "general-purpose")

	a.cancelActiveToolCalls("sess-1")

	a.mu.RLock()
	_, shellPreserved := a.activeToolCalls["shell-1"]
	_, subagentPreserved := a.activeToolCalls["subagent-1"]
	a.mu.RUnlock()

	if shellPreserved {
		t.Error("non-subagent tool call must be cancelled and removed from activeToolCalls")
	}
	if !subagentPreserved {
		t.Error("subagent_task tool call must be preserved so a later authoritative tool_call_update can land")
	}

	var cancelledIDs []string
	for _, ev := range drainEvents(a) {
		if ev.Type == streams.EventTypeToolUpdate && ev.ToolStatus == toolStatusCancelled {
			cancelledIDs = append(cancelledIDs, ev.ToolCallID)
		}
	}

	for _, id := range cancelledIDs {
		if id == "subagent-1" {
			t.Error("must not emit cancelled tool_update for subagent_task — leave terminal status to the SDK")
		}
	}
	if len(cancelledIDs) != 1 || cancelledIDs[0] != "shell-1" {
		t.Errorf("cancelled IDs = %v, want [shell-1]", cancelledIDs)
	}
}

func TestBuildPromptContentBlocks_PathModeAttachmentSavesWritableFile(t *testing.T) {
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	workDir := t.TempDir()
	a.attachMgr = shared.NewAttachmentManager(workDir, a.logger.Zap())
	a.attachMgr.SetSessionID("sess-path")
	a.capabilities = acp.AgentCapabilities{
		PromptCapabilities: acp.PromptCapabilities{
			Image:           true,
			EmbeddedContext: true,
		},
	}

	encoded := base64.StdEncoding.EncodeToString([]byte("remote bytes"))
	blocks := a.buildPromptContentBlocks("inspect this", []v1.MessageAttachment{{
		Type:         "image",
		Data:         encoded,
		MimeType:     "image/png",
		Name:         "shot.png",
		DeliveryMode: "path",
	}})

	if len(blocks) != 2 {
		t.Fatalf("expected text prompt plus attachment path prompt, got %d blocks", len(blocks))
	}
	if blocks[1].Text == nil {
		t.Fatalf("expected path-mode attachment to be sent as text, got %#v", blocks[1])
	}
	if blocks[1].Image != nil {
		t.Fatalf("path-mode attachment should not be sent as an image block")
	}
	text := blocks[1].Text.Text
	if !strings.Contains(text, ".kandev/attachments/sess-path/shot.png") {
		t.Fatalf("attachment prompt did not include saved path: %q", text)
	}
	if !strings.Contains(text, "writable") {
		t.Fatalf("attachment prompt did not include writable contract: %q", text)
	}

	savedPath := filepath.Join(workDir, ".kandev", "attachments", "sess-path", "shot.png")
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("expected attachment to be written to workspace: %v", err)
	}
	if string(data) != "remote bytes" {
		t.Fatalf("saved attachment = %q, want %q", string(data), "remote bytes")
	}
}

// TestCancelActiveToolCalls_SubagentLateUpdate_LandsAsComplete drives the full
// race scenario end-to-end through the real adapter methods (no mocks):
//
//  1. ACP delivers the initial subagent tool_call (Claude `_meta.claudeCode.toolName=Agent`).
//  2. session/prompt returns early with stop_reason (anthropics/claude-code#47936)
//     so the adapter sweeps in-flight tool calls via cancelActiveToolCalls.
//  3. Seconds later the SDK delivers the real terminal tool_call_update with the
//     subagent metrics (status=completed, totalDurationMs, totalTokens, ToolUseCount).
//
// With the fix, the subagent payload stays in activeToolCalls through the cancel
// sweep, so the late tool_call_update finds it and enriches the card with the
// real result. Without the fix the payload would have been deleted and the
// subagent card would terminate as "cancelled" with no metrics.
func TestCancelActiveToolCalls_SubagentLateUpdate_LandsAsComplete(t *testing.T) {
	a := newTestAdapter()

	// 1. Initial tool_call for the subagent — Claude tags it via _meta.claudeCode.toolName=Agent.
	initial := &acp.SessionUpdateToolCall{
		ToolCallId: "sub-1",
		Title:      "Agent",
		Kind:       acp.ToolKind("other"),
		Meta: map[string]any{
			"claudeCode": map[string]any{"toolName": "Agent"},
		},
		RawInput: map[string]any{
			"description":   "Investigate flaky test",
			"prompt":        "Find root cause",
			"subagent_type": "general-purpose",
		},
	}
	if ev := a.convertToolCallUpdate("s1", initial); ev == nil {
		t.Fatalf("seed: convertToolCallUpdate returned nil")
	}

	a.mu.RLock()
	stored := a.activeToolCalls["sub-1"]
	a.mu.RUnlock()
	if stored == nil || stored.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("seed: activeToolCalls['sub-1'] kind = %v, want subagent_task", stored)
	}

	// 2. session/prompt returns early — adapter sweeps active tool calls.
	_ = drainEvents(a)
	a.cancelActiveToolCalls("s1")

	a.mu.RLock()
	stillStored := a.activeToolCalls["sub-1"]
	a.mu.RUnlock()
	if stillStored == nil {
		t.Fatal("subagent payload must survive the cancel sweep so a late tool_call_update can enrich it")
	}

	// 3. Late terminal tool_call_update arrives with Claude's toolResponse metadata.
	completed := acp.ToolCallStatus("completed")
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "sub-1",
		Status:     &completed,
		Meta: map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
			"agentId":           "agent_abc",
			"agentType":         "general-purpose",
			"status":            "completed",
			"totalDurationMs":   float64(12345),
			"totalTokens":       float64(6789),
			"totalToolUseCount": float64(11),
		}}},
	}

	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected terminal event from late tool_call_update")
	}
	if ev.ToolStatus != toolStatusComplete {
		t.Errorf("ToolStatus = %q, want %q (late update must drive the card to complete, not cancelled)",
			ev.ToolStatus, toolStatusComplete)
	}
	if ev.NormalizedPayload == nil {
		t.Fatal("expected NormalizedPayload on terminal event")
	}
	sa := ev.NormalizedPayload.SubagentTask()
	if sa == nil {
		t.Fatalf("expected subagent payload, got %v", ev.NormalizedPayload.Kind())
	}
	if sa.Description != "Investigate flaky test" {
		t.Errorf("Description = %q, want preserved from initial tool_call", sa.Description)
	}
	if sa.Status != "completed" {
		t.Errorf("subagent status = %q, want completed", sa.Status)
	}
	if sa.DurationMs != 12345 || sa.TotalTokens != 6789 {
		t.Errorf("metrics = %d/%d, want 12345/6789", sa.DurationMs, sa.TotalTokens)
	}
	if sa.ToolUseCount == nil || *sa.ToolUseCount != 11 {
		t.Errorf("ToolUseCount = %v, want 11", sa.ToolUseCount)
	}
}

// TestConvertToolCallResultUpdate_AsyncLaunched_TerminalComplete drives the
// real claude-acp async-launched envelope through convertToolCallUpdate +
// convertToolCallResultUpdate end-to-end. The bug:
//
//   - Initial tool_call lands (Task, _meta.claudeCode.toolName=Agent).
//   - tool_call_update arrives with top-level Status=nil and
//     _meta.claudeCode.toolResponse.status="async_launched" (plus isAsync,
//     outputFile, canReadOutputFile).
//   - Without the fix, status stays empty → orchestrator drops the update →
//     card stays "pending" forever in the UI.
//
// Reproduction file: acp-debug/claude-acp-prompt-20260602-110119.jsonl
// (3 subagents stuck async_launched; session/prompt returned end_turn; no
// subsequent terminal updates).
func TestConvertToolCallResultUpdate_AsyncLaunched_TerminalComplete(t *testing.T) {
	a := newTestAdapter()

	initial := &acp.SessionUpdateToolCall{
		ToolCallId: "toolu_async_1",
		Title:      "Task",
		Kind:       acp.ToolKind("think"),
		Meta:       map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}},
	}
	if ev := a.convertToolCallUpdate("s1", initial); ev == nil {
		t.Fatalf("seed: convertToolCallUpdate returned nil")
	}
	_ = drainEvents(a)

	// Replay the exact frame shape observed in run-2/run-6 (top-level Status=nil).
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "toolu_async_1",
		Status:     nil,
		Meta: map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
			"agentId":           "ab9a9f3ed453c911e",
			"description":       "Sleep and write file 1",
			"isAsync":           true,
			"outputFile":        "/tmp/tasks/ab9a9f3ed453c911e.output",
			"canReadOutputFile": true,
			"status":            "async_launched",
			"prompt":            "Run this exact bash command: `sleep 120 && echo done`. Report when done.",
		}}},
	}

	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected terminal event for async_launched")
	}
	if ev.ToolStatus != toolStatusComplete {
		t.Errorf("ToolStatus = %q, want %q (async_launched is terminal for the Task tool)",
			ev.ToolStatus, toolStatusComplete)
	}
	if ev.NormalizedPayload == nil {
		t.Fatal("expected NormalizedPayload")
	}
	sa := ev.NormalizedPayload.SubagentTask()
	if sa == nil {
		t.Fatalf("expected subagent payload, got kind=%v", ev.NormalizedPayload.Kind())
	}
	if sa.Status != "async_launched" {
		t.Errorf("payload.Status = %q, want async_launched (preserved verbatim for UI)", sa.Status)
	}
	if !sa.IsAsync {
		t.Error("payload.IsAsync = false, want true")
	}
	if sa.OutputFile != "/tmp/tasks/ab9a9f3ed453c911e.output" {
		t.Errorf("payload.OutputFile = %q", sa.OutputFile)
	}
	if !sa.CanReadOutputFile {
		t.Error("payload.CanReadOutputFile = false, want true")
	}
	if sa.AgentID != "ab9a9f3ed453c911e" {
		t.Errorf("payload.AgentID = %q", sa.AgentID)
	}

	// Subsequent cancel sweep must NOT re-cancel the now-terminal subagent.
	// Since isTerminal was true, convertToolCallResultUpdate already removed
	// the entry from activeToolCalls.
	a.mu.RLock()
	_, stillTracked := a.activeToolCalls["toolu_async_1"]
	a.mu.RUnlock()
	if stillTracked {
		t.Error("terminal async_launched update should remove the tool call from activeToolCalls")
	}
}

// TestConvertToolCallResultUpdate_AsyncLaunched_NonSubagentNotTerminated guards
// the kind gate: a non-Task tool whose meta happens to carry
// `claudeCode.toolResponse.status == "async_launched"` (hypothetical future
// claude-acp tool reusing the literal) must NOT be marked complete by the
// override. Only subagent_task payloads earn the dispatch-is-terminal treatment.
func TestConvertToolCallResultUpdate_AsyncLaunched_NonSubagentNotTerminated(t *testing.T) {
	a := newTestAdapter()

	// Seed a non-Task tool call (a Bash) into activeToolCalls.
	initial := &acp.SessionUpdateToolCall{
		ToolCallId: "bash-x",
		Title:      "Bash",
		Kind:       acp.ToolKind("execute"),
		Meta:       map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput:   map[string]any{"command": "ls"},
	}
	if ev := a.convertToolCallUpdate("s1", initial); ev == nil {
		t.Fatalf("seed: convertToolCallUpdate returned nil")
	}
	_ = drainEvents(a)

	// Update frame carries async_launched but the underlying payload is shell_exec.
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "bash-x",
		Status:     nil,
		Meta: map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
			"status":  "async_launched",
			"isAsync": true,
		}}},
	}
	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.ToolStatus == toolStatusComplete {
		t.Errorf("ToolStatus = %q, want non-complete (kind gate must reject non-subagent tools)", ev.ToolStatus)
	}

	// Card must remain in activeToolCalls (not deleted as a terminal update).
	a.mu.RLock()
	_, stillTracked := a.activeToolCalls["bash-x"]
	a.mu.RUnlock()
	if !stillTracked {
		t.Error("non-subagent tool must NOT be removed from activeToolCalls on async_launched envelope")
	}
}

// TestConvertToolCallResultUpdate_AsyncLaunched_OverridesInProgress guards the
// removal of the `status == ""` guard. If a future SDK version adds Title or
// RawInput to the async_launched frame, the earlier in_progress synthesis
// (line ~159) would set status="in_progress" and a gated override would never
// fire — leaving the card stuck. The unconditional override prevents that.
func TestConvertToolCallResultUpdate_AsyncLaunched_OverridesInProgress(t *testing.T) {
	a := newTestAdapter()

	initial := &acp.SessionUpdateToolCall{
		ToolCallId: "toolu_async_2",
		Title:      "Task",
		Kind:       acp.ToolKind("think"),
		Meta:       map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}},
	}
	if ev := a.convertToolCallUpdate("s1", initial); ev == nil {
		t.Fatalf("seed: nil")
	}
	_ = drainEvents(a)

	// Simulate a hypothetical future SDK frame: async_launched envelope plus a
	// Title that triggers the in_progress synthesis. The override must still win.
	newTitle := "Task: backgrounded"
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "toolu_async_2",
		Status:     nil,
		Title:      &newTitle,
		Meta: map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
			"status":     "async_launched",
			"isAsync":    true,
			"outputFile": "/tmp/x.output",
		}}},
	}
	ev := a.convertToolCallResultUpdate("s1", tcu)
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.ToolStatus != toolStatusComplete {
		t.Errorf("ToolStatus = %q, want %q — async_launched override must beat the in_progress synthesis even when Title is set",
			ev.ToolStatus, toolStatusComplete)
	}
}

// TestCancelActiveToolCalls_NestedBashCancelledSubagentPreserved verifies the
// realistic claude-acp shape: a subagent (Agent tool) has a child Bash tool_call
// tagged with parentToolUseId. On early prompt return, the child Bash must be
// cancelled (it really is dead from the SDK's perspective), but the parent
// subagent card must be preserved.
func TestCancelActiveToolCalls_NestedBashCancelledSubagentPreserved(t *testing.T) {
	a := newTestAdapter()

	parent := &acp.SessionUpdateToolCall{
		ToolCallId: "sub-1",
		Title:      "Agent",
		Kind:       acp.ToolKind("other"),
		Meta:       map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}},
		RawInput:   map[string]any{"description": "Do work", "prompt": "p", "subagent_type": "general-purpose"},
	}
	child := &acp.SessionUpdateToolCall{
		ToolCallId: "bash-1",
		Title:      "Bash",
		Kind:       acp.ToolKind("execute"),
		Meta: map[string]any{"claudeCode": map[string]any{
			"toolName":        "Bash",
			"parentToolUseId": "sub-1",
		}},
		RawInput: map[string]any{"command": "sleep 30"},
	}
	if ev := a.convertToolCallUpdate("s1", parent); ev == nil {
		t.Fatalf("seed parent: nil")
	}
	if ev := a.convertToolCallUpdate("s1", child); ev == nil {
		t.Fatalf("seed child: nil")
	}
	_ = drainEvents(a)

	a.cancelActiveToolCalls("s1")

	a.mu.RLock()
	_, parentKept := a.activeToolCalls["sub-1"]
	_, childKept := a.activeToolCalls["bash-1"]
	a.mu.RUnlock()

	if !parentKept {
		t.Error("parent subagent card must be preserved through cancel sweep")
	}
	if childKept {
		t.Error("nested bash must be cancelled and removed (it really is dead from the SDK's perspective)")
	}

	var cancelEvents []AgentEvent
	for _, ev := range drainEvents(a) {
		if ev.Type == streams.EventTypeToolUpdate && ev.ToolStatus == toolStatusCancelled {
			cancelEvents = append(cancelEvents, ev)
		}
	}
	if len(cancelEvents) != 1 || cancelEvents[0].ToolCallID != "bash-1" {
		t.Errorf("expected exactly one cancelled event for bash-1, got %d events", len(cancelEvents))
	}
}
