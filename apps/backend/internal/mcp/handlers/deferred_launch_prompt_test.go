package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const pendingLaunchTaskID = "task-pending-launch"

func seedPendingLaunchTask(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: pendingLaunchTaskID, WorkspaceID: "ws-1", WorkflowID: "wf-1",
		Title: "Chain step", Description: "original description",
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent": "start", "agent_profile_id": "profile-1",
				"prompt": "stale brief",
				models.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}))
}

func updateTaskHandler(t *testing.T, svc *service.Service) *Handlers {
	t.Helper()
	return &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
}

func launchPrompt(t *testing.T, repo *sqliterepo.Repository) interface{} {
	t.Helper()
	task, err := repo.GetTask(context.Background(), pendingLaunchTaskID)
	require.NoError(t, err)
	launch, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "task must still carry a deferred launch record")
	return launch["prompt"]
}

// The bug 4 fix reaching the tool: update_task_kandev can now correct a pending
// launch prompt, which previously had no editable surface at all.
func TestHandleUpdateTask_RewritesThePendingLaunchPrompt(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedPendingLaunchTask(t, repo)
	h := updateTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id":                pendingLaunchTaskID,
		"deferred_launch_prompt": "here is what the work actually learned",
	})
	resp, err := h.handleUpdateTask(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	assert.Equal(t, "here is what the work actually learned", launchPrompt(t, repo))
}

// A rejected prompt edit aborts the whole call. Half-applying it would report
// success for a task whose stale brief is still what will run.
func TestHandleUpdateTask_RejectedLaunchPromptAppliesNoOtherField(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedPendingLaunchTask(t, repo)
	ctx := context.Background()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-1", TaskID: pendingLaunchTaskID,
		AgentProfileID: "profile-1", State: models.TaskSessionStateRunning, IsPrimary: true,
	}))
	h := updateTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id":                pendingLaunchTaskID,
		"description":            "a new description",
		"deferred_launch_prompt": "too late",
	})
	resp, err := h.handleUpdateTask(ctx, msg)
	require.NoError(t, err)

	payload := errorPayload(t, resp)
	assert.Equal(t, ws.ErrorCodeConflict, payload.Code)
	assert.Contains(t, payload.Message, "already started")
	assert.Contains(t, payload.Message, "message_task_kandev")

	assert.Equal(t, "stale brief", launchPrompt(t, repo))
	task, err := repo.GetTask(ctx, pendingLaunchTaskID)
	require.NoError(t, err)
	assert.Equal(t, "original description", task.Description,
		"a rejected prompt edit must not half-apply the rest of the call")
}

// A task that never had a deferred launch is a different mistake from one that
// already started, and gets its own message.
func TestHandleUpdateTask_LaunchPromptOnATaskWithoutOne(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: pendingLaunchTaskID, WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Ordinary",
	}))
	h := updateTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id":                pendingLaunchTaskID,
		"deferred_launch_prompt": "hello",
	})
	resp, err := h.handleUpdateTask(ctx, msg)
	require.NoError(t, err)

	payload := errorPayload(t, resp)
	assert.Equal(t, ws.ErrorCodeConflict, payload.Code)
	assert.Contains(t, payload.Message, "no pending deferred launch")
}

// Omitting the field leaves the pending launch alone — the ordinary
// update_task_kandev call must not disturb it.
func TestHandleUpdateTask_WithoutTheFieldLeavesTheLaunchAlone(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedPendingLaunchTask(t, repo)
	h := updateTaskHandler(t, svc)

	msg := makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id": pendingLaunchTaskID,
		"title":   "Renamed",
	})
	resp, err := h.handleUpdateTask(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	assert.Equal(t, "stale brief", launchPrompt(t, repo))
}
