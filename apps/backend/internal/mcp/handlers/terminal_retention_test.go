package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestUpdateTaskTerminalRetentionRequiresDirectParent(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	for _, task := range []*models.Task{
		{ID: "retention-parent", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, Title: "Parent"},
		{ID: "retention-sibling", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, Title: "Sibling"},
		{ID: "retention-child", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, ParentID: "retention-parent", Title: "Child"},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	call := func(caller, target string) *ws.Message {
		t.Helper()
		principalCtx := scope.WithPrincipal(ctx, scope.Principal{WorkspaceID: workspaces[0].ID, CallerTaskID: caller, CallerSessionID: "session"})
		resp, err := h.handleUpdateTask(principalCtx, makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
			"task_id": target, "terminal_retention": true,
		}))
		require.NoError(t, err)
		return resp
	}
	for _, caller := range []string{"retention-child", "retention-sibling"} {
		resp := call(caller, "retention-child")
		require.Equal(t, ws.MessageTypeError, resp.Type)
	}
	resp := call("retention-parent", "retention-child")
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	child, err := repo.GetTask(ctx, "retention-child")
	require.NoError(t, err)
	require.True(t, models.IsTerminalRetentionHeld(child.Metadata))
}
