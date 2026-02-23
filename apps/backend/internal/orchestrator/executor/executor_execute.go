package executor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// runAgentProcessAsync starts the agent subprocess in a background goroutine.
// On error it marks both the session and task as FAILED.
// On success it calls onSuccess with a non-cancellable context derived from ctx.
// ctx is used with WithoutCancel so trace spans are preserved without inheriting cancellation.
func (e *Executor) runAgentProcessAsync(ctx context.Context, taskID, sessionID, agentExecutionID string, onSuccess func(context.Context)) {
	go func() {
		startCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		updateCtx := context.WithoutCancel(ctx)

		if err := e.agentManager.StartAgentProcess(startCtx, agentExecutionID); err != nil {
			e.logger.Error("failed to start agent process",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("agent_execution_id", agentExecutionID),
				zap.Error(err))
			if updateErr := e.updateSessionState(updateCtx, taskID, sessionID, models.TaskSessionStateFailed, err.Error()); updateErr != nil {
				e.logger.Warn("failed to mark session as failed after start error",
					zap.String("session_id", sessionID),
					zap.Error(updateErr))
			}
			if updateErr := e.updateTaskState(updateCtx, taskID, v1.TaskStateFailed); updateErr != nil {
				e.logger.Warn("failed to mark task as failed after start error",
					zap.String("task_id", taskID),
					zap.Error(updateErr))
			}
			return
		}

		onSuccess(updateCtx)
	}()
}

// startAgentProcessAsync starts the agent subprocess and transitions the task to IN_PROGRESS on success.
func (e *Executor) startAgentProcessAsync(ctx context.Context, taskID, sessionID, agentExecutionID string) {
	e.runAgentProcessAsync(ctx, taskID, sessionID, agentExecutionID, func(updCtx context.Context) {
		if updateErr := e.updateTaskState(updCtx, taskID, v1.TaskStateInProgress); updateErr != nil {
			e.logger.Warn("failed to update task state to IN_PROGRESS after agent start",
				zap.String("task_id", taskID),
				zap.Error(updateErr))
		}
	})
}

// updateTaskState updates a task's state, using the callback if set for event publishing,
// or falling back to the raw repository.
func (e *Executor) updateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	if e.onTaskStateChange != nil {
		return e.onTaskStateChange(ctx, taskID, state)
	}
	return e.repo.UpdateTaskState(ctx, taskID, state)
}

// updateSessionState updates a session's state, using the callback if set for event publishing,
// or falling back to the raw repository.
func (e *Executor) updateSessionState(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
	if e.onSessionStateChange != nil {
		return e.onSessionStateChange(ctx, taskID, sessionID, state, errorMessage)
	}
	return e.repo.UpdateTaskSessionState(ctx, sessionID, state, errorMessage)
}

// shouldUseWorktree returns true if the given executor type should use Git worktrees.
func shouldUseWorktree(executorType string) bool {
	return models.ExecutorType(executorType) == models.ExecutorTypeWorktree
}

func shouldApplyPreferredShell(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree:
		return true
	default:
		return false
	}
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

func (e *Executor) applyPreferredShellEnv(ctx context.Context, executorType string, env map[string]string) map[string]string {
	if !shouldApplyPreferredShell(executorType) {
		return env
	}
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
	return e.ExecuteWithFullProfile(ctx, task, "", "", "", task.Description, "")
}

// ExecuteWithProfile starts agent execution for a task using an explicit agent profile.
// The executorID parameter specifies which executor to use (determines runtime: local, worktree, local_docker, etc.).
// If executorID is empty, falls back to workspace's default executor.
// The prompt parameter is the initial prompt to send to the agent.
// The workflowStepID parameter associates the session with a workflow step for transitions.
func (e *Executor) ExecuteWithProfile(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, prompt string, workflowStepID string) (*TaskExecution, error) {
	return e.ExecuteWithFullProfile(ctx, task, agentProfileID, executorID, "", prompt, workflowStepID)
}

// ExecuteWithFullProfile starts agent execution for a task using an explicit agent profile and executor profile.
func (e *Executor) ExecuteWithFullProfile(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, prompt string, workflowStepID string) (*TaskExecution, error) {
	// Create session entry in database first
	sessionID, err := e.PrepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID)
	if err != nil {
		return nil, err
	}

	// Launch the agent for the prepared session
	return e.LaunchPreparedSession(ctx, task, sessionID, LaunchOptions{
		AgentProfileID: agentProfileID,
		ExecutorID:     executorID,
		Prompt:         prompt,
		WorkflowStepID: workflowStepID,
		StartAgent:     true,
	})
}

// PrepareSession creates a session entry in the database without launching the agent.
// This allows the caller to get the session ID immediately and launch the agent later.
// Returns the session ID.
func (e *Executor) PrepareSession(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, workflowStepID string) (string, error) {
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
	agentProfileSnapshot, isPassthrough := e.resolveAgentProfileSnapshot(ctx, agentProfileID)

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
		IsPassthrough:        isPassthrough,
	}
	if workflowStepID != "" {
		session.WorkflowStepID = &workflowStepID
	}

	// Store executor profile ID on session
	if executorProfileID != "" {
		session.ExecutorProfileID = executorProfileID
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = executorProfileID
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

// resolveAgentProfileSnapshot resolves an agent profile ID to a snapshot map and passthrough flag.
func (e *Executor) resolveAgentProfileSnapshot(ctx context.Context, agentProfileID string) (map[string]interface{}, bool) {
	profileInfo, err := e.agentManager.ResolveAgentProfile(ctx, agentProfileID)
	if err != nil || profileInfo == nil {
		return map[string]interface{}{
			"id":    agentProfileID,
			"model": "",
		}, false
	}
	return map[string]interface{}{
		"id":                           profileInfo.ProfileID,
		"name":                         profileInfo.ProfileName,
		"agent_id":                     profileInfo.AgentID,
		"agent_name":                   profileInfo.AgentName,
		"model":                        profileInfo.Model,
		"auto_approve":                 profileInfo.AutoApprove,
		"dangerously_skip_permissions": profileInfo.DangerouslySkipPermissions,
		"cli_passthrough":              profileInfo.CLIPassthrough,
	}, profileInfo.CLIPassthrough
}

// LaunchPreparedSession launches the workspace (and optionally the agent) for a pre-created session.
// The session must have been created using PrepareSession.
// When opts.StartAgent is false, only the workspace infrastructure (agentctl) is launched; the agent
// subprocess is not started and the session state remains CREATED.
// When opts.StartAgent is true and the workspace was already launched (AgentExecutionID set), only the
// agent subprocess is started.
func (e *Executor) LaunchPreparedSession(ctx context.Context, task *v1.Task, sessionID string, opts LaunchOptions) (*TaskExecution, error) {
	agentProfileID := opts.AgentProfileID
	executorID := opts.ExecutorID
	prompt := opts.Prompt
	startAgent := opts.StartAgent
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

	repoInfo, err := e.resolvePrimaryRepoInfo(ctx, task.ID)
	if err != nil {
		return nil, err
	}

	req, err := e.buildLaunchAgentRequest(ctx, task, session, agentProfileID, executorID, prompt, repoInfo)
	if err != nil {
		return nil, err
	}

	e.logger.Info("launching agent for prepared session",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_type", req.ExecutorType),
		zap.Bool("use_worktree", req.UseWorktree))

	req.Env = e.applyPreferredShellEnv(ctx, req.ExecutorType, req.Env)

	// Call the AgentManager to launch the container
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		return nil, e.handleLaunchFailure(ctx, task.ID, sessionID, err)
	}

	return e.finalizeLaunch(ctx, task, session, agentProfileID, sessionID, repoInfo, resp, startAgent)
}

// handleLaunchFailure marks the session and task as FAILED and returns the original error.
func (e *Executor) handleLaunchFailure(ctx context.Context, taskID, sessionID string, launchErr error) error {
	e.logger.Error("failed to launch agent",
		zap.String("task_id", taskID),
		zap.Error(launchErr))
	if updateErr := e.updateSessionState(ctx, taskID, sessionID, models.TaskSessionStateFailed, launchErr.Error()); updateErr != nil {
		e.logger.Warn("failed to mark session as failed after launch error",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	if updateErr := e.updateTaskState(ctx, taskID, v1.TaskStateFailed); updateErr != nil {
		e.logger.Warn("failed to mark task as failed after launch error",
			zap.String("task_id", taskID),
			zap.Error(updateErr))
	}
	return launchErr
}

// finalizeLaunch persists launch state and returns the resulting TaskExecution.
func (e *Executor) finalizeLaunch(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, sessionID string, repoInfo *repoInfo, resp *LaunchAgentResponse, startAgent bool) (*TaskExecution, error) {
	now := time.Now().UTC()
	e.persistLaunchState(ctx, task.ID, sessionID, session, resp, startAgent, now)
	e.persistWorktreeAssociation(ctx, task.ID, session.ID, repoInfo.RepositoryID, resp)

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
		e.startAgentProcessAsync(ctx, task.ID, sessionID, resp.AgentExecutionID)
	}

	e.logger.Info("agent launched for prepared session",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", resp.AgentExecutionID))

	return execution, nil
}

// buildLaunchAgentRequest constructs a LaunchAgentRequest for a new session launch,
// applying executor config, repository/worktree settings, and remote docker URL as needed.
func (e *Executor) buildLaunchAgentRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, executorID, prompt string, repoInfo *repoInfo) (*LaunchAgentRequest, error) {
	metadata := cloneMetadata(task.Metadata)
	if session.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	sessionID := session.ID
	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		TaskTitle:       task.Title,
		AgentProfileID:  agentProfileID,
		TaskDescription: prompt,
		Priority:        task.Priority,
		SessionID:       sessionID,
	}

	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		metadata = execConfig.Metadata
		req.ExecutorType = execConfig.ExecutorType
		req.ExecutorConfig = execConfig.ExecutorCfg
		req.SetupScript = execConfig.SetupScript
		// Merge profile env vars into request env
		if len(execConfig.ProfileEnv) > 0 {
			if req.Env == nil {
				req.Env = make(map[string]string)
			}
			for k, v := range execConfig.ProfileEnv {
				req.Env[k] = v
			}
		}
	}

	if repoInfo.RepositoryPath != "" {
		req.UseWorktree = shouldUseWorktree(execConfig.ExecutorType)
		req.RepositoryPath = repoInfo.RepositoryPath
		req.BaseBranch = repoInfo.BaseBranch
		req.WorktreeBranchPrefix = repoInfo.WorktreeBranchPrefix
		req.PullBeforeWorktree = repoInfo.PullBeforeWorktree
		if repoInfo.Repository != nil && repoInfo.Repository.SetupScript != "" {
			if metadata == nil {
				metadata = make(map[string]interface{})
			}
			metadata[lifecycle.MetadataKeyRepoSetupScript] = repoInfo.Repository.SetupScript
		}
	}

	// Remote Docker needs a clone URL since the remote host has no access to the local filesystem.
	if models.ExecutorType(execConfig.ExecutorType) == models.ExecutorTypeRemoteDocker && repoInfo.Repository != nil {
		cloneURL := repositoryCloneURL(repoInfo.Repository)
		if cloneURL == "" {
			return nil, ErrRemoteDockerNoRepoURL
		}
		req.RepositoryURL = cloneURL
	}

	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	return req, nil
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
	e.startAgentProcessAsync(ctx, task.ID, session.ID, session.AgentExecutionID)

	e.logger.Info("agent starting on existing workspace",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_execution_id", session.AgentExecutionID))

	return execution, nil
}
