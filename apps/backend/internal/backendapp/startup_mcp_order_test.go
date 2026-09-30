package backendapp

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/retention"
	"github.com/kandev/kandev/internal/orchestrator"
	systemsvc "github.com/kandev/kandev/internal/system"
	systemstorage "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type discoveryCallbackExecutor struct {
	onRecover func(ctx context.Context, records []*models.ExecutorRunning)
}

func (e *discoveryCallbackExecutor) Name() executor.Name {
	return executor.NameStandalone
}

func (e *discoveryCallbackExecutor) HealthCheck(ctx context.Context) error {
	return nil
}

func (e *discoveryCallbackExecutor) CreateInstance(ctx context.Context, req *lifecycle.ExecutorCreateRequest) (*lifecycle.ExecutorInstance, error) {
	return nil, nil
}

func (e *discoveryCallbackExecutor) StopInstance(ctx context.Context, instance *lifecycle.ExecutorInstance, force bool) error {
	return nil
}

func (e *discoveryCallbackExecutor) RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*lifecycle.ExecutorInstance, error) {
	if e.onRecover != nil {
		e.onRecover(ctx, records)
	}
	return nil, nil
}

func (e *discoveryCallbackExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func (e *discoveryCallbackExecutor) RequiresCloneURL() bool {
	return false
}

func (e *discoveryCallbackExecutor) ShouldApplyPreferredShell() bool {
	return false
}

func (e *discoveryCallbackExecutor) IsAlwaysResumable() bool {
	return false
}

func hasRoute(routes gin.RoutesInfo, method, path string) bool {
	for _, r := range routes {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	return false
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1
// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.3
// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.8
func TestStartupMCPReadyBeforeRecoveryAndLaunch(t *testing.T) {
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
	taskResult, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Test recovery task",
	})
	require.NoError(t, err)
	testTask := taskResult.Task

	testSession := &models.TaskSession{
		ID:     "sess-test",
		TaskID: testTask.ID,
		State:  models.TaskSessionStateRunning,
	}
	require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, testSession))

	log := logger.Default()
	eventBus := bus.NewMemoryEventBus(log)
	registry := lifecycle.NewExecutorRegistry(log)
	var recoverySeenDispatcher bool
	var recoverySeenResponseAction string
	var recoverySeenResponseType ws.MessageType

	lifecycleMgr := lifecycle.NewManager(
		nil, eventBus, registry, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log,
	)
	t.Cleanup(func() { _ = lifecycleMgr.Stop() })

	execMock := &discoveryCallbackExecutor{
		onRecover: func(recoveryCtx context.Context, _ []*models.ExecutorRunning) {
			proxy := lifecycleMgr.MCPHandlerFor(&lifecycle.AgentExecution{
				ID:        "exec-test",
				TaskID:    testTask.ID,
				SessionID: "sess-test",
			})
			req := &ws.Message{
				ID:     "msg-discover",
				Action: ws.ActionMCPListWorkspaces,
				Type:   ws.MessageTypeRequest,
			}
			resp, err := proxy.Dispatch(recoveryCtx, req)
			if err == nil && resp != nil {
				recoverySeenDispatcher = true
				recoverySeenResponseAction = resp.Action
				recoverySeenResponseType = resp.Type
			}
		},
	}
	registry.Register(execMock)

	cfg := &config.Config{
		Features: config.FeaturesConfig{Office: true},
	}

	gateway := gateways.NewGateway(log)
	taskRepoAdapter := &taskRepositoryAdapter{repo: harness.taskRepo, svc: harness.taskSvc}
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, nil,
		taskRepoAdapter, harness.taskRepo, nil, nil, nil, log,
	)
	t.Cleanup(func() { _ = orchestratorSvc.Stop() })

	services := &Services{
		Task:     harness.taskSvc,
		Workflow: harness.workflowSvc,
		User:     harness.userSvc,
	}

	officeRepo, err := officesqlite.NewWithDB(harness.db, harness.db, nil)
	require.NoError(t, err)
	settingsRepo, cleanup, err := settingsstore.Provide(harness.db, harness.db, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	repos := &Repositories{
		Task:          harness.taskRepo,
		Office:        officeRepo,
		AgentSettings: settingsRepo,
	}

	// 1. Construct Office, storage, and retention dependencies
	runProcessorSvc, ok := constructOfficeServices(
		ctx, cfg, log, repos,
		services, orchestratorSvc, eventBus, "", func(fn func() error) { t.Cleanup(func() { _ = fn() }) },
		lifecycleMgr, nil,
	)
	require.True(t, ok)

	dbPool := db.NewPool(harness.db, harness.db)

	systemSvc := systemsvc.Provide(cfg, log, dbPool, eventBus, systemsvc.BuildInfo{
		Version: Version, Commit: Commit, BuildTime: BuildTime,
	}, systemsvc.Wiring{
		TaskSessions: harness.taskRepo,
	})
	systemSvc.Storage = &systemstorage.Handler{}
	services.Retention = retention.NewRuntime(dbPool, nil, func(string, error) {})

	agentSettingsCtrl := agentsettingscontroller.NewController(repos.AgentSettings, nil, nil, nil, log)

	// 2. Build HTTP server and register routes BEFORE recovery
	server, err := buildHTTPServer(
		cfg, log, gateway, repos,
		services, agentSettingsCtrl, lifecycleMgr, eventBus, orchestratorSvc,
		nil, nil, nil, nil, nil,
		func(fn func() error) { t.Cleanup(func() { _ = fn() }) },
		nil, systemSvc, nil, nil, dbPool, nil, nil, nil,
	)
	require.NoError(t, err)
	router, ok := server.Handler.(*gin.Engine)
	require.True(t, ok)

	// Assert Office, storage, and retention routes are mounted
	routes := router.Routes()
	require.True(t, hasRoute(routes, http.MethodGet, "/api/v1/office/onboarding-state"), "office route should be mounted")
	require.True(t, hasRoute(routes, http.MethodGet, "/api/v1/system/storage"), "storage route should be mounted")
	require.True(t, hasRoute(routes, http.MethodGet, "/api/v1/system/retention"), "retention route should be mounted")

	// 3. Run session recovery - callback dispatches MCP tool request through proxy
	err = lifecycleMgr.Start(ctx)
	require.NoError(t, err)
	require.True(t, recoverySeenDispatcher, "recovery callback should access fully wired MCP dispatcher")
	require.Equal(t, ws.MessageTypeResponse, recoverySeenResponseType)
	require.Equal(t, ws.ActionMCPListWorkspaces, recoverySeenResponseAction)

	// 4. Run Office activation
	activated := activateOfficeServices(ctx, cfg, repos, services, eventBus, log)
	require.True(t, activated)
	require.NotNil(t, runProcessorSvc)

	// 5. Run first-launch execution MCP dispatch through proxy
	launchProxy := lifecycleMgr.MCPHandlerFor(&lifecycle.AgentExecution{
		ID:        "exec-launch",
		TaskID:    testTask.ID,
		SessionID: "sess-test",
	})
	launchReq := &ws.Message{
		ID:     "msg-launch-discover",
		Action: ws.ActionMCPListWorkspaces,
		Type:   ws.MessageTypeRequest,
	}
	launchResp, err := launchProxy.Dispatch(ctx, launchReq)
	require.NoError(t, err)
	require.NotNil(t, launchResp)
	require.Equal(t, ws.MessageTypeResponse, launchResp.Type)
	require.Equal(t, ws.ActionMCPListWorkspaces, launchResp.Action)
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.8
func TestStartupOfficeDisabled_MountsStorageAndRetention(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	log := logger.Default()
	eventBus := bus.NewMemoryEventBus(log)
	registry := lifecycle.NewExecutorRegistry(log)

	lifecycleMgr := lifecycle.NewManager(
		nil, eventBus, registry, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log,
	)
	t.Cleanup(func() { _ = lifecycleMgr.Stop() })

	cfg := &config.Config{
		Features: config.FeaturesConfig{Office: false},
	}

	gateway := gateways.NewGateway(log)
	taskRepoAdapter := &taskRepositoryAdapter{repo: harness.taskRepo, svc: harness.taskSvc}
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, nil,
		taskRepoAdapter, harness.taskRepo, nil, nil, nil, log,
	)
	t.Cleanup(func() { _ = orchestratorSvc.Stop() })

	services := &Services{
		Task:     harness.taskSvc,
		Workflow: harness.workflowSvc,
		User:     harness.userSvc,
	}

	settingsRepo, cleanup, err := settingsstore.Provide(harness.db, harness.db, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	repos := &Repositories{
		Task:          harness.taskRepo,
		AgentSettings: settingsRepo,
	}

	_, ok := constructOfficeServices(
		ctx, cfg, log, repos,
		services, orchestratorSvc, eventBus, "", func(fn func() error) { t.Cleanup(func() { _ = fn() }) },
		lifecycleMgr, nil,
	)
	require.True(t, ok)

	dbPool := db.NewPool(harness.db, harness.db)

	systemSvc := systemsvc.Provide(cfg, log, dbPool, eventBus, systemsvc.BuildInfo{
		Version: Version, Commit: Commit, BuildTime: BuildTime,
	}, systemsvc.Wiring{
		TaskSessions: harness.taskRepo,
	})
	systemSvc.Storage = &systemstorage.Handler{}
	services.Retention = retention.NewRuntime(dbPool, nil, func(string, error) {})

	agentSettingsCtrl := agentsettingscontroller.NewController(repos.AgentSettings, nil, nil, nil, log)

	server, err := buildHTTPServer(
		cfg, log, gateway, repos,
		services, agentSettingsCtrl, lifecycleMgr, eventBus, orchestratorSvc,
		nil, nil, nil, nil, nil,
		func(fn func() error) { t.Cleanup(func() { _ = fn() }) },
		nil, systemSvc, nil, nil, dbPool, nil, nil, nil,
	)
	require.NoError(t, err)
	router, ok := server.Handler.(*gin.Engine)
	require.True(t, ok)

	routes := router.Routes()
	require.False(t, hasRoute(routes, http.MethodGet, "/api/v1/office/onboarding-state"), "office route should not be mounted when disabled")
	require.True(t, hasRoute(routes, http.MethodGet, "/api/v1/system/storage"), "storage route should be mounted")
	require.True(t, hasRoute(routes, http.MethodGet, "/api/v1/system/retention"), "retention route should be mounted")
}
