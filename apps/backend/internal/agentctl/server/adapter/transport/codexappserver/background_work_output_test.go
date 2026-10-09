package codexappserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

func TestCodexOutputDeltaFinalReconciliation(t *testing.T) {
	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()

	adapter.mu.Lock()
	adapter.backgrounds["term-out-1"] = protocol.BackgroundTerminal{
		ItemID:    "term-out-1",
		ProcessID: "proc-out-1",
		Command:   "go test ./...",
		CWD:       "/repo",
	}
	adapter.mu.Unlock()

	var emittedEvents []streams.AgentEvent
	done := make(chan struct{})
	go func() {
		for ev := range adapter.Updates() {
			emittedEvents = append(emittedEvents, ev)
			if ev.Type == streams.EventTypeToolUpdate {
				close(done)
				return
			}
		}
	}()

	// 1. Initial commandExecution tool call
	adapter.emitCommandExecution(map[string]any{
		"id":      "term-out-1",
		"command": "go test ./...",
		"cwd":     "/repo",
		"status":  "running",
	}, "thread-out", "", false)

	// 2. Completed commandExecution with aggregatedOutput and exitCode
	adapter.emitCommandExecution(map[string]any{
		"id":               "term-out-1",
		"command":          "go test ./...",
		"cwd":              "/repo",
		"status":           "completed",
		"aggregatedOutput": "PASS\nok  test\n",
		"exitCode":         float64(0),
	}, "thread-out", "", true)

	<-done

	require.NotEmpty(t, emittedEvents)
	var finalToolUpdate *streams.AgentEvent
	for i := range emittedEvents {
		if emittedEvents[i].Type == streams.EventTypeToolUpdate && emittedEvents[i].ToolCallID == "term-out-1" {
			finalToolUpdate = &emittedEvents[i]
			break
		}
	}
	require.NotNil(t, finalToolUpdate)
	require.NotNil(t, finalToolUpdate.NormalizedPayload)
	require.NotNil(t, finalToolUpdate.NormalizedPayload.ShellExec())
	require.NotNil(t, finalToolUpdate.NormalizedPayload.ShellExec().Output)
	require.Equal(t, "PASS\nok  test\n", finalToolUpdate.NormalizedPayload.ShellExec().Output.Stdout)
	require.Equal(t, 0, *finalToolUpdate.NormalizedPayload.ShellExec().Output.ExitCode)
}

func TestCodexBackgroundCapabilities(t *testing.T) {
	termCaps := codexBackgroundTerminalCapabilities()
	require.Equal(t, "snapshot", termCaps.Discovery)
	require.Equal(t, "snapshot", termCaps.Output)
	require.False(t, termCaps.Transcript)
	require.False(t, termCaps.Parentage)
	require.True(t, termCaps.Actions[streams.WorkloadActionStop].Supported)
	require.True(t, termCaps.Actions[streams.WorkloadActionStop].Available)
	require.False(t, termCaps.Actions[streams.WorkloadActionWriteInput].Supported)

	subagentCaps := codexSubagentCapabilities()
	require.Equal(t, "events_only", subagentCaps.Discovery)
	require.Equal(t, "none", subagentCaps.Output)
	require.True(t, subagentCaps.Parentage)
	require.True(t, subagentCaps.AttributableUsage)
	require.True(t, subagentCaps.Actions[streams.WorkloadActionInterrupt].Supported)
	require.True(t, subagentCaps.Actions[streams.WorkloadActionInterrupt].Available)
	require.False(t, subagentCaps.Actions[streams.WorkloadActionStop].Supported)

	adapter := NewAdapter(&shared.Config{WorkDir: "/repo"}, logger.Default())
	defer func() { _ = adapter.Close() }()

	// Execute unsupported write_input
	resp, err := adapter.ExecuteBackgroundAction(context.Background(), streams.BackgroundWorkActionRequest{
		WorkID: "some-work",
		Action: streams.WorkloadActionWriteInput,
		Data:   "hello",
	}, "session-1")
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Contains(t, resp.Error, "unsupported")
}
