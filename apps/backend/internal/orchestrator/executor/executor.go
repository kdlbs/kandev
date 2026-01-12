// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// Common errors
var (
	ErrMaxConcurrentReached = errors.New("maximum concurrent executions reached")
	ErrNoAgentType          = errors.New("task has no agent_type configured")
	ErrExecutionNotFound    = errors.New("execution not found")
)

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

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

	// PromptAgent sends a prompt to a running agent
	// Returns PromptResult indicating if the agent needs input
	PromptAgent(ctx context.Context, agentInstanceID string, prompt string) (*PromptResult, error)

	// RespondToPermission sends a response to a permission request
	RespondToPermissionByTaskID(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error

	// GetRecoveredInstances returns instances recovered from Docker during startup
	GetRecoveredInstances() []RecoveredInstanceInfo
}

// LaunchAgentRequest contains parameters for launching an agent
type LaunchAgentRequest struct {
	TaskID          string
	AgentType       string
	RepositoryURL   string
	Branch          string
	TaskDescription string // Task description to send via ACP prompt
	Priority        int
	Metadata        map[string]interface{}

	// Worktree configuration for concurrent agent execution
	UseWorktree    bool   // Whether to use a Git worktree for isolation
	RepositoryID   string // Repository ID for worktree tracking
	RepositoryPath string // Path to the main repository (for worktree creation)
	BaseBranch     string // Base branch for the worktree (e.g., "main")
}

// LaunchAgentResponse contains the result of launching an agent
type LaunchAgentResponse struct {
	AgentInstanceID string
	ContainerID     string
	Status          v1.AgentStatus
	WorktreePath    string
	WorktreeBranch  string
}

// TaskExecution tracks an active task execution (kept for API compatibility)
type TaskExecution struct {
	TaskID          string
	AgentInstanceID string
	AgentType       string
	StartedAt       time.Time
	Status          v1.AgentStatus
	Progress        int
	LastUpdate      time.Time
	// SessionID is the database ID of the agent session
	SessionID string
	// Worktree info for the agent
	WorktreePath   string
	WorktreeBranch string
}

// FromAgentSession converts a models.AgentSession to TaskExecution
func FromAgentSession(s *models.AgentSession) *TaskExecution {
	return &TaskExecution{
		TaskID:          s.TaskID,
		AgentInstanceID: s.AgentInstanceID,
		AgentType:       s.AgentType,
		StartedAt:       s.StartedAt,
		Status:          agentSessionStatusToV1(s.Status),
		Progress:        s.Progress,
		LastUpdate:      s.UpdatedAt,
		SessionID:       s.ID,
	}
}

// agentSessionStatusToV1 converts models.AgentSessionStatus to v1.AgentStatus
func agentSessionStatusToV1(status models.AgentSessionStatus) v1.AgentStatus {
	switch status {
	case models.AgentSessionStatusPending:
		return v1.AgentStatusStarting
	case models.AgentSessionStatusRunning:
		return v1.AgentStatusRunning
	case models.AgentSessionStatusWaiting:
		return v1.AgentStatusRunning // Waiting is still "running" from agent perspective
	case models.AgentSessionStatusCompleted:
		return v1.AgentStatusCompleted
	case models.AgentSessionStatusFailed:
		return v1.AgentStatusFailed
	case models.AgentSessionStatusStopped:
		return v1.AgentStatusStopped
	default:
		return v1.AgentStatusPending
	}
}

// v1StatusToAgentSessionStatus converts v1.AgentStatus to models.AgentSessionStatus
func v1StatusToAgentSessionStatus(status v1.AgentStatus) models.AgentSessionStatus {
	switch status {
	case v1.AgentStatusStarting:
		return models.AgentSessionStatusPending
	case v1.AgentStatusRunning:
		return models.AgentSessionStatusRunning
	case v1.AgentStatusCompleted:
		return models.AgentSessionStatusCompleted
	case v1.AgentStatusFailed:
		return models.AgentSessionStatusFailed
	case v1.AgentStatusStopped:
		return models.AgentSessionStatusStopped
	default:
		return models.AgentSessionStatusPending
	}
}

// Executor manages agent execution for tasks
type Executor struct {
	agentManager AgentManagerClient
	repo         repository.Repository
	logger       *logger.Logger

	// In-memory cache for fast lookups (synced with database)
	executions map[string]*TaskExecution
	mu         sync.RWMutex

	// Configuration
	maxConcurrent   int
	retryLimit      int
	retryDelay      time.Duration
	worktreeEnabled bool // Whether to use Git worktrees for agent isolation
}

// ExecutorConfig holds configuration for the Executor
type ExecutorConfig struct {
	MaxConcurrent   int  // Maximum concurrent agent executions
	WorktreeEnabled bool // Whether to use Git worktrees for agent isolation
}

// NewExecutor creates a new executor
func NewExecutor(agentManager AgentManagerClient, repo repository.Repository, log *logger.Logger, cfg ExecutorConfig) *Executor {
	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5 // default
	}
	return &Executor{
		agentManager:    agentManager,
		repo:            repo,
		logger:          log.WithFields(zap.String("component", "executor")),
		executions:      make(map[string]*TaskExecution),
		maxConcurrent:   maxConcurrent,
		retryLimit:      3,
		retryDelay:      5 * time.Second,
		worktreeEnabled: cfg.WorktreeEnabled,
	}
}

// SetWorktreeEnabled enables or disables worktree mode
func (e *Executor) SetWorktreeEnabled(enabled bool) {
	e.worktreeEnabled = enabled
}

// LoadActiveSessionsFromDB loads active agent sessions from the database into memory
// This should be called on startup to restore state after a restart
func (e *Executor) LoadActiveSessionsFromDB(ctx context.Context) error {
	sessions, err := e.repo.ListActiveAgentSessions(ctx)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, session := range sessions {
		e.executions[session.TaskID] = FromAgentSession(session)
		e.logger.Info("restored agent session from database",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID),
			zap.String("status", string(session.Status)))
	}

	e.logger.Info("loaded active agent sessions from database",
		zap.Int("count", len(sessions)))

	return nil
}

// RecoveredInstanceInfo contains info about an instance recovered from Docker
type RecoveredInstanceInfo struct {
	InstanceID  string
	TaskID      string
	ContainerID string
	AgentType   string
}

// SyncWithRecoveredInstances ensures the executor's cache is in sync with
// instances recovered from Docker by the lifecycle manager.
// For each recovered instance, if not already in cache, add it.
func (e *Executor) SyncWithRecoveredInstances(ctx context.Context, instances []RecoveredInstanceInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, inst := range instances {
		if _, exists := e.executions[inst.TaskID]; exists {
			continue // Already have this execution
		}

		// Add to cache - this instance is running in Docker but wasn't in DB as active
		e.executions[inst.TaskID] = &TaskExecution{
			TaskID:          inst.TaskID,
			AgentInstanceID: inst.InstanceID,
			AgentType:       inst.AgentType,
			StartedAt:       time.Now(), // We don't know exact start time
			Status:          v1.AgentStatusRunning,
			Progress:        0,
			LastUpdate:      time.Now(),
		}

		e.logger.Info("synced recovered instance to executor cache",
			zap.String("task_id", inst.TaskID),
			zap.String("instance_id", inst.InstanceID))
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
		TaskID:          task.ID,
		AgentType:       *task.AgentType,
		TaskDescription: task.Description,
		Priority:        task.Priority,
		Metadata:        task.Metadata,
	}
	if task.RepositoryURL != nil {
		req.RepositoryURL = *task.RepositoryURL
	}
	if task.Branch != nil {
		req.Branch = *task.Branch
	}

	// Configure worktree if enabled and repository path is available
	if e.worktreeEnabled && req.RepositoryURL != "" {
		req.UseWorktree = true
		req.RepositoryPath = req.RepositoryURL // RepositoryURL is actually a local path
		req.RepositoryID = task.ID             // Use task ID as repository identifier for worktree tracking
		if req.Branch != "" {
			req.BaseBranch = req.Branch
		} else {
			req.BaseBranch = "main" // Default base branch
		}
	}

	e.logger.Info("launching agent for task",
		zap.String("task_id", task.ID),
		zap.String("agent_type", *task.AgentType),
		zap.Bool("use_worktree", req.UseWorktree))

	// Call the AgentManager to launch the container
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to launch agent",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return nil, err
	}

	// Create agent session in database
	sessionID := uuid.New().String()
	now := time.Now().UTC()
	session := &models.AgentSession{
		ID:              sessionID,
		TaskID:          task.ID,
		AgentInstanceID: resp.AgentInstanceID,
		ContainerID:     resp.ContainerID,
		AgentType:       *task.AgentType,
		Status:          v1StatusToAgentSessionStatus(resp.Status),
		Progress:        0,
		StartedAt:       now,
		UpdatedAt:       now,
	}

	if err := e.repo.CreateAgentSession(ctx, session); err != nil {
		e.logger.Error("failed to persist agent session",
			zap.String("task_id", task.ID),
			zap.Error(err))
		// Continue anyway - the agent is already running
	}

	// Track the execution in the in-memory cache
	execution := &TaskExecution{
		TaskID:          task.ID,
		AgentInstanceID: resp.AgentInstanceID,
		AgentType:       *task.AgentType,
		StartedAt:       now,
		Status:          resp.Status,
		Progress:        0,
		LastUpdate:      now,
		SessionID:       sessionID,
		WorktreePath:    resp.WorktreePath,
		WorktreeBranch:  resp.WorktreeBranch,
	}

	e.mu.Lock()
	e.executions[task.ID] = execution
	e.mu.Unlock()

	e.logger.Info("agent launched successfully",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_instance_id", resp.AgentInstanceID),
		zap.String("container_id", resp.ContainerID),
		zap.String("worktree_path", resp.WorktreePath),
		zap.String("worktree_branch", resp.WorktreeBranch))

	return execution, nil
}

// Stop stops an active execution
func (e *Executor) Stop(ctx context.Context, taskID string, reason string, force bool) error {
	execution, err := e.getOrLoadExecution(ctx, taskID)
	if err != nil {
		return err
	}

	e.logger.Info("stopping execution",
		zap.String("task_id", taskID),
		zap.String("agent_instance_id", execution.AgentInstanceID),
		zap.String("reason", reason),
		zap.Bool("force", force))

	err = e.agentManager.StopAgent(ctx, execution.AgentInstanceID, force)
	if err != nil {
		// Log the error but continue to clean up execution state
		// The agent instance may already be gone (container stopped externally)
		e.logger.Warn("failed to stop agent (may already be stopped)",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Update execution status in memory and database regardless of agent stop result
	e.mu.Lock()
	if exec, ok := e.executions[taskID]; ok {
		exec.Status = v1.AgentStatusStopped
		exec.LastUpdate = time.Now()
	}
	delete(e.executions, taskID) // Remove from cache so GetExecution returns false
	e.mu.Unlock()

	// Update database
	if execution.SessionID != "" {
		if dbErr := e.repo.UpdateAgentSessionStatus(ctx, execution.SessionID, models.AgentSessionStatusStopped, reason); dbErr != nil {
			e.logger.Error("failed to update agent session status in database",
				zap.String("task_id", taskID),
				zap.String("session_id", execution.SessionID),
				zap.Error(dbErr))
		}
	}

	return nil
}

// Prompt sends a follow-up prompt to a running agent for a task
// Returns PromptResult indicating if the agent needs input
func (e *Executor) Prompt(ctx context.Context, taskID string, prompt string) (*PromptResult, error) {
	execution, err := e.getOrLoadExecution(ctx, taskID)
	if err != nil {
		return nil, err
	}

	e.logger.Info("sending prompt to agent",
		zap.String("task_id", taskID),
		zap.String("agent_instance_id", execution.AgentInstanceID),
		zap.Int("prompt_length", len(prompt)))

	return e.agentManager.PromptAgent(ctx, execution.AgentInstanceID, prompt)
}

// getOrLoadExecution gets execution from memory cache or loads from database
func (e *Executor) getOrLoadExecution(ctx context.Context, taskID string) (*TaskExecution, error) {
	// First check in-memory cache
	e.mu.RLock()
	execution, exists := e.executions[taskID]
	e.mu.RUnlock()

	if exists {
		return execution, nil
	}

	// Try to load from database
	session, err := e.repo.GetActiveAgentSessionByTaskID(ctx, taskID)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "no active agent session") {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}

	// Cache it in memory
	execution = FromAgentSession(session)
	e.mu.Lock()
	e.executions[taskID] = execution
	e.mu.Unlock()

	e.logger.Info("loaded agent session from database",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID))

	return execution, nil
}

// RespondToPermission sends a response to a permission request for a task
func (e *Executor) RespondToPermission(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error {
	e.logger.Info("responding to permission request",
		zap.String("task_id", taskID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	return e.agentManager.RespondToPermissionByTaskID(ctx, taskID, pendingID, optionID, cancelled)
}

// GetExecution returns the current execution state for a task
func (e *Executor) GetExecution(taskID string) (*TaskExecution, bool) {
	e.mu.RLock()
	execution, exists := e.executions[taskID]
	e.mu.RUnlock()

	if exists {
		// Return a copy to avoid data races
		execCopy := *execution
		return &execCopy, true
	}

	// Try to load from database
	ctx := context.Background()
	session, err := e.repo.GetActiveAgentSessionByTaskID(ctx, taskID)
	if err != nil {
		return nil, false
	}

	// Cache it in memory
	execution = FromAgentSession(session)
	e.mu.Lock()
	e.executions[taskID] = execution
	e.mu.Unlock()

	execCopy := *execution
	return &execCopy, true
}

// GetExecutionWithContext returns the current execution state for a task with context
func (e *Executor) GetExecutionWithContext(ctx context.Context, taskID string) (*TaskExecution, bool) {
	e.mu.RLock()
	execution, exists := e.executions[taskID]
	e.mu.RUnlock()

	if exists {
		execCopy := *execution
		return &execCopy, true
	}

	// Try to load from database
	session, err := e.repo.GetActiveAgentSessionByTaskID(ctx, taskID)
	if err != nil {
		return nil, false
	}

	// Cache it in memory
	execution = FromAgentSession(session)
	e.mu.Lock()
	e.executions[taskID] = execution
	e.mu.Unlock()

	execCopy := *execution
	return &execCopy, true
}

// ListExecutions returns all active executions
func (e *Executor) ListExecutions() []*TaskExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*TaskExecution, 0, len(e.executions))
	for _, exec := range e.executions {
		execCopy := *exec
		result = append(result, &execCopy)
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
func (e *Executor) UpdateProgress(ctx context.Context, taskID string, progress int, status v1.AgentStatus) {
	e.mu.Lock()
	var sessionID string
	if exec, ok := e.executions[taskID]; ok {
		exec.Progress = progress
		exec.Status = status
		exec.LastUpdate = time.Now()
		sessionID = exec.SessionID

		e.logger.Debug("updated execution progress",
			zap.String("task_id", taskID),
			zap.Int("progress", progress),
			zap.String("status", string(status)))
	}
	e.mu.Unlock()

	// Update database asynchronously (don't block on this)
	if sessionID != "" {
		go func() {
			session, err := e.repo.GetAgentSession(ctx, sessionID)
			if err != nil {
				return
			}
			session.Progress = progress
			session.Status = v1StatusToAgentSessionStatus(status)
			if err := e.repo.UpdateAgentSession(ctx, session); err != nil {
				e.logger.Error("failed to update agent session progress in database",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}()
	}
}

// MarkCompleted marks an execution as completed
func (e *Executor) MarkCompleted(ctx context.Context, taskID string, status v1.AgentStatus) {
	e.mu.Lock()
	var sessionID string
	if exec, ok := e.executions[taskID]; ok {
		exec.Status = status
		exec.Progress = 100
		exec.LastUpdate = time.Now()
		sessionID = exec.SessionID

		e.logger.Info("execution completed",
			zap.String("task_id", taskID),
			zap.String("status", string(status)))
	}
	e.mu.Unlock()

	// Update database
	if sessionID != "" {
		dbStatus := v1StatusToAgentSessionStatus(status)
		if err := e.repo.UpdateAgentSessionStatus(ctx, sessionID, dbStatus, ""); err != nil {
			e.logger.Error("failed to update agent session status in database",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

// RemoveExecution removes a completed/failed execution from in-memory tracking
// Note: The database record is preserved for history
func (e *Executor) RemoveExecution(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.executions[taskID]; ok {
		delete(e.executions, taskID)
		e.logger.Debug("removed execution from tracking", zap.String("task_id", taskID))
	}
}

// GetActiveSessionID returns the active session ID for a task, if any
func (e *Executor) GetActiveSessionID(ctx context.Context, taskID string) (string, error) {
	// First check in-memory cache
	e.mu.RLock()
	execution, exists := e.executions[taskID]
	e.mu.RUnlock()

	if exists && execution.SessionID != "" {
		return execution.SessionID, nil
	}

	// Try to load from database
	session, err := e.repo.GetActiveAgentSessionByTaskID(ctx, taskID)
	if err != nil {
		return "", err
	}

	return session.ID, nil
}

// UpdateSessionACPSessionID updates the ACP session ID for an agent session
func (e *Executor) UpdateSessionACPSessionID(ctx context.Context, taskID, acpSessionID string) error {
	session, err := e.repo.GetActiveAgentSessionByTaskID(ctx, taskID)
	if err != nil {
		return err
	}

	session.ACPSessionID = acpSessionID
	return e.repo.UpdateAgentSession(ctx, session)
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

// PromptAgent mocks sending a prompt to an agent
func (m *MockAgentManagerClient) PromptAgent(ctx context.Context, agentInstanceID string, prompt string) (*PromptResult, error) {
	m.logger.Info("mock: prompting agent",
		zap.String("agent_instance_id", agentInstanceID),
		zap.Int("prompt_length", len(prompt)))
	return &PromptResult{StopReason: "end_turn"}, nil
}

// RespondToPermissionByTaskID mocks responding to a permission request
func (m *MockAgentManagerClient) RespondToPermissionByTaskID(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error {
	m.logger.Info("mock: responding to permission",
		zap.String("task_id", taskID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))
	return nil
}

// GetRecoveredInstances mocks getting recovered instances (returns empty for mock)
func (m *MockAgentManagerClient) GetRecoveredInstances() []RecoveredInstanceInfo {
	return nil
}
