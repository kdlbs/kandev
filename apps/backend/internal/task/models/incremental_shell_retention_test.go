package models

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIncrementalShellRetentionReduction(t *testing.T) {
	metadata := map[string]any{"tool_call_id": "tool", "shell_output_storage": 1, "normalized": map[string]any{"kind": "shell_exec", "shell_exec": map[string]any{"command": "cmd", "output": map[string]any{"has_output": true, "stdout_bytes": 8192, "stderr_bytes": 0, "exit_code": 0}}}}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	reduced, err := ReduceToolPayload("tool_execute", raw, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Empty(t, reduced.Reason)
	var after map[string]any
	require.NoError(t, json.Unmarshal(reduced.Metadata, &after))
	require.True(t, ToolPayloadRemoved(after))
	require.NotContains(t, after, "shell_output_storage")
	require.Greater(t, reduced.RemovedBytes, int64(7000))
}

func TestIncrementalShellProjectionHidesStorageReference(t *testing.T) {
	metadata := map[string]any{ShellOutputStorageKey: 1, "normalized": map[string]any{"shell_exec": map[string]any{"output": map[string]any{"has_output": true, "stdout_bytes": 7, "stderr_bytes": 0}}}}
	projected := ProjectMessageMetadata(metadata)
	require.NotContains(t, projected, ShellOutputStorageKey)
	require.Contains(t, metadata, ShellOutputStorageKey)
}

func TestIncrementalShellRetentionRemovesSmallStoredOutputs(t *testing.T) {
	for _, bytes := range []int{0, 7} {
		t.Run(strconv.Itoa(bytes), func(t *testing.T) {
			metadata := map[string]any{ShellOutputStorageKey: 1, "normalized": map[string]any{"kind": "shell_exec", "shell_exec": map[string]any{"command": "cmd", "output": map[string]any{"has_output": bytes > 0, "stdout_bytes": bytes, "stderr_bytes": 0}}}}
			raw, err := json.Marshal(metadata)
			require.NoError(t, err)
			reduced, err := ReduceToolPayload("tool_execute", raw, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.Empty(t, reduced.Reason, "removing retained output must not depend on receipt size")
			var after map[string]any
			require.NoError(t, json.Unmarshal(reduced.Metadata, &after))
			require.True(t, ToolPayloadRemoved(after))
			require.NotContains(t, after, ShellOutputStorageKey)
			require.Zero(t, reduced.RemovedBytes, "a larger tombstone does not reclaim bytes")
		})
	}
}
