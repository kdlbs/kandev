package mcp

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeleteTaskForwardsRetainMode(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"success": true}}
	s := newTestServer(t, backend)
	result := callTool(t, s, "delete_task_kandev", map[string]interface{}{"task_id": "damaged", "workspace_cleanup": "retain"})
	require.False(t, result.IsError)
	payload, err := json.Marshal(backend.lastPayload)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(payload, &fields))
	require.Equal(t, "retain", fields["workspace_cleanup"])
}

func TestDeleteTaskForwardsUnverifiedConsent(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"success": true}}
	s := newTestServer(t, backend)
	result := callTool(t, s, "delete_task_kandev", map[string]interface{}{"task_id": "damaged", "discard_worktree_changes": true, "allow_unverified_workspace_changes": true})
	require.False(t, result.IsError)
	payload, err := json.Marshal(backend.lastPayload)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(payload, &fields))
	require.Equal(t, true, fields["discard_worktree_changes"])
	require.Equal(t, true, fields["allow_unverified_workspace_changes"])
}
