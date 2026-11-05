package codexappserver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

func nextCodexEvent(t *testing.T, adapter *Adapter) streams.AgentEvent {
	t.Helper()
	select {
	case event := <-adapter.Updates():
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an adapter event")
		return streams.AgentEvent{}
	}
}

// TestCodexSubagentFramesCarryNativeCapabilities pins that the native adapter's
// own verified declaration travels on the normalized frame. Without it the
// shared viewer's live row has no Interrupt action after the initial snapshot.
func TestCodexSubagentFramesCarryNativeCapabilities(t *testing.T) {
	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()

	adapter.emitCollabToolCall(map[string]any{
		"id":                "call-child",
		"tool":              "explore",
		"prompt":            "find files",
		"receiverThreadIds": []any{"thread-child"},
	}, "thread-root", "turn-1", false)

	started := nextCodexEvent(t, adapter)
	require.NotNil(t, started.NormalizedPayload)
	capabilities := started.NormalizedPayload.BackgroundWork().Capabilities
	require.NotNil(t, capabilities, "live child frame must carry the native declaration")
	require.True(t, capabilities.Actions[streams.WorkloadActionInterrupt].Supported)
	require.True(t, capabilities.Actions[streams.WorkloadActionInterrupt].Available)
	require.False(t, capabilities.Actions[streams.WorkloadActionStop].Supported)

	adapter.emitSubagentActivity(map[string]any{
		"agentThreadId": "thread-child",
		"kind":          "running",
	})
	var updated streams.AgentEvent
	for i := 0; i < 5; i++ {
		updated = nextCodexEvent(t, adapter)
		if updated.NormalizedPayload != nil && updated.NormalizedPayload.BackgroundWork() != nil {
			break
		}
	}
	require.NotNil(t, updated.NormalizedPayload)
	require.NotNil(t, updated.NormalizedPayload.BackgroundWork())
	require.NotNil(t, updated.NormalizedPayload.BackgroundWork().Capabilities)
}
