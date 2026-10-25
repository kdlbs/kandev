package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleArchiveTask_ScheduledAutomationSelfArchiveSucceeds pins that
// validateAutomationArchiveTarget's binding check, scoped to
// TriggerTypeGitHubPRMerged callers, does not block a scheduled automation
// from archiving its own hidden task at this second guard.
func TestHandleArchiveTask_ScheduledAutomationSelfArchiveSucceeds(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-self", Name: "Self"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-self", WorkspaceID: "ws-self", Name: "Board"}))
	caller := &models.Task{
		ID: "automation-run", WorkspaceID: "ws-self", WorkflowID: "wf-self",
		Title: "Scheduled run", State: v1.TaskStateTODO,
		Origin:   models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{"trigger_type": "scheduled"},
	}
	require.NoError(t, repo.CreateTask(ctx, caller))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]string{
		"task_id": caller.ID, "caller_task_id": caller.ID,
	})
	resp, err := h.handleArchiveTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	archived, err := svc.GetTask(ctx, caller.ID)
	require.NoError(t, err)
	assert.NotNil(t, archived.ArchivedAt)
}

// TestHandleArchiveTask_MergedPRRunRejectsSelfArchive pins that a
// github_pr_merged automation cannot archive its own run task in place of
// its bound PR target. The binding check in validateAutomationArchiveTarget
// is scoped to TriggerTypeGitHubPRMerged callers, so the run task ID never
// equals the expected PR target and the guard rejects it.
func TestHandleArchiveTask_MergedPRRunRejectsSelfArchive(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-self-pr", Name: "Self PR"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-self-pr", WorkspaceID: "ws-self-pr", Name: "Board"}))
	prTarget := &models.Task{
		ID: "pr-target", WorkspaceID: "ws-self-pr", WorkflowID: "wf-self-pr",
		Title: "PR task", State: v1.TaskStateTODO,
	}
	require.NoError(t, repo.CreateTask(ctx, prTarget))
	caller := &models.Task{
		ID: "automation-run", WorkspaceID: "ws-self-pr", WorkflowID: "wf-self-pr",
		Title: "PR merged run", State: v1.TaskStateTODO,
		Origin: models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			"trigger_type":                       "github_pr_merged",
			models.MetaKeyAutomationTargetTaskID: prTarget.ID,
		},
	}
	require.NoError(t, repo.CreateTask(ctx, caller))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]string{
		"task_id": caller.ID, "caller_task_id": caller.ID,
	})
	resp, err := h.handleArchiveTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	unchanged, err := svc.GetTask(ctx, caller.ID)
	require.NoError(t, err)
	assert.Nil(t, unchanged.ArchivedAt, "pr_merged run must not self-archive when bound to a different target")
}
