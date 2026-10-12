package handlers

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// newTestCoordinatorService builds a real, minimally-wired *coordinator.Service
// backed by its own in-memory SQLite store, so that RegisterHandlers
// registers coordinator.ActionProposeTask exactly as production does when
// features.coordinator is on. No proposal deps are set: these tests only
// reach the guard, never coordinator.Service.ProposeTask itself.
func newTestCoordinatorService(t *testing.T) *coordinator.Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	validator := coordinator.NewValidator(nil, nil)
	return coordinator.NewService(store, validator, nil, testLogger(t))
}

// newCoordinatorGuardTestHandlers builds a *Handlers wired the way production
// does when features.coordinator is on: a real taskSvc (for the reference-
// field checks) and a real coordinatorSvc (so ActionProposeTask registers).
func newCoordinatorGuardTestHandlers(t *testing.T) (*Handlers, *sqliterepo.Repository) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	h.SetCoordinatorService(newTestCoordinatorService(t))
	return h, repo
}

func coordinatorTestPrincipal(workspaceID string) mcpscope.Principal {
	return mcpscope.Principal{
		CoordinatorID:   "coordinator-1",
		WorkspaceID:     workspaceID,
		CallerTaskID:    "coordinator-conversation-task",
		CallerSessionID: "coordinator-session",
		Surface:         mcpprofile.SurfaceCoordinator,
	}
}

// TestAuthorizeCoordinatorRequest_TableOverEveryRegisteredAction is the
// completeness table the work order requires: for a coordinator principal,
// every action RegisterHandlers actually registers is refused unless it is
// one of the six coordinatorSurfaceActions, so a newly added action defaults
// to denied rather than silently open to a coordinator session.
func TestAuthorizeCoordinatorRequest_TableOverEveryRegisteredAction(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	actions := dispatcher.Actions()
	require.NotEmpty(t, actions)
	require.Contains(t, actions, coordinator.ActionProposeTask,
		"fixture must register propose_task, or this table cannot cover it")

	ctx := context.Background()
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	principalCtx := mcpscope.WithPrincipal(ctx, coordinatorTestPrincipal(workspaces[0].ID))

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			msg := makeWSMessage(t, action, map[string]interface{}{})
			guarded, replacement, err := h.authorizeCoordinatorRequest(principalCtx, msg)
			require.NoError(t, err)
			_, allowed := coordinatorSurfaceActions[action]
			if allowed {
				require.Nil(t, guarded, "allowlisted action %q must not be guarded", action)
				require.NotNil(t, replacement)
			} else {
				assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
			}
		})
	}
}

// TestAuthorizeCoordinatorRequest_ForeignWorkspaceIDRefused covers
// list_workflows_kandev and list_repositories_kandev, whose only scope is a
// client-supplied workspace_id: a value other than the principal's own is
// refused before the handler runs.
func TestAuthorizeCoordinatorRequest_ForeignWorkspaceIDRefused(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	ctx := context.Background()
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	principalCtx := mcpscope.WithPrincipal(ctx, coordinatorTestPrincipal(workspaces[0].ID))

	for _, action := range []string{ws.ActionMCPListWorkflows, ws.ActionMCPListRepositories} {
		t.Run(action, func(t *testing.T) {
			msg := makeWSMessage(t, action, map[string]interface{}{"workspace_id": "some-other-workspace"})
			guarded, _, err := h.authorizeCoordinatorRequest(principalCtx, msg)
			require.NoError(t, err)
			assertWSError(t, guarded, ws.ErrorCodeNotFound)
		})
	}
}

// TestAuthorizeCoordinatorRequest_OwnWorkspaceIDAllowed proves the same two
// tools are not refused when workspace_id matches the principal's own.
func TestAuthorizeCoordinatorRequest_OwnWorkspaceIDAllowed(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	ctx := context.Background()
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workspaceID := workspaces[0].ID
	principalCtx := mcpscope.WithPrincipal(ctx, coordinatorTestPrincipal(workspaceID))

	for _, action := range []string{ws.ActionMCPListWorkflows, ws.ActionMCPListRepositories} {
		t.Run(action, func(t *testing.T) {
			msg := makeWSMessage(t, action, map[string]interface{}{"workspace_id": workspaceID})
			guarded, replacement, err := h.authorizeCoordinatorRequest(principalCtx, msg)
			require.NoError(t, err)
			require.Nil(t, guarded)
			require.NotNil(t, replacement)
		})
	}
}

// TestAuthorizeCoordinatorRequest_ForeignWorkflowOrTaskRefused proves
// list_workflow_steps_kandev / list_tasks_kandev's workflow_id and
// get_task_conversation_kandev's task_id must resolve inside the
// coordinator's own workspace.
func TestAuthorizeCoordinatorRequest_ForeignWorkflowOrTaskRefused(t *testing.T) {
	h, repo := newCoordinatorGuardTestHandlers(t)
	ctx := context.Background()
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	homeWorkspaceID := workspaces[0].ID

	now := time.Now().UTC()
	foreignWorkspace := &models.Workspace{ID: "ws-foreign-coordinator", Name: "Foreign", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkspace(ctx, foreignWorkspace))
	foreignWorkflow := &models.Workflow{ID: "wf-foreign-coordinator", WorkspaceID: foreignWorkspace.ID, Name: "Foreign workflow"}
	require.NoError(t, repo.CreateWorkflow(ctx, foreignWorkflow))
	foreignTask := &models.Task{
		ID: "task-foreign-coordinator", WorkspaceID: foreignWorkspace.ID, Title: "Foreign task",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTask(ctx, foreignTask))

	principalCtx := mcpscope.WithPrincipal(ctx, coordinatorTestPrincipal(homeWorkspaceID))

	t.Run(ws.ActionMCPListWorkflowSteps, func(t *testing.T) {
		msg := makeWSMessage(t, ws.ActionMCPListWorkflowSteps, map[string]interface{}{"workflow_id": foreignWorkflow.ID})
		guarded, _, err := h.authorizeCoordinatorRequest(principalCtx, msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeNotFound)
	})
	t.Run(ws.ActionMCPListTasks, func(t *testing.T) {
		msg := makeWSMessage(t, ws.ActionMCPListTasks, map[string]interface{}{"workflow_id": foreignWorkflow.ID})
		guarded, _, err := h.authorizeCoordinatorRequest(principalCtx, msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeNotFound)
	})
	t.Run(ws.ActionMCPGetTaskConversation, func(t *testing.T) {
		msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{"task_id": foreignTask.ID})
		guarded, _, err := h.authorizeCoordinatorRequest(principalCtx, msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeNotFound)
	})
}

// TestAuthorizeCoordinatorRequest_UnknownWorkflowOrTaskRefused proves an id
// naming no entity reads as not found for a coordinator, the same as another
// workspace's id (a page chip's id may name either).
func TestAuthorizeCoordinatorRequest_UnknownWorkflowOrTaskRefused(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	ctx := context.Background()
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	principalCtx := mcpscope.WithPrincipal(ctx, coordinatorTestPrincipal(workspaces[0].ID))

	cases := map[string]map[string]interface{}{
		ws.ActionMCPListWorkflowSteps:   {"workflow_id": "wf-does-not-exist"},
		ws.ActionMCPListTasks:           {"workflow_id": "wf-does-not-exist"},
		ws.ActionMCPGetTaskConversation: {"task_id": "task-does-not-exist"},
	}
	for action, payload := range cases {
		t.Run(action, func(t *testing.T) {
			msg := makeWSMessage(t, action, payload)
			guarded, _, err := h.authorizeCoordinatorRequest(principalCtx, msg)
			require.NoError(t, err)
			assertWSError(t, guarded, ws.ErrorCodeNotFound)
		})
	}
}

// TestAuthorizeCoordinatorRequest_ProposeTaskRequiresCoordinatorPrincipal
// covers the guard's other half: coordinator.propose_task from a principal
// that is not a coordinator is refused, naming the action as unknown rather
// than exposing that the action exists at all.
func TestAuthorizeCoordinatorRequest_ProposeTaskRequiresCoordinatorPrincipal(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	msg := makeWSMessage(t, coordinator.ActionProposeTask, map[string]interface{}{})

	t.Run("no principal", func(t *testing.T) {
		guarded, _, err := h.authorizeCoordinatorRequest(context.Background(), msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	})

	t.Run("kanban principal", func(t *testing.T) {
		ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
			WorkspaceID:     "ws-1",
			CallerTaskID:    "kanban-task",
			CallerSessionID: "kanban-session",
			Surface:         mcpprofile.SurfaceKanbanTask,
		})
		guarded, _, err := h.authorizeCoordinatorRequest(ctx, msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	})

	t.Run("automation principal", func(t *testing.T) {
		ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
			AutomationID:    "automation-1",
			WorkspaceID:     "ws-1",
			CallerTaskID:    "automation-task",
			CallerSessionID: "automation-session",
			Surface:         mcpprofile.SurfaceAutomation,
		})
		guarded, _, err := h.authorizeCoordinatorRequest(ctx, msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	})
}

// TestAuthorizeCoordinatorRequest_GetItemRequiresCoordinatorPrincipal covers
// coordinator.get_item's other half, mirroring propose_task
// (copilot-tools.md#tool-surface): from a principal that is not a
// coordinator, it is refused as unknown rather than exposing that the action
// exists at all.
func TestAuthorizeCoordinatorRequest_GetItemRequiresCoordinatorPrincipal(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	msg := makeWSMessage(t, coordinator.ActionGetItem, map[string]interface{}{"kind": "proposal", "id": "prop-1"})

	t.Run("no principal", func(t *testing.T) {
		guarded, _, err := h.authorizeCoordinatorRequest(context.Background(), msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	})

	t.Run("kanban principal", func(t *testing.T) {
		ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
			WorkspaceID:     "ws-1",
			CallerTaskID:    "kanban-task",
			CallerSessionID: "kanban-session",
			Surface:         mcpprofile.SurfaceKanbanTask,
		})
		guarded, _, err := h.authorizeCoordinatorRequest(ctx, msg)
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	})
}

// TestAuthorizeCoordinatorRequest_ReservedDecisionNamesNeverReachable covers
// the guard's three-layer defense over approve and reject
// (proposals.md#security, AC-COORDINATOR-PROPOSALS-002.15): the reserved
// names are refused by the guard itself, before the propose check and before
// the allowlist, for a coordinator principal and for an unresolved (no)
// principal, and never appear in the allowlist or in the registered action
// set RegisterHandlers actually builds — nothing registers a handler for
// them, so an ordinary principal reaching the dispatcher gets the same
// unknown-action answer through a different path.
func TestAuthorizeCoordinatorRequest_ReservedDecisionNamesNeverReachable(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	actions := dispatcher.Actions()

	for _, reserved := range []string{coordinator.ActionApproveProposal, coordinator.ActionRejectProposal} {
		if _, ok := coordinatorSurfaceActions[reserved]; ok {
			t.Errorf("%q must never be in coordinatorSurfaceActions", reserved)
		}
		if slices.Contains(actions, reserved) {
			t.Errorf("%q must never be a registered action", reserved)
		}

		t.Run(reserved+"/coordinator principal", func(t *testing.T) {
			ctx := context.Background()
			workspaces, err := h.taskSvc.ListWorkspaces(ctx)
			require.NoError(t, err)
			require.Len(t, workspaces, 1)
			principalCtx := mcpscope.WithPrincipal(ctx, coordinatorTestPrincipal(workspaces[0].ID))
			msg := makeWSMessage(t, reserved, map[string]interface{}{})
			guarded, _, err := h.authorizeCoordinatorRequest(principalCtx, msg)
			require.NoError(t, err)
			assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
		})

		t.Run(reserved+"/no principal", func(t *testing.T) {
			msg := makeWSMessage(t, reserved, map[string]interface{}{})
			guarded, _, err := h.authorizeCoordinatorRequest(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
		})

		t.Run(reserved+"/ordinary principal passes through the guard untouched", func(t *testing.T) {
			ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
				WorkspaceID:     "ws-1",
				CallerTaskID:    "kanban-task",
				CallerSessionID: "kanban-session",
				Surface:         mcpprofile.SurfaceKanbanTask,
			})
			msg := makeWSMessage(t, reserved, map[string]interface{}{})
			guarded, replacement, err := h.authorizeCoordinatorRequest(ctx, msg)
			require.NoError(t, err)
			require.Nil(t, guarded)
			require.Same(t, msg, replacement)
		})
	}
}

// TestAuthorizeCoordinatorRequest_NonCoordinatorPrincipalUnaffected proves
// the guard only applies to the coordinator surface: for any other action, a
// kanban-task principal (or no principal at all) passes through untouched.
func TestAuthorizeCoordinatorRequest_NonCoordinatorPrincipalUnaffected(t *testing.T) {
	h, _ := newCoordinatorGuardTestHandlers(t)
	msg := makeWSMessage(t, ws.ActionMCPListWorkflows, map[string]interface{}{"workspace_id": "any-workspace"})

	t.Run("no principal", func(t *testing.T) {
		guarded, replacement, err := h.authorizeCoordinatorRequest(context.Background(), msg)
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
		guarded, replacement, err := h.authorizeCoordinatorRequest(ctx, msg)
		require.NoError(t, err)
		require.Nil(t, guarded)
		require.Same(t, msg, replacement)
	})
}
