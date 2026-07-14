package acp

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
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

func TestConvertToolCallResultUpdate_StatusLessLocationsOnlyBecomesInProgress(t *testing.T) {
	a := newTestAdapter()
	seedReadToolCall(t, a, "tc-read-loc")

	tcu := &acp.SessionToolCallUpdate{
		ToolCallId: "tc-read-loc",
		Locations: []acp.ToolCallLocation{
			{Path: "/workspace/README.md"},
		},
	}

	ev := a.convertToolCallResultUpdate("session-1", tcu)
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.ToolStatus != "in_progress" {
		t.Errorf("ToolStatus = %q, want %q (locations-only updates must route through orchestrator)", ev.ToolStatus, "in_progress")
	}
	if ev.NormalizedPayload == nil || ev.NormalizedPayload.ReadFile() == nil {
		t.Fatalf("expected ReadFile payload, got %+v", ev.NormalizedPayload)
	}
	if got := ev.NormalizedPayload.ReadFile().FilePath; got != "/workspace/README.md" {
		t.Errorf("ReadFile.FilePath = %q, want /workspace/README.md", got)
	}
}

func seedReadToolCall(t *testing.T, a *Adapter, toolCallID string) {
	t.Helper()
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Title:      "Read",
		Status:     acp.ToolCallStatus("pending"),
		Kind:       acp.ToolKind("read"),
		RawInput:   map[string]any{},
	}
	if ev := a.convertToolCallUpdate("session-1", tc); ev == nil {
		t.Fatalf("seed: convertToolCallUpdate returned nil")
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

func TestConvertToolCallResultUpdate_AppendsTerminalOutputDeltas(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-delta")

	for _, chunk := range []string{"first\n", "second\n"} {
		event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
			ToolCallId: "tc-delta",
			Meta: map[string]any{
				"terminal_output_delta": map[string]any{"data": chunk},
			},
		})
		require.Equal(t, toolStatusInProgress, event.ToolStatus)
	}

	payload := a.activeToolCalls["tc-delta"]
	require.NotNil(t, payload)
	require.Equal(t, "first\nsecond\n", payload.ShellExec().Output.Stdout)
	require.Nil(t, payload.ShellExec().Output.ExitCode)
}

func TestConvertToolCallResultUpdate_ReplacesCumulativeContent(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-content")

	for _, cumulative := range []string{"first\n", "first\nsecond\n"} {
		event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
			ToolCallId: "tc-content",
			Content: []acp.ToolCallContent{{
				Content: &acp.ToolCallContentContent{
					Content: acp.TextBlock(cumulative),
					Type:    "content",
				},
			}},
		})
		require.Equal(t, toolStatusInProgress, event.ToolStatus)
	}

	payload := a.activeToolCalls["tc-content"]
	require.NotNil(t, payload)
	require.Equal(t, "first\nsecond\n", payload.ShellExec().Output.Stdout)
}

func TestConvertToolCallResultUpdate_FinalOutputReplacesLiveWithoutDuplication(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-final")

	a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-final",
		Meta: map[string]any{
			"terminal_output_delta": map[string]any{"data": "first\n"},
		},
	})
	completed := acp.ToolCallStatus("completed")
	event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-final",
		Status:     &completed,
		RawOutput: map[string]any{
			"formatted_output": "first\nsecond\n",
			"exit_code":        float64(4),
		},
	})

	require.Equal(t, toolStatusError, event.ToolStatus)
	require.Equal(t, "first\nsecond\n", event.NormalizedPayload.ShellExec().Output.Stdout)
	require.NotNil(t, event.NormalizedPayload.ShellExec().Output.ExitCode)
	require.Equal(t, 4, *event.NormalizedPayload.ShellExec().Output.ExitCode)
	require.NotContains(t, a.activeToolCalls, "tc-final")
}

func TestConvertToolCallResultUpdate_TerminalMetadataTakesPrecedence(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-terminal")
	completed := acp.ToolCallStatus("completed")

	event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-terminal",
		Status:     &completed,
		RawOutput:  "fallback output",
		Meta: map[string]any{
			"terminal_output": map[string]any{"data": "structured output\n"},
			"terminal_exit":   map[string]any{"exit_code": float64(3)},
		},
	})

	require.Equal(t, toolStatusError, event.ToolStatus)
	require.Equal(t, "structured output\n", event.NormalizedPayload.ShellExec().Output.Stdout)
	require.NotNil(t, event.NormalizedPayload.ShellExec().Output.ExitCode)
	require.Equal(t, 3, *event.NormalizedPayload.ShellExec().Output.ExitCode)
}

func TestConvertToolCallResultUpdate_FinalWithoutOutputPreservesLiveText(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-preserve")

	a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-preserve",
		Meta: map[string]any{
			"terminal_output_delta": map[string]any{"data": "retained output\n"},
		},
	})
	completed := acp.ToolCallStatus("completed")
	event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-preserve",
		Status:     &completed,
		RawOutput:  map[string]any{"exit_code": float64(0)},
	})

	require.Equal(t, toolStatusComplete, event.ToolStatus)
	require.Equal(t, "retained output\n", event.NormalizedPayload.ShellExec().Output.Stdout)
	require.NotNil(t, event.NormalizedPayload.ShellExec().Output.ExitCode)
	require.Equal(t, 0, *event.NormalizedPayload.ShellExec().Output.ExitCode)
}

func TestConvertToolCallResultUpdate_NonzeroExitPreservesCancellation(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-cancelled")
	cancelled := acp.ToolCallStatus("cancelled")

	event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-cancelled",
		Status:     &cancelled,
		RawOutput: map[string]any{
			"formatted_output": "interrupted\n",
			"exit_code":        float64(130),
		},
	})

	require.Equal(t, toolStatusCancelled, event.ToolStatus)
	require.NotNil(t, event.NormalizedPayload.ShellExec().Output.ExitCode)
	require.Equal(t, 130, *event.NormalizedPayload.ShellExec().Output.ExitCode)
}

func TestConvertToolCallResultUpdate_StatuslessExactExitCompletesTool(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-exit-only")

	event := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-exit-only",
		Meta: map[string]any{
			"terminal_exit": map[string]any{"exit_code": float64(0)},
		},
	})

	require.Equal(t, toolStatusComplete, event.ToolStatus)
	require.NotContains(t, a.activeToolCalls, "tc-exit-only")
}
