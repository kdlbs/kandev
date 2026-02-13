// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/lifecycle"
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
	ErrRemoteDockerNoRepoURL   = errors.New("remote_docker executor requires a repository with provider owner and name set")
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
	// Attachments (images) are passed to the agent if provided
	PromptAgent(ctx context.Context, agentExecutionID string, prompt string, attachments []v1.MessageAttachment) (*PromptResult, error)

	// CancelAgent interrupts the current agent turn without terminating the process.
	CancelAgent(ctx context.Context, sessionID string) error

	// RespondToPermission sends a response to a permission request
	RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error

	// IsAgentRunningForSession checks if an agent is actually running for a session
	// This probes the actual agent (Docker container or standalone process) rather than relying on cached state
	IsAgentRunningForSession(ctx context.Context, sessionID string) bool

	// ResolveAgentProfile resolves an agent profile ID to profile information
	ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error)

	// SetExecutionDescription updates the task description in an existing execution's metadata.
	// Used when starting an agent on a session whose workspace was already launched.
	SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error
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
	ExecutorType    string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig  map[string]string // Executor config (docker_host, git_token, etc.)

	// Worktree configuration for concurrent agent execution
	UseWorktree          bool   // Whether to use a Git worktree for isolation
	RepositoryID         string // Repository ID for worktree tracking
	RepositoryPath       string // Path to the main repository (for worktree creation)
	BaseBranch           string // Base branch for the worktree (e.g., "main")
	WorktreeBranchPrefix string // Branch prefix for worktree branches
	PullBeforeWorktree   bool   // Whether to pull from remote before creating the worktree
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

// TaskStateChangeFunc is called when the executor needs to update a task's state.
// When set, it replaces direct repo.UpdateTaskState calls so the caller can
// publish events (e.g. WebSocket notifications) alongside the DB update.
type TaskStateChangeFunc func(ctx context.Context, taskID string, state v1.TaskState) error

// Executor manages agent execution for tasks
type Executor struct {
	agentManager AgentManagerClient
	repo         repository.Repository
	shellPrefs   ShellPreferenceProvider
	logger       *logger.Logger

	// Configuration
	retryLimit int
	retryDelay time.Duration

	// Callback for task state changes that need event publishing.
	// Set by the orchestrator to route through the task service layer.
	onTaskStateChange TaskStateChangeFunc

	// Per-session locks to prevent concurrent resume/launch operations on the same session.
	// This prevents race conditions when the backend restarts and multiple resume requests
	// arrive simultaneously (e.g., from frontend auto-resume).
	sessionLocks sync.Map // map[string]*sync.Mutex
}

// ExecutorConfig holds configuration for the Executor
type ExecutorConfig struct {
	ShellPrefs ShellPreferenceProvider
}

type ShellPreferenceProvider interface {
	PreferredShell(ctx context.Context) (string, error)
}

// NewExecutor creates a new executor
func NewExecutor(agentManager AgentManagerClient, repo repository.Repository, log *logger.Logger, cfg ExecutorConfig) *Executor {
	return &Executor{
		agentManager: agentManager,
		repo:         repo,
		shellPrefs:   cfg.ShellPrefs,
		logger:       log.WithFields(zap.String("component", "executor")),
		retryLimit:   3,
		retryDelay:   5 * time.Second,
	}
}

// SetOnTaskStateChange sets a callback for task state changes.
// This allows the orchestrator to route state changes through the task service layer
// which publishes WebSocket events. Without this, async goroutines would only update
// the database, leaving the frontend out of sync.
func (e *Executor) SetOnTaskStateChange(fn TaskStateChangeFunc) {
	e.onTaskStateChange = fn
}

// startAgentProcessAsync starts the agent subprocess in a background goroutine.
// On success it transitions the task to IN_PROGRESS; on failure it marks both the
// session and task as FAILED.
func (e *Executor) startAgentProcessAsync(taskID, sessionID, agentExecutionID string) {
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := e.agentManager.StartAgentProcess(startCtx, agentExecutionID); err != nil {
			e.logger.Error("failed to start agent process",
				zap.String("task_id", taskID),
				zap.String("agent_execution_id", agentExecutionID),
				zap.Error(err))
			if updateErr := e.repo.UpdateTaskSessionState(context.Background(), sessionID, models.TaskSessionStateFailed, err.Error()); updateErr != nil {
				e.logger.Warn("failed to mark session as failed after start error",
					zap.String("session_id", sessionID),
					zap.Error(updateErr))
			}
			if updateErr := e.updateTaskState(context.Background(), taskID, v1.TaskStateFailed); updateErr != nil {
				e.logger.Warn("failed to mark task as failed after start error",
					zap.String("task_id", taskID),
					zap.Error(updateErr))
			}
			return
		}

		if updateErr := e.updateTaskState(context.Background(), taskID, v1.TaskStateInProgress); updateErr != nil {
			e.logger.Warn("failed to update task state to IN_PROGRESS after agent start",
				zap.String("task_id", taskID),
				zap.Error(updateErr))
		}
	}()
}

// updateTaskState updates a task's state, using the callback if set for event publishing,
// or falling back to the raw repository.
func (e *Executor) updateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	if e.onTaskStateChange != nil {
		return e.onTaskStateChange(ctx, taskID, state)
	}
	return e.repo.UpdateTaskState(ctx, taskID, state)
}

// shouldUseWorktree returns true if the given executor type should use Git worktrees.
func shouldUseWorktree(executorType string) bool {
	return models.ExecutorType(executorType) == models.ExecutorTypeWorktree
}

// repositoryCloneURL builds an HTTPS clone URL from the repository's provider info.
// Returns an empty string if the repository has no provider owner/name or if the
// provider is not recognized.
func repositoryCloneURL(repo *models.Repository) string {
	if repo.ProviderOwner == "" || repo.ProviderName == "" {
		return ""
	}
	var host string
	switch strings.ToLower(repo.Provider) {
	case "github", "":
		host = "github.com"
	case "gitlab":
		host = "gitlab.com"
	case "bitbucket":
		host = "bitbucket.org"
	default:
		return ""
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, repo.ProviderOwner, repo.ProviderName)
}

// getSessionLock returns a per-session mutex, creating one if it doesn't exist.
// This serializes concurrent resume/launch operations on the same session to prevent
// duplicate agent processes after backend restart.
func (e *Executor) getSessionLock(sessionID string) *sync.Mutex {
	val, _ := e.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return val.(*sync.Mutex)
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
	return e.ExecuteWithProfile(ctx, task, "", "", task.Description, "")
}

// ExecuteWithProfile starts agent execution for a task using an explicit agent profile.
// The executorID parameter specifies which executor to use (determines runtime: local, worktree, local_docker, etc.).
// If executorID is empty, falls back to workspace's default executor.
// The prompt parameter is the initial prompt to send to the agent.
// The workflowStepID parameter associates the session with a workflow step for transitions.
func (e *Executor) ExecuteWithProfile(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, prompt string, workflowStepID string) (*TaskExecution, error) {
	// Create session entry in database first
	sessionID, err := e.PrepareSession(ctx, task, agentProfileID, executorID, workflowStepID)
	if err != nil {
		return nil, err
	}

	// Launch the agent for the prepared session
	return e.LaunchPreparedSession(ctx, task, sessionID, agentProfileID, executorID, prompt, workflowStepID, true)
}

// PrepareSession creates a session entry in the database without launching the agent.
// This allows the caller to get the session ID immediately and launch the agent later.
// Returns the session ID.
func (e *Executor) PrepareSession(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, workflowStepID string) (string, error) {
	if agentProfileID == "" {
		e.logger.Error("task has no agent_profile_id configured", zap.String("task_id", task.ID))
		return "", ErrNoAgentProfileID
	}

	metadata := cloneMetadata(task.Metadata)
	var repositoryID string
	var baseBranch string

	// Get the primary repository for this task
	primaryTaskRepo, err := e.repo.GetPrimaryTaskRepository(ctx, task.ID)
	if err != nil {
		e.logger.Error("failed to get primary task repository",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return "", err
	}

	if primaryTaskRepo != nil {
		repositoryID = primaryTaskRepo.RepositoryID
		baseBranch = primaryTaskRepo.BaseBranch
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
			"cli_passthrough":              profileInfo.CLIPassthrough,
		}
	} else {
		agentProfileSnapshot = map[string]interface{}{
			"id":    agentProfileID,
			"model": "",
		}
	}

	// Create agent session in database
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
		IsPrimary:            true,
	}
	if workflowStepID != "" {
		session.WorkflowStepID = &workflowStepID
	}

	// Resolve executor configuration
	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		session.ExecutorID = execConfig.ExecutorID
	}

	if err := e.repo.CreateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to persist agent session",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return "", err
	}

	// Clear primary flag on any other sessions for this task
	if err := e.repo.SetSessionPrimary(ctx, sessionID); err != nil {
		e.logger.Warn("failed to update primary session flag",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	e.logger.Info("session entry created",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID))

	return sessionID, nil
}

// LaunchPreparedSession launches the workspace (and optionally the agent) for a pre-created session.
// The session must have been created using PrepareSession.
// When startAgent is false, only the workspace infrastructure (agentctl) is launched; the agent
// subprocess is not started and the session state remains CREATED.
// When startAgent is true and the workspace was already launched (AgentExecutionID set), only the
// agent subprocess is started.
func (e *Executor) LaunchPreparedSession(ctx context.Context, task *v1.Task, sessionID string, agentProfileID string, executorID string, prompt string, workflowStepID string, startAgent bool) (*TaskExecution, error) {
	// Fetch the session to get its configuration
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		e.logger.Error("failed to get session for launch",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, err
	}

	if session.TaskID != task.ID {
		return nil, fmt.Errorf("session does not belong to task")
	}

	// Fast path: workspace already launched (e.g., from PrepareSession with workspace).
	// Only start the agent subprocess if requested; otherwise return early.
	if session.AgentExecutionID != "" {
		return e.startAgentOnExistingWorkspace(ctx, task, session, prompt, startAgent)
	}

	metadata := cloneMetadata(task.Metadata)
	var repositoryPath string
	var repositoryID string
	var baseBranch string
	var worktreeBranchPrefix string
	var pullBeforeWorktree bool
	var repository *models.Repository

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

		if repositoryID != "" {
			repository, err = e.repo.GetRepository(ctx, repositoryID)
			if err != nil {
				e.logger.Error("failed to get repository",
					zap.String("repository_id", repositoryID),
					zap.Error(err))
				return nil, err
			}
			repositoryPath = repository.LocalPath
			worktreeBranchPrefix = repository.WorktreeBranchPrefix
			pullBeforeWorktree = repository.PullBeforeWorktree
			if baseBranch == "" && repository.DefaultBranch != "" {
				baseBranch = repository.DefaultBranch
			}
		}
	}

	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		TaskTitle:       task.Title,
		AgentProfileID:  agentProfileID,
		TaskDescription: prompt,
		Priority:        task.Priority,
		SessionID:       sessionID,
	}

	// Resolve executor configuration
	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		metadata = execConfig.Metadata
		req.ExecutorType = execConfig.ExecutorType
		req.ExecutorConfig = execConfig.ExecutorCfg
	}

	if repositoryPath != "" {
		req.UseWorktree = shouldUseWorktree(execConfig.ExecutorType)
		req.RepositoryPath = repositoryPath
		req.BaseBranch = baseBranch
		req.WorktreeBranchPrefix = worktreeBranchPrefix
		req.PullBeforeWorktree = pullBeforeWorktree
	}

	// Remote Docker needs a clone URL since the remote host has no access to the local filesystem.
	if models.ExecutorType(execConfig.ExecutorType) == models.ExecutorTypeRemoteDocker && repository != nil {
		cloneURL := repositoryCloneURL(repository)
		if cloneURL == "" {
			return nil, ErrRemoteDockerNoRepoURL
		}
		req.RepositoryURL = cloneURL
	}

	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	e.logger.Info("launching agent for prepared session",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
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
				zap.String("session_id", sessionID),
				zap.Error(updateErr))
		}
		if updateErr := e.updateTaskState(ctx, task.ID, v1.TaskStateFailed); updateErr != nil {
			e.logger.Warn("failed to mark task as failed after launch error",
				zap.String("task_id", task.ID),
				zap.Error(updateErr))
		}
		return nil, err
	}

	now := time.Now().UTC()
	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
	if startAgent {
		session.State = models.TaskSessionStateStarting
	}
	session.ErrorMessage = ""
	session.UpdatedAt = now

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

	sessionState := v1.TaskSessionStateCreated
	if startAgent {
		sessionState = v1.TaskSessionStateStarting
	}
	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: resp.AgentExecutionID,
		AgentProfileID:   agentProfileID,
		StartedAt:        session.StartedAt,
		SessionState:     sessionState,
		LastUpdate:       now,
		SessionID:        sessionID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}

	if startAgent {
		e.startAgentProcessAsync(task.ID, sessionID, resp.AgentExecutionID)
	}

	e.logger.Info("agent launched for prepared session",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", resp.AgentExecutionID))

	return execution, nil
}

// startAgentOnExistingWorkspace handles the case where LaunchPreparedSession is called on a session
// whose workspace (agentctl) was already launched. It optionally starts just the agent subprocess.
func (e *Executor) startAgentOnExistingWorkspace(ctx context.Context, task *v1.Task, session *models.TaskSession, prompt string, startAgent bool) (*TaskExecution, error) {
	if !startAgent {
		// Workspace already launched, nothing else to do
		now := time.Now().UTC()
		return &TaskExecution{
			TaskID:           task.ID,
			AgentExecutionID: session.AgentExecutionID,
			AgentProfileID:   session.AgentProfileID,
			StartedAt:        session.StartedAt,
			SessionState:     v1.TaskSessionState(session.State),
			LastUpdate:       now,
			SessionID:        session.ID,
		}, nil
	}

	// Update the task description in the existing execution so StartAgentProcess picks it up
	if prompt != "" {
		if err := e.agentManager.SetExecutionDescription(ctx, session.AgentExecutionID, prompt); err != nil {
			e.logger.Warn("failed to set execution description for existing workspace",
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", session.AgentExecutionID),
				zap.Error(err))
			// Non-fatal: agent may start without description
		}
	}

	// Transition session to STARTING
	now := time.Now().UTC()
	session.State = models.TaskSessionStateStarting
	session.ErrorMessage = ""
	session.UpdatedAt = now
	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update session state for agent start",
			zap.String("session_id", session.ID),
			zap.Error(err))
	}

	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: session.AgentExecutionID,
		AgentProfileID:   session.AgentProfileID,
		StartedAt:        now,
		SessionState:     v1.TaskSessionStateStarting,
		LastUpdate:       now,
		SessionID:        session.ID,
	}

	// Start the agent process asynchronously
	e.startAgentProcessAsync(task.ID, session.ID, session.AgentExecutionID)

	e.logger.Info("agent starting on existing workspace",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_execution_id", session.AgentExecutionID))

	return execution, nil
}

// ResumeSession restarts an existing task session using its stored worktree.
// When startAgent is false, only the executor runtime is started (agent process is not launched).
func (e *Executor) ResumeSession(ctx context.Context, session *models.TaskSession, startAgent bool) (*TaskExecution, error) {
	if session == nil {
		return nil, ErrExecutionNotFound
	}

	// Acquire per-session lock to prevent concurrent resume/launch operations.
	// This is critical after backend restart when multiple resume requests may arrive
	// simultaneously (e.g., frontend auto-resume hook firing on page open).
	sessionLock := e.getSessionLock(session.ID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

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
		TaskDescription: task.Description,
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
	var worktreeBranchPrefix string
	var pullBeforeWorktree bool
	var repository *models.Repository
	if repositoryID == "" && len(task.Repositories) > 0 {
		repositoryID = task.Repositories[0].RepositoryID
	}
	if repositoryID != "" {
		repository, err = e.repo.GetRepository(ctx, repositoryID)
		if err != nil {
			e.logger.Error("failed to load repository for task session resume",
				zap.String("task_id", task.ID),
				zap.String("repository_id", repositoryID),
				zap.Error(err))
			return nil, err
		}
		repositoryPath = repository.LocalPath
		worktreeBranchPrefix = repository.WorktreeBranchPrefix
		pullBeforeWorktree = repository.PullBeforeWorktree
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

	// Remote Docker needs a clone URL since the remote host has no access to the local filesystem.
	if models.ExecutorType(req.ExecutorType) == models.ExecutorTypeRemoteDocker && repository != nil {
		cloneURL := repositoryCloneURL(repository)
		if cloneURL == "" {
			return nil, ErrRemoteDockerNoRepoURL
		}
		req.RepositoryURL = cloneURL
	}

	if shouldUseWorktree(req.ExecutorType) && repositoryPath != "" {
		req.UseWorktree = true
		req.RepositoryPath = repositoryPath
		req.RepositoryID = repositoryID
		if baseBranch != "" {
			req.BaseBranch = baseBranch
		} else {
			req.BaseBranch = "main"
		}
		req.WorktreeBranchPrefix = worktreeBranchPrefix
		req.PullBeforeWorktree = pullBeforeWorktree
	}

	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, session.ID); err == nil && running != nil {
		if running.ResumeToken != "" && startAgent {
			req.ACPSessionID = running.ResumeToken
			// Clear TaskDescription so the agent doesn't receive an automatic prompt on resume.
			// The session context is restored via ACP session/load; sending a prompt here would
			// cause the agent to start working immediately instead of waiting for user input.
			req.TaskDescription = ""
			e.logger.Info("found resume token for session resumption",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID))
		}
	}

	e.logger.Debug("resuming agent session",
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
				if updateErr := e.updateTaskState(context.Background(), task.ID, v1.TaskStateFailed); updateErr != nil {
					e.logger.Warn("failed to mark task as failed after start error on resume",
						zap.String("task_id", task.ID),
						zap.Error(updateErr))
				}
				return
			}

			// Agent resumed successfully - sync task state with session state.
			// If the session is waiting for input, the task should be in REVIEW state.
			// This ensures the task state reflects the actual agent status.
			if session.State == models.TaskSessionStateWaitingForInput {
				if updateErr := e.updateTaskState(context.Background(), task.ID, v1.TaskStateReview); updateErr != nil {
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
// Attachments (images) are passed to the agent if provided
func (e *Executor) Prompt(ctx context.Context, taskID, sessionID string, prompt string, attachments []v1.MessageAttachment) (*PromptResult, error) {
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
		zap.Int("prompt_length", len(prompt)),
		zap.Int("attachments_count", len(attachments)))

	result, err := e.agentManager.PromptAgent(ctx, session.AgentExecutionID, prompt, attachments)
	if err != nil {
		if errors.Is(err, lifecycle.ErrExecutionNotFound) {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}
	return result, nil
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
		repository, repoErr := e.repo.GetRepository(ctx, session.RepositoryID)
		if repoErr == nil && repository != nil {
			repositoryPath = repository.LocalPath
			req.RepositoryURL = repository.LocalPath
			req.Branch = session.BaseBranch

			// Remote Docker needs a clone URL instead of a local path.
			if models.ExecutorType(execConfig.ExecutorType) == models.ExecutorTypeRemoteDocker {
				cloneURL := repositoryCloneURL(repository)
				if cloneURL == "" {
					return nil, ErrRemoteDockerNoRepoURL
				}
				req.RepositoryURL = cloneURL
			}
		}
	}

	// Configure worktree if executor type requires it - reuse existing worktree from session
	if shouldUseWorktree(execConfig.ExecutorType) && repositoryPath != "" {
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

// executorConfig holds resolved executor configuration.
type executorConfig struct {
	ExecutorID   string
	ExecutorType string
	ExecutorCfg  map[string]string // The executor record's Config map (docker_host, etc.)
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

	if resolved == "" {
		return executorConfig{Metadata: metadata}
	}

	metadata["executor_id"] = resolved

	executor, err := e.repo.GetExecutor(ctx, resolved)
	if err != nil || executor == nil {
		return executorConfig{
			ExecutorID: resolved,
			Metadata:   metadata,
		}
	}

	if policyJSON := strings.TrimSpace(executor.Config["mcp_policy"]); policyJSON != "" {
		metadata["executor_mcp_policy"] = policyJSON
	}

	return executorConfig{
		ExecutorID:   resolved,
		ExecutorType: string(executor.Type),
		ExecutorCfg:  executor.Config,
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
