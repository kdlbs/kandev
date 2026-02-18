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
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
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
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	taskRepoImpl, cleanup, err := repository.Provide(sqlxDB)
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
	if _, err := worktree.NewSQLiteStore(sqlxDB); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}

	// Initialize workflow service
	_, workflowSvc, _, err := workflow.Provide(sqlxDB, log)
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
	workspaceController := taskcontroller.NewWorkspaceController(taskSvc)
	taskWorkflowController := taskcontroller.NewWorkflowController(taskSvc)
	taskWorkflowController.SetWorkflowStepLister(workflowSvc)
	taskController := taskcontroller.NewTaskController(taskSvc)
	executorController := taskcontroller.NewExecutorController(taskSvc)
	environmentController := taskcontroller.NewEnvironmentController(taskSvc)
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)
	workflowCtrl := workflowcontroller.NewController(workflowSvc)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, workspaceController, log)
	taskhandlers.RegisterWorkflowRoutes(router, gateway.Dispatcher, taskWorkflowController, log)
	planService := taskservice.NewPlanService(taskRepo, eventBus, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskController, nil, taskRepo, planService, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, repositoryController, log)
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, executorController, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, environmentController, log)
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
