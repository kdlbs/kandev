// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// OrchestratorTestServer extends TestServer with orchestrator components
type OrchestratorTestServer struct {
	Server          *httptest.Server
	Gateway         *gateways.Gateway
	TaskRepo        repository.Repository
	TaskSvc         *taskservice.Service
	WorkflowSvc     *workflowservice.Service
	EventBus        bus.EventBus
	OrchestratorSvc *orchestrator.Service
	AgentManager    *SimulatedAgentManagerClient
	Logger          *logger.Logger
	ctx             context.Context
	cancelFunc      context.CancelFunc
}

// taskRepositoryAdapter adapts the task repository for the orchestrator
type taskRepositoryAdapter struct {
	repo repository.Repository
	svc  *taskservice.Service
}

func (a *taskRepositoryAdapter) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	task, err := a.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

func (a *taskRepositoryAdapter) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	_, err := a.svc.UpdateTaskState(ctx, taskID, state)
	return err
}

// testMessageCreatorAdapter adapts the task service to the orchestrator.MessageCreator interface for tests
type testMessageCreatorAdapter struct {
	svc *taskservice.Service
}

func (a *testMessageCreatorAdapter) CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
	})
	return err
}

func (a *testMessageCreatorAdapter) CreateUserMessage(ctx context.Context, taskID, content, agentSessionID, turnID string, metadata map[string]interface{}) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	return err
}

func (a *testMessageCreatorAdapter) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, title, status, agentSessionID, turnID string, normalized *streams.NormalizedPayload) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	if parentToolCallID != "" {
		metadata["parent_tool_call_id"] = parentToolCallID
	}
	if normalized != nil {
		metadata["normalized"] = normalized
	}
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          "tool_call",
		Metadata:      metadata,
	})
	return err
}

func (a *testMessageCreatorAdapter) UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, status, result, agentSessionID, title, turnID, msgType string, normalized *streams.NormalizedPayload) error {
	return a.svc.UpdateToolCallMessageWithCreate(ctx, agentSessionID, toolCallID, parentToolCallID, status, result, title, normalized, taskID, turnID, msgType)
}

func (a *testMessageCreatorAdapter) CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
		Type:          messageType,
		Metadata:      metadata,
		RequestsInput: requestsInput,
	})
	return err
}

func (a *testMessageCreatorAdapter) CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, pendingID, toolCallID, title, turnID string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error) {
	metadata := map[string]interface{}{
		"pending_id":     pendingID,
		"tool_call_id":   toolCallID,
		"options":        options,
		"action_type":    actionType,
		"action_details": actionDetails,
	}
	msg, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          "permission_request",
		Metadata:      metadata,
	})
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

func (a *testMessageCreatorAdapter) UpdatePermissionMessage(ctx context.Context, sessionID, pendingID, status string) error {
	return a.svc.UpdatePermissionMessage(ctx, sessionID, pendingID, status)
}

func (a *testMessageCreatorAdapter) CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
		Type:          "content",
	})
	return err
}

func (a *testMessageCreatorAdapter) AppendAgentMessage(ctx context.Context, messageID, additionalContent string) error {
	return a.svc.AppendMessageContent(ctx, messageID, additionalContent)
}

func (a *testMessageCreatorAdapter) CreateThinkingMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       "",
		AuthorType:    "agent",
		Type:          "thinking",
		Metadata: map[string]interface{}{
			"thinking": content,
		},
	})
	return err
}

func (a *testMessageCreatorAdapter) AppendThinkingMessage(ctx context.Context, messageID, additionalContent string) error {
	return a.svc.AppendThinkingContent(ctx, messageID, additionalContent)
}

// testTurnServiceAdapter adapts the task service to the orchestrator.TurnService interface for tests
type testTurnServiceAdapter struct {
	svc *taskservice.Service
}

func (a *testTurnServiceAdapter) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.StartTurn(ctx, sessionID)
}

func (a *testTurnServiceAdapter) CompleteTurn(ctx context.Context, turnID string) error {
	return a.svc.CompleteTurn(ctx, turnID)
}

func (a *testTurnServiceAdapter) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.GetActiveTurn(ctx, sessionID)
}

// NewOrchestratorTestServer creates a test server with full orchestrator support
func NewOrchestratorTestServer(t *testing.T) *OrchestratorTestServer {
	t.Helper()

	// Initialize logger (quiet for tests)
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "console",
	})
	require.NoError(t, err)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize event bus
	eventBus := bus.NewMemoryEventBus(log)

	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	taskRepoImpl, cleanup, err := repository.Provide(sqlxDB, sqlxDB)
	require.NoError(t, err)
	taskRepo := taskRepoImpl
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if cleanup != nil {
			if err := cleanup(); err != nil {
				t.Errorf("failed to close task repo: %v", err)
			}
		}
	})
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}

	// Initialize workflow service
	_, workflowSvc, _, err := workflow.Provide(sqlxDB, sqlxDB, log)
	require.NoError(t, err)

	// Initialize task service and wire workflow step creator
	taskSvc := taskservice.NewService(taskRepo, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	taskSvc.SetWorkflowStepCreator(workflowSvc)

	// Create simulated agent manager
	agentManager := NewSimulatedAgentManager(eventBus, log)

	// Create task repository adapter
	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}

	// Create orchestrator service
	cfg := orchestrator.DefaultServiceConfig()
	cfg.Scheduler.ProcessInterval = 50 * time.Millisecond // Faster for tests
	orchestratorSvc := orchestrator.NewService(cfg, eventBus, agentManager, taskRepoAdapter, taskRepo, nil, log)

	// Wire message creator for message persistence (similar to cmd/kandev/orchestrator.go)
	msgCreator := &testMessageCreatorAdapter{svc: taskSvc}
	orchestratorSvc.SetMessageCreator(msgCreator)
	orchestratorSvc.SetTurnService(&testTurnServiceAdapter{svc: taskSvc})

	// Create WebSocket gateway
	gateway := gateways.NewGateway(log)

	// Register orchestrator handlers (Pattern A)
	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorSvc, log)
	orchestratorHandlers.RegisterHandlers(gateway.Dispatcher)

	// Start hub
	go gateway.Hub.Run(ctx)

	// Register task notifications to broadcast events to WebSocket clients
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)

	// Start orchestrator
	require.NoError(t, orchestratorSvc.Start(ctx))

	// Create router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	gateway.SetupRoutes(router)

	// Register handlers (HTTP + WS)
	workflowCtrl := workflowcontroller.NewController(workflowSvc)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, taskSvc, log)
	taskhandlers.RegisterWorkflowRoutes(router, gateway.Dispatcher, taskSvc, workflowSvc, log)
	planService := taskservice.NewPlanService(taskRepo, eventBus, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskSvc, nil, taskRepo, planService, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, taskSvc, log)
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, taskSvc, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, taskSvc, log)
	workflowhandlers.RegisterRoutes(router, gateway.Dispatcher, workflowCtrl, log)

	// Create test server
	server := httptest.NewServer(router)

	return &OrchestratorTestServer{
		Server:          server,
		Gateway:         gateway,
		TaskRepo:        taskRepo,
		TaskSvc:         taskSvc,
		WorkflowSvc:     workflowSvc,
		EventBus:        eventBus,
		OrchestratorSvc: orchestratorSvc,
		AgentManager:    agentManager,
		Logger:          log,
		ctx:             ctx,
		cancelFunc:      cancel,
	}
}

// Close shuts down the test server
func (ts *OrchestratorTestServer) Close() {
	if err := ts.OrchestratorSvc.Stop(); err != nil {
		ts.Logger.Warn("failed to stop orchestrator", zap.Error(err))
	}
	ts.AgentManager.Close()
	ts.cancelFunc()
	ts.Server.Close()
	if err := ts.TaskRepo.Close(); err != nil {
		ts.Logger.Warn("failed to close task repo", zap.Error(err))
	}
	ts.EventBus.Close()
}

// CreateTestTask creates a task for testing.
func (ts *OrchestratorTestServer) CreateTestTask(t *testing.T, agentProfileID string, priority int) string {
	t.Helper()

	workspace, err := ts.TaskSvc.CreateWorkspace(context.Background(), &taskservice.CreateWorkspaceRequest{
		Name: "Test Workspace",
	})
	require.NoError(t, err)

	// Create workflow first (workflow steps are created automatically via default template)
	defaultTemplateID := "simple"
	wf, err := ts.TaskSvc.CreateWorkflow(context.Background(), &taskservice.CreateWorkflowRequest{
		WorkspaceID:        workspace.ID,
		Name:               "Test Workflow",
		Description:        "Test workflow for orchestrator",
		WorkflowTemplateID: &defaultTemplateID,
	})
	require.NoError(t, err)

	// Get first workflow step from workflow
	steps, err := ts.WorkflowSvc.ListStepsByWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps, "workflow should have workflow steps")
	workflowStepID := steps[0].ID

	// Create task with agent profile ID
	repository, err := ts.TaskSvc.CreateRepository(context.Background(), &taskservice.CreateRepositoryRequest{
		WorkspaceID: workspace.ID,
		Name:        "Test Repo",
		LocalPath:   createTempRepoDir(t),
	})
	require.NoError(t, err)

	task, err := ts.TaskSvc.CreateTask(context.Background(), &taskservice.CreateTaskRequest{
		WorkspaceID:    workspace.ID,
		WorkflowID:     wf.ID,
		WorkflowStepID: workflowStepID,
		Title:          "Test Task",
		Description:    "This is a test task for the orchestrator",
		Priority:       priority,
		Repositories: []taskservice.TaskRepositoryInput{
			{
				RepositoryID: repository.ID,
				BaseBranch:   "main",
			},
		},
	})
	require.NoError(t, err)

	return task.ID
}
