package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleCreateTask_ExternalIDWithDefaultWorkspaceDedupes covers the MCP
// scenario where the caller omits workspace_id entirely and it auto-resolves
// to the instance's single workspace: a retry against a settled task holding
// that identity must still dedupe correctly through the auto-resolved
// workspace, not just an explicitly-passed one.
func TestHandleCreateTask_ExternalIDWithDefaultWorkspaceDedupes(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1, "this scenario requires exactly one workspace to exercise auto-resolution")
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	first, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workflow_id":      workflows[0].ID,
		"title":            "Task",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	if first.Type == ws.MessageTypeError {
		t.Fatalf("first create returned error: %s", string(first.Payload))
	}
	var firstResult map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Payload, &firstResult))
	firstID := firstResult["id"].(string)

	retry, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workflow_id":      workflows[0].ID,
		"title":            "Retry",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-1",
	}))
	require.NoError(t, err)
	if retry.Type == ws.MessageTypeError {
		t.Fatalf("retry returned error: %s", string(retry.Payload))
	}
	var retryResult map[string]interface{}
	require.NoError(t, json.Unmarshal(retry.Payload, &retryResult))
	require.Equal(t, firstID, retryResult["id"])
	require.Equal(t, true, retryResult["deduplicated"])
}
