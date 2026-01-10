// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// Common errors
var (
	ErrMaxConcurrentReached = errors.New("maximum concurrent executions reached")
	ErrNoAgentType          = errors.New("task has no agent_type configured")
	ErrExecutionNotFound    = errors.New("execution not found")
)

// AgentManagerClient is an interface for the Agent Manager service
// This will be implemented via gRPC or HTTP client
type AgentManagerClient interface {
	// LaunchAgent starts a new agent container for a task
	LaunchAgent(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error)

	// StopAgent stops a running agent
	StopAgent(ctx context.Context, agentInstanceID string, force bool) error

	// GetAgentStatus returns the status of an agent instance
	GetAgentStatus(ctx context.Context, agentInstanceID string) (*v1.AgentInstance, error)

	// ListAgentTypes returns available agent types
	ListAgentTypes(ctx context.Context) ([]*v1.AgentType, error)
}

// LaunchAgentRequest contains parameters for launching an agent
type LaunchAgentRequest struct {
	TaskID        string
	AgentType     string
	RepositoryURL string
	Branch        string
	Priority      int
	Metadata      map[string]interface{}
}

// LaunchAgentResponse contains the result of launching an agent
type LaunchAgentResponse struct {
	AgentInstanceID string
	ContainerID     string
	Status          v1.AgentStatus
}

// TaskExecution tracks an active task execution
type TaskExecution struct {
	TaskID          string
	AgentInstanceID string
	AgentType       string
	StartedAt       time.Time
	Status          v1.AgentStatus
	Progress        int
	LastUpdate      time.Time
}

// Executor manages agent execution for tasks
type Executor struct {
	agentManager AgentManagerClient
	logger       *logger.Logger

	// Track active executions
	executions map[string]*TaskExecution
	mu         sync.RWMutex

	// Configuration
	maxConcurrent int
	retryLimit    int
	retryDelay    time.Duration
}

// NewExecutor creates a new executor
func NewExecutor(agentManager AgentManagerClient, log *logger.Logger, maxConcurrent int) *Executor {
	if maxConcurrent <= 0 {
		maxConcurrent = 5 // default
	}
	return &Executor{
		agentManager:  agentManager,
		logger:        log.WithFields(zap.String("component", "executor")),
		executions:    make(map[string]*TaskExecution),
		maxConcurrent: maxConcurrent,
		retryLimit:    3,
		retryDelay:    5 * time.Second,
	}
}

// Execute starts agent execution for a task
func (e *Executor) Execute(ctx context.Context, task *v1.Task) (*TaskExecution, error) {
	// Check if max concurrent limit is reached
	if !e.CanExecute() {
		e.logger.Warn("max concurrent executions reached",
			zap.Int("max", e.maxConcurrent),
			zap.Int("current", e.ActiveCount()))
		return nil, ErrMaxConcurrentReached
	}

	// Verify task has an agent_type configured
	if task.AgentType == nil || *task.AgentType == "" {
		e.logger.Error("task has no agent_type configured", zap.String("task_id", task.ID))
		return nil, ErrNoAgentType
	}

	// Create a LaunchAgentRequest from the task
	req := &LaunchAgentRequest{
		TaskID:    task.ID,
		AgentType: *task.AgentType,
		Priority:  task.Priority,
		Metadata:  task.Metadata,
	}
	if task.RepositoryURL != nil {
		req.RepositoryURL = *task.RepositoryURL
	}
	if task.Branch != nil {
		req.Branch = *task.Branch
	}

	e.logger.Info("launching agent for task",
		zap.String("task_id", task.ID),
		zap.String("agent_type", *task.AgentType))

	// Call the AgentManager to launch the container
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to launch agent",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return nil, err
	}

	// Track the execution in the executions map
	now := time.Now()
	execution := &TaskExecution{
		TaskID:          task.ID,
		AgentInstanceID: resp.AgentInstanceID,
		AgentType:       *task.AgentType,
		StartedAt:       now,
		Status:          resp.Status,
		Progress:        0,
		LastUpdate:      now,
	}

	e.mu.Lock()
	e.executions[task.ID] = execution
	e.mu.Unlock()

	e.logger.Info("agent launched successfully",
		zap.String("task_id", task.ID),
		zap.String("agent_instance_id", resp.AgentInstanceID),
		zap.String("container_id", resp.ContainerID))

	return execution, nil
}

// Stop stops an active execution
func (e *Executor) Stop(ctx context.Context, taskID string, reason string, force bool) error {
	e.mu.RLock()
	execution, exists := e.executions[taskID]
	e.mu.RUnlock()

	if !exists {
		return ErrExecutionNotFound
	}

	e.logger.Info("stopping execution",
		zap.String("task_id", taskID),
		zap.String("agent_instance_id", execution.AgentInstanceID),
		zap.String("reason", reason),
		zap.Bool("force", force))

	err := e.agentManager.StopAgent(ctx, execution.AgentInstanceID, force)
	if err != nil {
		e.logger.Error("failed to stop agent",
			zap.String("task_id", taskID),
			zap.Error(err))
		return err
	}

	// Update execution status
	e.mu.Lock()
	if exec, ok := e.executions[taskID]; ok {
		exec.Status = v1.AgentStatusStopped
		exec.LastUpdate = time.Now()
	}
	e.mu.Unlock()

	return nil
}

// GetExecution returns the current execution state for a task
func (e *Executor) GetExecution(taskID string) (*TaskExecution, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	execution, exists := e.executions[taskID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid data races
	copy := *execution
	return &copy, true
}

// ListExecutions returns all active executions
func (e *Executor) ListExecutions() []*TaskExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*TaskExecution, 0, len(e.executions))
	for _, exec := range e.executions {
		copy := *exec
		result = append(result, &copy)
	}
	return result
}

// ActiveCount returns the number of active executions
func (e *Executor) ActiveCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.executions)
}

// CanExecute returns true if there's capacity for another execution
func (e *Executor) CanExecute() bool {
	return e.ActiveCount() < e.maxConcurrent
}

// UpdateProgress updates the progress of an execution (called when ACP progress messages are received)
func (e *Executor) UpdateProgress(taskID string, progress int, status v1.AgentStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if exec, ok := e.executions[taskID]; ok {
		exec.Progress = progress
		exec.Status = status
		exec.LastUpdate = time.Now()

		e.logger.Debug("updated execution progress",
			zap.String("task_id", taskID),
			zap.Int("progress", progress),
			zap.String("status", string(status)))
	}
}

// MarkCompleted marks an execution as completed
func (e *Executor) MarkCompleted(taskID string, status v1.AgentStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if exec, ok := e.executions[taskID]; ok {
		exec.Status = status
		exec.Progress = 100
		exec.LastUpdate = time.Now()

		e.logger.Info("execution completed",
			zap.String("task_id", taskID),
			zap.String("status", string(status)))
	}
}

// RemoveExecution removes a completed/failed execution from tracking
func (e *Executor) RemoveExecution(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.executions[taskID]; ok {
		delete(e.executions, taskID)
		e.logger.Debug("removed execution from tracking", zap.String("task_id", taskID))
	}
}

// MockAgentManagerClient is a placeholder implementation
type MockAgentManagerClient struct {
	logger *logger.Logger
}

// NewMockAgentManagerClient creates a new mock agent manager client
func NewMockAgentManagerClient(log *logger.Logger) *MockAgentManagerClient {
	return &MockAgentManagerClient{
		logger: log.WithFields(zap.String("component", "mock_agent_manager")),
	}
}

// LaunchAgent mocks launching an agent container
func (m *MockAgentManagerClient) LaunchAgent(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
	m.logger.Info("mock: launching agent",
		zap.String("task_id", req.TaskID),
		zap.String("agent_type", req.AgentType),
		zap.String("repository_url", req.RepositoryURL),
		zap.String("branch", req.Branch))

	return &LaunchAgentResponse{
		AgentInstanceID: uuid.New().String(),
		ContainerID:     "mock-container-" + uuid.New().String()[:8],
		Status:          v1.AgentStatusStarting,
	}, nil
}

// StopAgent mocks stopping an agent
func (m *MockAgentManagerClient) StopAgent(ctx context.Context, agentInstanceID string, force bool) error {
	m.logger.Info("mock: stopping agent",
		zap.String("agent_instance_id", agentInstanceID),
		zap.Bool("force", force))
	return nil
}

// GetAgentStatus mocks getting agent status
func (m *MockAgentManagerClient) GetAgentStatus(ctx context.Context, agentInstanceID string) (*v1.AgentInstance, error) {
	m.logger.Info("mock: getting agent status",
		zap.String("agent_instance_id", agentInstanceID))

	return &v1.AgentInstance{
		ID:        agentInstanceID,
		Status:    v1.AgentStatusRunning,
		AgentType: "mock-agent",
	}, nil
}

// ListAgentTypes mocks listing available agent types
func (m *MockAgentManagerClient) ListAgentTypes(ctx context.Context) ([]*v1.AgentType, error) {
	m.logger.Info("mock: listing agent types")

	return []*v1.AgentType{
		{
			ID:          "auggie-cli",
			Name:        "Auggie CLI Agent",
			Description: "CLI-based agent for code tasks",
			DockerImage: "auggie-cli",
			DockerTag:   "latest",
			Enabled:     true,
		},
	}, nil
}
