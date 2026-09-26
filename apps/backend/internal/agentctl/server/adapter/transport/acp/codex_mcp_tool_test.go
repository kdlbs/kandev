package acp

import (
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestCodexMCPExecuteFrameNormalizesAsGenericToolCall(t *testing.T) {
	t.Parallel()

	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	a.dialect = newACPDialect(codexAgentID)

	arguments := map[string]any{
		"version": float64(1),
		"title":   "Failed requests",
		"blocks": []any{map[string]any{
			"type":       "chart",
			"chart_type": "bar",
			"title":      "Failed requests by service",
			"summary":    "Failures in the latest interval.",
			"labels":     []any{"API", "Billing"},
			"series": []any{map[string]any{
				"label":  "Failed requests",
				"values": []any{float64(42), float64(27)},
			}},
		}},
	}
	rawInput := map[string]any{
		"server":    "kandev",
		"tool":      "show_rich_output_kandev",
		"arguments": arguments,
	}

	initial := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		Meta:          map[string]any{"is_mcp_tool_call": true},
		Kind:          acpsdk.ToolKind("execute"),
		RawInput:      rawInput,
		SessionUpdate: "tool_call",
		Status:        acpsdk.ToolCallStatus("in_progress"),
		Title:         "mcp.kandev.show_rich_output_kandev",
		ToolCallId:    acpsdk.ToolCallId("mcp-call-1"),
	})

	require.NotNil(t, initial)
	require.Equal(t, streams.EventTypeToolCall, initial.Type)
	require.Equal(t, "kandev/show_rich_output_kandev", initial.ToolName)
	require.Equal(t, streams.ToolKindGeneric, initial.NormalizedPayload.Kind())
	require.True(t, initial.NormalizedPayload.IsMCPTool())
	require.Equal(t, "kandev/show_rich_output_kandev", initial.NormalizedPayload.Generic().Name)
	require.Equal(t, arguments, initial.NormalizedPayload.Generic().Input)
	require.Nil(t, initial.NormalizedPayload.ShellExec())

	structured := map[string]any{
		"version": float64(1),
		"resolved_charts": []any{map[string]any{
			"block_index": float64(0),
			"labels":      []any{"API", "Billing"},
			"series": []any{map[string]any{
				"label":  "Failed requests",
				"values": []any{float64(42), float64(27)},
			}},
		}},
	}
	callToolResult := map[string]any{
		"content": []any{map[string]any{
			"type": "text",
			"text": `{"version":1,"resolved_charts":[]}`,
		}},
		"structuredContent": structured,
	}
	completed := acpsdk.ToolCallStatus("completed")
	terminal := a.convertToolCallResultUpdate("session-1", &acpsdk.SessionToolCallUpdate{
		RawInput:      rawInput,
		RawOutput:     map[string]any{"error": nil, "result": callToolResult},
		SessionUpdate: "tool_call_update",
		Status:        &completed,
		ToolCallId:    acpsdk.ToolCallId("mcp-call-1"),
	})

	require.NotNil(t, terminal)
	require.Equal(t, streams.EventTypeToolUpdate, terminal.Type)
	require.Equal(t, toolStatusComplete, terminal.ToolStatus)
	require.Equal(t, streams.ToolKindGeneric, terminal.NormalizedPayload.Kind())
	require.True(t, terminal.NormalizedPayload.IsMCPTool())
	require.Equal(t, arguments, terminal.NormalizedPayload.Generic().Input)
	require.Equal(t, callToolResult, terminal.NormalizedPayload.Generic().Output)
}

func TestCodexMCPExecuteFramePreservesServerIdentity(t *testing.T) {
	t.Parallel()

	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	a.dialect = newACPDialect(codexAgentID)

	event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		Meta:          map[string]any{"is_mcp_tool_call": true},
		Kind:          acpsdk.ToolKind("execute"),
		RawInput:      map[string]any{"server": "other", "tool": "show_rich_output_kandev", "arguments": map[string]any{}},
		SessionUpdate: "tool_call",
		Status:        acpsdk.ToolCallStatus("in_progress"),
		ToolCallId:    acpsdk.ToolCallId("mcp-call-foreign-server"),
	})

	require.NotNil(t, event)
	require.Equal(t, "other/show_rich_output_kandev", event.NormalizedPayload.Generic().Name)
}

func TestCodexExecuteWithoutMCPMarkerRemainsShellExecution(t *testing.T) {
	t.Parallel()

	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	a.dialect = newACPDialect(codexAgentID)

	event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		Kind:          acpsdk.ToolKind("execute"),
		RawInput:      map[string]any{"command": "pwd"},
		SessionUpdate: "tool_call",
		Status:        acpsdk.ToolCallStatus("in_progress"),
		Title:         "Run pwd",
		ToolCallId:    acpsdk.ToolCallId("shell-call-1"),
	})

	require.NotNil(t, event)
	require.Equal(t, streams.ToolKindShellExec, event.NormalizedPayload.Kind())
	require.Equal(t, "pwd", event.NormalizedPayload.ShellExec().Command)
}
