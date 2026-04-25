package acp

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

// seedExecuteToolCall registers a pending Bash tool_call (Claude-acp pattern: empty
// rawInput, default "Terminal" title) so subsequent tool_call_update tests have an
// activeToolCalls entry to update.
func seedExecuteToolCall(t *testing.T, a *Adapter, toolCallID string) {
	t.Helper()
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Title:      "Terminal",
		Status:     acp.ToolCallStatus("pending"),
		Kind:       acp.ToolKind("execute"),
		RawInput:   map[string]any{},
	}
	if ev := a.convertToolCallUpdate("session-1", tc); ev == nil {
		t.Fatalf("seed: convertToolCallUpdate returned nil")
	}
}

// TestConvertToolCallResultUpdate_StatusLessUpdateBecomesInProgress reproduces the
// claude-acp Bash flow: an initial tool_call with empty rawInput and "Terminal"
// placeholder title, followed by a tool_call_update that carries the actual command
// and title but no Status field. Without this fix the orchestrator drops the update
// (its switch only matches known statuses) and the message stays on "Terminal".
func TestConvertToolCallResultUpdate_StatusLessUpdateBecomesInProgress(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-1")

	cmdTitle := "ls -la /tmp | head -5"
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-1",
		Title:      &cmdTitle,
		RawInput: map[string]any{
			"command":     "ls -la /tmp | head -5",
			"description": "List first 5 entries in /tmp",
		},
	}

	ev := a.convertToolCallResultUpdate("session-1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.ToolStatus != "in_progress" {
		t.Errorf("ToolStatus = %q, want %q (status-less updates with content must route through orchestrator)", ev.ToolStatus, "in_progress")
	}
	if ev.ToolTitle != cmdTitle {
		t.Errorf("ToolTitle = %q, want %q", ev.ToolTitle, cmdTitle)
	}
	if ev.NormalizedPayload == nil || ev.NormalizedPayload.ShellExec() == nil {
		t.Fatalf("expected ShellExec payload, got %+v", ev.NormalizedPayload)
	}
	if got := ev.NormalizedPayload.ShellExec().Command; got != "ls -la /tmp | head -5" {
		t.Errorf("ShellExec.Command = %q, want command from rawInput", got)
	}
}

func TestConvertToolCallResultUpdate_StatusLessRawInputOnlyBecomesInProgress(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-2")

	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-2",
		RawInput:   map[string]any{"command": "pwd"},
	}

	ev := a.convertToolCallResultUpdate("session-1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.ToolStatus != "in_progress" {
		t.Errorf("ToolStatus = %q, want %q", ev.ToolStatus, "in_progress")
	}
}

func TestConvertToolCallResultUpdate_StatusLessContentOnlyBecomesInProgress(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-3")

	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-3",
		Content: []acp.ToolCallContent{
			{
				Content: &acp.ToolCallContentContent{
					Content: acp.TextBlock("partial output"),
					Type:    "content",
				},
			},
		},
	}

	ev := a.convertToolCallResultUpdate("session-1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.ToolStatus != "in_progress" {
		t.Errorf("ToolStatus = %q, want %q", ev.ToolStatus, "in_progress")
	}
}

func TestConvertToolCallResultUpdate_FullyEmptyUpdateKeepsEmptyStatus(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-4")

	// A no-op update — no status, no title, no rawInput, no content. Should not
	// be promoted to in_progress; orchestrator already ignores empty status, so
	// behaviour is unchanged from today.
	tcu := &acp.SessionToolCallUpdate{ToolCallId: "tc-4"}

	ev := a.convertToolCallResultUpdate("session-1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.ToolStatus != "" {
		t.Errorf("ToolStatus = %q, want empty (no synthesized status for no-op update)", ev.ToolStatus)
	}
}

func TestConvertToolCallResultUpdate_ExplicitCompletedStatusUnchanged(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-5")

	completed := acp.ToolCallStatus("completed")
	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-5",
		Status:     &completed,
		RawOutput:  "ok",
	}

	ev := a.convertToolCallResultUpdate("session-1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	// "completed" is normalized to "complete" by the existing logic — regression guard.
	if ev.ToolStatus != "complete" {
		t.Errorf("ToolStatus = %q, want %q", ev.ToolStatus, "complete")
	}
}
