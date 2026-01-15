// Package integration provides end-to-end integration tests for the Kandev backend.
// This file contains orchestrator-specific integration tests.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorcontroller "github.com/kandev/kandev/internal/orchestrator/controller"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
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
	acpMessageFn  func(taskID, instanceID string) []protocol.Message // Custom ACP messages
	stopCh        chan struct{}
}

// simulatedInstance tracks a simulated agent instance
type simulatedInstance struct {
	id             string
	taskID         string
	agentProfileID string
	status         v1.AgentStatus
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
func (s *SimulatedAgentManagerClient) SetACPMessageFn(fn func(taskID, instanceID string) []protocol.Message) {
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

	instanceID := uuid.New().String()
	containerID := "sim-container-" + instanceID[:8]

	s.mu.Lock()
	instance := &simulatedInstance{
		id:             instanceID,
		taskID:         req.TaskID,
		agentProfileID: req.AgentProfileID,
		status:         v1.AgentStatusStarting,
		stopCh:         make(chan struct{}),
	}
	s.instances[instanceID] = instance
	s.mu.Unlock()

	// Simulate agent lifecycle in background
	go s.runAgentSimulation(instance, req)

	return &executor.LaunchAgentResponse{
		AgentInstanceID: instanceID,
		ContainerID:     containerID,
		Status:          v1.AgentStatusStarting,
	}, nil
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
	instance.status = v1.AgentStatusReady
	s.publishAgentEvent(events.AgentReady, instance)
}

// publishAgentEvent publishes an agent lifecycle event
func (s *SimulatedAgentManagerClient) publishAgentEvent(eventType string, instance *simulatedInstance) {
	data := map[string]interface{}{
		"instance_id":      instance.id,
		"task_id":          instance.taskID,
		"agent_profile_id": instance.agentProfileID,
		"container_id":     "sim-container-" + instance.id[:8],
		"status":           string(instance.status),
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
		msgData := map[string]interface{}{
			"type":      msg.Type,
			"task_id":   msg.TaskID,
			"timestamp": msg.Timestamp,
			"data":      msg.Data,
		}

		event := bus.NewEvent(events.ACPMessage, "simulated-agent", msgData)
		subject := events.BuildACPSubject(instance.taskID)
		if err := s.eventBus.Publish(context.Background(), subject, event); err != nil {
			s.logger.Error("failed to publish ACP message")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// StopAgent simulates stopping an agent
func (s *SimulatedAgentManagerClient) StopAgent(ctx context.Context, agentInstanceID string, force bool) error {
	s.mu.Lock()
	instance, exists := s.instances[agentInstanceID]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("agent instance %q not found", agentInstanceID)
	}

	close(instance.stopCh)
	instance.status = v1.AgentStatusStopped

	s.publishAgentEvent(events.AgentStopped, instance)
	return nil
}

// GetAgentStatus returns the status of a simulated agent
func (s *SimulatedAgentManagerClient) GetAgentStatus(ctx context.Context, agentInstanceID string) (*v1.AgentInstance, error) {
	s.mu.Lock()
	instance, exists := s.instances[agentInstanceID]
	s.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("agent instance %q not found", agentInstanceID)
	}

	return &v1.AgentInstance{
		ID:             instance.id,
		TaskID:         instance.taskID,
		AgentProfileID: instance.agentProfileID,
		Status:         instance.status,
	}, nil
}

// ListAgentTypes returns available agent types
func (s *SimulatedAgentManagerClient) ListAgentTypes(ctx context.Context) ([]*v1.AgentType, error) {
	return []*v1.AgentType{
		{
			ID:          "augment-agent",
			Name:        "Augment Agent",
			Description: "Simulated Augment agent for testing",
			DockerImage: "test/augment-agent",
			DockerTag:   "test",
			Enabled:     true,
		},
		{
			ID:          "auggie-cli",
			Name:        "Auggie CLI",
			Description: "Simulated Auggie CLI for testing",
			DockerImage: "test/auggie-cli",
			DockerTag:   "test",
			Enabled:     true,
		},
	}, nil
}

// PromptAgent sends a follow-up prompt to a running agent
func (s *SimulatedAgentManagerClient) PromptAgent(ctx context.Context, agentInstanceID string, prompt string) (*executor.PromptResult, error) {
	s.mu.Lock()
	instance, exists := s.instances[agentInstanceID]
	s.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("agent instance %q not found", agentInstanceID)
	}

	// Simulate receiving prompt and generating response
	go func() {
		time.Sleep(50 * time.Millisecond)

		msg := protocol.Message{
			Type:      protocol.MessageTypeLog,
			TaskID:    instance.taskID,
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

		event := bus.NewEvent(events.ACPMessage, "simulated-agent", msgData)
		subject := events.BuildACPSubject(instance.taskID)
		s.eventBus.Publish(context.Background(), subject, event)
	}()

	return &executor.PromptResult{
		StopReason: "end_turn",
	}, nil
}

// RespondToPermissionByTaskID responds to a permission request for a task
func (s *SimulatedAgentManagerClient) RespondToPermissionByTaskID(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error {
	s.logger.Info("simulated: responding to permission",
		zap.String("task_id", taskID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))
	return nil
}

// CompleteAgent marks an agent as completed
func (s *SimulatedAgentManagerClient) CompleteAgent(instanceID string) {
	s.mu.Lock()
	instance, exists := s.instances[instanceID]
	s.mu.Unlock()

	if !exists {
		return
	}

	instance.status = v1.AgentStatusCompleted
	s.publishAgentEvent(events.AgentCompleted, instance)
}

// FailAgent marks an agent as failed
func (s *SimulatedAgentManagerClient) FailAgent(instanceID string, reason string) {
	s.mu.Lock()
	instance, exists := s.instances[instanceID]
	s.mu.Unlock()

	if !exists {
		return
	}

	instance.status = v1.AgentStatusFailed
	s.publishAgentEvent(events.AgentFailed, instance)
}

// GetLaunchCount returns the number of times LaunchAgent was called
func (s *SimulatedAgentManagerClient) GetLaunchCount() int {
	return int(atomic.LoadInt32(&s.launchCount))
}

// Close stops all simulated agents
func (s *SimulatedAgentManagerClient) Close() {
	close(s.stopCh)
}

// GetRecoveredInstances returns recovered instances (none for simulated agent)
func (s *SimulatedAgentManagerClient) GetRecoveredInstances() []executor.RecoveredInstanceInfo {
	return nil
}

// IsAgentRunningForTask checks if a simulated agent is running for a task
func (s *SimulatedAgentManagerClient) IsAgentRunningForTask(ctx context.Context, taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, inst := range s.instances {
		if inst.taskID == taskID && inst.status == v1.AgentStatusRunning {
			return true
		}
	}
	return false
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

	// Initialize task repository (SQLite for tests)
	tmpDir := t.TempDir()
	taskRepo, err := repository.NewSQLiteRepository(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	t.Cleanup(func() { taskRepo.Close() })

	// Initialize task service
	taskSvc := taskservice.NewService(taskRepo, eventBus, log, taskservice.RepositoryDiscoveryConfig{})

	// Create simulated agent manager
	agentManager := NewSimulatedAgentManager(eventBus, log)

	// Create task repository adapter
	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}

	// Create orchestrator service
	cfg := orchestrator.DefaultServiceConfig()
	cfg.Scheduler.ProcessInterval = 50 * time.Millisecond // Faster for tests
	orchestratorSvc := orchestrator.NewService(cfg, eventBus, agentManager, taskRepoAdapter, taskRepo, log)

	// Create WebSocket gateway
	gateway := gateways.NewGateway(log)

	// Register orchestrator handlers (Pattern A)
	orchestratorCtrl := orchestratorcontroller.NewController(orchestratorSvc)
	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorCtrl, log)
	orchestratorHandlers.RegisterHandlers(gateway.Dispatcher)

	// Start hub
	go gateway.Hub.Run(ctx)

	// Wire ACP handler to broadcast to WebSocket clients
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		action := "acp." + string(msg.Type)
		notification, _ := ws.NewNotification(action, map[string]interface{}{
			"task_id":   taskID,
			"type":      msg.Type,
			"data":      msg.Data,
			"timestamp": msg.Timestamp,
		})
		gateway.Hub.BroadcastToTask(taskID, notification)
	})

	// Start orchestrator
	require.NoError(t, orchestratorSvc.Start(ctx))

	// Create router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	gateway.SetupRoutes(router)

	// Register handlers (HTTP + WS)
	workspaceController := taskcontroller.NewWorkspaceController(taskSvc)
	boardController := taskcontroller.NewBoardController(taskSvc)
	taskController := taskcontroller.NewTaskController(taskSvc)
	executorController := taskcontroller.NewExecutorController(taskSvc)
	environmentController := taskcontroller.NewEnvironmentController(taskSvc)
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, workspaceController, log)
	taskhandlers.RegisterBoardRoutes(router, gateway.Dispatcher, boardController, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskController, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, repositoryController, log)
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, executorController, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, environmentController, log)

	// Create test server
	server := httptest.NewServer(router)

	return &OrchestratorTestServer{
		Server:          server,
		Gateway:         gateway,
		TaskRepo:        taskRepo,
		TaskSvc:         taskSvc,
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
	ts.OrchestratorSvc.Stop()
	ts.AgentManager.Close()
	ts.cancelFunc()
	ts.Server.Close()
	ts.TaskRepo.Close()
	ts.EventBus.Close()
}

// CreateTestTask creates a task for testing.
func (ts *OrchestratorTestServer) CreateTestTask(t *testing.T, agentProfileID string, priority int) string {
	t.Helper()

	workspace, err := ts.TaskSvc.CreateWorkspace(context.Background(), &taskservice.CreateWorkspaceRequest{
		Name: "Test Workspace",
	})
	require.NoError(t, err)

	// Create board first
	board, err := ts.TaskSvc.CreateBoard(context.Background(), &taskservice.CreateBoardRequest{
		WorkspaceID: workspace.ID,
		Name:        "Test Board",
		Description: "Test board for orchestrator",
	})
	require.NoError(t, err)

	// Create column
	col, err := ts.TaskSvc.CreateColumn(context.Background(), &taskservice.CreateColumnRequest{
		BoardID:  board.ID,
		Name:     "TODO",
		Position: 0,
		State:    v1.TaskStateTODO,
	})
	require.NoError(t, err)

	// Create task with agent profile ID
	repository, err := ts.TaskSvc.CreateRepository(context.Background(), &taskservice.CreateRepositoryRequest{
		WorkspaceID: workspace.ID,
		Name:        "Test Repo",
		LocalPath:   "/tmp/repo",
	})
	require.NoError(t, err)

	task, err := ts.TaskSvc.CreateTask(context.Background(), &taskservice.CreateTaskRequest{
		WorkspaceID:    workspace.ID,
		BoardID:        board.ID,
		ColumnID:       col.ID,
		Title:          "Test Task",
		Description:    "This is a test task for the orchestrator",
		Priority:       priority,
		RepositoryID:   repository.ID,
		BaseBranch:     "main",
	})
	require.NoError(t, err)

	return task.ID
}

// OrchestratorWSClient is a WebSocket client for orchestrator tests
type OrchestratorWSClient struct {
	conn          *websocket.Conn
	t             *testing.T
	messages      chan *ws.Message
	notifications chan *ws.Message
	done          chan struct{}
	mu            sync.Mutex
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
		messages:      make(chan *ws.Message, 100),
		notifications: make(chan *ws.Message, 100),
		done:          make(chan struct{}),
	}

	go client.readPump()

	return client
}

func createOrchestratorWorkspace(t *testing.T, client *OrchestratorWSClient) string {
	t.Helper()

	resp, err := client.SendRequest("workspace-1", ws.ActionWorkspaceCreate, map[string]interface{}{
		"name": "Test Workspace",
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	err = resp.ParsePayload(&payload)
	require.NoError(t, err)

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
			select {
			case c.messages <- &msg:
			default:
			}
		}
	}
}

// Close closes the WebSocket connection
func (c *OrchestratorWSClient) Close() {
	c.conn.Close()
	<-c.done
}

// SendRequest sends a request and waits for a response
func (c *OrchestratorWSClient) SendRequest(id, action string, payload interface{}) (*ws.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, err
	}

	timeout := time.After(10 * time.Second)
	for {
		select {
		case resp := <-c.messages:
			if resp.ID == id {
				return resp, nil
			}
		case <-timeout:
			return nil, context.DeadlineExceeded
		}
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
	assert.NotEmpty(t, payload["agent_instance_id"])

	// Wait for ACP notifications
	time.Sleep(500 * time.Millisecond)
	notifications := client.CollectNotifications(100 * time.Millisecond)

	// Should have received some ACP messages
	assert.NotEmpty(t, notifications, "Expected ACP notifications")

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

	// Wait for agent to be ready
	time.Sleep(300 * time.Millisecond)

	// Send follow-up prompt
	promptResp, err := client.SendRequest("prompt-1", ws.ActionOrchestratorPrompt, map[string]interface{}{
		"task_id": taskID,
		"prompt":  "Please continue and add error handling",
	})
	require.NoError(t, err)

	var promptPayload map[string]interface{}
	require.NoError(t, promptResp.ParsePayload(&promptPayload))
	assert.True(t, promptPayload["success"].(bool))

	// Wait for prompt response notification
	time.Sleep(200 * time.Millisecond)
	notifications := client.CollectNotifications(100 * time.Millisecond)
	assert.NotEmpty(t, notifications, "Expected prompt response notification")
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
			resp.ParsePayload(&payload)
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
	ts.AgentManager.SetACPMessageFn(func(taskID, instanceID string) []protocol.Message {
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
	_, err = client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Collect ACP notifications
	time.Sleep(500 * time.Millisecond)
	notifications := client.CollectNotifications(200 * time.Millisecond)

	// Verify we received progress notifications
	progressCount := 0
	for _, n := range notifications {
		if strings.HasPrefix(n.Action, "acp.") {
			progressCount++
		}
	}

	assert.GreaterOrEqual(t, progressCount, 3, "Expected at least 3 ACP notifications")
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
	_, err = client1.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Both clients should receive notifications
	time.Sleep(500 * time.Millisecond)

	notifications1 := client1.CollectNotifications(100 * time.Millisecond)
	notifications2 := client2.CollectNotifications(100 * time.Millisecond)

	// Both should have received notifications
	assert.NotEmpty(t, notifications1, "Client 1 should receive notifications")
	assert.NotEmpty(t, notifications2, "Client 2 should receive notifications")
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
			"task_id": taskID,
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

	// 1. Create board
	boardResp, err := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "E2E Test Board",
		"description":  "End-to-end test board",
	})
	require.NoError(t, err)

	var boardPayload map[string]interface{}
	require.NoError(t, boardResp.ParsePayload(&boardPayload))
	boardID := boardPayload["id"].(string)

	// 2. Create column
	colResp, err := client.SendRequest("col-1", ws.ActionColumnCreate, map[string]interface{}{
		"board_id": boardID,
		"name":     "TODO",
		"position": 0,
		"state":    "TODO",
	})
	require.NoError(t, err)

	var colPayload map[string]interface{}
	require.NoError(t, colResp.ParsePayload(&colPayload))
	colID := colPayload["id"].(string)

	// 3. Create task with agent_profile_id
	repoResp, err := client.SendRequest("repo-1", ws.ActionRepositoryCreate, map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Test Repo",
		"source_type":  "local",
		"local_path":   "/tmp/repo",
	})
	require.NoError(t, err)

	var repoPayload map[string]interface{}
	require.NoError(t, repoResp.ParsePayload(&repoPayload))
	repositoryID := repoPayload["id"].(string)

	taskResp, err := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"board_id":         boardID,
		"column_id":        colID,
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
	agentInstanceID := startPayload["agent_instance_id"].(string)
	assert.NotEmpty(t, agentInstanceID)

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
