package process

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
func TestBoundedToolResultContinuesToTerminal(t *testing.T) {
	for _, status := range []string{"complete", "error", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			manager := newIncrementalShellManager(t)
			original := strings.Repeat("\x00世", 1<<19)
			payload := streams.NewMCPTool("github.fetch", map[string]any{"url": "/actions/runs"})
			payload.Generic().Output = map[string]any{"content": []any{map[string]any{"type": "text", "text": original}}}
			updates := make(chan adapter.AgentEvent, 3)
			updates <- adapter.AgentEvent{
				Type: streams.EventTypeToolUpdate, ToolCallID: "fetch", ParentToolCallID: "parent", ToolStatus: status,
				NormalizedPayload: payload,
				ToolCallContents:  []streams.ToolCallContentItem{{Type: "content", Content: &streams.ContentBlock{Type: "text", Text: original}}},
			}
			updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "continued"}
			updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete}
			close(updates)
			manager.wg.Add(1)
			manager.forwardUpdates(&stubAdapter{updatesCh: updates}, make(chan struct{}))

			events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
			require.NoError(t, err)
			require.Len(t, events, 3, "oversized output must not stop forwarding subsequent events")
			var retained streams.AgentEvent
			require.NoError(t, json.Unmarshal(events[0].Payload, &retained))
			require.Equal(t, status, retained.ToolStatus)
			require.Equal(t, "fetch", retained.ToolCallID)
			require.Equal(t, "parent", retained.ParentToolCallID)
			require.Equal(t, true, retained.NormalizedPayload.Generic().Output.(map[string]any)["truncated"])
			require.Less(t, len(events[0].Payload), int(deliverylimits.DisplayBytes)+1)
			require.Equal(t, adapter.EventTypeComplete, events[2].Type)
			require.True(t, events[2].Terminal)
			require.NotEqual(t, deliveryStorageError, manager.DeliveryHealth().State)
			require.Equal(t, original, payload.Generic().Output.(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
		})
	}
}
