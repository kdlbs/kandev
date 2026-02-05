// Package integration provides end-to-end integration tests for the Kandev backend.
// These tests start a real server and communicate via WebSocket.
package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestServer holds the test server and its dependencies
type TestServer struct {
	Server     *httptest.Server
	Gateway    *gateways.Gateway
	TaskRepo   repository.Repository
	TaskSvc    *taskservice.Service
	EventBus   bus.EventBus
	Logger     *logger.Logger
	cancelFunc context.CancelFunc
}

// NewTestServer creates a new test server with all components initialized
func NewTestServer(t *testing.T) *TestServer {
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

	// Create WebSocket gateway
	gateway := gateways.NewGateway(log)

	// Start hub
	go gateway.Hub.Run(ctx)

	// Create router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	gateway.SetupRoutes(router)

	// Register handlers (HTTP + WS)
	workflowCtrl := workflowcontroller.NewController(workflowSvc)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, taskSvc, log)
	boardHandlers := taskhandlers.RegisterBoardRoutes(router, gateway.Dispatcher, taskSvc, log)
	boardHandlers.SetWorkflowStepLister(workflowSvc)
	planService := taskservice.NewPlanService(taskRepo, eventBus, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskSvc, nil, taskRepo, planService, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, taskSvc, log)
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, taskSvc, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, taskSvc, log)
	workflowhandlers.RegisterRoutes(router, gateway.Dispatcher, workflowCtrl, log)

	// Create test server
	server := httptest.NewServer(router)

	return &TestServer{
		Server:     server,
		Gateway:    gateway,
		TaskRepo:   taskRepo,
		TaskSvc:    taskSvc,
		EventBus:   eventBus,
		Logger:     log,
		cancelFunc: cancel,
	}
}

// Close shuts down the test server
func (ts *TestServer) Close() {
	ts.cancelFunc()
	ts.Server.Close()
	if err := ts.TaskRepo.Close(); err != nil {
		ts.Logger.Error("failed to close task repo: " + err.Error())
	}
	ts.EventBus.Close()
}

// WSClient is a helper for WebSocket communication in tests
type WSClient struct {
	conn     *websocket.Conn
	t        *testing.T
	messages chan *ws.Message
	done     chan struct{}
	mu       sync.Mutex
}

// NewWSClient creates a WebSocket connection to the test server
func NewWSClient(t *testing.T, serverURL string) *WSClient {
	t.Helper()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	client := &WSClient{
		conn:     conn,
		t:        t,
		messages: make(chan *ws.Message, 100),
		done:     make(chan struct{}),
	}

	// Start reading messages
	go client.readPump()

	return client
}

func createWorkspace(t *testing.T, client *WSClient) string {
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

func createRepository(t *testing.T, client *WSClient, workspaceID string) string {
	t.Helper()

	repoPath := createTempRepoDir(t)
	resp, err := client.SendRequest("repo-1", ws.ActionRepositoryCreate, map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Test Repo",
		"source_type":  "local",
		"local_path":   repoPath,
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	err = resp.ParsePayload(&payload)
	require.NoError(t, err)

	return payload["id"].(string)
}

func createTempRepoDir(t *testing.T) string {
	t.Helper()

	baseDir := t.TempDir()
	repoPath := filepath.Join(baseDir, "repo")
	require.NoError(t, os.MkdirAll(repoPath, 0o755))

	return repoPath
}

// readPump reads messages from the WebSocket connection
func (c *WSClient) readPump() {
	defer close(c.done)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		// Handle newline-separated messages (server batches messages with newlines)
		parts := strings.Split(string(data), "\n")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			var msg ws.Message
			if err := json.Unmarshal([]byte(part), &msg); err != nil {
				continue
			}

			select {
			case c.messages <- &msg:
			default:
				// Buffer full, drop message
			}
		}
	}
}

// Close closes the WebSocket connection
func (c *WSClient) Close() {
	if err := c.conn.Close(); err != nil {
		c.t.Logf("failed to close websocket: %v", err)
	}
	<-c.done
}

// SendRequest sends a request and waits for a response
func (c *WSClient) SendRequest(id, action string, payload interface{}) (*ws.Message, error) {
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

	// Wait for response with matching ID
	timeout := time.After(5 * time.Second)
	for {
		select {
		case resp := <-c.messages:
			if resp.ID == id {
				return resp, nil
			}
			// Not our response, put it back (or buffer it)
		case <-timeout:
			return nil, context.DeadlineExceeded
		}
	}
}

// WaitForNotification waits for a notification with the given action
func (c *WSClient) WaitForNotification(action string, timeout time.Duration) (*ws.Message, error) {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-c.messages:
			if msg.Type == ws.MessageTypeNotification && msg.Action == action {
				return msg, nil
			}
		case <-deadline:
			return nil, context.DeadlineExceeded
		}
	}
}

// ============================================
// HEALTH CHECK TESTS
// ============================================

func TestHealthCheck(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("health-1", ws.ActionHealthCheck, map[string]interface{}{})
	require.NoError(t, err)

	assert.Equal(t, "health-1", resp.ID)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, ws.ActionHealthCheck, resp.Action)

	var payload map[string]interface{}
	err = resp.ParsePayload(&payload)
	require.NoError(t, err)

	assert.Equal(t, "ok", payload["status"])
	assert.Equal(t, "kandev", payload["service"])
}

// ============================================
// BOARD TESTS
// ============================================

func TestBoardCRUD(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)

	// Create board
	t.Run("CreateBoard", func(t *testing.T) {
		resp, err := client.SendRequest("board-create-1", ws.ActionBoardCreate, map[string]interface{}{
			"workspace_id":         workspaceID,
			"name":                 "Test Board",
			"description":          "A test board for integration testing",
			"workflow_template_id": "simple",
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.NotEmpty(t, payload["id"])
		assert.Equal(t, "Test Board", payload["name"])
	})

	// List boards
	t.Run("ListBoards", func(t *testing.T) {
		resp, err := client.SendRequest("board-list-1", ws.ActionBoardList, map[string]interface{}{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		boards, ok := payload["boards"].([]interface{})
		require.True(t, ok)
		assert.Len(t, boards, 1)
	})
}

// ============================================
// WORKFLOW STEP TESTS
// ============================================

func TestWorkflowStepList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)

	// Create a board (workflow steps are created automatically with default template)
	boardResp, err := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Workflow Step Test Board",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var boardPayload map[string]interface{}
	err = boardResp.ParsePayload(&boardPayload)
	require.NoError(t, err)
	boardID := boardPayload["id"].(string)

	// List workflow steps
	t.Run("ListWorkflowSteps", func(t *testing.T) {
		resp, err := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
			"board_id": boardID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		steps, ok := payload["steps"].([]interface{})
		require.True(t, ok)
		// Default workflow template creates 4 steps: Todo, In Progress, Review, Done
		assert.GreaterOrEqual(t, len(steps), 1)

		// Verify first step has expected fields
		firstStep := steps[0].(map[string]interface{})
		assert.NotEmpty(t, firstStep["id"])
		assert.NotEmpty(t, firstStep["name"])
	})
}

// ============================================
// TASK TESTS
// ============================================

func TestTaskCRUD(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create board (workflow steps are created automatically)
	boardResp, err := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Task Test Board",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var boardPayload map[string]interface{}
	err = boardResp.ParsePayload(&boardPayload)
	require.NoError(t, err)
	boardID := boardPayload["id"].(string)

	// Get first workflow step
	stepResp, err := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	require.NoError(t, err)

	var stepListPayload map[string]interface{}
	err = stepResp.ParsePayload(&stepListPayload)
	require.NoError(t, err)
	steps := stepListPayload["steps"].([]interface{})
	require.NotEmpty(t, steps)
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	var taskID string

	// Create task
	t.Run("CreateTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-create-1", ws.ActionTaskCreate, map[string]interface{}{
			"workspace_id":     workspaceID,
			"board_id":         boardID,
			"workflow_step_id": workflowStepID,
			"title":            "Test Task",
			"description":      "A test task for integration testing",
			"priority":         3, // HIGH priority (1=LOW, 2=MEDIUM, 3=HIGH)
			"repository_id":    repositoryID,
			"base_branch":      "main",
		})
		require.NoError(t, err)

		// Debug: print error if there is one
		if resp.Type == ws.MessageTypeError {
			var errPayload ws.ErrorPayload
			if err := resp.ParsePayload(&errPayload); err != nil {
				t.Fatalf("failed to parse error payload: %v", err)
			}
			t.Logf("Error response: %+v", errPayload)
		}

		require.Equal(t, ws.MessageTypeResponse, resp.Type, "Expected response but got %s", resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		taskID = payload["id"].(string)
		assert.NotEmpty(t, taskID)
		assert.Equal(t, "Test Task", payload["title"])
		assert.Equal(t, "CREATED", payload["state"])
	})

	// Get task
	t.Run("GetTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-get-1", ws.ActionTaskGet, map[string]interface{}{
			"id": taskID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, taskID, payload["id"])
		assert.Equal(t, "Test Task", payload["title"])
	})

	// Update task
	t.Run("UpdateTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-update-1", ws.ActionTaskUpdate, map[string]interface{}{
			"id":    taskID,
			"title": "Updated Task Title",
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, "Updated Task Title", payload["title"])
	})

	// List tasks
	t.Run("ListTasks", func(t *testing.T) {
		resp, err := client.SendRequest("task-list-1", ws.ActionTaskList, map[string]interface{}{
			"board_id": boardID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		tasks, ok := payload["tasks"].([]interface{})
		require.True(t, ok)
		assert.Len(t, tasks, 1)
	})

	// Delete task
	t.Run("DeleteTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-delete-1", ws.ActionTaskDelete, map[string]interface{}{
			"id": taskID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, true, payload["success"])

		// Verify task is deleted
		listResp, err := client.SendRequest("task-list-2", ws.ActionTaskList, map[string]interface{}{
			"board_id": boardID,
		})
		require.NoError(t, err)

		var listPayload map[string]interface{}
		err = listResp.ParsePayload(&listPayload)
		require.NoError(t, err)

		tasks, ok := listPayload["tasks"].([]interface{})
		require.True(t, ok)
		assert.Len(t, tasks, 0)
	})
}

// ============================================
// TASK STATE TESTS
// ============================================

func TestTaskStateTransitions(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create board and task (workflow steps are created automatically)
	boardResp, _ := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "State Test Board",
		"workflow_template_id": "simple",
	})
	var boardPayload map[string]interface{}
	if err := boardResp.ParsePayload(&boardPayload); err != nil {
		t.Fatalf("failed to parse board payload: %v", err)
	}
	boardID := boardPayload["id"].(string)

	// Get first workflow step
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	taskResp, _ := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"board_id":         boardID,
		"workflow_step_id": workflowStepID,
		"title":            "State Test Task",
		"description":      "Test state transitions",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	var taskPayload map[string]interface{}
	if err := taskResp.ParsePayload(&taskPayload); err != nil {
		t.Fatalf("failed to parse task payload: %v", err)
	}
	taskID := taskPayload["id"].(string)

	// Test state transitions
	stateTests := []struct {
		name     string
		newState string
	}{
		{"ToInProgress", "IN_PROGRESS"},
		{"ToCompleted", "COMPLETED"},
		{"BackToTodo", "TODO"},
		{"ToBlocked", "BLOCKED"},
	}

	for i, tc := range stateTests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.SendRequest(
				"state-"+string(rune('0'+i)),
				ws.ActionTaskState,
				map[string]interface{}{
					"id":    taskID,
					"state": tc.newState,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, ws.MessageTypeResponse, resp.Type)

			var payload map[string]interface{}
			err = resp.ParsePayload(&payload)
			require.NoError(t, err)

			assert.Equal(t, tc.newState, payload["state"])
		})
	}
}

// ============================================
// TASK MOVE TESTS
// ============================================

func TestTaskMove(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create board (workflow steps are created automatically)
	boardResp, _ := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Move Test Board",
		"workflow_template_id": "simple",
	})
	var boardPayload map[string]interface{}
	if err := boardResp.ParsePayload(&boardPayload); err != nil {
		t.Fatalf("failed to parse board payload: %v", err)
	}
	boardID := boardPayload["id"].(string)

	// Get workflow steps (at least 2 should exist from default template)
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	require.GreaterOrEqual(t, len(steps), 2, "Expected at least 2 workflow steps")
	step1ID := steps[0].(map[string]interface{})["id"].(string)
	step2ID := steps[1].(map[string]interface{})["id"].(string)

	// Create task in first step
	taskResp, _ := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"board_id":         boardID,
		"workflow_step_id": step1ID,
		"title":            "Movable Task",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	var taskPayload map[string]interface{}
	if err := taskResp.ParsePayload(&taskPayload); err != nil {
		t.Fatalf("failed to parse task payload: %v", err)
	}
	taskID := taskPayload["id"].(string)

	// Move task to second step
	t.Run("MoveToStep2", func(t *testing.T) {
		resp, err := client.SendRequest("move-1", ws.ActionTaskMove, map[string]interface{}{
			"id":               taskID,
			"board_id":         boardID,
			"workflow_step_id": step2ID,
			"position":         0,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		// Response is {"task": {...}, "workflow_step": {...}}
		taskData := payload["task"].(map[string]interface{})
		assert.Equal(t, step2ID, taskData["workflow_step_id"])
	})

	// Verify task is now in step 2
	t.Run("VerifyMove", func(t *testing.T) {
		resp, err := client.SendRequest("get-1", ws.ActionTaskGet, map[string]interface{}{
			"id": taskID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, step2ID, payload["workflow_step_id"])
	})
}

// ============================================
// MULTIPLE CLIENTS TESTS
// ============================================

func TestMultipleClients(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client1 := NewWSClient(t, ts.Server.URL)
	defer client1.Close()

	client2 := NewWSClient(t, ts.Server.URL)
	defer client2.Close()

	workspaceID := createWorkspace(t, client1)
	repositoryID := createRepository(t, client1, workspaceID)

	// Client 1 creates a board
	boardResp, err := client1.SendRequest("c1-board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Shared Board",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var boardPayload map[string]interface{}
	err = boardResp.ParsePayload(&boardPayload)
	require.NoError(t, err)
	boardID := boardPayload["id"].(string)

	// Client 2 can see the board (filter by workspace to avoid default board)
	listResp, err := client2.SendRequest("c2-list-1", ws.ActionBoardList, map[string]interface{}{
		"workspace_id": workspaceID,
	})
	require.NoError(t, err)

	var listPayload map[string]interface{}
	err = listResp.ParsePayload(&listPayload)
	require.NoError(t, err)

	boards, ok := listPayload["boards"].([]interface{})
	require.True(t, ok)
	assert.Len(t, boards, 1)

	// Client 2 lists workflow steps (created automatically with board)
	stepResp, err := client2.SendRequest("c2-step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	require.NoError(t, err)

	var stepPayload map[string]interface{}
	err = stepResp.ParsePayload(&stepPayload)
	require.NoError(t, err)
	steps := stepPayload["steps"].([]interface{})
	require.NotEmpty(t, steps)
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// Client 1 can also see the workflow steps
	stepListResp, err := client1.SendRequest("c1-step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	require.NoError(t, err)

	var stepListPayload map[string]interface{}
	err = stepListResp.ParsePayload(&stepListPayload)
	require.NoError(t, err)

	stepsList, ok := stepListPayload["steps"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(stepsList), 1)

	// Client 1 creates a task in the workflow step
	taskResp, err := client1.SendRequest("c1-task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"board_id":         boardID,
		"workflow_step_id": workflowStepID,
		"title":            "Task by Client 1",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	require.NoError(t, err)

	var taskPayload map[string]interface{}
	err = taskResp.ParsePayload(&taskPayload)
	require.NoError(t, err)

	// Client 2 can see the task
	taskListResp, err := client2.SendRequest("c2-task-list-1", ws.ActionTaskList, map[string]interface{}{
		"board_id": boardID,
	})
	require.NoError(t, err)

	var taskListPayload map[string]interface{}
	err = taskListResp.ParsePayload(&taskListPayload)
	require.NoError(t, err)

	tasks, ok := taskListPayload["tasks"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tasks, 1)
}

// ============================================
// ERROR HANDLING TESTS
// ============================================

func TestErrorHandling(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	t.Run("UnknownAction", func(t *testing.T) {
		resp, err := client.SendRequest("err-1", "unknown.action", map[string]interface{}{})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)

		var payload ws.ErrorPayload
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, ws.ErrorCodeUnknownAction, payload.Code)
	})

	t.Run("GetNonExistentBoard", func(t *testing.T) {
		resp, err := client.SendRequest("err-2", ws.ActionBoardGet, map[string]interface{}{
			"id": "non-existent-board-id",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("GetNonExistentTask", func(t *testing.T) {
		resp, err := client.SendRequest("err-3", ws.ActionTaskGet, map[string]interface{}{
			"id": "non-existent-task-id",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("CreateTaskWithoutBoard", func(t *testing.T) {
		resp, err := client.SendRequest("err-4", ws.ActionTaskCreate, map[string]interface{}{
			"title": "Orphan Task",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("ListWorkflowStepsWithoutBoard", func(t *testing.T) {
		resp, err := client.SendRequest("err-5", ws.ActionWorkflowStepList, map[string]interface{}{})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})
}

// ============================================
// TASK SUBSCRIPTION TESTS
// ============================================

func TestTaskSubscription(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create a task first (workflow steps are created automatically with board)
	boardResp, _ := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Sub Test Board",
		"workflow_template_id": "simple",
	})
	var boardPayload map[string]interface{}
	if err := boardResp.ParsePayload(&boardPayload); err != nil {
		t.Fatalf("failed to parse board payload: %v", err)
	}
	boardID := boardPayload["id"].(string)

	// Get first workflow step
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	taskResp, _ := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"board_id":         boardID,
		"workflow_step_id": workflowStepID,
		"title":            "Subscribable Task",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	var taskPayload map[string]interface{}
	if err := taskResp.ParsePayload(&taskPayload); err != nil {
		t.Fatalf("failed to parse task payload: %v", err)
	}
	taskID := taskPayload["id"].(string)

	// Subscribe to the task
	t.Run("Subscribe", func(t *testing.T) {
		resp, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]interface{}{
			"task_id": taskID,
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, true, payload["success"])
		assert.Equal(t, taskID, payload["task_id"])
	})

	// Unsubscribe from the task
	t.Run("Unsubscribe", func(t *testing.T) {
		resp, err := client.SendRequest("unsub-1", ws.ActionTaskUnsubscribe, map[string]interface{}{
			"task_id": taskID,
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, true, payload["success"])
	})
}

// ============================================
// CONCURRENT REQUEST TESTS
// ============================================

func TestConcurrentRequests(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create board (workflow steps are created automatically)
	boardResp, _ := client.SendRequest("board-1", ws.ActionBoardCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Concurrent Test Board",
		"workflow_template_id": "simple",
	})
	var boardPayload map[string]interface{}
	if err := boardResp.ParsePayload(&boardPayload); err != nil {
		t.Fatalf("failed to parse board payload: %v", err)
	}
	boardID := boardPayload["id"].(string)

	// Get first workflow step
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"board_id": boardID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// Create multiple tasks concurrently
	numTasks := 10
	var wg sync.WaitGroup
	results := make(chan error, numTasks)

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Create separate client for concurrency
			c := NewWSClient(t, ts.Server.URL)
			defer c.Close()

			_, err := c.SendRequest(
				"task-"+string(rune('0'+idx)),
				ws.ActionTaskCreate,
				map[string]interface{}{
					"workspace_id":     workspaceID,
					"board_id":         boardID,
					"workflow_step_id": workflowStepID,
					"title":            "Concurrent Task " + string(rune('0'+idx)),
					"repository_id":    repositoryID,
					"base_branch":      "main",
				},
			)
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	// Verify all tasks were created
	for err := range results {
		assert.NoError(t, err)
	}

	// Verify task count
	listResp, err := client.SendRequest("list-1", ws.ActionTaskList, map[string]interface{}{
		"board_id": boardID,
	})
	require.NoError(t, err)

	var listPayload map[string]interface{}
	err = listResp.ParsePayload(&listPayload)
	require.NoError(t, err)

	tasks, ok := listPayload["tasks"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tasks, numTasks)
}
