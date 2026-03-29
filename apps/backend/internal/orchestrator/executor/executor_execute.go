package executor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// isConfigModeSession returns true if the session has config_mode: true in its metadata.
// Config-mode sessions are dedicated settings-chat sessions that get config MCP tools.
func isConfigModeSession(session *models.TaskSession) bool {
	if session == nil || session.Metadata == nil {
		return false
	}
	cm, ok := session.Metadata["config_mode"].(bool)
	return ok && cm
}

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
			// Let the orchestrator handle auth errors as recoverable failures.
			if e.onAgentStartFailed != nil && e.onAgentStartFailed(updateCtx, taskID, sessionID, agentExecutionID, err) {
				return
			}
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
			// Clean up the execution environment (e.g., destroy remote Sprites instance).
			// Use force=true since the agent process never fully started.
			if stopErr := e.agentManager.StopAgent(updateCtx, agentExecutionID, true); stopErr != nil {
				e.logger.Warn("failed to clean up agent after start failure",
					zap.String("agent_execution_id", agentExecutionID),
					zap.Error(stopErr))
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
	if e.capabilities == nil || !e.capabilities.ShouldApplyPreferredShell(executorType) {
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

	// Determine if this new session should become primary.
	// Only the first session for a task is primary by default; subsequent sessions
	// leave the existing primary unchanged so the user's explicit choice is preserved.
	existingSessions, _ := e.repo.ListTaskSessions(ctx, task.ID)
	hasPrimary := false
	for _, s := range existingSessions {
		if s.IsPrimary {
			hasPrimary = true
			break
		}
	}
	isFirstSession := !hasPrimary

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
		IsPrimary:            isFirstSession,
		IsPassthrough:        isPassthrough,
		Metadata:             metadata,
	}
	// workflow_step_id is a task-level field; no longer stored on sessions.

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

	// Set primary flag only for the first session (no existing primary).
	// Subsequent sessions do not override the established primary.
	if isFirstSession {
		if err := e.repo.SetSessionPrimary(ctx, sessionID); err != nil {
			e.logger.Warn("failed to update primary session flag",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
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

	// Inject session handover context if there are previous sessions for this task.
	prompt = e.injectHandoverIfNeeded(ctx, task.ID, sessionID, prompt)

	// Fast path: workspace already launched (e.g., from PrepareSession with workspace).
	// Only start the agent subprocess if requested; otherwise return early.
	if session.AgentExecutionID != "" {
		return e.startAgentOnExistingWorkspace(ctx, task, session, prompt, startAgent, opts.McpMode)
	}

	repoInfo, err := e.resolvePrimaryRepoInfo(ctx, task.ID)
	if err != nil {
		return nil, err
	}

	req, execCfg, err := e.buildLaunchAgentRequest(ctx, task, session, agentProfileID, executorID, prompt, repoInfo)
	if err != nil {
		return nil, err
	}

	// Apply McpMode from options (takes precedence over session metadata check in buildLaunchAgentRequest)
	if opts.McpMode != "" {
		req.McpMode = opts.McpMode
	}

	// Pass attachments for the initial prompt
	if len(opts.Attachments) > 0 {
		req.Attachments = opts.Attachments
	}

	// Check for an existing task environment to reuse its worktree
	existingEnv, _ := e.repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if existingEnv != nil && existingEnv.WorktreeID != "" && req.UseWorktree {
		req.WorktreeID = existingEnv.WorktreeID
		e.logger.Info("reusing existing task environment worktree",
			zap.String("task_id", task.ID),
			zap.String("worktree_id", existingEnv.WorktreeID))
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

	// Create or update the task environment with launch results
	e.persistTaskEnvironment(ctx, task.ID, session, existingEnv, req, resp, execCfg)

	// Capture the current HEAD commit as the base commit for this session asynchronously.
	// This allows us to filter git log to only show commits made during the session.
	// We do this async to avoid delaying session launch while waiting for agentctl to be ready.
	// Use a bounded timeout context to prevent blocking indefinitely if agentctl never becomes ready.
	go func(sid string) {
		captureCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		e.captureBaseCommit(captureCtx, sid)
	}(sessionID)

	return e.finalizeLaunch(ctx, task, session, agentProfileID, sessionID, repoInfo, resp, startAgent, execCfg)
}

// handleLaunchFailure marks the session and task as FAILED and returns the original error.
func (e *Executor) handleLaunchFailure(ctx context.Context, taskID, sessionID string, launchErr error) error {
	// Detach from caller context so failure bookkeeping completes even if the
	// original request context was cancelled.
	failCtx := context.WithoutCancel(ctx)
	e.logger.Error("failed to launch agent",
		zap.String("task_id", taskID),
		zap.Error(launchErr))
	// Call onLaunchFailed before state updates so it can set the suppressToast
	// flag that updateSessionState will propagate to the frontend.
	if e.onLaunchFailed != nil {
		e.onLaunchFailed(failCtx, taskID, sessionID, launchErr)
	}
	if updateErr := e.updateSessionState(failCtx, taskID, sessionID, models.TaskSessionStateFailed, launchErr.Error()); updateErr != nil {
		e.logger.Warn("failed to mark session as failed after launch error",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	if updateErr := e.updateTaskState(failCtx, taskID, v1.TaskStateFailed); updateErr != nil {
		e.logger.Warn("failed to mark task as failed after launch error",
			zap.String("task_id", taskID),
			zap.Error(updateErr))
	}
	return launchErr
}

// finalizeLaunch persists launch state and returns the resulting TaskExecution.
func (e *Executor) finalizeLaunch(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, sessionID string, repoInfo *repoInfo, resp *LaunchAgentResponse, startAgent bool, execCfg executorConfig) (*TaskExecution, error) {
	now := time.Now().UTC()
	// On initial launch there is no existing ExecutorRunning record to carry forward.
	e.persistLaunchState(ctx, task.ID, sessionID, session, resp, startAgent, now, execCfg, nil)
	e.persistWorktreeAssociation(ctx, task.ID, session, repoInfo.RepositoryID, resp)

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
func (e *Executor) buildLaunchAgentRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, executorID, prompt string, repoInfo *repoInfo) (*LaunchAgentRequest, executorConfig, error) {
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
		req.CheckoutBranch = repoInfo.CheckoutBranch
		req.WorktreeBranchPrefix = repoInfo.WorktreeBranchPrefix
		req.PullBeforeWorktree = repoInfo.PullBeforeWorktree
		// Task directory mode: place worktree inside per-task directory
		if req.UseWorktree && repoInfo.Repository != nil && repoInfo.Repository.Name != "" {
			req.TaskDirName = worktree.SemanticWorktreeName(task.Title, worktree.SmallSuffix(3))
			req.RepoName = repoInfo.Repository.Name
		}
		if repoInfo.Repository != nil && repoInfo.Repository.SetupScript != "" {
			if metadata == nil {
				metadata = make(map[string]interface{})
			}
			metadata[lifecycle.MetadataKeyRepoSetupScript] = repoInfo.Repository.SetupScript
		}
	}

	// Remote executors need a clone URL since the remote host has no access to the local filesystem.
	if e.capabilities != nil && e.capabilities.RequiresCloneURL(execConfig.ExecutorType) && repoInfo.Repository != nil {
		cloneURL := repositoryCloneURL(repoInfo.Repository)
		if cloneURL == "" {
			return nil, execConfig, ErrNoCloneURL
		}
		req.RepositoryURL = cloneURL
	}

	// Activate config-mode MCP tools when config_mode is set in session metadata.
	if isConfigModeSession(session) {
		req.McpMode = McpModeConfig
	}

	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	return req, execConfig, nil
}

// startAgentOnExistingWorkspace handles the case where LaunchPreparedSession is called on a session
// whose workspace (agentctl) was already launched. It optionally starts just the agent subprocess.
func (e *Executor) startAgentOnExistingWorkspace(ctx context.Context, task *v1.Task, session *models.TaskSession, prompt string, startAgent bool, mcpMode string) (*TaskExecution, error) {
	// Resolve the actual execution ID from the in-memory store. After a backend
	// restart, EnsureWorkspaceExecutionForSession may have created a new execution
	// with a different ID than what the database still holds. Using the stale DB
	// value would cause "execution not found" errors.
	executionID := session.AgentExecutionID
	if liveID, err := e.agentManager.GetExecutionIDForSession(ctx, session.ID); err == nil && liveID != "" && liveID != executionID {
		e.logger.Info("correcting stale execution ID from DB with live in-memory value",
			zap.String("session_id", session.ID),
			zap.String("stale_id", executionID),
			zap.String("live_id", liveID))
		executionID = liveID
		session.AgentExecutionID = liveID
		if updateErr := e.repo.UpdateTaskSession(ctx, session); updateErr != nil {
			e.logger.Warn("failed to persist corrected execution ID",
				zap.String("session_id", session.ID),
				zap.Error(updateErr))
		}
	}

	if !startAgent {
		// Workspace already launched, nothing else to do
		now := time.Now().UTC()
		return &TaskExecution{
			TaskID:           task.ID,
			AgentExecutionID: executionID,
			AgentProfileID:   session.AgentProfileID,
			StartedAt:        session.StartedAt,
			SessionState:     v1.TaskSessionState(session.State),
			LastUpdate:       now,
			SessionID:        session.ID,
		}, nil
	}

	// Update the task description in the existing execution so StartAgentProcess picks it up
	if prompt != "" {
		if err := e.agentManager.SetExecutionDescription(ctx, executionID, prompt); err != nil {
			e.logger.Warn("failed to set execution description for existing workspace",
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", executionID),
				zap.Error(err))
			// Non-fatal: agent may start without description
		}
	}

	// If config MCP mode is needed, reconfigure the MCP server before starting the agent.
	// The workspace may have been prepared before config_mode was set on the session.
	effectiveMcpMode := mcpMode
	if effectiveMcpMode == "" && isConfigModeSession(session) {
		effectiveMcpMode = McpModeConfig
	}
	if effectiveMcpMode != "" {
		if err := e.agentManager.SetMcpMode(ctx, executionID, effectiveMcpMode); err != nil {
			e.logger.Error("failed to set MCP mode for existing workspace",
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", executionID),
				zap.String("mcp_mode", effectiveMcpMode),
				zap.Error(err))
			return nil, fmt.Errorf("set MCP mode %q: %w", effectiveMcpMode, err)
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
		AgentExecutionID: executionID,
		AgentProfileID:   session.AgentProfileID,
		StartedAt:        now,
		SessionState:     v1.TaskSessionStateStarting,
		LastUpdate:       now,
		SessionID:        session.ID,
	}

	// Start the agent process asynchronously
	e.startAgentProcessAsync(ctx, task.ID, session.ID, executionID)

	e.logger.Info("agent starting on existing workspace",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_execution_id", executionID))

	return execution, nil
}

// captureBaseCommit retrieves the merge-base commit from agentctl and stores it
// as the base commit for the session. This allows calculating cumulative diffs
// that show all changes on the branch relative to the target branch (e.g., main).
func (e *Executor) captureBaseCommit(ctx context.Context, sessionID string) {
	// Wait for agentctl to be ready before trying to get git status.
	// LaunchAgent returns before agentctl is fully ready (waits in goroutine),
	// so we need to explicitly wait here.
	if err := e.agentManager.WaitForAgentctlReady(ctx, sessionID); err != nil {
		e.logger.Warn("agentctl not ready for base commit capture",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	status, err := e.agentManager.GetGitStatus(ctx, sessionID)
	if err != nil {
		e.logger.Warn("failed to get git status for base commit capture",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	// Prefer BaseCommit (merge-base with target branch) over HeadCommit.
	// BaseCommit gives us the common ancestor with main/origin, which is correct
	// for showing all changes on the feature branch. HeadCommit would only show
	// changes made after the session started, missing commits already on the branch.
	baseCommit := status.BaseCommit
	if baseCommit == "" {
		// Fallback to HeadCommit if no merge-base is available (e.g., detached HEAD)
		baseCommit = status.HeadCommit
	}
	if baseCommit == "" {
		e.logger.Debug("no base commit available for capture",
			zap.String("session_id", sessionID))
		return
	}

	// Update the session's base commit in the database
	if err := e.repo.UpdateTaskSessionBaseCommit(ctx, sessionID, baseCommit); err != nil {
		e.logger.Warn("failed to update session base commit",
			zap.String("session_id", sessionID),
			zap.String("base_commit", baseCommit),
			zap.Error(err))
		return
	}

	e.logger.Info("captured base commit for session",
		zap.String("session_id", sessionID),
		zap.String("base_commit", baseCommit),
		zap.String("head_commit", status.HeadCommit))
}

// injectHandoverIfNeeded prepends session handover context to the prompt when the task
// already has previous sessions. The context includes the session count and the task plan
// (if one exists) so the new agent avoids repeating already-completed work.
func (e *Executor) injectHandoverIfNeeded(ctx context.Context, taskID, currentSessionID, prompt string) string {
	sessions, err := e.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list sessions for handover context",
			zap.String("task_id", taskID),
			zap.Error(err))
		return prompt
	}

	// Count previous sessions (exclude the current one being launched).
	var previousCount int
	for _, s := range sessions {
		if s.ID != currentSessionID {
			previousCount++
		}
	}
	if previousCount == 0 {
		return prompt
	}

	// Build the plan section if a plan exists.
	var planSection string
	plan, err := e.repo.GetTaskPlan(ctx, taskID)
	if err == nil && plan != nil && plan.Content != "" {
		planSection = fmt.Sprintf("\nThe task has an implementation plan:\n\n%s\n", plan.Content)
	}

	e.logger.Info("injecting session handover context",
		zap.String("task_id", taskID),
		zap.String("session_id", currentSessionID),
		zap.Int("previous_sessions", previousCount))

	return sysprompt.InjectSessionHandover(previousCount, planSection, prompt)
}

// persistTaskEnvironment creates or updates the task environment record after a successful launch.
// It also links the session to the environment via TaskEnvironmentID.
func (e *Executor) persistTaskEnvironment(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	existingEnv *models.TaskEnvironment,
	req *LaunchAgentRequest,
	resp *LaunchAgentResponse,
	execCfg executorConfig,
) {
	if existingEnv != nil {
		// Update the existing environment with new execution info
		existingEnv.AgentExecutionID = resp.AgentExecutionID
		existingEnv.Status = models.TaskEnvironmentStatusReady
		if err := e.repo.UpdateTaskEnvironment(ctx, existingEnv); err != nil {
			e.logger.Warn("failed to update task environment",
				zap.String("task_id", taskID),
				zap.String("env_id", existingEnv.ID),
				zap.Error(err))
		}
		session.TaskEnvironmentID = existingEnv.ID
		return
	}

	// Create a new task environment
	workspacePath := resp.WorktreePath
	if workspacePath == "" {
		workspacePath = req.RepositoryPath
	}
	// Task directory mode: WorkspacePath = task root, WorktreePath = repo subdir
	if req.TaskDirName != "" && resp.WorktreePath != "" {
		workspacePath = filepath.Dir(resp.WorktreePath)
	}
	env := &models.TaskEnvironment{
		TaskID:            taskID,
		RepositoryID:      req.RepositoryID,
		ExecutorType:      req.ExecutorType,
		ExecutorID:        execCfg.ExecutorID,
		ExecutorProfileID: session.ExecutorProfileID,
		AgentExecutionID:  resp.AgentExecutionID,
		Status:            models.TaskEnvironmentStatusReady,
		WorktreeID:        resp.WorktreeID,
		WorktreePath:      resp.WorktreePath,
		WorktreeBranch:    resp.WorktreeBranch,
		WorkspacePath:     workspacePath,
		ContainerID:       resp.ContainerID,
	}
	if err := e.repo.CreateTaskEnvironment(ctx, env); err != nil {
		e.logger.Warn("failed to create task environment",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	session.TaskEnvironmentID = env.ID
}
