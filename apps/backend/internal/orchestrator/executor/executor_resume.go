package executor

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

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
func (e *Executor) persistLaunchState(ctx context.Context, taskID, sessionID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool, now time.Time) {
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

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := e.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}
	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           taskID,
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

	req, repositoryID, err := e.buildResumeRequest(ctx, task, session, startAgent)
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

	req.Env = e.applyPreferredShellEnv(ctx, req.Env)

	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to relaunch agent for session",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, err
	}

	e.persistResumeState(ctx, task.ID, session, resp, startAgent)
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
		e.startAgentProcessOnResume(task.ID, session, resp.AgentExecutionID)
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
func (e *Executor) buildResumeRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, startAgent bool) (*LaunchAgentRequest, string, error) {
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

	executorWasEmpty := session.ExecutorID == ""
	execConfig := e.resolveExecutorConfig(ctx, session.ExecutorID, task.WorkspaceID, metadata)
	session.ExecutorID = execConfig.ExecutorID
	metadata = execConfig.Metadata
	req.ExecutorType = execConfig.ExecutorType

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

	repositoryID, err := e.applyResumeRepoConfig(ctx, task, session, req)
	if err != nil {
		return nil, "", err
	}

	if running, runErr := e.repo.GetExecutorRunningBySessionID(ctx, session.ID); runErr == nil && running != nil {
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

	return req, repositoryID, nil
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
func (e *Executor) persistResumeState(ctx context.Context, taskID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool) {
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

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := e.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}
	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           taskID,
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

// startAgentProcessOnResume starts the agent process asynchronously after a session resume.
func (e *Executor) startAgentProcessOnResume(taskID string, session *models.TaskSession, agentExecutionID string) {
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := e.agentManager.StartAgentProcess(startCtx, agentExecutionID); err != nil {
			e.logger.Error("failed to start agent process on resume",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", agentExecutionID),
				zap.Error(err))
			if updateErr := e.repo.UpdateTaskSessionState(context.Background(), session.ID, models.TaskSessionStateFailed, err.Error()); updateErr != nil {
				e.logger.Warn("failed to mark session as failed after start error on resume",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.Error(updateErr))
			}
			if updateErr := e.updateTaskState(context.Background(), taskID, v1.TaskStateFailed); updateErr != nil {
				e.logger.Warn("failed to mark task as failed after start error on resume",
					zap.String("task_id", taskID),
					zap.Error(updateErr))
			}
			return
		}

		// Agent resumed successfully - sync task state with session state.
		if session.State == models.TaskSessionStateWaitingForInput {
			if updateErr := e.updateTaskState(context.Background(), taskID, v1.TaskStateReview); updateErr != nil {
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
	}()
}
