package acp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeExecute_BackgroundFlag covers the two ways an execute/bash tool
// call signals background work at initial-normalize time: Claude's
// run_in_background:true and the older wait:false shape. Foreground calls
// (wait:true, or neither field) must stay Background:false.
func TestNormalizeExecute_BackgroundFlag(t *testing.T) {
	n := NewNormalizer("")
	cases := []struct {
		name     string
		rawInput map[string]any
		want     bool
	}{
		{"run_in_background true", map[string]any{"command": "sleep 30", "run_in_background": true}, true},
		{"run_in_background false", map[string]any{"command": "ls", "run_in_background": false}, false},
		{"wait false", map[string]any{"command": "sleep 30", "wait": false}, true},
		{"wait true", map[string]any{"command": "ls", "wait": true}, false},
		{"neither field", map[string]any{"command": "ls"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := n.NormalizeToolCall("execute", map[string]any{
				"kind":      "execute",
				"raw_input": tc.rawInput,
			})
			se := payload.ShellExec()
			require.NotNil(t, se, "expected a ShellExec payload")
			require.Equal(t, tc.want, se.Background)
		})
	}
}

// TestUpdateShellExecInput_BackgroundFromUpdate reproduces the exact captured
// Claude wire shape for a background Bash: an initial tool_call arrives with an
// empty rawInput (Background:false), then a tool_call_update carries the
// command plus run_in_background:true. The merge path must honor the flag so
// ShellExec.Background flips to true — otherwise the orchestrator never sees
// the session as "waiting on background" and locks the operator out.
func TestUpdateShellExecInput_BackgroundFromUpdate(t *testing.T) {
	n := NewNormalizer("")

	// Initial tool_call: kind=execute, empty rawInput.
	payload := n.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{},
	})
	require.NotNil(t, payload.ShellExec())
	require.False(t, payload.ShellExec().Background, "initial empty tool_call must not be background yet")

	// tool_call_update: command + run_in_background:true.
	n.UpdatePayloadInput(payload, map[string]any{
		"command":           "npm run dev",
		"run_in_background": true,
	}, nil)

	require.Equal(t, "npm run dev", payload.ShellExec().Command)
	require.True(t, payload.ShellExec().Background, "run_in_background:true in a tool_call_update must set Background")
}

// TestUpdateShellExecInput_ForegroundUpdateDoesNotClearBackground guards the
// merge invariant: once an earlier frame established Background:true, a later
// update that omits the flag (or carries wait:true) must not clear it.
func TestUpdateShellExecInput_ForegroundUpdateDoesNotClearBackground(t *testing.T) {
	n := NewNormalizer("")
	payload := n.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"run_in_background": true},
	})
	require.True(t, payload.ShellExec().Background)

	// A later update with no background signal must leave Background:true.
	n.UpdatePayloadInput(payload, map[string]any{"cwd": "/repo"}, nil)
	require.True(t, payload.ShellExec().Background, "a later foreground update must not clear an established background flag")
}
