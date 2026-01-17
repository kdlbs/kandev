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
	ErrMaxConcurrentReached    = errors.New("maximum concurrent executions reached")
	ErrNoAgentProfileID        = errors.New("task has no agent_profile_id configured")
	ErrExecutionNotFound       = errors.New("execution not found")
	ErrExecutionAlreadyRunning = errors.New("execution already running")
)

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// AgentManagerClient is an interface for the Agent Manager service
// This will be implemented via gRPC or HTTP client
type AgentManagerClient interface {
	// LaunchAgent creates a new agentctl instance for a task (agent not started yet)
	LaunchAgent(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error)

	// StartAgentProcess starts the agent subprocess for an execution.
	// The command is built internally based on the execution's agent profile.
	StartAgentProcess(ctx context.Context, agentExecutionID string) error

	// StopAgent stops a running agent
	StopAgent(ctx context.Context, agentExecutionID string, force bool) error

	// GetAgentStatus returns the status of an agent execution
	GetAgentStatus(ctx context.Context, agentExecutionID string) (*v1.AgentExecution, error)

	// ListAgentTypes returns available agent types
	ListAgentTypes(ctx context.Context) ([]*v1.AgentType, error)

	// PromptAgent sends a prompt to a running agent
	// Returns PromptResult indicating if the agent needs input
	PromptAgent(ctx context.Context, agentExecutionID string, prompt string) (*PromptResult, error)

	// RespondToPermission sends a response to a permission request
	RespondToPermissionByTaskID(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error

	// GetRecoveredExecutions returns executions recovered from Docker during startup
	GetRecoveredExecutions() []RecoveredExecutionInfo

	// IsAgentRunningForTask checks if an agent is actually running for a task
	// This probes the actual agent (Docker container or standalone process) rather than relying on cached state
	IsAgentRunningForTask(ctx context.Context, taskID string) bool

	// CleanupStaleExecutionByTaskID removes a stale agent execution from tracking without trying to stop it.
	// This is used when we detect the agent process has stopped but the execution is still tracked.
	CleanupStaleExecutionByTaskID(ctx context.Context, taskID string) error
}

// LaunchAgentRequest contains parameters for launching an agent
type LaunchAgentRequest struct {
	TaskID          string
	SessionID       string
	TaskTitle       string // Human-readable task title for semantic worktree naming
	AgentProfileID  string
	RepositoryURL   string
	Branch          string
	TaskDescription string // Task description to send via ACP prompt
	Priority        int
	Metadata        map[string]interface{}
	Env             map[string]string
	ACPSessionID    string // ACP session ID to resume, if available

	// Worktree configuration for concurrent agent execution
	UseWorktree    bool   // Whether to use a Git worktree for isolation
	RepositoryID   string // Repository ID for worktree tracking
	RepositoryPath string // Path to the main repository (for worktree creation)
	BaseBranch     string // Base branch for the worktree (e.g., "main")
}

// LaunchAgentResponse contains the result of launching an agent
type LaunchAgentResponse struct {
	AgentExecutionID string
	ContainerID      string
	Status           v1.AgentStatus
	WorktreeID       string
	WorktreePath     string
	WorktreeBranch   string
}

// TaskExecution tracks an active task execution (kept for API compatibility)
type TaskExecution struct {
	TaskID           string
	AgentExecutionID string
	AgentProfileID   string
	StartedAt        time.Time
	SessionState     v1.TaskSessionState
	Progress         int
	LastUpdate       time.Time
	// SessionID is the database ID of the agent session
	SessionID string
	// Worktree info for the agent
	WorktreePath   string
	WorktreeBranch string
}

// FromTaskSession converts a models.TaskSession to TaskExecution
func FromTaskSession(s *models.TaskSession) *TaskExecution {
	execution := &TaskExecution{
		TaskID:           s.TaskID,
		AgentExecutionID: s.AgentExecutionID,
		AgentProfileID:   s.AgentProfileID,
		StartedAt:        s.StartedAt,
		SessionState:     agentSessionStateToV1(s.State),
		Progress:         s.Progress,
		LastUpdate:       s.UpdatedAt,
		SessionID:        s.ID,
	}
	if len(s.Worktrees) > 0 {
		execution.WorktreePath = s.Worktrees[0].WorktreePath
		execution.WorktreeBranch = s.Worktrees[0].WorktreeBranch
	}
	return execution
}

// agentSessionStateToV1 converts models.TaskSessionState to v1.TaskSessionState
func agentSessionStateToV1(state models.TaskSessionState) v1.TaskSessionState {
	return v1.TaskSessionState(state)
}

// Executor manages agent execution for tasks
type Executor struct {
	agentManager AgentManagerClient
	repo         repository.Repository
	shellPrefs   ShellPreferenceProvider
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
	ShellPrefs      ShellPreferenceProvider
}

type ShellPreferenceProvider interface {
	PreferredShell(ctx context.Context) (string, error)
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
		shellPrefs:      cfg.ShellPrefs,
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

func (e *Executor) applyPreferredShellEnv(ctx context.Context, env map[string]string) map[string]string {
	if e.shellPrefs == nil {
		return env
	}
	preferred, err := e.shellPrefs.PreferredShell(ctx)
	if err != nil {
		return env
	}
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		return env
	}
	if env == nil {
		env = make(map[string]string)
	}
	env["AGENTCTL_SHELL_COMMAND"] = preferred
	env["SHELL"] = preferred
	return env
}

// LoadActiveSessionsFromDB loads active agent sessions from the database into memory
// This should be called on startup to restore state after a restart
func (e *Executor) LoadActiveSessionsFromDB(ctx context.Context) error {
	sessions, err := e.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, session := range sessions {
		e.executions[session.TaskID] = FromTaskSession(session)
		e.logger.Info("restored agent session from database",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID),
			zap.String("state", string(session.State)))
	}

	e.logger.Info("loaded active agent sessions from database",
		zap.Int("count", len(sessions)))

	return nil
}

// RecoveredExecutionInfo contains info about an execution recovered from Docker
type RecoveredExecutionInfo struct {
	ExecutionID    string
	TaskID         string
	ContainerID    string
	AgentProfileID string
}

// SyncWithRecoveredExecutions ensures the executor's cache is in sync with
// executions recovered from Docker by the lifecycle manager.
// For each recovered execution, if not already in cache, add it.
func (e *Executor) SyncWithRecoveredExecutions(ctx context.Context, executions []RecoveredExecutionInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, exec := range executions {
		if _, exists := e.executions[exec.TaskID]; exists {
			continue // Already have this execution
		}

		// Add to cache - this execution is running in Docker but wasn't in DB as active
		e.executions[exec.TaskID] = &TaskExecution{
			TaskID:           exec.TaskID,
			AgentExecutionID: exec.ExecutionID,
			AgentProfileID:   exec.AgentProfileID,
			StartedAt:        time.Now(), // We don't know exact start time
			SessionState:     v1.TaskSessionStateRunning,
			Progress:         0,
			LastUpdate:       time.Now(),
		}

		e.logger.Info("synced recovered execution to executor cache",
			zap.String("task_id", exec.TaskID),
			zap.String("execution_id", exec.ExecutionID))
	}
}

// Execute starts agent execution for a task
func (e *Executor) Execute(ctx context.Context, task *v1.Task) (*TaskExecution, error) {
	return e.ExecuteWithProfile(ctx, task, "")
}

// ExecuteWithProfile starts agent execution for a task using an explicit agent profile.
func (e *Executor) ExecuteWithProfile(ctx context.Context, task *v1.Task, agentProfileID string) (*TaskExecution, error) {
	// Check if max concurrent limit is reached
	if !e.CanExecute() {
		e.logger.Warn("max concurrent executions reached",
			zap.Int("max", e.maxConcurrent),
			zap.Int("current", e.ActiveCount()))
		return nil, ErrMaxConcurrentReached
	}

	if agentProfileID == "" {
		e.logger.Error("task has no agent_profile_id configured", zap.String("task_id", task.ID))
		return nil, ErrNoAgentProfileID
	}

	// Create a LaunchAgentRequest from the task
	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		TaskTitle:       task.Title,
		AgentProfileID:  agentProfileID,
		TaskDescription: task.Description,
		Priority:        task.Priority,
		Metadata:        task.Metadata,
	}
	var repositoryPath string
	var repositoryID string
	var baseBranch string

	// Get the primary repository for this task
	primaryTaskRepo, err := e.repo.GetPrimaryTaskRepository(ctx, task.ID)
	if err != nil {
		e.logger.Error("failed to get primary task repository",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return nil, err
	}

	if primaryTaskRepo != nil {
		repositoryID = primaryTaskRepo.RepositoryID
		baseBranch = primaryTaskRepo.BaseBranch

		repository, err := e.repo.GetRepository(ctx, repositoryID)
		if err != nil {
			e.logger.Error("failed to load repository for task",
				zap.String("task_id", task.ID),
				zap.String("repository_id", repositoryID),
				zap.Error(err))
			return nil, err
		}
		repositoryPath = repository.LocalPath
		if repositoryPath != "" {
			req.RepositoryURL = repositoryPath
		}
		if baseBranch == "" && repository.DefaultBranch != "" {
			baseBranch = repository.DefaultBranch
		}
		if baseBranch != "" {
			req.Branch = baseBranch
		}
	}

	// Configure worktree if enabled and repository path is available
	if e.worktreeEnabled && repositoryPath != "" {
		req.UseWorktree = true
		req.RepositoryPath = repositoryPath
		req.RepositoryID = repositoryID
		if baseBranch != "" {
			req.BaseBranch = baseBranch
		} else {
			req.BaseBranch = "main"
		}
	}

	// Create agent session in database before launch so worktree associations can persist.
	sessionID := uuid.New().String()
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         task.ID,
		AgentProfileID: agentProfileID,
		RepositoryID:   repositoryID,
		BaseBranch:     baseBranch,
		State:          models.TaskSessionStateCreated,
		Progress:       0,
		StartedAt:      now,
		UpdatedAt:      now,
	}

	if err := e.repo.CreateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to persist agent session before launch",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return nil, err
	}

	req.SessionID = sessionID

	e.logger.Info("launching agent for task",
		zap.String("task_id", task.ID),
		zap.String("agent_profile_id", agentProfileID),
		zap.Bool("use_worktree", req.UseWorktree))

	req.Env = e.applyPreferredShellEnv(ctx, req.Env)

	// Call the AgentManager to launch the container
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to launch agent",
			zap.String("task_id", task.ID),
			zap.Error(err))
		if updateErr := e.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error()); updateErr != nil {
			e.logger.Warn("failed to mark session as failed after launch error",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID),
				zap.Error(updateErr))
		}
		return nil, err
	}

	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
	session.State = models.TaskSessionStateStarting
	session.Progress = 0
	session.ErrorMessage = ""
	session.UpdatedAt = time.Now().UTC()

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update agent session after launch",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	if resp.WorktreeID != "" {
		sessionWorktree := &models.TaskSessionWorktree{
			SessionID:    session.ID,
			WorktreeID:   resp.WorktreeID,
			RepositoryID: repositoryID,
			Position:     0,
			WorktreePath: resp.WorktreePath,
			WorktreeBranch: resp.WorktreeBranch,
		}
		if err := e.repo.CreateTaskSessionWorktree(ctx, sessionWorktree); err != nil {
			e.logger.Error("failed to persist session worktree association",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID),
				zap.String("worktree_id", resp.WorktreeID),
				zap.Error(err))
		}
	}

	// Track the execution in the in-memory cache
	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: resp.AgentExecutionID,
		AgentProfileID:   agentProfileID,
		StartedAt:        now,
		SessionState:     v1.TaskSessionStateStarting,
		Progress:         0,
		LastUpdate:       now,
		SessionID:        sessionID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}

	e.mu.Lock()
	e.executions[task.ID] = execution
	e.mu.Unlock()

	// Start the agent process (agentctl execution was created above)
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := e.agentManager.StartAgentProcess(startCtx, resp.AgentExecutionID); err != nil {
			e.logger.Error("failed to start agent process",
				zap.String("task_id", task.ID),
				zap.String("agent_execution_id", resp.AgentExecutionID),
				zap.Error(err))
		}
	}()

	e.logger.Info("agent launched successfully",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", resp.AgentExecutionID),
		zap.String("container_id", resp.ContainerID),
		zap.String("worktree_path", resp.WorktreePath),
		zap.String("worktree_branch", resp.WorktreeBranch))

	return execution, nil
}

// ResumeSession restarts an existing task session using its stored worktree.
func (e *Executor) ResumeSession(ctx context.Context, task *v1.Task, session *models.TaskSession) (*TaskExecution, error) {
	if session == nil {
		return nil, ErrExecutionNotFound
	}

	if !e.CanExecute() {
		e.logger.Warn("max concurrent executions reached",
			zap.Int("max", e.maxConcurrent),
			zap.Int("current", e.ActiveCount()))
		return nil, ErrMaxConcurrentReached
	}

	if session.AgentProfileID == "" {
		e.logger.Error("task session has no agent_profile_id configured",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID))
		return nil, ErrNoAgentProfileID
	}

	if existing, ok := e.GetExecutionWithContext(ctx, task.ID); ok && existing != nil {
		return nil, ErrExecutionAlreadyRunning
	}

	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		SessionID:       session.ID,
		TaskTitle:       task.Title,
		AgentProfileID:  session.AgentProfileID,
		TaskDescription: "",
		Priority:        task.Priority,
	}

	metadata := map[string]interface{}{}
	if session.Metadata != nil {
		for key, value := range session.Metadata {
			metadata[key] = value
		}
	}
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreeID != "" {
		metadata["worktree_id"] = session.Worktrees[0].WorktreeID
	}
	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	repositoryID := session.RepositoryID
	var repositoryPath string
	if repositoryID == "" && len(task.Repositories) > 0 {
		repositoryID = task.Repositories[0].RepositoryID
	}
	if repositoryID != "" {
		repository, err := e.repo.GetRepository(ctx, repositoryID)
		if err != nil {
			e.logger.Error("failed to load repository for task session resume",
				zap.String("task_id", task.ID),
				zap.String("repository_id", repositoryID),
				zap.Error(err))
			return nil, err
		}
		repositoryPath = repository.LocalPath
		if repositoryPath != "" {
			req.RepositoryURL = repositoryPath
		}
	}

	baseBranch := session.BaseBranch
	if baseBranch == "" && len(task.Repositories) > 0 && task.Repositories[0].BaseBranch != "" {
		baseBranch = task.Repositories[0].BaseBranch
	}
	if baseBranch != "" {
		req.Branch = baseBranch
	}

	if e.worktreeEnabled && repositoryPath != "" {
		req.UseWorktree = true
		req.RepositoryPath = repositoryPath
		req.RepositoryID = repositoryID
		if baseBranch != "" {
			req.BaseBranch = baseBranch
		} else {
			req.BaseBranch = "main"
		}
	}

	if session.Metadata != nil {
		if acpSessionID, ok := session.Metadata["acp_session_id"].(string); ok && acpSessionID != "" {
			req.ACPSessionID = acpSessionID
			e.logger.Info("found acp_session_id in session metadata for resumption",
				zap.String("task_id", task.ID),
				zap.String("acp_session_id", acpSessionID))
		}
	}

	e.logger.Info("resuming agent session",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_profile_id", session.AgentProfileID),
		zap.String("acp_session_id", req.ACPSessionID),
		zap.Bool("use_worktree", req.UseWorktree))

	req.Env = e.applyPreferredShellEnv(ctx, req.Env)

	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to relaunch agent for session",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, err
	}

	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
	session.Progress = 0
	session.ErrorMessage = ""
	session.State = models.TaskSessionStateStarting
	session.CompletedAt = nil

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update task session for resume",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
	}

	if resp.WorktreeID != "" {
		hasWorktree := false
		for _, wt := range session.Worktrees {
			if wt.WorktreeID == resp.WorktreeID {
				hasWorktree = true
				break
			}
		}
		if !hasWorktree {
			sessionWorktree := &models.TaskSessionWorktree{
				SessionID:    session.ID,
				WorktreeID:   resp.WorktreeID,
				RepositoryID: repositoryID,
				Position:     0,
				WorktreePath: resp.WorktreePath,
				WorktreeBranch: resp.WorktreeBranch,
			}
			if err := e.repo.CreateTaskSessionWorktree(ctx, sessionWorktree); err != nil {
				e.logger.Error("failed to persist session worktree association on resume",
					zap.String("task_id", task.ID),
					zap.String("session_id", session.ID),
					zap.String("worktree_id", resp.WorktreeID),
					zap.Error(err))
			}
		}
	}

	worktreePath := resp.WorktreePath
	worktreeBranch := resp.WorktreeBranch
	if worktreePath == "" && len(session.Worktrees) > 0 {
		worktreePath = session.Worktrees[0].WorktreePath
		worktreeBranch = session.Worktrees[0].WorktreeBranch
	}

	now := time.Now().UTC()
	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: resp.AgentExecutionID,
		AgentProfileID:   session.AgentProfileID,
		StartedAt:        now,
		SessionState:     v1.TaskSessionStateStarting,
		Progress:         0,
		LastUpdate:       now,
		SessionID:        session.ID,
		WorktreePath:     worktreePath,
		WorktreeBranch:   worktreeBranch,
	}

	e.mu.Lock()
	e.executions[task.ID] = execution
	e.mu.Unlock()

	// Start the agent process (agentctl execution was created above)
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := e.agentManager.StartAgentProcess(startCtx, resp.AgentExecutionID); err != nil {
			e.logger.Error("failed to start agent process on resume",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", resp.AgentExecutionID),
				zap.Error(err))
		}
	}()

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
		zap.String("agent_execution_id", execution.AgentExecutionID),
		zap.String("reason", reason),
		zap.Bool("force", force))

	err = e.agentManager.StopAgent(ctx, execution.AgentExecutionID, force)
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
		exec.SessionState = v1.TaskSessionStateCancelled
		exec.LastUpdate = time.Now()
	}
	delete(e.executions, taskID) // Remove from cache so GetExecution returns false
	e.mu.Unlock()

	// Update database
	if execution.SessionID != "" {
		if dbErr := e.repo.UpdateTaskSessionState(ctx, execution.SessionID, models.TaskSessionStateCancelled, reason); dbErr != nil {
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
		zap.String("agent_execution_id", execution.AgentExecutionID),
		zap.Int("prompt_length", len(prompt)))

	return e.agentManager.PromptAgent(ctx, execution.AgentExecutionID, prompt)
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
	session, err := e.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "no active agent session") {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}

	// Cache it in memory
	execution = FromTaskSession(session)
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
	ctx := context.Background()

	e.mu.RLock()
	execution, exists := e.executions[taskID]
	e.mu.RUnlock()

	if exists {
		// Verify the agent is actually running by probing agentctl
		if !e.agentManager.IsAgentRunningForTask(ctx, taskID) {
			// Agent stopped - clean up in-memory state
			e.mu.Lock()
			delete(e.executions, taskID)
			e.mu.Unlock()

			// Also update DB if we have a session
			if execution.SessionID != "" {
				_ = e.repo.UpdateTaskSessionState(ctx, execution.SessionID, models.TaskSessionStateCancelled, "agent process stopped")
			}

			// Clean up the stale execution from the agent manager
			if err := e.agentManager.CleanupStaleExecutionByTaskID(ctx, taskID); err != nil {
				e.logger.Warn("failed to cleanup stale agent execution",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
			return nil, false
		}

		// Return a copy to avoid data races
		execCopy := *execution
		return &execCopy, true
	}

	// Try to load from database
	session, err := e.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		return nil, false
	}

	// Verify the agent is actually running by probing the lifecycle manager
	// This handles the case where backend was restarted and DB has stale "running" sessions
	if !e.agentManager.IsAgentRunningForTask(ctx, taskID) {
		// Agent is not running - mark the session as stopped in DB and return false
		e.logger.Info("stale agent session detected - agent not running",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID))
		_ = e.repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCancelled, "agent not running after backend restart")

		// Clean up the stale execution from the agent manager
		if err := e.agentManager.CleanupStaleExecutionByTaskID(ctx, taskID); err != nil {
			e.logger.Warn("failed to cleanup stale agent execution",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return nil, false
	}

	// Cache it in memory
	execution = FromTaskSession(session)
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
		// Verify the agent is actually running by probing agentctl
		if !e.agentManager.IsAgentRunningForTask(ctx, taskID) {
			// Agent stopped - clean up in-memory state
			e.mu.Lock()
			delete(e.executions, taskID)
			e.mu.Unlock()

			// Also update DB if we have a session
			if execution.SessionID != "" {
				_ = e.repo.UpdateTaskSessionState(ctx, execution.SessionID, models.TaskSessionStateCancelled, "agent process stopped")
			}

			// Clean up the stale execution from the agent manager
			if err := e.agentManager.CleanupStaleExecutionByTaskID(ctx, taskID); err != nil {
				e.logger.Warn("failed to cleanup stale agent execution",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
			return nil, false
		}

		execCopy := *execution
		return &execCopy, true
	}

	// Try to load from database
	session, err := e.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		return nil, false
	}

	// Verify the agent is actually running by probing the lifecycle manager
	if !e.agentManager.IsAgentRunningForTask(ctx, taskID) {
		e.logger.Info("stale agent session detected - agent not running",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID))
		_ = e.repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCancelled, "agent not running after backend restart")

		// Clean up the stale execution from the agent manager
		if err := e.agentManager.CleanupStaleExecutionByTaskID(ctx, taskID); err != nil {
			e.logger.Warn("failed to cleanup stale agent execution",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return nil, false
	}

	// Cache it in memory
	execution = FromTaskSession(session)
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
func (e *Executor) UpdateProgress(ctx context.Context, taskID string, progress int, state v1.TaskSessionState) {
	e.mu.Lock()
	var sessionID string
	if exec, ok := e.executions[taskID]; ok {
		exec.Progress = progress
		exec.SessionState = state
		exec.LastUpdate = time.Now()
		sessionID = exec.SessionID

		e.logger.Debug("updated execution progress",
			zap.String("task_id", taskID),
			zap.Int("progress", progress),
			zap.String("state", string(state)))
	}
	e.mu.Unlock()

	// Update database asynchronously (don't block on this)
	if sessionID != "" {
		go func() {
			session, err := e.repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return
			}
			session.Progress = progress
			session.State = models.TaskSessionState(state)
			if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
				e.logger.Error("failed to update agent session progress in database",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}()
	}
}

// MarkCompleted marks an execution as completed
func (e *Executor) MarkCompleted(ctx context.Context, taskID string, state v1.TaskSessionState) {
	e.mu.Lock()
	var sessionID string
	if exec, ok := e.executions[taskID]; ok {
		exec.SessionState = state
		exec.Progress = 100
		exec.LastUpdate = time.Now()
		sessionID = exec.SessionID

		e.logger.Info("execution completed",
			zap.String("task_id", taskID),
			zap.String("state", string(state)))
	}
	e.mu.Unlock()

	// Update database
	if sessionID != "" {
		dbState := models.TaskSessionState(state)
		if err := e.repo.UpdateTaskSessionState(ctx, sessionID, dbState, ""); err != nil {
			e.logger.Error("failed to update agent session status in database",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

// UpdateExecutionState updates the in-memory execution state for a task.
func (e *Executor) UpdateExecutionState(taskID string, state v1.TaskSessionState) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if exec, ok := e.executions[taskID]; ok {
		exec.SessionState = state
		exec.LastUpdate = time.Now()
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
	session, err := e.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		return "", err
	}

	return session.ID, nil
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
		zap.String("agent_profile_id", req.AgentProfileID),
		zap.String("repository_url", req.RepositoryURL),
		zap.String("branch", req.Branch))

	return &LaunchAgentResponse{
		AgentExecutionID: uuid.New().String(),
		ContainerID:      "mock-container-" + uuid.New().String()[:8],
		Status:           v1.AgentStatusStarting,
	}, nil
}

// StartAgentProcess mocks starting the agent subprocess
func (m *MockAgentManagerClient) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	m.logger.Info("mock: starting agent process",
		zap.String("agent_execution_id", agentExecutionID))
	return nil
}

// StopAgent mocks stopping an agent
func (m *MockAgentManagerClient) StopAgent(ctx context.Context, agentExecutionID string, force bool) error {
	m.logger.Info("mock: stopping agent",
		zap.String("agent_execution_id", agentExecutionID),
		zap.Bool("force", force))
	return nil
}

// GetAgentStatus mocks getting agent status
func (m *MockAgentManagerClient) GetAgentStatus(ctx context.Context, agentExecutionID string) (*v1.AgentExecution, error) {
	m.logger.Info("mock: getting agent status",
		zap.String("agent_execution_id", agentExecutionID))

	return &v1.AgentExecution{
		ID:             agentExecutionID,
		Status:         v1.AgentStatusRunning,
		AgentProfileID: "mock-agent",
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
func (m *MockAgentManagerClient) PromptAgent(ctx context.Context, agentExecutionID string, prompt string) (*PromptResult, error) {
	m.logger.Info("mock: prompting agent",
		zap.String("agent_execution_id", agentExecutionID),
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

// GetRecoveredExecutions mocks getting recovered executions (returns empty for mock)
func (m *MockAgentManagerClient) GetRecoveredExecutions() []RecoveredExecutionInfo {
	return nil
}

// IsAgentRunningForTask mocks checking if an agent is running (always returns false for mock)
func (m *MockAgentManagerClient) IsAgentRunningForTask(ctx context.Context, taskID string) bool {
	return false
}

// CleanupStaleExecutionByTaskID mocks cleaning up a stale execution (no-op for mock)
func (m *MockAgentManagerClient) CleanupStaleExecutionByTaskID(ctx context.Context, taskID string) error {
	return nil
}
