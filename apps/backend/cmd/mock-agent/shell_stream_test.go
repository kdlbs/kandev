package main

import (
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestScriptShellStream(t *testing.T) {
	e, mock := newTestEmitter()
	executeScript(e, "", `e2e:shell_stream({"action":"start","id":"shell","command":"watch","seed_bytes":262144})
e2e:shell_stream({"action":"delta","id":"shell","delta":"\nnew\n"})
e2e:shell_stream({"action":"finish","id":"shell","stdout":"final","stderr":"error","exit_code":7})`)
	updates := mock.getUpdates()
	require.Len(t, updates, 4)
	require.Equal(t, acp.ToolKindExecute, updates[0].notification.Update.ToolCall.Kind)
	seed := updates[1].notification.Update.ToolCallUpdate
	require.Equal(t, strings.Repeat("x", 262144), seed.Meta["terminal_output_delta"].(map[string]any)["data"])
	delta := updates[2].notification.Update.ToolCallUpdate
	require.Equal(t, "\nnew\n", delta.Meta["terminal_output_delta"].(map[string]any)["data"])
	final := updates[3].notification.Update.ToolCallUpdate
	require.Equal(t, acp.ToolCallStatusCompleted, *final.Status)
	require.Equal(t, "final", final.RawOutput.(map[string]any)["stdout"])
}
