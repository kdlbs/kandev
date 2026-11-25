package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHandleDeleteTaskRejectsUnknownCleanupMode(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPDeleteTask, map[string]string{"task_id": "task", "workspace_cleanup": "purge"})
	response, err := h.handleDeleteTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
}

type mcpUnavailableDeleteCleanup struct{}

func (mcpUnavailableDeleteCleanup) OnTaskDeleted(context.Context, string) error { return nil }
func (mcpUnavailableDeleteCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return []*worktree.Worktree{{ID: "broken", TaskID: "task", Path: "/tasks/task/repo"}}, nil
}
func (mcpUnavailableDeleteCleanup) InspectDirtyWorktrees(context.Context, []*worktree.Worktree) ([]worktree.DirtyWorktree, error) {
	return nil, &worktree.CleanupInspectionError{Stage: worktree.CleanupInspectionStageStatus, Reason: worktree.CleanupInspectionReasonRepoUnavailable, Err: errors.New("bad marker")}
}
func (mcpUnavailableDeleteCleanup) CaptureCleanupHeadOIDs(context.Context, []*worktree.Worktree) (map[string]string, error) {
	return map[string]string{"broken": "1111111111111111111111111111111111111111"}, nil
}

func TestDeleteUnavailableMCPOutcome(t *testing.T) {
	_, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf", WorkspaceID: "ws", Name: "Workflow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Damaged", WorkspaceID: "ws", WorkflowID: "wf"}))
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo, ResourceCleanups: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo}, nil, testLogger(t), service.RepositoryDiscoveryConfig{})
	svc.SetWorktreeCleanup(mcpUnavailableDeleteCleanup{})
	t.Cleanup(svc.StopTaskResourceCleanupWorker)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	for _, ack := range []bool{false, true} {
		msg := makeWSMessage(t, ws.ActionMCPDeleteTask, map[string]any{"task_id": "task", "discard_worktree_changes": true, "allow_unverified_workspace_changes": ack})
		response, err := h.handleDeleteTask(ctx, msg)
		require.NoError(t, err)
		if !ack {
			assertWSError(t, response, ws.ErrorCodeInternalError)
			_, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			continue
		}
		require.Equal(t, ws.MessageTypeResponse, response.Type)
		var result map[string]any
		require.NoError(t, json.Unmarshal(response.Payload, &result))
		require.Equal(t, true, result["success"])
		require.Equal(t, "delete", result["workspace_cleanup"])
		require.Equal(t, true, result["workspace_cleanup_pending"])
		require.Equal(t, false, result["runtime_stop_pending"])
		require.Len(t, result["cleanup_operation_ids"], 1)
		_, err = repo.GetTask(ctx, "task")
		require.Error(t, err)
	}
}
