package executor

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// isStaleExecutionError checks if an error from LaunchAgent indicates a stale
// execution in the lifecycle manager's in-memory store. This happens when
// PrepareTaskSession registered an execution but the agent was never started,
// or the agent exited without proper cleanup.
func isStaleExecutionError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already has an agent running")
}

// repoInfo holds resolved repository details for agent launch.
type repoInfo struct {
	RepositoryID         string
	RepositoryPath       string
	BaseBranch           string
	WorktreeBranchPrefix string
	PullBeforeWorktree   bool
	Repository           *models.Repository
}

// resolvePrimaryRepoInfo fetches and resolves the primary repository info for a task.
func (e *Executor) resolvePrimaryRepoInfo(ctx context.Context, taskID string) (*repoInfo, error) {
	info := &repoInfo{}
	primaryTaskRepo, err := e.repo.GetPrimaryTaskRepository(ctx, taskID)
	if err != nil {
		e.logger.Error("failed to get primary task repository",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}
	if primaryTaskRepo == nil {
		return info, nil
	}
	info.RepositoryID = primaryTaskRepo.RepositoryID
	info.BaseBranch = primaryTaskRepo.BaseBranch
	if info.RepositoryID == "" {
		return info, nil
	}
	repo, err := e.repo.GetRepository(ctx, info.RepositoryID)
	if err != nil {
		e.logger.Error("failed to get repository",
			zap.String("repository_id", info.RepositoryID),
			zap.Error(err))
		return nil, err
	}
	info.Repository = repo
	info.RepositoryPath = repo.LocalPath
	info.WorktreeBranchPrefix = repo.WorktreeBranchPrefix
	info.PullBeforeWorktree = repo.PullBeforeWorktree
	if info.BaseBranch == "" && repo.DefaultBranch != "" {
		info.BaseBranch = repo.DefaultBranch
	}
	return info, nil
}

// persistLaunchState updates the session and executor running records after a successful agent launch.
// If existingRunning is provided, resume token and message UUID are carried forward without a DB read.
func (e *Executor) persistLaunchState(ctx context.Context, taskID, sessionID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool, now time.Time, execCfg executorConfig, existingRunning *models.ExecutorRunning) {
	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
	if startAgent {
		session.State = models.TaskSessionStateStarting
	}
	session.ErrorMessage = ""
	session.UpdatedAt = now

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update agent session after launch",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           taskID,
		ExecutorID:       session.ExecutorID,
		Runtime:          execCfg.RuntimeName,
		Status:           "starting",
		Resumable:        execCfg.Resumable,
		AgentExecutionID: resp.AgentExecutionID,
		ContainerID:      resp.ContainerID,
		WorktreeID:       resp.WorktreeID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}
	if existingRunning != nil {
		running.ResumeToken = existingRunning.ResumeToken
		running.LastMessageUUID = existingRunning.LastMessageUUID
	}
	if err := e.repo.UpsertExecutorRunning(ctx, running); err != nil {
		e.logger.Warn("failed to persist executor runtime after launch",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// persistWorktreeAssociation creates a TaskSessionWorktree record if the response contains a worktree.
func (e *Executor) persistWorktreeAssociation(ctx context.Context, taskID, sessionID, repositoryID string, resp *LaunchAgentResponse) {
	if resp.WorktreeID == "" {
		return
	}
	sessionWorktree := &models.TaskSessionWorktree{
		SessionID:      sessionID,
		WorktreeID:     resp.WorktreeID,
		RepositoryID:   repositoryID,
		Position:       0,
		WorktreePath:   resp.WorktreePath,
		WorktreeBranch: resp.WorktreeBranch,
	}
	if err := e.repo.CreateTaskSessionWorktree(ctx, sessionWorktree); err != nil {
		e.logger.Error("failed to persist session worktree association",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("worktree_id", resp.WorktreeID),
			zap.Error(err))
	}
}

// ResumeSession restarts an existing task session using its stored worktree.
// When startAgent is false, only the executor runtime is started (agent process is not launched).
func (e *Executor) ResumeSession(ctx context.Context, session *models.TaskSession, startAgent bool) (*TaskExecution, error) {
	task, unlock, err := e.validateAndLockResume(ctx, session)
	if err != nil {
		return nil, err
	}
	defer unlock()

	req, repositoryID, execCfg, existingRunning, err := e.buildResumeRequest(ctx, task, session, startAgent)
	if err != nil {
		return nil, err
	}

	e.logger.Debug("resuming agent session",
		zap.String("task_id", session.TaskID),
		zap.String("session_id", session.ID),
		zap.String("agent_profile_id", session.AgentProfileID),
		zap.String("executor_type", req.ExecutorType),
		zap.String("resume_token", req.ACPSessionID),
		zap.Bool("use_worktree", req.UseWorktree))

	req.Env = e.applyPreferredShellEnv(ctx, req.ExecutorType, req.Env)

	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil && isStaleExecutionError(err) {
		// The lifecycle manager's in-memory store has a stale execution (agent not actually
		// running). This happens when PrepareTaskSession registered an execution but the
		// agent was never started, or the agent exited without cleanup. Clean up the stale
		// entry and retry.
		e.logger.Info("cleaning up stale execution and retrying launch",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID))
		if cleanupErr := e.agentManager.CleanupStaleExecutionBySessionID(ctx, session.ID); cleanupErr != nil {
			e.logger.Warn("failed to clean up stale execution",
				zap.String("session_id", session.ID),
				zap.Error(cleanupErr))
		}
		resp, err = e.agentManager.LaunchAgent(ctx, req)
	}
	if err != nil {
		e.logger.Error("failed to relaunch agent for session",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, err
	}

	e.persistResumeState(ctx, task.ID, session, resp, startAgent, execCfg, existingRunning)
	e.persistResumeWorktree(ctx, task.ID, session, repositoryID, resp)

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
		e.startAgentProcessOnResume(ctx, task.ID, session, resp.AgentExecutionID)
	}

	return execution, nil
}

// validateAndLockResume validates the session is resumable, acquires the per-session lock,
// and loads the associated task. Returns the task, an unlock function, and any error.
// The caller must call unlock() when the critical section is complete.
func (e *Executor) validateAndLockResume(ctx context.Context, session *models.TaskSession) (*v1.Task, func(), error) {
	if session == nil {
		return nil, func() {}, ErrExecutionNotFound
	}

	// Acquire per-session lock to prevent concurrent resume/launch operations.
	// This is critical after backend restart when multiple resume requests may arrive
	// simultaneously (e.g., frontend auto-resume hook firing on page open).
	sessionLock := e.getSessionLock(session.ID)
	sessionLock.Lock()
	unlock := func() { sessionLock.Unlock() }

	taskModel, err := e.repo.GetTask(ctx, session.TaskID)
	if err != nil {
		unlock()
		e.logger.Error("failed to load task for session resume",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, func() {}, err
	}
	task := taskModel.ToAPI()
	if task == nil {
		unlock()
		return nil, func() {}, ErrExecutionNotFound
	}

	if session.AgentProfileID == "" {
		unlock()
		e.logger.Error("task session has no agent_profile_id configured",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID))
		return nil, func() {}, ErrNoAgentProfileID
	}

	if existing, ok := e.GetExecutionBySession(session.ID); ok && existing != nil {
		unlock()
		return nil, func() {}, ErrExecutionAlreadyRunning
	}

	return task, unlock, nil
}

// buildResumeRequest constructs the LaunchAgentRequest for a session resume, resolving executor config,
// repository details, worktree settings, and ACP resume token.
// Returns the request, repository ID, executor config, existing ExecutorRunning record (may be nil), and error.
func (e *Executor) buildResumeRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, startAgent bool) (*LaunchAgentRequest, string, executorConfig, *models.ExecutorRunning, error) {
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
	if session.ExecutorProfileID != "" {
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreeID != "" {
		metadata["worktree_id"] = session.Worktrees[0].WorktreeID
	}

	execConfig := e.applyExecutorConfigToResumeRequest(ctx, req, task, session, metadata)

	repositoryID, err := e.applyResumeRepoConfig(ctx, task, session, req)
	if err != nil {
		return nil, "", execConfig, nil, err
	}

	existingRunning := e.applyRunningRecordToResumeRequest(ctx, req, task, session, startAgent)

	return req, repositoryID, execConfig, existingRunning, nil
}

// applyExecutorConfigToResumeRequest resolves executor config and applies it to the
// resume request, persisting executor assignment if newly resolved.
func (e *Executor) applyExecutorConfigToResumeRequest(ctx context.Context, req *LaunchAgentRequest, task *v1.Task, session *models.TaskSession, metadata map[string]interface{}) executorConfig {
	executorWasEmpty := session.ExecutorID == ""
	execConfig := e.resolveExecutorConfig(ctx, session.ExecutorID, task.WorkspaceID, metadata)
	session.ExecutorID = execConfig.ExecutorID
	req.ExecutorType = execConfig.ExecutorType
	req.ExecutorConfig = execConfig.ExecutorCfg
	req.SetupScript = execConfig.SetupScript
	if len(execConfig.ProfileEnv) > 0 {
		if req.Env == nil {
			req.Env = make(map[string]string)
		}
		for k, v := range execConfig.ProfileEnv {
			req.Env[k] = v
		}
	}

	if executorWasEmpty && session.ExecutorID != "" {
		session.UpdatedAt = time.Now().UTC()
		if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
			e.logger.Warn("failed to persist executor assignment for session",
				zap.String("session_id", session.ID),
				zap.String("executor_id", session.ExecutorID),
				zap.Error(err))
		}
	}
	if len(execConfig.Metadata) > 0 {
		req.Metadata = execConfig.Metadata
	}

	return execConfig
}

// applyRunningRecordToResumeRequest loads the ExecutorRunning record and applies
// resume-related fields (remote reconnect, resume token) to the request.
func (e *Executor) applyRunningRecordToResumeRequest(ctx context.Context, req *LaunchAgentRequest, task *v1.Task, session *models.TaskSession, startAgent bool) *models.ExecutorRunning {
	running, runErr := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if runErr != nil || running == nil {
		return nil
	}

	if req.ExecutorType == string(models.ExecutorTypeSprites) && running.AgentExecutionID != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata[lifecycle.MetadataKeyRemoteReconnect] = true
		req.Metadata[lifecycle.MetadataKeyRemoteExecID] = running.AgentExecutionID
		req.Metadata[lifecycle.MetadataKeyRemoteName] = spriteNameFromExecutionID(running.AgentExecutionID)
	}

	if running.ResumeToken != "" && startAgent {
		req.ACPSessionID = running.ResumeToken
		// Clear TaskDescription so the agent doesn't receive an automatic prompt on resume.
		// The session context is restored via ACP session/load; sending a prompt here would
		// cause the agent to start working immediately instead of waiting for user input.
		req.TaskDescription = ""
		e.logger.Info("found resume token for session resumption",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID))
	} else if startAgent && session.State == models.TaskSessionStateWaitingForInput {
		// Fresh-start resume (no resume token): don't auto-prompt with the task description.
		req.TaskDescription = ""
		e.logger.Info("fresh-start resume, clearing task description to avoid auto-prompt",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID))
	}

	return running
}

func spriteNameFromExecutionID(executionID string) string {
	if executionID == "" {
		return ""
	}
	trimmed := executionID
	if len(trimmed) > 12 {
		trimmed = trimmed[:12]
	}
	return "kandev-" + trimmed
}

// applyResumeRepoConfig resolves repository details and applies them to req.
// Returns the resolved repositoryID.
func (e *Executor) applyResumeRepoConfig(ctx context.Context, task *v1.Task, session *models.TaskSession, req *LaunchAgentRequest) (string, error) {
	repositoryID := session.RepositoryID
	if repositoryID == "" && len(task.Repositories) > 0 {
		repositoryID = task.Repositories[0].RepositoryID
	}

	baseBranch := session.BaseBranch
	if baseBranch == "" && len(task.Repositories) > 0 && task.Repositories[0].BaseBranch != "" {
		baseBranch = task.Repositories[0].BaseBranch
	}
	if baseBranch != "" {
		req.Branch = baseBranch
	}

	if repositoryID == "" {
		return "", nil
	}

	repository, err := e.repo.GetRepository(ctx, repositoryID)
	if err != nil {
		e.logger.Error("failed to load repository for task session resume",
			zap.String("task_id", task.ID),
			zap.String("repository_id", repositoryID),
			zap.Error(err))
		return "", err
	}

	repositoryPath := repository.LocalPath
	if repositoryPath != "" {
		req.RepositoryURL = repositoryPath
	}
	if repository.SetupScript != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata[lifecycle.MetadataKeyRepoSetupScript] = repository.SetupScript
	}

	if models.ExecutorType(req.ExecutorType) == models.ExecutorTypeRemoteDocker {
		cloneURL := repositoryCloneURL(repository)
		if cloneURL == "" {
			return "", ErrRemoteDockerNoRepoURL
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
			req.BaseBranch = defaultBaseBranch
		}
		req.WorktreeBranchPrefix = repository.WorktreeBranchPrefix
		req.PullBeforeWorktree = repository.PullBeforeWorktree
	}

	return repositoryID, nil
}

// persistResumeState updates session and executor running records after a successful resume launch.
func (e *Executor) persistResumeState(ctx context.Context, taskID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool, execCfg executorConfig, existingRunning *models.ExecutorRunning) {
	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
	session.ErrorMessage = ""
	if startAgent {
		session.State = models.TaskSessionStateStarting
		session.CompletedAt = nil
	}

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update task session for resume",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           taskID,
		ExecutorID:       session.ExecutorID,
		Runtime:          execCfg.RuntimeName,
		Status:           "starting",
		Resumable:        execCfg.Resumable,
		AgentExecutionID: resp.AgentExecutionID,
		ContainerID:      resp.ContainerID,
		WorktreeID:       resp.WorktreeID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
	}
	if existingRunning != nil {
		running.ResumeToken = existingRunning.ResumeToken
		running.LastMessageUUID = existingRunning.LastMessageUUID
	}
	if err := e.repo.UpsertExecutorRunning(ctx, running); err != nil {
		e.logger.Warn("failed to persist executor runtime after resume",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
	}
}

// persistResumeWorktree creates a worktree association if a new worktree was allocated during resume.
func (e *Executor) persistResumeWorktree(ctx context.Context, taskID string, session *models.TaskSession, repositoryID string, resp *LaunchAgentResponse) {
	if resp.WorktreeID == "" {
		return
	}
	for _, wt := range session.Worktrees {
		if wt.WorktreeID == resp.WorktreeID {
			return
		}
	}
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
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("worktree_id", resp.WorktreeID),
			zap.Error(err))
	}
}

// startAgentProcessOnResume starts the agent process asynchronously after a session resume,
// syncing the task state (REVIEW or unchanged) on success.
func (e *Executor) startAgentProcessOnResume(ctx context.Context, taskID string, session *models.TaskSession, agentExecutionID string) {
	e.runAgentProcessAsync(ctx, taskID, session.ID, agentExecutionID, func(updCtx context.Context) {
		// Agent resumed successfully - sync task state with session state.
		if session.State == models.TaskSessionStateWaitingForInput {
			if updateErr := e.updateTaskState(updCtx, taskID, v1.TaskStateReview); updateErr != nil {
				e.logger.Warn("failed to update task state to REVIEW after resume",
					zap.String("task_id", taskID),
					zap.Error(updateErr))
			} else {
				e.logger.Debug("task state synced to REVIEW after resume (session waiting for input)",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID))
			}
		} else {
			e.logger.Debug("agent resumed successfully, task state unchanged",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.String("session_state", string(session.State)))
		}
	})
}
