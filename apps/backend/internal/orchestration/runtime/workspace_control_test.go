package runtime

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func workspaceControlCaller(t *testing.T, s *Service, task string) (*gin.Engine, string, string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "control", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, workspaceCoordinatorAudience, run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT("chief", task, "ws", run.ID, "session", workspaceCoordinatorAudience)
	require.NoError(t, err)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	return router, token, run.ID
}

func TestWorkspaceOrchestratorCreatesDeliveryWithoutPrivateSetup(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, run := workspaceControlCaller(t, s, task)
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, run,
		map[string]any{"title": "Synthetic feature", "description": "Implement a sample feature", "execution_mode": "execute", "assignee": "personal"})
	require.Equal(t, 201, response.Code, response.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
	require.Equal(t, "execute", manager.lastSpec.ExecutionMode)
	require.Equal(t, "ws", manager.lastSpec.WorkspaceID)
	require.NoError(t, s.Runs.FinishRun(context.Background(), run, "finished", nil))
	require.Equal(t, 403, runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, run, map[string]string{"title": "Stale"}).Code)
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestWorkspaceOrchestratorDiscoveryWithoutPrivateSetup(t *testing.T) {
	s, _, task := newRuntime(t)
	router, token, run := workspaceControlCaller(t, s, task)
	for _, path := range []string{"capabilities", "memory"} {
		response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/"+path, token, run, nil)
		require.Equal(t, 200, response.Code, response.Body.String())
		if path == "capabilities" {
			require.Contains(t, response.Body.String(), "create_task")
			require.Contains(t, response.Body.String(), "delete")
			require.NotContains(t, response.Body.String(), "create_objective")
		}
	}
}

type workspaceCommandManager struct {
	*assistantTaskManager
	commands []models.WorkspaceTaskCommand
}

func (m *workspaceCommandManager) ManageWorkspaceTask(_ context.Context, command models.WorkspaceTaskCommand) error {
	m.commands = append(m.commands, command)
	return nil
}
func TestWorkspaceControlUsesSignedScope(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &workspaceCommandManager{assistantTaskManager: &assistantTaskManager{}}
	s.Manager = manager
	router, token, run := workspaceControlCaller(t, s, task)
	for _, action := range []string{"edit", "move", "assign", "adopt", "start", "stop", "message", "archive", "delete"} {
		response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks/target/manage", token, run,
			map[string]any{"action": action, "WorkspaceID": "foreign", "ChiefID": "foreign", "TaskID": "foreign", "title": "Synthetic edit", "workflow_step_id": "progress"})
		require.Equal(t, 200, response.Code, response.Body.String())
		got := manager.commands[len(manager.commands)-1]
		require.Equal(t, "ws", got.WorkspaceID)
		require.Equal(t, "chief", got.ChiefID)
		require.Equal(t, "target", got.TaskID)
		require.True(t, got.DirectProfile)
	}
	require.NoError(t, s.Runs.FinishRun(context.Background(), run, "finished", nil))
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks/target/manage", token, run, map[string]string{"action": "delete"})
	require.Equal(t, 403, response.Code)
	require.Len(t, manager.commands, 9)
}

func TestPrivateTaskDeletionDoesNotCreateObjectiveLink(t *testing.T) {
	s, db, task := newRuntime(t)
	s.Manager = &assistantTaskManager{}
	router, token, run := assistantRuntimeCaller(t, s, task)
	body := assistantDeliveryRequest(t, s, db, task, router, token, run, "delete-example")
	body["action"] = "delete"
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks/created-task/manage", token, run, body)
	require.Equal(t, 200, response.Code, response.Body.String())
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM orchestration_objective_tasks WHERE task_id='created-task'"))
	require.Zero(t, count, "deleting a task must not add a delivery link")
}
