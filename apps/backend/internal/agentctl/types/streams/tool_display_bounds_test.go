package streams

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
	"github.com/stretchr/testify/require"
)

func TestBoundedToolSmallValuesUnchanged(t *testing.T) {
	type toolText string
	for _, value := range []any{nil, "output", true, float64(1), strings.Repeat("x", 32<<10), map[string]any{"content": []any{"text", nil}}, map[string]string(nil), []string(nil), toolText("named string"), json.RawMessage(`{"ok":true}`)} {
		before, err := json.Marshal(value)
		require.NoError(t, err)
		after, err := json.Marshal(boundToolValue(value))
		require.NoError(t, err)
		require.True(t, bytes.Equal(before, after), "%T changed JSON", value)
	}
}

func TestBoundedToolLargeValues(t *testing.T) {
	deep := any("leaf")
	for range 30 {
		deep = []any{deep}
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	for name, value := range map[string]any{
		"incident": strings.Repeat("x", 1048605),
		"escaped":  strings.Repeat("\x00", 1<<20),
		"unicode":  strings.Repeat("世", 1<<20),
		"nested":   map[string]any{"content": []any{map[string]any{"text": strings.Repeat("x", 1<<20)}}},
		"wide":     make([]any, 100000), "deep": deep, "cycle": cycle,
	} {
		t.Run(name, func(t *testing.T) {
			result := boundToolValue(value).(map[string]any)
			require.Equal(t, true, result["truncated"])
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.Less(t, len(encoded), toolValueBytes)
			require.True(t, utf8.Valid(encoded))
		})
	}
}

func TestBoundedToolEventIsImmutableAndIdempotent(t *testing.T) {
	output := map[string]any{"text": strings.Repeat("x", 1<<20)}
	payload := NewMCPTool("fetch", map[string]any{"query": strings.Repeat("x", 1<<20)})
	payload.Generic().Output = output
	payload.SetMonitorIdentity("monitor", false)
	event := AgentEvent{Type: EventTypeToolUpdate, ToolCallID: "tool", ParentToolCallID: "parent", ToolStatus: "error", NormalizedPayload: payload,
		ToolCallContents: []ToolCallContentItem{{Type: "content", Content: &ContentBlock{Type: "text", Text: strings.Repeat("x", 1<<20)}}}}
	bounded := BoundGenericToolEvent(event)
	first, err := json.Marshal(bounded)
	require.NoError(t, err)
	second, err := json.Marshal(BoundGenericToolEvent(bounded))
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Less(t, len(first), int(deliverylimits.DisplayBytes))
	require.True(t, bounded.NormalizedPayload.IsMCPTool())
	require.Equal(t, "monitor", bounded.NormalizedPayload.Monitor().TaskID)
	require.Equal(t, "error", bounded.ToolStatus)
	require.Equal(t, "parent", bounded.ParentToolCallID)
	output["text"] = "changed"
	unchanged, err := json.Marshal(bounded)
	require.NoError(t, err)
	require.Equal(t, first, unchanged)
	require.Len(t, event.ToolCallContents[0].Content.Text, 1<<20)
}

func TestBoundedToolEventKeepsControlAndShell(t *testing.T) {
	for _, event := range []AgentEvent{
		{Type: EventTypePermissionRequest, NormalizedPayload: NewGeneric("permission", nil)},
		{Type: EventTypeToolUpdate, NormalizedPayload: NewShellExec("cmd", "", "", 0, false)},
	} {
		require.Equal(t, event, BoundGenericToolEvent(event))
	}
}

func TestBoundedToolPreviewRemainsStable(t *testing.T) {
	deep := any("leaf")
	for range 17 {
		deep = []any{deep}
	}
	for name, value := range map[string]any{"deep": deep, "nodes": make([]any, toolValueNodes-1)} {
		t.Run(name, func(t *testing.T) {
			first := truncatedToolValue(value)
			for range 3 {
				require.Equal(t, first, boundToolValue(first))
			}
		})
	}
}

func TestBoundedToolContentOnlyPreview(t *testing.T) {
	event := AgentEvent{Type: EventTypeToolUpdate, NormalizedPayload: NewGeneric("tool", nil),
		ToolCallContents: []ToolCallContentItem{{Type: "content", Content: &ContentBlock{Type: "text", Text: strings.Repeat("result世", 500000)}}}}
	bounded := BoundGenericToolEvent(event)
	output := bounded.NormalizedPayload.Generic().Output.(map[string]any)
	require.Equal(t, true, output["truncated"])
	preview, ok := output["preview"].(string)
	require.True(t, ok, "retain a text preview of the content-only result")
	require.True(t, strings.HasPrefix(preview, "result世"))
	require.True(t, utf8.ValidString(preview))
	require.LessOrEqual(t, len(preview), toolPreviewBytes)
	require.Equal(t, bounded, BoundGenericToolEvent(bounded))
}

func TestBoundedToolFittingContentsAreDetached(t *testing.T) {
	oldText, size := "before", 42
	event := AgentEvent{Type: EventTypeToolUpdate, NormalizedPayload: NewGeneric("tool", nil),
		ToolCallContents: []ToolCallContentItem{{Type: "content", OldText: &oldText, Content: &ContentBlock{Type: "text", Text: "result", Size: &size}}}}
	bounded := BoundGenericToolEvent(event)
	before, err := json.Marshal(bounded)
	require.NoError(t, err)
	oldText, size = "changed", 100
	event.ToolCallContents[0].Content.Text = "changed"
	event.ToolCallContents[0].Type = "changed"
	after, err := json.Marshal(bounded)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
