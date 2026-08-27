package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

// TestHandleCreateTask_ExternalIDCrossWorkspaceDeniedByIdentityScope covers
// the spec's cross-workspace identity-scoping scenario: an in-session agent
// whose stream belongs to a task in workspace W must be denied when it
// targets a different workspace W2 with an external_id, and the denial must
// reveal nothing about W2 — not the task it holds, not its title, nothing
// beyond the standard not-found-shaped error. This drives the same
// handler->service path production uses, scoped via mcpscope.Resolver the
// way the lifecycle stream manager scopes it (see mcp_identity_scope_test.go
// for the pattern).
func TestHandleCreateTask_ExternalIDCrossWorkspaceDeniedByIdentityScope(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Workspace A owns the agent's own stream task.
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-a", Name: "Workspace A", OwnerID: "user-a", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-a", WorkspaceID: "ws-a", Name: "Board A", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-a-stream", WorkspaceID: "ws-a", WorkflowID: "wf-a",
		Title: "Stream owner task", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))

	// Workspace B, owned by a different user, already holds a settled task
	// under the external_id the agent is about to try.
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-b", Name: "Workspace B", OwnerID: "user-b", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-b", WorkspaceID: "ws-b", Name: "Board B", CreatedAt: now, UpdatedAt: now,
	}))
	bResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-b", WorkflowID: "wf-b", Title: "B's secret task", ExternalID: "ext-cross",
	})
	require.NoError(t, err)
	ok, err := repo.SettleTaskExternalID(ctx, bResult.Task.ID, "ext-cross", now)
	require.NoError(t, err)
	require.True(t, ok)

	identities := stubIdentityLookup{
		"user-a": {UserID: "user-a", Role: authn.RoleMember},
		"user-b": {UserID: "user-b", Role: authn.RoleMember},
	}
	resolver := mcpscope.NewResolver(repo, identities, func() bool { return true }, testLogger(t))
	scopedCtx, err := resolver.Scope(ctx, "task-a-stream")
	require.NoError(t, err)

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	resp, err := h.handleCreateTask(scopedCtx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     "ws-b",
		"workflow_id":      "wf-b",
		"title":            "Trying to reach B",
		"description":      "should be denied",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"external_id":      "ext-cross",
	}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resp.Type, "user A's agent must not reach user B's workspace via external_id")
	assert.NotContains(t, string(resp.Payload), bResult.Task.ID, "the denial must not leak B's task id")
	assert.NotContains(t, string(resp.Payload), "B's secret task", "the denial must not leak B's task title")

	tasksInB, err := repo.ListTasks(ctx, "wf-b")
	require.NoError(t, err)
	require.Len(t, tasksInB, 1, "the denied call must not have created a second task in B's workspace")
}
