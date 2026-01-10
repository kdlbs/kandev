package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

// MockLifecycleManager implements a mock for lifecycle.Manager
type MockLifecycleManager struct {
	LaunchFn             func(ctx context.Context, req *lifecycle.LaunchRequest) (*lifecycle.AgentInstance, error)
	StopAgentFn          func(ctx context.Context, instanceID string, force bool) error
	GetInstanceFn        func(instanceID string) (*lifecycle.AgentInstance, bool)
	ListInstancesFn      func() []*lifecycle.AgentInstance
	GetInstanceByTaskIDFn func(taskID string) (*lifecycle.AgentInstance, bool)
}

func (m *MockLifecycleManager) Launch(ctx context.Context, req *lifecycle.LaunchRequest) (*lifecycle.AgentInstance, error) {
	if m.LaunchFn != nil {
		return m.LaunchFn(ctx, req)
	}
	return &lifecycle.AgentInstance{
		ID:          "mock-instance-id",
		TaskID:      req.TaskID,
		AgentType:   req.AgentType,
		ContainerID: "mock-container-id",
		Status:      v1.AgentStatusRunning,
		StartedAt:   time.Now(),
	}, nil
}

func (m *MockLifecycleManager) StopAgent(ctx context.Context, instanceID string, force bool) error {
	if m.StopAgentFn != nil {
		return m.StopAgentFn(ctx, instanceID, force)
	}
	return nil
}

func (m *MockLifecycleManager) GetInstance(instanceID string) (*lifecycle.AgentInstance, bool) {
	if m.GetInstanceFn != nil {
		return m.GetInstanceFn(instanceID)
	}
	return nil, false
}

func (m *MockLifecycleManager) ListInstances() []*lifecycle.AgentInstance {
	if m.ListInstancesFn != nil {
		return m.ListInstancesFn()
	}
	return []*lifecycle.AgentInstance{}
}

func (m *MockLifecycleManager) GetInstanceByTaskID(taskID string) (*lifecycle.AgentInstance, bool) {
	if m.GetInstanceByTaskIDFn != nil {
		return m.GetInstanceByTaskIDFn(taskID)
	}
	return nil, false
}

// MockDockerClient for handler tests
type MockDockerClient struct {
	GetContainerLogsFn func(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error)
}

func (m *MockDockerClient) GetContainerLogs(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error) {
	if m.GetContainerLogsFn != nil {
		return m.GetContainerLogsFn(ctx, containerID, follow, tail)
	}
	return io.NopCloser(strings.NewReader("test log line")), nil
}

func newTestRegistry() *registry.Registry {
	log := newTestLogger()
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	return reg
}

// Helper to create a test router with handlers
func setupTestRouter(
	mockLM *MockLifecycleManager,
	reg *registry.Registry,
	mockDocker *MockDockerClient,
) *gin.Engine {
	log := newTestLogger()

	router := gin.New()
	apiV1 := router.Group("/api/v1")

	// Create handler with lifecycle manager wrapper
	handler := &Handler{
		registry: reg,
		docker:   mockDocker,
		logger:   log,
	}

	// Set up routes manually for testing
	agents := apiV1.Group("/agents")
	{
		agents.GET("", func(c *gin.Context) {
			instances := mockLM.ListInstances()
			agentsList := make([]AgentInstanceResponse, 0, len(instances))
			for _, instance := range instances {
				agentsList = append(agentsList, instanceToResponse(instance))
			}
			c.JSON(http.StatusOK, AgentsListResponse{Agents: agentsList, Total: len(agentsList)})
		})

		agents.POST("/launch", func(c *gin.Context) {
			var req LaunchAgentRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			launchReq := &lifecycle.LaunchRequest{
				TaskID:        req.TaskID,
				AgentType:     req.AgentType,
				WorkspacePath: req.WorkspacePath,
				Env:           req.Env,
				Metadata:      req.Metadata,
			}
			instance, err := mockLM.Launch(c.Request.Context(), launchReq)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, instanceToResponse(instance))
		})

		agents.GET("/types", handler.ListAgentTypes)
		agents.GET("/types/:typeId", handler.GetAgentType)

		agents.GET("/:instanceId/status", func(c *gin.Context) {
			instanceID := c.Param("instanceId")
			instance, found := mockLM.GetInstance(instanceID)
			if !found {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.JSON(http.StatusOK, instanceToResponse(instance))
		})

		agents.DELETE("/:instanceId", func(c *gin.Context) {
			instanceID := c.Param("instanceId")
			var req StopAgentRequest
			_ = c.ShouldBindJSON(&req)
			err := mockLM.StopAgent(c.Request.Context(), instanceID, req.Force)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "agent stopped successfully"})
		})
	}

	router.GET("/health", handler.HealthCheck)

	return router
}

func TestHandler_LaunchAgent(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	reqBody := LaunchAgentRequest{
		TaskID:        "task-123",
		AgentType:     "augment-agent",
		WorkspacePath: "/path/to/workspace",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/agents/launch", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp AgentInstanceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %q", resp.TaskID)
	}
}

func TestHandler_LaunchAgent_InvalidRequest(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	// Missing required fields
	reqBody := map[string]string{}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/agents/launch", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandler_StopAgent(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("DELETE", "/api/v1/agents/instance-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestHandler_StopAgent_NotFound(t *testing.T) {
	mockLM := &MockLifecycleManager{
		StopAgentFn: func(ctx context.Context, instanceID string, force bool) error {
			return &mockError{msg: "instance not found"}
		},
	}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("DELETE", "/api/v1/agents/non-existent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func TestHandler_GetAgentStatus(t *testing.T) {
	now := time.Now()
	mockLM := &MockLifecycleManager{
		GetInstanceFn: func(instanceID string) (*lifecycle.AgentInstance, bool) {
			if instanceID == "instance-123" {
				return &lifecycle.AgentInstance{
					ID:          "instance-123",
					TaskID:      "task-456",
					AgentType:   "augment-agent",
					ContainerID: "container-789",
					Status:      v1.AgentStatusRunning,
					StartedAt:   now,
					Progress:    50,
				}, true
			}
			return nil, false
		},
	}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents/instance-123/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp AgentInstanceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.ID != "instance-123" {
		t.Errorf("expected ID 'instance-123', got %q", resp.ID)
	}
	if resp.Status != "RUNNING" {
		t.Errorf("expected status 'RUNNING', got %q", resp.Status)
	}
	if resp.Progress != 50 {
		t.Errorf("expected progress 50, got %d", resp.Progress)
	}
}

func TestHandler_GetAgentStatus_NotFound(t *testing.T) {
	mockLM := &MockLifecycleManager{
		GetInstanceFn: func(instanceID string) (*lifecycle.AgentInstance, bool) {
			return nil, false
		},
	}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents/non-existent/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_ListAgents(t *testing.T) {
	now := time.Now()
	mockLM := &MockLifecycleManager{
		ListInstancesFn: func() []*lifecycle.AgentInstance {
			return []*lifecycle.AgentInstance{
				{
					ID:          "instance-1",
					TaskID:      "task-1",
					AgentType:   "augment-agent",
					ContainerID: "container-1",
					Status:      v1.AgentStatusRunning,
					StartedAt:   now,
				},
				{
					ID:          "instance-2",
					TaskID:      "task-2",
					AgentType:   "augment-agent",
					ContainerID: "container-2",
					Status:      v1.AgentStatusCompleted,
					StartedAt:   now,
				},
			}
		},
	}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp AgentsListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(resp.Agents))
	}
}

func TestHandler_ListAgents_Empty(t *testing.T) {
	mockLM := &MockLifecycleManager{
		ListInstancesFn: func() []*lifecycle.AgentInstance {
			return []*lifecycle.AgentInstance{}
		},
	}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp AgentsListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

func TestHandler_ListAgentTypes(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents/types", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp AgentTypesListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Should have at least the default agents
	if resp.Total == 0 {
		t.Error("expected at least one agent type")
	}
}

func TestHandler_GetAgentType(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents/types/augment-agent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp AgentTypeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.ID != "augment-agent" {
		t.Errorf("expected ID 'augment-agent', got %q", resp.ID)
	}
}

func TestHandler_GetAgentType_NotFound(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/api/v1/agents/types/non-existent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_HealthCheck(t *testing.T) {
	mockLM := &MockLifecycleManager{}
	reg := newTestRegistry()
	mockDocker := &MockDockerClient{}

	router := setupTestRouter(mockLM, reg, mockDocker)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", resp.Status)
	}
}

func TestInstanceToResponse(t *testing.T) {
	now := time.Now()
	exitCode := 0
	finishedAt := now.Add(time.Hour)

	instance := &lifecycle.AgentInstance{
		ID:           "test-id",
		TaskID:       "task-id",
		AgentType:    "test-agent",
		ContainerID:  "container-id",
		Status:       v1.AgentStatusCompleted,
		StartedAt:    now,
		FinishedAt:   &finishedAt,
		ExitCode:     &exitCode,
		ErrorMessage: "",
		Progress:     100,
		Metadata:     map[string]interface{}{"key": "value"},
	}

	resp := instanceToResponse(instance)

	if resp.ID != instance.ID {
		t.Errorf("expected ID %q, got %q", instance.ID, resp.ID)
	}
	if resp.TaskID != instance.TaskID {
		t.Errorf("expected TaskID %q, got %q", instance.TaskID, resp.TaskID)
	}
	if resp.Status != string(instance.Status) {
		t.Errorf("expected Status %q, got %q", instance.Status, resp.Status)
	}
	if resp.Progress != instance.Progress {
		t.Errorf("expected Progress %d, got %d", instance.Progress, resp.Progress)
	}
	if resp.ExitCode == nil || *resp.ExitCode != exitCode {
		t.Errorf("expected ExitCode %d, got %v", exitCode, resp.ExitCode)
	}
	if resp.Metadata["key"] != "value" {
		t.Error("expected metadata to be preserved")
	}
}

