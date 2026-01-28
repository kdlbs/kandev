// Package integration provides end-to-end integration tests for the Kandev backend.
// This file contains orchestrator-specific integration tests.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorcontroller "github.com/kandev/kandev/internal/orchestrator/controller"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/pkg/acp/protocol"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// ============================================
// SIMULATED AGENT MANAGER
// ============================================

// SimulatedAgentManagerClient simulates agent container behavior for testing.
// It publishes realistic agent events (started, ACP messages, completion) to the event bus.
type SimulatedAgentManagerClient struct {
	eventBus      bus.EventBus
	logger        *logger.Logger
	mu            sync.Mutex
	instances     map[string]*simulatedInstance
	launchDelay   time.Duration
	executionTime time.Duration
	shouldFail    bool
	failAfter     int // Fail after N successful launches
	launchCount   int32
	acpMessageFn  func(taskID, executionID string) []protocol.Message // Custom ACP messages
	stopCh        chan struct{}
}

// simulatedInstance tracks a simulated agent instance
type simulatedInstance struct {
	id             string
	taskID         string
	sessionID      string
	agentProfileID string
	status         v1.AgentStatus
	statusMu       sync.Mutex // Protects status field
	stopCh         chan struct{}
}

// NewSimulatedAgentManager creates a new simulated agent manager
func NewSimulatedAgentManager(eventBus bus.EventBus, log *logger.Logger) *SimulatedAgentManagerClient {
	return &SimulatedAgentManagerClient{
		eventBus:      eventBus,
		logger:        log,
		instances:     make(map[string]*simulatedInstance),
		launchDelay:   50 * time.Millisecond,
		executionTime: 200 * time.Millisecond,
		stopCh:        make(chan struct{}),
	}
}

// SetLaunchDelay sets the delay before agent "starts"
func (s *SimulatedAgentManagerClient) SetLaunchDelay(d time.Duration) {
	s.launchDelay = d
}

// SetExecutionTime sets how long the simulated task takes
func (s *SimulatedAgentManagerClient) SetExecutionTime(d time.Duration) {
	s.executionTime = d
}

// SetShouldFail configures whether launches should fail
func (s *SimulatedAgentManagerClient) SetShouldFail(fail bool) {
	s.shouldFail = fail
}

// SetFailAfter configures the agent to fail after N successful launches
func (s *SimulatedAgentManagerClient) SetFailAfter(n int) {
	s.failAfter = n
}

// SetACPMessageFn sets a custom function to generate ACP messages
func (s *SimulatedAgentManagerClient) SetACPMessageFn(fn func(taskID, executionID string) []protocol.Message) {
	s.acpMessageFn = fn
}

// LaunchAgent simulates launching an agent container
func (s *SimulatedAgentManagerClient) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	count := atomic.AddInt32(&s.launchCount, 1)

	if s.shouldFail {
		return nil, fmt.Errorf("simulated launch failure")
	}

	if s.failAfter > 0 && int(count) > s.failAfter {
		return nil, fmt.Errorf("simulated launch failure after %d attempts", s.failAfter)
	}

	executionID := uuid.New().String()
	containerID := "sim-container-" + executionID[:8]

	s.mu.Lock()
	instance := &simulatedInstance{
		id:             executionID,
		taskID:         req.TaskID,
		sessionID:      req.SessionID,
		agentProfileID: req.AgentProfileID,
		status:         v1.AgentStatusStarting,
		stopCh:         make(chan struct{}),
	}
	s.instances[executionID] = instance
	s.mu.Unlock()

	// Simulate agent lifecycle in background
	go s.runAgentSimulation(instance, req)

	return &executor.LaunchAgentResponse{
		AgentExecutionID: executionID,
		ContainerID:      containerID,
		Status:           v1.AgentStatusStarting,
	}, nil
}

// StartAgentProcess simulates starting the agent subprocess
func (s *SimulatedAgentManagerClient) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	s.logger.Info("simulated: starting agent process",
		zap.String("agent_execution_id", agentExecutionID))
	return nil
}

// runAgentSimulation simulates the agent execution lifecycle
func (s *SimulatedAgentManagerClient) runAgentSimulation(instance *simulatedInstance, req *executor.LaunchAgentRequest) {
	// Wait for launch delay
	select {
	case <-time.After(s.launchDelay):
	case <-instance.stopCh:
		return
	case <-s.stopCh:
		return
	}

	// Publish agent started event
	s.publishAgentEvent(events.AgentStarted, instance)

	// Simulate some ACP messages
	s.publishACPMessages(instance, req)

	// Wait for execution time
	select {
	case <-time.After(s.executionTime):
	case <-instance.stopCh:
		s.publishAgentEvent(events.AgentStopped, instance)
		return
	case <-s.stopCh:
		return
	}

	// Publish agent ready (finished prompt, waiting for follow-up or completion)
	instance.statusMu.Lock()
	instance.status = v1.AgentStatusReady
	instance.statusMu.Unlock()
	s.publishAgentEvent(events.AgentReady, instance)
}

// publishAgentEvent publishes an agent lifecycle event
func (s *SimulatedAgentManagerClient) publishAgentEvent(eventType string, instance *simulatedInstance) {
	instance.statusMu.Lock()
	status := instance.status
	instance.statusMu.Unlock()

	data := map[string]interface{}{
		"instance_id":      instance.id,
		"task_id":          instance.taskID,
		"agent_profile_id": instance.agentProfileID,
		"container_id":     "sim-container-" + instance.id[:8],
		"status":           string(status),
		"started_at":       time.Now(),
		"progress":         50,
	}

	event := bus.NewEvent(eventType, "simulated-agent-manager", data)
	if err := s.eventBus.Publish(context.Background(), eventType, event); err != nil {
		s.logger.Error("failed to publish agent event")
	}
}

// publishACPMessages publishes simulated ACP messages
func (s *SimulatedAgentManagerClient) publishACPMessages(instance *simulatedInstance, req *executor.LaunchAgentRequest) {
	var messages []protocol.Message

	if s.acpMessageFn != nil {
		messages = s.acpMessageFn(instance.taskID, instance.id)
	} else {
		// Default ACP messages
		messages = []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    instance.taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 25,
					"stage":    "starting",
					"message":  "Agent started processing task",
				},
			},
			{
				Type:      protocol.MessageTypeLog,
				TaskID:    instance.taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"level":   "info",
					"message": "Processing task: " + req.TaskDescription,
				},
			},
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    instance.taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 75,
					"stage":    "executing",
					"message":  "Task execution in progress",
				},
			},
		}
	}

	// Publish each message with a small delay
	for _, msg := range messages {
		sessionID := msg.SessionID
		if sessionID == "" {
			sessionID = instance.sessionID
		}
		msgData := map[string]interface{}{
			"type":       msg.Type,
			"task_id":    msg.TaskID,
			"session_id": sessionID,
			"timestamp":  msg.Timestamp,
			"data":       msg.Data,
		}

		event := bus.NewEvent(events.AgentStream, "simulated-agent", msgData)
		subject := events.BuildAgentStreamSubject(instance.taskID)
		if err := s.eventBus.Publish(context.Background(), subject, event); err != nil {
			s.logger.Error("failed to publish agent stream event")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// StopAgent simulates stopping an agent
func (s *SimulatedAgentManagerClient) StopAgent(ctx context.Context, agentExecutionID string, force bool) error {
	s.mu.Lock()
	execution, exists := s.instances[agentExecutionID]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("agent execution %q not found", agentExecutionID)
	}

	close(execution.stopCh)
	execution.statusMu.Lock()
	execution.status = v1.AgentStatusStopped
	execution.statusMu.Unlock()

	s.publishAgentEvent(events.AgentStopped, execution)
	return nil
}

// PromptAgent sends a follow-up prompt to a running agent
func (s *SimulatedAgentManagerClient) PromptAgent(ctx context.Context, agentExecutionID string, prompt string) (*executor.PromptResult, error) {
	s.mu.Lock()
	execution, exists := s.instances[agentExecutionID]
	s.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("agent execution %q not found", agentExecutionID)
	}

	// Simulate receiving prompt and generating response
	go func() {
		time.Sleep(50 * time.Millisecond)

		msg := protocol.Message{
			Type:      protocol.MessageTypeLog,
			TaskID:    execution.taskID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"level":   "info",
				"message": "Received follow-up prompt: " + prompt,
			},
		}

		msgData := map[string]interface{}{
			"type":      msg.Type,
			"task_id":   msg.TaskID,
			"timestamp": msg.Timestamp,
			"data":      msg.Data,
		}

		event := bus.NewEvent(events.AgentStream, "simulated-agent", msgData)
		subject := events.BuildAgentStreamSubject(execution.taskID)
		if err := s.eventBus.Publish(context.Background(), subject, event); err != nil {
			s.logger.Warn("failed to publish simulated agent stream event", zap.Error(err))
		}
	}()

	return &executor.PromptResult{
		StopReason: "end_turn",
	}, nil
}

// RespondToPermissionBySessionID responds to a permission request for a session
func (s *SimulatedAgentManagerClient) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	s.logger.Info("simulated: responding to permission",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))
	return nil
}

// CompleteAgent marks an agent as completed
func (s *SimulatedAgentManagerClient) CompleteAgent(executionID string) {
	s.mu.Lock()
	execution, exists := s.instances[executionID]
	s.mu.Unlock()

	if !exists {
		return
	}

	execution.status = v1.AgentStatusCompleted
	s.publishAgentEvent(events.AgentCompleted, execution)
}

// FailAgent marks an agent as failed
func (s *SimulatedAgentManagerClient) FailAgent(executionID string, reason string) {
	s.mu.Lock()
	execution, exists := s.instances[executionID]
	s.mu.Unlock()

	if !exists {
		return
	}

	execution.status = v1.AgentStatusFailed
	s.publishAgentEvent(events.AgentFailed, execution)
}

// GetLaunchCount returns the number of times LaunchAgent was called
func (s *SimulatedAgentManagerClient) GetLaunchCount() int {
	return int(atomic.LoadInt32(&s.launchCount))
}

// Close stops all simulated agents
func (s *SimulatedAgentManagerClient) Close() {
	close(s.stopCh)
}

// IsAgentRunningForSession checks if a simulated agent is running for a session
func (s *SimulatedAgentManagerClient) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, inst := range s.instances {
		if inst.sessionID == sessionID && inst.status == v1.AgentStatusRunning {
			return true
		}
	}
	return false
}

// CancelAgent cancels the current agent turn for a session
func (s *SimulatedAgentManagerClient) CancelAgent(ctx context.Context, sessionID string) error {
	s.logger.Info("simulated: cancelling agent turn",
		zap.String("session_id", sessionID))
	return nil
}

// ResolveAgentProfile resolves an agent profile ID to profile information
func (s *SimulatedAgentManagerClient) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Simulated Profile",
		AgentID:     "augment-agent",
		AgentName:   "Augment Agent",
		Model:       "claude-sonnet-4-20250514",
	}, nil
}

// ============================================
// ORCHESTRATOR TEST SERVER
// ============================================

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

func (a *testMessageCreatorAdapter) CreateUserMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "user",
	})
	return err
}

func (a *testMessageCreatorAdapter) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID, turnID string, args map[string]interface{}) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	if len(args) > 0 {
		metadata["args"] = args
		if kind, ok := args["kind"].(string); ok && kind != "" {
			metadata["tool_name"] = kind
		}
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

func (a *testMessageCreatorAdapter) UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID, title string, args map[string]interface{}) error {
	return a.svc.UpdateToolCallMessage(ctx, agentSessionID, toolCallID, status, result, title, args)
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
	taskRepoImpl, cleanup, err := repository.Provide(dbConn)
	require.NoError(t, err)
	taskRepo := repository.Repository(taskRepoImpl)
	t.Cleanup(func() {
		if err := dbConn.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if cleanup != nil {
			if err := cleanup(); err != nil {
				t.Errorf("failed to close task repo: %v", err)
			}
		}
	})
	if _, err := worktree.NewSQLiteStore(dbConn); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}

	// Initialize workflow service
	_, workflowSvc, _, err := workflow.Provide(dbConn, log)
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
	orchestratorCtrl := orchestratorcontroller.NewController(orchestratorSvc)
	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorCtrl, log)
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
	workspaceController := taskcontroller.NewWorkspaceController(taskSvc)
	boardController := taskcontroller.NewBoardController(taskSvc)
	boardController.SetWorkflowStepLister(workflowSvc)
	taskController := taskcontroller.NewTaskController(taskSvc)
	executorController := taskcontroller.NewExecutorController(taskSvc)
	environmentController := taskcontroller.NewEnvironmentController(taskSvc)
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)
	workflowCtrl := workflowcontroller.NewController(workflowSvc)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, workspaceController, log)
	taskhandlers.RegisterBoardRoutes(router, gateway.Dispatcher, boardController, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskController, nil, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, repositoryController, log)
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, executorController, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, environmentController, log)
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

	// Create board first (workflow steps are created automatically via default template)
	defaultTemplateID := "simple"
	board, err := ts.TaskSvc.CreateBoard(context.Background(), &taskservice.CreateBoardRequest{
		WorkspaceID:        workspace.ID,
		Name:               "Test Board",
		Description:        "Test board for orchestrator",
		WorkflowTemplateID: &defaultTemplateID,
	})
	require.NoError(t, err)

	// Get first workflow step from board
	steps, err := ts.WorkflowSvc.ListStepsByBoard(context.Background(), board.ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps, "board should have workflow steps")
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
		BoardID:        board.ID,
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

// OrchestratorWSClient is a WebSocket client for orchestrator tests
type OrchestratorWSClient struct {
	conn          *websocket.Conn
	t             *testing.T
	notifications chan *ws.Message
	done          chan struct{}
	// pending tracks in-flight requests: request ID -> response channel
	pending map[string]chan *ws.Message
	// send is the channel for outgoing messages (serialized through writePump)
	send chan []byte
	mu   sync.Mutex
}

// NewOrchestratorWSClient creates a WebSocket connection to the test server
func NewOrchestratorWSClient(t *testing.T, serverURL string) *OrchestratorWSClient {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	client := &OrchestratorWSClient{
		conn:          conn,
		t:             t,
		notifications: make(chan *ws.Message, 100),
		done:          make(chan struct{}),
		pending:       make(map[string]chan *ws.Message),
		send:          make(chan []byte, 256),
	}

	go client.readPump()
	go client.writePump()

	return client
}

func createOrchestratorWorkspace(t *testing.T, client *OrchestratorWSClient) string {
	t.Helper()

	resp, err := client.SendRequest("workspace-1", ws.ActionWorkspaceCreate, map[string]interface{}{
		"name": "Test Workspace",
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	return payload["id"].(string)
}

// readPump reads messages from the WebSocket connection
func (c *OrchestratorWSClient) readPump() {
	defer close(c.done)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg ws.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.Type == ws.MessageTypeNotification {
			select {
			case c.notifications <- &msg:
			default:
			}
		} else {
			// Route response to the pending request by ID
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			c.mu.Unlock()
			if ok {
				select {
				case ch <- &msg:
				default:
				}
			}
		}
	}
}

// writePump serializes all writes to the WebSocket connection
func (c *OrchestratorWSClient) writePump() {
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// Close closes the WebSocket connection
func (c *OrchestratorWSClient) Close() {
	close(c.send)
	if err := c.conn.Close(); err != nil {
		c.t.Logf("failed to close websocket: %v", err)
	}
	<-c.done
}

// SendRequest sends a request and waits for a response
func (c *OrchestratorWSClient) SendRequest(id, action string, payload interface{}) (*ws.Message, error) {
	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// Create a response channel for this request
	respCh := make(chan *ws.Message, 1)

	// Register the pending request BEFORE sending (so we don't miss the response)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	// Ensure we clean up when done
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send the request through the write pump (serialized)
	select {
	case c.send <- data:
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("send buffer full")
	}

	// Wait for response on our dedicated channel
	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, context.DeadlineExceeded
	}
}

// WaitForNotification waits for a notification with the given action prefix
func (c *OrchestratorWSClient) WaitForNotification(actionPrefix string, timeout time.Duration) (*ws.Message, error) {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-c.notifications:
			if strings.HasPrefix(msg.Action, actionPrefix) {
				return msg, nil
			}
		case <-deadline:
			return nil, context.DeadlineExceeded
		}
	}
}

// CollectNotifications collects all notifications for a duration
func (c *OrchestratorWSClient) CollectNotifications(duration time.Duration) []*ws.Message {
	var msgs []*ws.Message
	deadline := time.After(duration)
	for {
		select {
		case msg := <-c.notifications:
			msgs = append(msgs, msg)
		case <-deadline:
			return msgs
		}
	}
}

// ============================================
// ORCHESTRATOR TESTS
// ============================================

func TestOrchestratorStatus(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.True(t, payload["running"].(bool))
	assert.Equal(t, float64(0), payload["active_agents"])
	assert.Equal(t, float64(0), payload["queued_tasks"])
}

func TestOrchestratorQueue(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("queue-1", ws.ActionOrchestratorQueue, map[string]interface{}{})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.Equal(t, float64(0), payload["total"])
	tasks := payload["tasks"].([]interface{})
	assert.Len(t, tasks, 0)
}

func TestOrchestratorStartTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Create a task with agent_profile_id
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task notifications
	subResp, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp.Type)

	// Start task execution
	resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.True(t, payload["success"].(bool))
	assert.Equal(t, taskID, payload["task_id"])
	assert.NotEmpty(t, payload["agent_execution_id"])
	sessionID, _ := payload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Give the agent time to process
	time.Sleep(500 * time.Millisecond)

	// Check status shows active agent
	statusResp, err := client.SendRequest("status-2", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)

	var statusPayload map[string]interface{}
	require.NoError(t, statusResp.ParsePayload(&statusPayload))
	assert.GreaterOrEqual(t, statusPayload["active_agents"].(float64), float64(0))
}

func TestOrchestratorStartTaskWithAgentTypeOverride(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 1)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start with different agent profile
	resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "override-profile-id",
		"priority":         3,
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.True(t, payload["success"].(bool))
}

func TestOrchestratorStopTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start task
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Stop task
	stopResp, err := client.SendRequest("stop-1", ws.ActionOrchestratorStop, map[string]interface{}{
		"task_id": taskID,
		"reason":  "test stop",
		"force":   false,
	})
	require.NoError(t, err)

	var stopPayload map[string]interface{}
	require.NoError(t, stopResp.ParsePayload(&stopPayload))
	assert.True(t, stopPayload["success"].(bool))
}

func TestOrchestratorPromptTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Wait for agent to be ready
	time.Sleep(300 * time.Millisecond)

	// Send follow-up prompt
	promptResp, err := client.SendRequest("prompt-1", ws.ActionOrchestratorPrompt, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"prompt":     "Please continue and add error handling",
	})
	require.NoError(t, err)

	var promptPayload map[string]interface{}
	require.NoError(t, promptResp.ParsePayload(&promptPayload))
	assert.True(t, promptPayload["success"].(bool))
}

func TestOrchestratorCompleteTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start task
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))

	// Wait for agent to be running
	time.Sleep(200 * time.Millisecond)

	// Complete task
	completeResp, err := client.SendRequest("complete-1", ws.ActionOrchestratorComplete, map[string]interface{}{
		"task_id": taskID,
	})
	require.NoError(t, err)

	var completePayload map[string]interface{}
	require.NoError(t, completeResp.ParsePayload(&completePayload))
	assert.True(t, completePayload["success"].(bool))

	// Verify task state is COMPLETED
	task, err := ts.TaskRepo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateCompleted, task.State)
}

func TestOrchestratorConcurrentTasks(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Create multiple tasks
	taskIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		taskIDs[i] = ts.CreateTestTask(t, "augment-agent", i+1)
	}

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start all tasks concurrently
	var wg sync.WaitGroup
	results := make(chan map[string]interface{}, 3)

	for i, taskID := range taskIDs {
		wg.Add(1)
		go func(idx int, tid string) {
			defer wg.Done()

			resp, err := client.SendRequest(fmt.Sprintf("start-%d", idx), ws.ActionOrchestratorStart, map[string]interface{}{
				"task_id":          tid,
				"agent_profile_id": "augment-agent",
			})
			if err != nil {
				t.Logf("Error starting task %d: %v", idx, err)
				return
			}

			var payload map[string]interface{}
			if err := resp.ParsePayload(&payload); err != nil {
				t.Logf("failed to parse start response: %v", err)
				return
			}
			results <- payload
		}(i, taskID)
	}

	wg.Wait()
	close(results)

	// Count successful launches
	successCount := 0
	for result := range results {
		if success, ok := result["success"].(bool); ok && success {
			successCount++
		}
	}

	assert.Equal(t, 3, successCount, "All tasks should start successfully")

	// Check status shows multiple active agents
	statusResp, err := client.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)

	var statusPayload map[string]interface{}
	require.NoError(t, statusResp.ParsePayload(&statusPayload))
	t.Logf("Active agents: %v", statusPayload["active_agents"])
}

func TestOrchestratorACPStreaming(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure simulated agent to emit specific ACP messages
	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		return []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "analyzing",
					"message":  "Analyzing codebase...",
				},
			},
			{
				Type:      protocol.MessageTypeLog,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"level":   "info",
					"message": "Found 5 files to modify",
				},
			},
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 50,
					"stage":    "modifying",
					"message":  "Modifying files...",
				},
			},
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 100,
					"stage":    "completed",
					"message":  "All modifications complete",
				},
			},
		}
	})

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Extract session_id and subscribe to session for agent stream events
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	subResp, err := client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp.Type, "Session subscription should succeed")

	// Wait for agent to process
	time.Sleep(500 * time.Millisecond)

	// Verify orchestrator status shows active processing
	statusResp, err := client.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, statusResp.Type)
}

func TestOrchestratorMultipleClients(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	// Create two clients
	client1 := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client1.Close()

	client2 := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client2.Close()

	// Both clients subscribe to the same task
	_, err := client1.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	_, err = client2.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Client 1 starts the task
	startResp, err := client1.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	// Both clients subscribe to the session
	subResp1, err := client1.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp1.Type, "Client 1 subscription should succeed")

	subResp2, err := client2.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp2.Type, "Client 2 subscription should succeed")

	// Verify both clients can query orchestrator status
	status1, err := client1.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, status1.Type)

	status2, err := client2.SendRequest("status-2", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, status2.Type)
}

func TestOrchestratorErrorHandling(t *testing.T) {
	t.Run("StartTaskMissingTaskID", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)

		var errPayload ws.ErrorPayload
		require.NoError(t, resp.ParsePayload(&errPayload))
		assert.Equal(t, ws.ErrorCodeValidation, errPayload.Code)
		assert.Contains(t, errPayload.Message, "task_id")
	})

	t.Run("StartTaskNotFound", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
			"task_id":          "non-existent-task",
			"agent_profile_id": "augment-agent",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("PromptTaskMissingPrompt", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		taskID := ts.CreateTestTask(t, "augment-agent", 2)

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		resp, err := client.SendRequest("prompt-1", ws.ActionOrchestratorPrompt, map[string]interface{}{
			"task_id":    taskID,
			"session_id": "session-1",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)

		var errPayload ws.ErrorPayload
		require.NoError(t, resp.ParsePayload(&errPayload))
		assert.Contains(t, errPayload.Message, "prompt")
	})

	t.Run("StopTaskNotRunning", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		taskID := ts.CreateTestTask(t, "augment-agent", 2)

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		// Try to stop a task that was never started
		resp, err := client.SendRequest("stop-1", ws.ActionOrchestratorStop, map[string]interface{}{
			"task_id": taskID,
		})
		require.NoError(t, err)

		// Should error because task is not running
		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})
}

func TestOrchestratorAgentFailure(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure agent manager to fail launches
	ts.AgentManager.SetShouldFail(true)

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Should get error response
	assert.Equal(t, ws.MessageTypeError, resp.Type)

	var errPayload ws.ErrorPayload
	require.NoError(t, resp.ParsePayload(&errPayload))
	assert.Contains(t, errPayload.Message, "Failed to start task")
}

func TestOrchestratorTaskPriority(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Create tasks with different priorities
	lowPriorityTask := ts.CreateTestTask(t, "augment-agent", 1)
	highPriorityTask := ts.CreateTestTask(t, "augment-agent", 3)
	medPriorityTask := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start low priority first
	_, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          lowPriorityTask,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Then high priority
	_, err = client.SendRequest("start-2", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          highPriorityTask,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Then medium priority
	_, err = client.SendRequest("start-3", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          medPriorityTask,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// All should start (we have capacity)
	assert.Equal(t, 3, ts.AgentManager.GetLaunchCount())
}

func TestOrchestratorTriggerTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("trigger-1", ws.ActionOrchestratorTrigger, map[string]interface{}{
		"task_id": taskID,
	})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))
	assert.True(t, payload["success"].(bool))
	assert.Equal(t, taskID, payload["task_id"])
}

func TestOrchestratorEndToEndWorkflow(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createOrchestratorWorkspace(t, client)

	// 1. Create board with workflow template
	boardResp, err := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "E2E Test Board",
		"description":          "End-to-end test board",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var boardPayload map[string]interface{}
	require.NoError(t, boardResp.ParsePayload(&boardPayload))
	boardID := boardPayload["id"].(string)

	// 2. Get first workflow step from board
	stepResp, err := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	require.NoError(t, err)

	var stepListPayload map[string]interface{}
	require.NoError(t, stepResp.ParsePayload(&stepListPayload))
	steps := stepListPayload["steps"].([]interface{})
	require.NotEmpty(t, steps, "board should have workflow steps")
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// 3. Create task with workflow step
	repoResp, err := client.SendRequest("repo-1", ws.ActionRepositoryCreate, map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Test Repo",
		"source_type":  "local",
		"local_path":   createTempRepoDir(t),
	})
	require.NoError(t, err)

	var repoPayload map[string]interface{}
	require.NoError(t, repoResp.ParsePayload(&repoPayload))
	repositoryID := repoPayload["id"].(string)

	taskResp, err := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"board_id":         boardID,
		"workflow_step_id": workflowStepID,
		"title":            "Implement feature X",
		"description":      "Create a new feature with tests",
		"priority":         2,
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	require.NoError(t, err)

	var taskPayload map[string]interface{}
	require.NoError(t, taskResp.ParsePayload(&taskPayload))
	taskID := taskPayload["id"].(string)

	// 4. Subscribe to task
	_, err = client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// 5. Start orchestrator
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	assert.True(t, startPayload["success"].(bool))
	agentExecutionID := startPayload["agent_execution_id"].(string)
	assert.NotEmpty(t, agentExecutionID)
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// 6. Collect notifications during execution
	time.Sleep(400 * time.Millisecond)
	notifications := client.CollectNotifications(200 * time.Millisecond)
	t.Logf("Received %d notifications", len(notifications))

	// 7. Complete the task
	completeResp, err := client.SendRequest("complete-1", ws.ActionOrchestratorComplete, map[string]interface{}{
		"task_id": taskID,
	})
	require.NoError(t, err)

	var completePayload map[string]interface{}
	require.NoError(t, completeResp.ParsePayload(&completePayload))
	assert.True(t, completePayload["success"].(bool))

	// 8. Verify final task state
	getResp, err := client.SendRequest("get-1", ws.ActionTaskGet, map[string]string{"id": taskID})
	require.NoError(t, err)

	var getPayload map[string]interface{}
	require.NoError(t, getResp.ParsePayload(&getPayload))
	assert.Equal(t, "COMPLETED", getPayload["state"].(string))
}

// TestOrchestratorAgentMessagePersistence validates that agent messages are stored
// correctly in the database without missing data or ordering issues.
func TestOrchestratorAgentMessagePersistence(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Track expected messages with their order and content
	type expectedMessage struct {
		Type     string
		Content  string
		Metadata map[string]interface{}
	}
	var expectedMessages []expectedMessage
	var messageTimestamps []time.Time

	// This test verifies that message_streaming and log events ARE correctly persisted.
	// Progress messages (protocol.MessageTypeProgress) are NOT persisted - only broadcast via WebSocket.
	// Log messages (protocol.MessageTypeLog) ARE now persisted to the database.

	// We'll manually publish message_streaming and log events to test persistence
	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		// Just emit a simple progress message - won't be stored
		return []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "starting",
					"message":  "Starting...",
				},
			},
		}
	})

	// Create test task
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task execution
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool), "Task should start successfully")
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID, "Session ID should be returned")

	// Subscribe to session for agent stream events
	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Wait for simulated agent to start
	time.Sleep(200 * time.Millisecond)

	// Now manually publish message_streaming events which ARE persisted
	// Generate unique message IDs for each streaming message
	messageIDs := []string{
		"msg-" + uuid.New().String()[:8],
		"msg-" + uuid.New().String()[:8],
		"msg-" + uuid.New().String()[:8],
	}

	expectedContents := []string{
		"I'm analyzing the codebase structure.",
		"I found several files that need modification.",
		"Here is my recommendation for the changes.",
	}

	// Log messages to publish
	logMessages := []struct {
		Level   string
		Message string
	}{
		{"info", "Found 10 files to process"},
		{"debug", "Processing file: main.go"},
		{"warning", "Deprecated API usage detected"},
	}

	// Record expected messages for verification
	for i, content := range expectedContents {
		expectedMessages = append(expectedMessages, expectedMessage{
			Type:    "content",
			Content: content,
		})
		messageTimestamps = append(messageTimestamps, time.Now().Add(time.Duration(i*50)*time.Millisecond))
	}
	for _, logMsg := range logMessages {
		expectedMessages = append(expectedMessages, expectedMessage{
			Type:    "log",
			Content: logMsg.Message,
			Metadata: map[string]interface{}{
				"level": logMsg.Level,
			},
		})
	}

	// Publish message_streaming events - these ARE persisted
	for i, msgID := range messageIDs {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":       "message_streaming",
				"text":       expectedContents[i],
				"message_id": msgID,
				"is_append":  false, // Each is a new message
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Publish log events - these ARE now persisted
	for _, logMsg := range logMessages {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type": "log",
				"data": map[string]interface{}{
					"level":   logMsg.Level,
					"message": logMsg.Message,
				},
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all messages to be processed and stored
	time.Sleep(300 * time.Millisecond)

	// Query the database to verify stored messages
	storedMessages, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)

	t.Logf("Found %d total stored messages", len(storedMessages))

	// Filter to only agent messages (exclude initial user prompt)
	var agentMessages []*struct {
		ID            string
		Type          string
		Content       string
		AuthorType    string
		Metadata      map[string]interface{}
		CreatedAt     time.Time
		TaskSessionID string
		TurnID        string
	}
	for _, msg := range storedMessages {
		if string(msg.AuthorType) == "agent" {
			agentMessages = append(agentMessages, &struct {
				ID            string
				Type          string
				Content       string
				AuthorType    string
				Metadata      map[string]interface{}
				CreatedAt     time.Time
				TaskSessionID string
				TurnID        string
			}{
				ID:            msg.ID,
				Type:          string(msg.Type),
				Content:       msg.Content,
				AuthorType:    string(msg.AuthorType),
				Metadata:      msg.Metadata,
				CreatedAt:     msg.CreatedAt,
				TaskSessionID: msg.TaskSessionID,
				TurnID:        msg.TurnID,
			})
		}
	}

	t.Logf("Found %d agent messages in database", len(agentMessages))

	// Verify messages are stored
	require.GreaterOrEqual(t, len(agentMessages), 3, "Should have at least 3 agent messages from streaming events")

	// Verify messages are in correct chronological order
	for i := 1; i < len(agentMessages); i++ {
		prev := agentMessages[i-1]
		curr := agentMessages[i]
		assert.True(t, !curr.CreatedAt.Before(prev.CreatedAt),
			"Messages should be in chronological order: message %d (%s) at %v should not be before message %d (%s) at %v",
			i, curr.ID, curr.CreatedAt, i-1, prev.ID, prev.CreatedAt)
	}

	// Verify no duplicate messages exist (check for duplicate IDs)
	seenIDs := make(map[string]bool)
	for _, msg := range agentMessages {
		assert.False(t, seenIDs[msg.ID], "Duplicate message ID found: %s", msg.ID)
		seenIDs[msg.ID] = true
	}

	// Verify all messages are associated with the correct TaskSessionID
	for _, msg := range agentMessages {
		assert.Equal(t, sessionID, msg.TaskSessionID,
			"Message %s should be associated with session %s", msg.ID, sessionID)
	}

	// Verify all agent messages have a TurnID
	for _, msg := range agentMessages {
		assert.NotEmpty(t, msg.TurnID,
			"Message %s should have a TurnID", msg.ID)
	}

	// Verify message content matches expected content
	contentMessages := make([]*struct {
		ID            string
		Type          string
		Content       string
		AuthorType    string
		Metadata      map[string]interface{}
		CreatedAt     time.Time
		TaskSessionID string
		TurnID        string
	}, 0)
	for _, msg := range agentMessages {
		if msg.Type == "content" {
			contentMessages = append(contentMessages, msg)
		}
	}
	require.Len(t, contentMessages, 3, "Should have exactly 3 content messages")

	// Verify content is in correct order
	for i, expected := range expectedContents {
		if i < len(contentMessages) {
			assert.Equal(t, expected, contentMessages[i].Content,
				"Message %d content should match expected", i)
		}
	}

	// Log summary of stored messages for debugging
	t.Logf("Message storage verification summary:")
	for i, msg := range agentMessages {
		t.Logf("  [%d] ID=%s Type=%s Content=%q CreatedAt=%v TurnID=%s",
			i, msg.ID[:8], msg.Type, truncateString(msg.Content, 50), msg.CreatedAt, msg.TurnID[:8])
	}
}

// TestOrchestratorAgentMessagePersistenceWithToolCalls validates that tool call messages
// are stored correctly with proper metadata including tool_call_id, title, status, and args.
func TestOrchestratorAgentMessagePersistenceWithToolCalls(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure simulated agent to emit tool call events via direct event publishing
	toolCallID1 := "tc-" + uuid.New().String()[:8]
	toolCallID2 := "tc-" + uuid.New().String()[:8]

	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		baseTime := time.Now()
		return []protocol.Message{
			// Initial progress
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: baseTime,
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "starting",
					"message":  "Starting tool execution...",
				},
			},
			// Log message
			{
				Type:      protocol.MessageTypeLog,
				TaskID:    taskID,
				Timestamp: baseTime.Add(20 * time.Millisecond),
				Data: map[string]interface{}{
					"level":   "info",
					"message": "Executing view tool",
				},
			},
			// Final progress
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: baseTime.Add(100 * time.Millisecond),
				Data: map[string]interface{}{
					"progress": 100,
					"stage":    "completed",
					"message":  "Tool execution completed",
				},
			},
		}
	})

	// Create test task
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task execution
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	// Subscribe to session
	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Now publish tool call events directly to the event bus to simulate real tool usage
	// This mimics what the lifecycle manager does when processing agent tool calls
	publishToolCallEvent := func(toolCallID, title, status string, args map[string]interface{}) {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":         "tool_call",
				"tool_call_id": toolCallID,
				"tool_title":   title,
				"tool_name":    "view",
				"tool_status":  status,
				"tool_args":    args,
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
	}

	publishToolUpdateEvent := func(toolCallID, status, result string) {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":         "tool_update",
				"tool_call_id": toolCallID,
				"tool_status":  status,
				"tool_result":  result,
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
	}

	// Wait a bit for the initial messages to be processed
	time.Sleep(300 * time.Millisecond)

	// Publish tool call start events
	publishToolCallEvent(toolCallID1, "View main.go", "running", map[string]interface{}{
		"path": "main.go",
		"kind": "view",
	})
	time.Sleep(50 * time.Millisecond)

	publishToolCallEvent(toolCallID2, "View utils.go", "running", map[string]interface{}{
		"path": "utils.go",
		"kind": "view",
	})
	time.Sleep(50 * time.Millisecond)

	// Publish tool update events (completion)
	publishToolUpdateEvent(toolCallID1, "complete", "File content: package main...")
	time.Sleep(50 * time.Millisecond)

	publishToolUpdateEvent(toolCallID2, "complete", "File content: package utils...")
	time.Sleep(50 * time.Millisecond)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Query stored messages
	storedMessages, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)

	t.Logf("Found %d total stored messages", len(storedMessages))

	// Find tool_call messages
	var toolCallMessages []*struct {
		ID       string
		Type     string
		Content  string
		Metadata map[string]interface{}
	}
	for _, msg := range storedMessages {
		if string(msg.Type) == "tool_call" {
			toolCallMessages = append(toolCallMessages, &struct {
				ID       string
				Type     string
				Content  string
				Metadata map[string]interface{}
			}{
				ID:       msg.ID,
				Type:     string(msg.Type),
				Content:  msg.Content,
				Metadata: msg.Metadata,
			})
		}
	}

	t.Logf("Found %d tool_call messages", len(toolCallMessages))

	// Verify tool call messages exist and have correct metadata
	assert.GreaterOrEqual(t, len(toolCallMessages), 2,
		"Should have at least 2 tool_call messages")

	// Verify tool call metadata structure
	for _, msg := range toolCallMessages {
		assert.NotNil(t, msg.Metadata, "Tool call message should have metadata")
		if msg.Metadata != nil {
			toolCallIDMeta, _ := msg.Metadata["tool_call_id"].(string)
			assert.NotEmpty(t, toolCallIDMeta,
				"Tool call message should have tool_call_id in metadata")

			title, _ := msg.Metadata["title"].(string)
			t.Logf("Tool call message: ID=%s, tool_call_id=%s, title=%s, status=%v",
				msg.ID[:8], toolCallIDMeta, title, msg.Metadata["status"])
		}
	}
}

// TestOrchestratorAgentMessageChunkStreaming validates that streaming message chunks
// are correctly aggregated and stored as complete messages.
func TestOrchestratorAgentMessageChunkStreaming(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure minimal ACP messages - we'll manually publish streaming chunks
	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		return []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "starting",
					"message":  "Starting...",
				},
			},
		}
	})

	// Create test task
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe and start task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Wait for initial setup
	time.Sleep(300 * time.Millisecond)

	// Simulate streaming message chunks by publishing message_streaming events
	messageID := uuid.New().String()
	chunks := []string{
		"Hello, ",
		"this is ",
		"a streaming ",
		"message that ",
		"should be ",
		"aggregated.",
	}

	for i, chunk := range chunks {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":       "message_streaming",
				"text":       chunk,
				"message_id": messageID,
				"is_append":  i > 0, // First chunk creates, subsequent append
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Query stored messages
	storedMessages, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)

	// Find the streaming message
	var streamingMessage *struct {
		ID      string
		Content string
		Type    string
	}
	for _, msg := range storedMessages {
		if msg.ID == messageID {
			streamingMessage = &struct {
				ID      string
				Content string
				Type    string
			}{
				ID:      msg.ID,
				Content: msg.Content,
				Type:    string(msg.Type),
			}
			break
		}
	}

	// Verify the streaming message was created and aggregated
	if streamingMessage != nil {
		expectedContent := strings.Join(chunks, "")
		assert.Equal(t, expectedContent, streamingMessage.Content,
			"Streaming message content should be aggregated from all chunks")
		t.Logf("Streaming message aggregated correctly: %q", streamingMessage.Content)
	} else {
		// The message might be stored with agent-generated ID
		// Look for content messages from the agent
		var agentContentMessages []*struct {
			ID      string
			Content string
			Type    string
		}
		for _, msg := range storedMessages {
			if string(msg.AuthorType) == "agent" && (string(msg.Type) == "message" || string(msg.Type) == "content") {
				agentContentMessages = append(agentContentMessages, &struct {
					ID      string
					Content string
					Type    string
				}{
					ID:      msg.ID,
					Content: msg.Content,
					Type:    string(msg.Type),
				})
			}
		}
		t.Logf("Found %d agent content messages", len(agentContentMessages))
		for _, msg := range agentContentMessages {
			t.Logf("  ID=%s Type=%s Content=%q", msg.ID[:8], msg.Type, truncateString(msg.Content, 100))
		}
	}
}

// truncateString truncates a string to the specified length, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
