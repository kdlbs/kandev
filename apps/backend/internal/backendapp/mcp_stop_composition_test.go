package backendapp

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/orchestrator"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestRegisterMCPAndDebugRoutesWiresCoordinatorTaskStopper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	parent, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Stop coordinator",
	})
	require.NoError(t, err)
	childID, err := harness.taskSvc.CreateChildTask(ctx, parent, taskservice.ChildTaskSpec{
		Title: "Unneeded child",
	})
	require.NoError(t, err)

	log, err := logger.NewLogger(logger.LoggingConfig{
		Level: "error", Format: "console", OutputPath: "stdout",
	})
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	taskRepoAdapter := &taskRepositoryAdapter{repo: harness.taskRepo, svc: harness.taskSvc}
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, nil,
		taskRepoAdapter, harness.taskRepo, nil, nil, nil, log,
	)
	lifecycleMgr := lifecycle.NewManager(
		nil, eventBus, nil, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log,
	)
	registerMCPStopTestCleanup(t, "lifecycle manager", lifecycleMgr.Stop)
	gateway := gateways.NewGateway(log)
	registerMCPAndDebugRoutes(routeParams{
		router:          gin.New(),
		gateway:         gateway,
		taskSvc:         harness.taskSvc,
		taskRepo:        harness.taskRepo,
		orchestratorSvc: orchestratorSvc,
		lifecycleMgr:    lifecycleMgr,
		eventBus:        eventBus,
		services:        &Services{Workflow: harness.workflowSvc},
		log:             log,
		addCleanup: func(cleanup func() error) {
			registerMCPStopTestCleanup(t, "MCP server", cleanup)
		},
	}, nil, nil, nil, nil, nil)

	require.True(t, gateway.Dispatcher.HasHandler(ws.ActionMCPStopTask),
		"registerMCPAndDebugRoutes did not register mcp.stop_task")
	request, err := ws.NewRequest("stop-1", ws.ActionMCPStopTask, map[string]string{
		"task_id":        childID,
		"sender_task_id": parent.ID,
	})
	require.NoError(t, err)
	response, err := gateway.Dispatcher.Dispatch(ctx, request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, "payload=%s", response.Payload)
	var payload struct {
		TaskID string                                 `json:"task_id"`
		Status orchestrator.CoordinatorTaskStopStatus `json:"status"`
	}
	require.NoError(t, response.ParsePayload(&payload))
	require.Equal(t, childID, payload.TaskID)
	require.Equal(t, orchestrator.CoordinatorTaskStopStatusNotRunning, payload.Status)
}

func registerMCPStopTestCleanup(t *testing.T, name string, cleanup func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("stop %s: %v", name, err)
		}
	})
}
