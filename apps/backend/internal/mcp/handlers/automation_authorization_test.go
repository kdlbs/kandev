package handlers

import (
	"context"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// TestAuthorizeAutomationRequest_SelfArchiveSucceeds is the regression guard
// for the Stall Session Watchdog defect: a hidden automation_run task
// archiving itself (its STEP 5 completion signal) must not be rejected by
// the automation self-target guard. Before the fix, ActionMCPArchiveTask was
// listed in automationSelfDeniedActions and this returned a NOT_FOUND guard
// response for every watchdog run, so its task was never archived.
func TestAuthorizeAutomationRequest_SelfArchiveSucceeds(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workspaceID := workspaces[0].ID

	now := time.Now().UTC()
	selfTask := &models.Task{
		ID:          "watchdog-self-task",
		WorkspaceID: workspaceID,
		Title:       "Watchdog run",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateTask(ctx, selfTask))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID:    "automation-watchdog",
		WorkspaceID:     workspaceID,
		CallerTaskID:    selfTask.ID,
		CallerSessionID: "automation-session",
		Surface:         mcpprofile.SurfaceAutomation,
	})
	msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]interface{}{
		"task_id": selfTask.ID,
	})

	guarded, replacement, err := h.authorizeAutomationRequest(ctx, msg)
	require.NoError(t, err)
	require.Nil(t, guarded, "self-archive must not be rejected by the automation guard")
	require.NotNil(t, replacement)
}

// TestAuthorizeAutomationRequest_SelfCoordinationActionsStillDenied proves
// the fix is scoped to archive only: a hidden automation task must still be
// blocked from messaging, stopping, or spawning a session on itself.
func TestAuthorizeAutomationRequest_SelfCoordinationActionsStillDenied(t *testing.T) {
	actions := []string{
		ws.ActionMCPMessageTask,
		ws.ActionMCPStopTask,
		ws.ActionMCPSpawnSession,
	}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			ctx := context.Background()
			workspaces, err := svc.ListWorkspaces(ctx)
			require.NoError(t, err)
			require.Len(t, workspaces, 1)
			workspaceID := workspaces[0].ID

			now := time.Now().UTC()
			selfTask := &models.Task{
				ID:          "watchdog-self-task-" + action,
				WorkspaceID: workspaceID,
				Title:       "Watchdog run",
				State:       v1.TaskStateInProgress,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, repo.CreateTask(ctx, selfTask))

			h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
			ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
				AutomationID:    "automation-watchdog",
				WorkspaceID:     workspaceID,
				CallerTaskID:    selfTask.ID,
				CallerSessionID: "automation-session",
				Surface:         mcpprofile.SurfaceAutomation,
			})
			msg := makeWSMessage(t, action, map[string]interface{}{
				"task_id": selfTask.ID,
			})

			guarded, _, err := h.authorizeAutomationRequest(ctx, msg)
			require.NoError(t, err)
			assertWSError(t, guarded, ws.ErrorCodeNotFound)
		})
	}
}

// TestAuthorizeAutomationRequest_CrossWorkspaceTaskStillDenied pins Defect
// 1's boundary (unrelated to this fix, and must not regress): an automation
// principal cannot target a task outside its own workspace, even for an
// action that is now allowed against itself.
func TestAuthorizeAutomationRequest_CrossWorkspaceTaskStillDenied(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	homeWorkspaceID := workspaces[0].ID

	now := time.Now().UTC()
	foreignWorkspace := &models.Workspace{
		ID:        "ws-foreign-automation",
		Name:      "Foreign",
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.CreateWorkspace(ctx, foreignWorkspace))
	foreignTask := &models.Task{
		ID:          "foreign-task",
		WorkspaceID: foreignWorkspace.ID,
		Title:       "Foreign task",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateTask(ctx, foreignTask))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID:    "automation-watchdog",
		WorkspaceID:     homeWorkspaceID,
		CallerTaskID:    "watchdog-self-task",
		CallerSessionID: "automation-session",
		Surface:         mcpprofile.SurfaceAutomation,
	})
	msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]interface{}{
		"task_id": foreignTask.ID,
	})

	guarded, _, err := h.authorizeAutomationRequest(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, guarded, ws.ErrorCodeNotFound)
}

// TestAuthorizeAutomationRequest_NonAutomationPrincipalUnaffected proves the
// guard only applies to the automation surface: a kanban-task principal (or
// no principal at all) passes through untouched, on the early-return path
// that never reaches automationSelfDeniedActions.
func TestAuthorizeAutomationRequest_NonAutomationPrincipalUnaffected(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPArchiveTask, map[string]interface{}{
		"task_id": "some-task",
	})

	t.Run("no principal", func(t *testing.T) {
		guarded, replacement, err := h.authorizeAutomationRequest(context.Background(), msg)
		require.NoError(t, err)
		require.Nil(t, guarded)
		require.Same(t, msg, replacement)
	})

	t.Run("kanban-task principal", func(t *testing.T) {
		ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
			WorkspaceID:     "ws-1",
			CallerTaskID:    "kanban-task",
			CallerSessionID: "kanban-session",
			Surface:         mcpprofile.SurfaceKanbanTask,
		})
		guarded, replacement, err := h.authorizeAutomationRequest(ctx, msg)
		require.NoError(t, err)
		require.Nil(t, guarded)
		require.Same(t, msg, replacement)
	})
}
