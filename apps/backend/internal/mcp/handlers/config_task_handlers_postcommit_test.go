package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type mcpPostCommitCleanupFailure struct {
	err error
}

func (c *mcpPostCommitCleanupFailure) CleanupTaskResources(context.Context, string, bool) {}
func (c *mcpPostCommitCleanupFailure) PrepareTaskResourceCleanup(
	context.Context,
	string,
	models.TaskResourceCleanupTrigger,
	string,
	bool,
) error {
	return nil
}
func (c *mcpPostCommitCleanupFailure) StartPreparedTaskResourceCleanup(context.Context, string) error {
	return c.err
}
func (c *mcpPostCommitCleanupFailure) CancelPreparedTaskResourceCleanup(context.Context, string) error {
	return nil
}

func TestMCPTaskLifecycleReturnsPendingAfterPostCommitHousekeepingFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Handlers, context.Context, *ws.Message) (*ws.Message, error)
	}{
		{name: "delete", call: (*Handlers).handleDeleteTask},
		{name: "archive", call: (*Handlers).handleArchiveTask},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			ctx := context.Background()
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-post-commit", Name: "Post commit"}))
			require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
				ID: "wf-post-commit", WorkspaceID: "ws-post-commit", Name: "Board",
			}))
			taskID := "task-post-commit-" + tc.name
			require.NoError(t, repo.CreateTask(ctx, &models.Task{
				ID: taskID, WorkspaceID: "ws-post-commit", WorkflowID: "wf-post-commit",
				Title: "Lifecycle", State: v1.TaskStateTODO,
			}))
			handoff := service.NewHandoffService(repo, repo, nil, nil, nil, testLogger(t))
			handoff.SetTaskResourceCleaner(&mcpPostCommitCleanupFailure{
				err: errors.New("cleanup unavailable"),
			})
			h := &Handlers{taskSvc: svc, handoffSvc: handoff, logger: testLogger(t).WithFields()}
			msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]string{"task_id": taskID})
			if tc.name == "delete" {
				msg = makeWSMessage(t, ws.ActionMCPDeleteTask, map[string]string{"task_id": taskID})
			}

			resp, err := tc.call(h, ctx, msg)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type)
			require.JSONEq(t, `{"success":false,"pending":true,"task_id":"`+taskID+`"}`, string(resp.Payload))
		})
	}
}
