package process

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func newAllToolManager(t *testing.T, eventLimit int64) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	store, err := journal.Open(journal.Config{Path: path, MaxEventBytes: eventLimit})
	require.NoError(t, err)
	manager := &Manager{
		cfg: &config.InstanceConfig{
			DurableJournalPath: path, SessionID: "session",
			DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1,
		},
		logger:          newTestLogger(t),
		deliveryJournal: store,
		deliveryLimit:   eventLimit,
	}
	t.Cleanup(func() { require.NoError(t, manager.closeDeliveryJournal()) })
	return manager
}

func incidentSearchEvent(status, text string) adapter.AgentEvent {
	payload := streams.NewCodeSearch("needle", "pattern", "/repo", "*.go")
	payload.CodeSearch().Output = &streams.CodeSearchOutput{Files: []string{"a.go", "b.go"}, FileCount: 2}
	return adapter.AgentEvent{
		Type: streams.EventTypeToolUpdate, ToolCallID: "search", ParentToolCallID: "parent",
		ToolStatus: status, NormalizedPayload: payload,
		ToolCallContents: []streams.ToolCallContentItem{
			{Type: "content", Content: &streams.ContentBlock{Type: "text", Text: text}},
		},
	}
}

func forwardAllToolUpdates(manager *Manager, events ...adapter.AgentEvent) {
	updates := make(chan adapter.AgentEvent, len(events))
	for _, event := range events {
		updates <- event
	}
	close(updates)
	manager.wg.Add(1)
	manager.forwardUpdates(&stubAdapter{updatesCh: updates}, make(chan struct{}))
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.2
func TestAllToolDeliverySearchContinuesToTerminal(t *testing.T) {
	const incidentBytes = 1_048_607
	text := strings.Repeat("match line\n", (incidentBytes/11)+1)[:incidentBytes]
	require.Len(t, text, incidentBytes)

	t.Run("default limit retains incident output intact", func(t *testing.T) {
		manager := newAllToolManager(t, deliverylimits.MaxEventBytes)
		forwardAllToolUpdates(manager,
			incidentSearchEvent("complete", text),
			adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "continued"},
			adapter.AgentEvent{Type: adapter.EventTypeComplete},
		)
		events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 3, "oversized typed output must not stop forwarding")
		var retained streams.AgentEvent
		require.NoError(t, json.Unmarshal(events[0].Payload, &retained))
		require.Equal(t, "complete", retained.ToolStatus)
		require.Equal(t, "search", retained.ToolCallID)
		require.Len(t, retained.ToolCallContents, 1)
		require.Len(t, retained.ToolCallContents[0].Content.Text, incidentBytes, "the observed incident result must fit unchanged")
		require.NotNil(t, retained.NormalizedPayload.CodeSearch())
		require.Equal(t, 2, retained.NormalizedPayload.CodeSearch().Output.FileCount)
		require.Equal(t, adapter.EventTypeComplete, events[2].Type)
		require.True(t, events[2].Terminal)
		require.NotEqual(t, deliveryStorageError, manager.DeliveryHealth().State)
	})

	t.Run("reduced fixture bounds every status and still reaches terminal", func(t *testing.T) {
		for _, status := range []string{"complete", "error", "cancelled"} {
			t.Run(status, func(t *testing.T) {
				manager := newAllToolManager(t, 128<<10)
				forwardAllToolUpdates(manager,
					incidentSearchEvent(status, text),
					adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "continued"},
					adapter.AgentEvent{Type: adapter.EventTypeComplete},
				)
				events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
				require.NoError(t, err)
				require.Len(t, events, 3)
				require.Less(t, len(events[0].Payload), 128<<10)
				var retained streams.AgentEvent
				require.NoError(t, json.Unmarshal(events[0].Payload, &retained))
				require.Equal(t, status, retained.ToolStatus)
				require.NotNil(t, retained.NormalizedPayload.CodeSearch())
				require.True(t, retained.NormalizedPayload.CodeSearch().Output.FileCount == 2)
				require.Less(t, len(retained.ToolCallContents[0].Content.Text), incidentBytes)
				require.True(t, strings.Contains(retained.ToolCallContents[0].Content.Text, "truncated by Kandev"))
				require.Equal(t, adapter.EventTypeComplete, events[2].Type)
				require.NotEqual(t, deliveryStorageError, manager.DeliveryHealth().State)
			})
		}
	})
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-002.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-002.3
func TestAllToolDeliveryReplayHashCompatibility(t *testing.T) {
	manager := newAllToolManager(t, deliverylimits.MaxEventBytes)
	text := strings.Repeat("match line\n", 100_000)
	prepared, err := manager.persistDeliveryEvent(incidentSearchEvent("complete", text))
	require.NoError(t, err)
	require.NotEmpty(t, prepared.DeliverySequence)

	events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)

	var decoded streams.AgentEvent
	require.NoError(t, json.Unmarshal(events[0].Payload, &decoded))
	reencoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.Equal(t, events[0].Payload, reencoded, "known fields did not round-trip")
	require.Equal(t, events[0].Payload, mustMarshal(t, streams.PrepareToolEvent(decoded)), "replay re-preparation changed accepted bytes")

	// Restart the journal and replay again: accepted bytes and order remain.
	require.NoError(t, manager.closeDeliveryJournal())
	reopened, err := journal.Open(journal.Config{Path: manager.cfg.DurableJournalPath})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	restarted, _, err := reopened.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, restarted, 1)
	require.Equal(t, events[0].Payload, restarted[0].Payload)
	require.Equal(t, events[0].Sequence, restarted[0].Sequence)
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}
