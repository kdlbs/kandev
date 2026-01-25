// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	// PromptAgent sends a prompt to a running agent
	// Returns PromptResult indicating if the agent needs input
	PromptAgent(ctx context.Context, agentExecutionID string, prompt string) (*PromptResult, error)

	// CancelAgent interrupts the current agent turn without terminating the process.
	CancelAgent(ctx context.Context, sessionID string) error

	// RespondToPermission sends a response to a permission request
	RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error

	// IsAgentRunningForSession checks if an agent is actually running for a session
	// This probes the actual agent (Docker container or standalone process) rather than relying on cached state
	IsAgentRunningForSession(ctx context.Context, sessionID string) bool

	// ResolveAgentProfile resolves an agent profile ID to profile information
	ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error)
}

// AgentProfileInfo contains resolved profile information
type AgentProfileInfo struct {
	ProfileID                  string
	ProfileName                string
	AgentID                    string
	AgentName                  string
	Model                      string
	AutoApprove                bool
	DangerouslySkipPermissions bool
	Plan                       string
	CLIPassthrough             bool
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
	ModelOverride   string // If set, use this model instead of the profile's model
	ExecutorType    string // Executor type (e.g., "local_pc", "local_docker") - determines runtime

	// Worktree configuration for concurrent agent execution
	UseWorktree          bool   // Whether to use a Git worktree for isolation
	RepositoryID         string // Repository ID for worktree tracking
	RepositoryPath       string // Path to the main repository (for worktree creation)
	BaseBranch           string // Base branch for the worktree (e.g., "main")
	WorktreeBranchPrefix string // Branch prefix for worktree branches
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

// TaskExecution tracks an active task execution
type TaskExecution struct {
	TaskID           string
	AgentExecutionID string
	AgentProfileID   string
	StartedAt        time.Time
	SessionState     v1.TaskSessionState
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

	// Configuration
	retryLimit      int
	retryDelay      time.Duration
	worktreeEnabled bool // Whether to use Git worktrees for agent isolation
}

// ExecutorConfig holds configuration for the Executor
type ExecutorConfig struct {
	WorktreeEnabled bool // Whether to use Git worktrees for agent isolation
	ShellPrefs      ShellPreferenceProvider
}

type ShellPreferenceProvider interface {
	PreferredShell(ctx context.Context) (string, error)
}

// NewExecutor creates a new executor
func NewExecutor(agentManager AgentManagerClient, repo repository.Repository, log *logger.Logger, cfg ExecutorConfig) *Executor {
	return &Executor{
		agentManager:    agentManager,
		repo:            repo,
		shellPrefs:      cfg.ShellPrefs,
		logger:          log.WithFields(zap.String("component", "executor")),
		retryLimit:      3,
		retryDelay:      5 * time.Second,
		worktreeEnabled: cfg.WorktreeEnabled,
	}
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

// Execute starts agent execution for a task
func (e *Executor) Execute(ctx context.Context, task *v1.Task) (*TaskExecution, error) {
	return e.ExecuteWithProfile(ctx, task, "", "", task.Description)
}

// ExecuteWithProfile starts agent execution for a task using an explicit agent profile.
// The executorID parameter specifies which executor to use (determines runtime: local_pc, local_docker, etc.).
// If executorID is empty, falls back to workspace's default executor.
// The prompt parameter is the initial prompt to send to the agent.
func (e *Executor) ExecuteWithProfile(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, prompt string) (*TaskExecution, error) {
	if agentProfileID == "" {
		e.logger.Error("task has no agent_profile_id configured", zap.String("task_id", task.ID))
		return nil, ErrNoAgentProfileID
	}

	// Create a LaunchAgentRequest from the task
	// Use the provided prompt instead of task.Description
	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		TaskTitle:       task.Title,
		AgentProfileID:  agentProfileID,
		TaskDescription: prompt, // Use the provided prompt
		Priority:        task.Priority,
	}
	metadata := cloneMetadata(task.Metadata)
	var repositoryPath string
	var repositoryID string
	var baseBranch string
	var worktreeBranchPrefix string

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
		worktreeBranchPrefix = repository.WorktreeBranchPrefix
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
		req.WorktreeBranchPrefix = worktreeBranchPrefix
	}

	// Resolve agent profile to get model and other settings for snapshot
	var agentProfileSnapshot map[string]interface{}
	if profileInfo, err := e.agentManager.ResolveAgentProfile(ctx, agentProfileID); err == nil && profileInfo != nil {
		agentProfileSnapshot = map[string]interface{}{
			"id":                           profileInfo.ProfileID,
			"name":                         profileInfo.ProfileName,
			"agent_id":                     profileInfo.AgentID,
			"agent_name":                   profileInfo.AgentName,
			"model":                        profileInfo.Model,
			"auto_approve":                 profileInfo.AutoApprove,
			"dangerously_skip_permissions": profileInfo.DangerouslySkipPermissions,
			"plan":                         profileInfo.Plan,
			"cli_passthrough":              profileInfo.CLIPassthrough,
		}
		e.logger.Info("resolved agent profile for snapshot",
			zap.String("profile_id", profileInfo.ProfileID),
			zap.String("model", profileInfo.Model),
			zap.Bool("cli_passthrough", profileInfo.CLIPassthrough))
	} else {
		// Create minimal snapshot even on failure - ensures model switching works
		e.logger.Warn("failed to resolve agent profile, using minimal snapshot",
			zap.String("agent_profile_id", agentProfileID),
			zap.Error(err))
		agentProfileSnapshot = map[string]interface{}{
			"id":    agentProfileID,
			"model": "", // Empty model allows any model switch
		}
	}

	// Create agent session in database before launch so worktree associations can persist.
	sessionID := uuid.New().String()
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID:                   sessionID,
		TaskID:               task.ID,
		AgentProfileID:       agentProfileID,
		RepositoryID:         repositoryID,
		BaseBranch:           baseBranch,
		State:                models.TaskSessionStateCreated,
		StartedAt:            now,
		UpdatedAt:            now,
		AgentProfileSnapshot: agentProfileSnapshot,
	}
	// Resolve executor configuration
	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		session.ExecutorID = execConfig.ExecutorID
		metadata = execConfig.Metadata
		req.ExecutorType = execConfig.ExecutorType
		e.logger.Debug("resolved executor for task",
			zap.String("task_id", task.ID),
			zap.String("executor_id", execConfig.ExecutorID),
			zap.String("executor_type", execConfig.ExecutorType))
	}

	if err := e.repo.CreateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to persist agent session before launch",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return nil, err
	}

	req.SessionID = sessionID
	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	e.logger.Info("launching agent for task",
		zap.String("task_id", task.ID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_type", req.ExecutorType),
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
	session.ErrorMessage = ""
	session.UpdatedAt = time.Now().UTC()

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update agent session after launch",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := e.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}
	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           task.ID,
		ExecutorID:       session.ExecutorID,
		Status:           "starting",
		Resumable:        resumable,
		AgentExecutionID: resp.AgentExecutionID,
		ContainerID:      resp.ContainerID,
		WorktreeID:       resp.WorktreeID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}
	if err := e.repo.UpsertExecutorRunning(ctx, running); err != nil {
		e.logger.Warn("failed to persist executor runtime after launch",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	if resp.WorktreeID != "" {
		sessionWorktree := &models.TaskSessionWorktree{
			SessionID:      session.ID,
			WorktreeID:     resp.WorktreeID,
			RepositoryID:   repositoryID,
			Position:       0,
			WorktreePath:   resp.WorktreePath,
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

	// Build result from stored session
	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: resp.AgentExecutionID,
		AgentProfileID:   agentProfileID,
		StartedAt:        now,
		SessionState:     v1.TaskSessionStateStarting,
		LastUpdate:       now,
		SessionID:        sessionID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}

	// Start the agent process asynchronously.
	// The initial prompt is sent as part of InitializeAndPrompt in the goroutine.
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := e.agentManager.StartAgentProcess(startCtx, resp.AgentExecutionID); err != nil {
			e.logger.Error("failed to start agent process",
				zap.String("task_id", task.ID),
				zap.String("agent_execution_id", resp.AgentExecutionID),
				zap.Error(err))
			// Update session state to failed
			if updateErr := e.repo.UpdateTaskSessionState(context.Background(), sessionID, models.TaskSessionStateFailed, err.Error()); updateErr != nil {
				e.logger.Warn("failed to mark session as failed after start error",
					zap.String("task_id", task.ID),
					zap.String("session_id", sessionID),
					zap.Error(updateErr))
			}
			return
		}

		// Agent started successfully - transition task from SCHEDULING to IN_PROGRESS
		if updateErr := e.repo.UpdateTaskState(context.Background(), task.ID, v1.TaskStateInProgress); updateErr != nil {
			e.logger.Warn("failed to update task state to IN_PROGRESS after agent start",
				zap.String("task_id", task.ID),
				zap.Error(updateErr))
		} else {
			e.logger.Debug("task transitioned to IN_PROGRESS after agent started",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID))
		}
	}()

	e.logger.Info("agent launched",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", resp.AgentExecutionID),
		zap.String("container_id", resp.ContainerID),
		zap.String("worktree_path", resp.WorktreePath),
		zap.String("worktree_branch", resp.WorktreeBranch))

	return execution, nil
}

// ResumeSession restarts an existing task session using its stored worktree.
// When startAgent is false, only the executor runtime is started (agent process is not launched).
func (e *Executor) ResumeSession(ctx context.Context, session *models.TaskSession, startAgent bool) (*TaskExecution, error) {
	if session == nil {
		return nil, ErrExecutionNotFound
	}

	taskModel, err := e.repo.GetTask(ctx, session.TaskID)
	if err != nil {
		e.logger.Error("failed to load task for session resume",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, err
	}
	task := taskModel.ToAPI()
	if task == nil {
		return nil, ErrExecutionNotFound
	}

	if session.AgentProfileID == "" {
		e.logger.Error("task session has no agent_profile_id configured",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID))
		return nil, ErrNoAgentProfileID
	}

	// Check if this specific session is already running
	if existing, ok := e.GetExecutionBySession(session.ID); ok && existing != nil {
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
	// Resolve executor configuration
	executorWasEmpty := session.ExecutorID == ""
	execConfig := e.resolveExecutorConfig(ctx, session.ExecutorID, task.WorkspaceID, metadata)
	session.ExecutorID = execConfig.ExecutorID
	metadata = execConfig.Metadata
	req.ExecutorType = execConfig.ExecutorType

	// Persist executor assignment if it was resolved from workspace default
	if executorWasEmpty && session.ExecutorID != "" {
		session.UpdatedAt = time.Now().UTC()
		if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
			e.logger.Warn("failed to persist executor assignment for session",
				zap.String("session_id", session.ID),
				zap.String("executor_id", session.ExecutorID),
				zap.Error(err))
			// Continue anyway - this is not fatal
		}
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

	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, session.ID); err == nil && running != nil {
		if running.ResumeToken != "" && startAgent {
			req.ACPSessionID = running.ResumeToken
			e.logger.Info("found resume token for session resumption",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID))
		}
	}

	e.logger.Info("resuming agent session",
		zap.String("task_id", session.TaskID),
		zap.String("session_id", session.ID),
		zap.String("agent_profile_id", session.AgentProfileID),
		zap.String("executor_type", req.ExecutorType),
		zap.String("resume_token", req.ACPSessionID),
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
	session.ErrorMessage = ""
	if startAgent {
		session.State = models.TaskSessionStateStarting
		session.CompletedAt = nil
	}

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update task session for resume",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
	}

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := e.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}
	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           task.ID,
		ExecutorID:       session.ExecutorID,
		Status:           "starting",
		Resumable:        resumable,
		AgentExecutionID: resp.AgentExecutionID,
		ContainerID:      resp.ContainerID,
		WorktreeID:       resp.WorktreeID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}
	if err := e.repo.UpsertExecutorRunning(ctx, running); err != nil {
		e.logger.Warn("failed to persist executor runtime after resume",
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
				SessionID:      session.ID,
				WorktreeID:     resp.WorktreeID,
				RepositoryID:   repositoryID,
				Position:       0,
				WorktreePath:   resp.WorktreePath,
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
		LastUpdate:       now,
		SessionID:        session.ID,
		WorktreePath:     worktreePath,
		WorktreeBranch:   worktreeBranch,
	}

	if startAgent {
		// Start the agent process asynchronously
		go func() {
			startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			if err := e.agentManager.StartAgentProcess(startCtx, resp.AgentExecutionID); err != nil {
				e.logger.Error("failed to start agent process on resume",
					zap.String("task_id", task.ID),
					zap.String("session_id", session.ID),
					zap.String("agent_execution_id", resp.AgentExecutionID),
					zap.Error(err))
				// Update session state to failed
				if updateErr := e.repo.UpdateTaskSessionState(context.Background(), session.ID, models.TaskSessionStateFailed, err.Error()); updateErr != nil {
					e.logger.Warn("failed to mark session as failed after start error on resume",
						zap.String("task_id", task.ID),
						zap.String("session_id", session.ID),
						zap.Error(updateErr))
				}
				return
			}

			// Agent resumed successfully - sync task state with session state.
			// If the session is waiting for input, the task should be in REVIEW state.
			// This ensures the task state reflects the actual agent status.
			if session.State == models.TaskSessionStateWaitingForInput {
				if updateErr := e.repo.UpdateTaskState(context.Background(), task.ID, v1.TaskStateReview); updateErr != nil {
					e.logger.Warn("failed to update task state to REVIEW after resume",
						zap.String("task_id", task.ID),
						zap.Error(updateErr))
				} else {
					e.logger.Debug("task state synced to REVIEW after resume (session waiting for input)",
						zap.String("task_id", task.ID),
						zap.String("session_id", session.ID))
				}
			} else {
				e.logger.Debug("agent resumed successfully, task state unchanged",
					zap.String("task_id", task.ID),
					zap.String("session_id", session.ID),
					zap.String("session_state", string(session.State)))
			}
		}()
	}

	return execution, nil
}

// Stop stops an active execution by session ID
func (e *Executor) Stop(ctx context.Context, sessionID string, reason string, force bool) error {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ErrExecutionNotFound
	}
	if session.AgentExecutionID == "" {
		return ErrExecutionNotFound
	}

	e.logger.Info("stopping execution",
		zap.String("task_id", session.TaskID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", session.AgentExecutionID),
		zap.String("reason", reason),
		zap.Bool("force", force))

	err = e.agentManager.StopAgent(ctx, session.AgentExecutionID, force)
	if err != nil {
		// Log the error but continue to clean up execution state
		// The agent instance may already be gone (container stopped externally)
		e.logger.Warn("failed to stop agent (may already be stopped)",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Update database
	if dbErr := e.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateCancelled, reason); dbErr != nil {
		e.logger.Error("failed to update agent session status in database",
			zap.String("session_id", sessionID),
			zap.Error(dbErr))
	}

	return nil
}

// StopByTaskID stops all active executions for a task
func (e *Executor) StopByTaskID(ctx context.Context, taskID string, reason string, force bool) error {
	// Get all active sessions for this task from database
	sessions, err := e.repo.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list active sessions for task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return ErrExecutionNotFound
	}

	if len(sessions) == 0 {
		return ErrExecutionNotFound
	}

	var lastErr error
	stoppedCount := 0
	for _, session := range sessions {
		if err := e.Stop(ctx, session.ID, reason, force); err != nil {
			e.logger.Warn("failed to stop session",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			lastErr = err
		} else {
			stoppedCount++
		}
	}

	if stoppedCount == 0 && lastErr != nil {
		return lastErr
	}

	return nil
}

// Prompt sends a follow-up prompt to a running agent for a task
// Returns PromptResult indicating if the agent needs input
func (e *Executor) Prompt(ctx context.Context, taskID, sessionID string, prompt string) (*PromptResult, error) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, ErrExecutionNotFound
	}
	if session.TaskID != taskID {
		return nil, ErrExecutionNotFound
	}
	if session.AgentExecutionID == "" {
		return nil, ErrExecutionNotFound
	}

	e.logger.Debug("sending prompt to agent",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", session.AgentExecutionID),
		zap.Int("prompt_length", len(prompt)))

	return e.agentManager.PromptAgent(ctx, session.AgentExecutionID, prompt)
}

// SwitchModel stops the current agent, restarts it with a new model, and sends the prompt.
// For agents that support session resume (can_recover: true), it attempts to resume context.
// For agents that don't support resume (can_recover: false), a fresh session is started.
func (e *Executor) SwitchModel(ctx context.Context, taskID, sessionID, newModel, prompt string) (*PromptResult, error) {
	e.logger.Info("switching model for session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("new_model", newModel))

	// Get the session
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session.TaskID != taskID {
		return nil, fmt.Errorf("session %s does not belong to task %s", sessionID, taskID)
	}

	// Get current execution ID
	oldAgentExecutionID := session.AgentExecutionID
	if oldAgentExecutionID == "" {
		return nil, ErrExecutionNotFound
	}

	// Get the task for re-launching
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Get executor running info to check for resume token (ACP session ID)
	var acpSessionID string
	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		acpSessionID = running.ResumeToken
	}

	// Stop the current agent
	e.logger.Info("stopping current agent for model switch",
		zap.String("agent_execution_id", oldAgentExecutionID))
	if err := e.agentManager.StopAgent(ctx, oldAgentExecutionID, false); err != nil {
		e.logger.Warn("failed to stop agent for model switch, continuing anyway",
			zap.Error(err),
			zap.String("agent_execution_id", oldAgentExecutionID))
	}

	// Resolve executor configuration
	execConfig := e.resolveExecutorConfig(ctx, session.ExecutorID, task.WorkspaceID, nil)

	// Build a new launch request with the model override
	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		SessionID:       sessionID, // Reuse the existing session ID
		TaskTitle:       task.Title,
		AgentProfileID:  session.AgentProfileID,
		TaskDescription: prompt,
		ModelOverride:   newModel, // This is the key - use the new model
		ACPSessionID:    acpSessionID,
		ExecutorType:    execConfig.ExecutorType,
		Metadata:        execConfig.Metadata,
	}

	// Get repository info if available
	var repositoryPath string
	if session.RepositoryID != "" {
		repository, err := e.repo.GetRepository(ctx, session.RepositoryID)
		if err == nil && repository != nil {
			repositoryPath = repository.LocalPath
			req.RepositoryURL = repository.LocalPath
			req.Branch = session.BaseBranch
		}
	}

	// Configure worktree if enabled - reuse existing worktree from session
	if e.worktreeEnabled && repositoryPath != "" {
		req.UseWorktree = true
		req.RepositoryPath = repositoryPath
		req.RepositoryID = session.RepositoryID
		if session.BaseBranch != "" {
			req.BaseBranch = session.BaseBranch
		} else {
			req.BaseBranch = "main"
		}
		// Pass existing worktree ID in metadata for reuse
		if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreeID != "" {
			if req.Metadata == nil {
				req.Metadata = make(map[string]interface{})
			}
			req.Metadata["worktree_id"] = session.Worktrees[0].WorktreeID
		}
	}

	// Get worktree info from running state (for workspace path)
	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		if running.WorktreePath != "" {
			req.RepositoryURL = running.WorktreePath
		}
	}

	req.Env = e.applyPreferredShellEnv(ctx, req.Env)

	e.logger.Info("launching new agent with model override",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("model", newModel),
		zap.String("executor_type", req.ExecutorType),
		zap.String("acp_session_id", acpSessionID),
		zap.Bool("use_worktree", req.UseWorktree),
		zap.String("repository_path", req.RepositoryPath))

	// Launch the new agent
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to launch agent with new model",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to launch agent with new model: %w", err)
	}

	// Update the session with the new execution ID
	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
	session.State = models.TaskSessionStateStarting
	session.UpdatedAt = time.Now().UTC()

	// Update the agent profile snapshot with the new model
	if session.AgentProfileSnapshot == nil {
		session.AgentProfileSnapshot = make(map[string]interface{})
	}
	session.AgentProfileSnapshot["model"] = newModel

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update session after model switch",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Update executor running state
	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		running.AgentExecutionID = resp.AgentExecutionID
		running.ContainerID = resp.ContainerID
		running.Status = "starting"
		if err := e.repo.UpsertExecutorRunning(ctx, running); err != nil {
			e.logger.Warn("failed to update executor running after model switch",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	// Start the agent process (this also handles initialization and prompting)
	if err := e.agentManager.StartAgentProcess(ctx, resp.AgentExecutionID); err != nil {
		e.logger.Error("failed to start agent process after model switch",
			zap.String("task_id", task.ID),
			zap.String("agent_execution_id", resp.AgentExecutionID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to start agent after model switch: %w", err)
	}

	e.logger.Info("model switch complete, agent started",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("new_model", newModel),
		zap.String("agent_execution_id", resp.AgentExecutionID))

	// The agent initialization and prompt are handled as part of StartAgentProcess
	// Return success - the actual prompt response will come via ACP events
	return &PromptResult{
		StopReason:   "model_switched",
		AgentMessage: "",
	}, nil
}

// RespondToPermission sends a response to a permission request for a session
func (e *Executor) RespondToPermission(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	e.logger.Debug("responding to permission request",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	return e.agentManager.RespondToPermissionBySessionID(ctx, sessionID, pendingID, optionID, cancelled)
}

// GetExecutionBySession returns the execution state for a specific session
func (e *Executor) GetExecutionBySession(sessionID string) (*TaskExecution, bool) {
	ctx := context.Background()
	const startupGracePeriod = 30 * time.Second

	// Load from database
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, false
	}
	if session.AgentExecutionID == "" {
		return nil, false
	}

	// Verify the agent is actually running
	if !e.agentManager.IsAgentRunningForSession(ctx, sessionID) {
		if (session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning) &&
			time.Since(session.UpdatedAt) < startupGracePeriod {
			return FromTaskSession(session), true
		}
		return nil, false
	}

	return FromTaskSession(session), true
}

// ListExecutions returns all active executions
func (e *Executor) ListExecutions() []*TaskExecution {
	ctx := context.Background()
	sessions, err := e.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		return nil
	}

	result := make([]*TaskExecution, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, FromTaskSession(session))
	}
	return result
}

// ActiveCount returns the number of active executions
func (e *Executor) ActiveCount() int {
	ctx := context.Background()
	sessions, err := e.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		return 0
	}
	return len(sessions)
}

// CanExecute returns true if there's capacity for another execution.
// Currently always returns true as there is no concurrent execution limit.
func (e *Executor) CanExecute() bool {
	return true
}

// MarkCompletedBySession marks an execution as completed by session ID
func (e *Executor) MarkCompletedBySession(ctx context.Context, sessionID string, state v1.TaskSessionState) {
	e.logger.Info("execution completed",
		zap.String("session_id", sessionID),
		zap.String("state", string(state)))

	// Update database
	dbState := models.TaskSessionState(state)
	if err := e.repo.UpdateTaskSessionState(ctx, sessionID, dbState, ""); err != nil {
		e.logger.Error("failed to update agent session status in database",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (e *Executor) defaultExecutorID(ctx context.Context, workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	workspace, err := e.repo.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil || workspace.DefaultExecutorID == nil {
		return ""
	}
	return strings.TrimSpace(*workspace.DefaultExecutorID)
}

func (e *Executor) applyExecutorMetadata(ctx context.Context, metadata map[string]interface{}, executorID string) map[string]interface{} {
	if executorID == "" {
		return metadata
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["executor_id"] = executorID
	executor, err := e.repo.GetExecutor(ctx, executorID)
	if err != nil || executor == nil {
		return metadata
	}
	if policyJSON := strings.TrimSpace(executor.Config["mcp_policy"]); policyJSON != "" {
		metadata["executor_mcp_policy"] = policyJSON
	}
	return metadata
}

// getExecutorType resolves an executor ID to its type string.
// Returns empty string if executor not found or on error.
func (e *Executor) getExecutorType(ctx context.Context, executorID string) string {
	if executorID == "" {
		return ""
	}
	executor, err := e.repo.GetExecutor(ctx, executorID)
	if err != nil || executor == nil {
		return ""
	}
	return string(executor.Type)
}

// executorConfig holds resolved executor configuration.
type executorConfig struct {
	ExecutorID   string
	ExecutorType string
	Metadata     map[string]interface{}
}

// resolveExecutorConfig resolves executor configuration from an executor ID.
// If executorID is empty, it falls back to the workspace default.
// Returns the resolved config with executor ID, type, and metadata.
func (e *Executor) resolveExecutorConfig(ctx context.Context, executorID, workspaceID string, existingMetadata map[string]interface{}) executorConfig {
	resolved := executorID
	if resolved == "" {
		resolved = e.defaultExecutorID(ctx, workspaceID)
	}

	metadata := existingMetadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	if resolved != "" {
		metadata = e.applyExecutorMetadata(ctx, metadata, resolved)
	}

	return executorConfig{
		ExecutorID:   resolved,
		ExecutorType: e.getExecutorType(ctx, resolved),
		Metadata:     metadata,
	}
}

func cloneMetadata(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
