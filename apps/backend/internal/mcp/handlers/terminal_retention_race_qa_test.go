package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type reparentBeforeRetentionWrite struct {
	taskrepository.TaskRepository
	base *taskrepo.Repository
}

func (r *reparentBeforeRetentionWrite) UpdateTaskWithTerminalRetentionIfParent(
	ctx context.Context, task *models.Task, parentID, workspaceID string, held bool,
) (bool, error) {
	current, err := r.base.GetTask(ctx, task.ID)
	if err != nil {
		return false, err
	}
	current.ParentID = "new-parent"
	if err := r.base.UpdateTask(ctx, current); err != nil {
		return false, err
	}
	return r.base.UpdateTaskWithTerminalRetentionIfParent(ctx, task, parentID, workspaceID, held)
}

func TestUpdateTaskWithTerminalRetentionDoesNotApplyOrdinaryFieldsAfterConcurrentReparent(t *testing.T) {
	svc, repo, _, _ := newTestTaskServiceWithWorkflowTasks(t, func(base *taskrepo.Repository) taskrepository.TaskRepository {
		return &reparentBeforeRetentionWrite{TaskRepository: base, base: base}
	})
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	for _, task := range []*models.Task{
		{ID: "old-parent", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, Title: "Old"},
		{ID: "new-parent", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, Title: "New"},
		{ID: "child", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, ParentID: "old-parent", Title: "Child"},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	requestCtx := scope.WithPrincipal(ctx, scope.Principal{WorkspaceID: workspaces[0].ID, CallerTaskID: "old-parent", CallerSessionID: "session"})
	resp, err := h.handleUpdateTask(requestCtx, makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id": "child", "title": "Changed", "state": "DONE", "terminal_retention": true,
	}))
	require.NoError(t, err)
	child, err := repo.GetTask(ctx, "child")
	require.NoError(t, err)
	if child.ParentID != "new-parent" {
		t.Errorf("parent = %q, want new-parent", child.ParentID)
	}
	if models.IsTerminalRetentionHeld(child.Metadata) {
		t.Error("former parent set retention hold")
	}
	require.Equal(t, "Child", child.Title)
	require.NotEqual(t, "DONE", string(child.State))
	if resp.Type != ws.MessageTypeError {
		t.Errorf("response type = %q, want error", resp.Type)
	}
}
