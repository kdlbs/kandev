package acp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShellOutputCompactEligibility(t *testing.T) {
	a := newTestAdapter()
	t.Cleanup(func() { require.NoError(t, a.Close()) })
	seedSnapshotTool(a, "shell")
	a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("live")), 1)
	first := drainEvents(a)
	require.Len(t, first, 1)
	require.True(t, first[0].ShellOutputEligible)
	final := shellSnapshotNotification(t, "shell", nil)
	final.Update.ToolCallUpdate.RawOutput = map[string]any{"stdout": "final", "exit_code": 0}
	a.handleACPUpdate(final, 1)
	last := drainEvents(a)
	require.Len(t, last, 1)
	require.True(t, last[0].ShellOutputEligible)
}
