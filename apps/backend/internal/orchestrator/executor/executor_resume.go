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

// isAgentAlreadyRunningError checks whether LaunchAgent refused because the
// lifecycle manager's in-memory store already has an execution for this session.
// The error is ambiguous on its own — it fires both when the execution is live
// (a concurrent resume raced us) and when it is stale (PrepareTaskSession
// registered an execution but the agent was never started, or the agent exited
// without proper cleanup). Callers must probe IsAgentRunningForSession to
// distinguish live from stale before deciding to clean up.
func isAgentAlreadyRunningError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already has an agent running")
}

// isTerminalSessionState reports whether a session state implies the agent
// process is no longer running. Stale in-memory execution or agentctl status
// for these states should be cleaned up rather than trusted.
func isTerminalSessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCancelled
}

// repoInfo holds resolved repository details for agent launch.
type repoInfo struct {
	RepositoryID         string
	RepositoryPath       string
	BaseBranch           string
	CheckoutBranch       string
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
	return e.resolveTaskRepoInfo(ctx, primaryTaskRepo)
}

// resolveAllRepoInfo returns the resolved repository info for every repository
// linked to the task, ordered by Position. Returns a single-element slice for
// single-repo tasks and an empty slice for repo-less tasks (e.g. quick chat).
// Each entry has LocalPath populated, cloning provider-backed repos on demand.
func (e *Executor) resolveAllRepoInfo(ctx context.Context, taskID string) ([]*repoInfo, error) {
	taskRepos, err := e.repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		e.logger.Error("failed to list task repositories",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}
	if len(taskRepos) == 0 {
		return nil, nil
	}
	out := make([]*repoInfo, 0, len(taskRepos))
	for _, tr := range taskRepos {
		info, resolveErr := e.resolveTaskRepoInfo(ctx, tr)
		if resolveErr != nil {
			return nil, resolveErr
		}
		out = append(out, info)
	}
	return out, nil
}

// resolveTaskRepoInfo turns a TaskRepository row into a fully-resolved repoInfo
// (loads the Repository entity, clones if necessary, fills defaults).
func (e *Executor) resolveTaskRepoInfo(ctx context.Context, tr *models.TaskRepository) (*repoInfo, error) {
	info := &repoInfo{
		RepositoryID:   tr.RepositoryID,
		BaseBranch:     tr.BaseBranch,
		CheckoutBranch: tr.CheckoutBranch,
	}
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

	// Clone provider-backed repos that have no local path yet
	if repo.LocalPath == "" && repo.ProviderOwner != "" && repo.ProviderName != "" {
		if localPath, cloneErr := e.ensureRepoCloned(ctx, repo); cloneErr != nil {
			return nil, cloneErr
		} else if localPath != "" {
			repo.LocalPath = localPath
		}
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

// ensureRepoCloned clones a provider-backed repository to local disk and updates its local path in the database.
// Returns the local path on success, or empty string if no cloner is configured.
func (e *Executor) ensureRepoCloned(ctx context.Context, repo *models.Repository) (string, error) {
	if e.repoCloner == nil {
		e.logger.Warn("repository has no local path and no cloner configured",
			zap.String("repository_id", repo.ID),
			zap.String("provider", repo.Provider),
			zap.String("owner", repo.ProviderOwner),
			zap.String("name", repo.ProviderName))
		return "", nil
	}

	cloneURL, urlErr := e.repoCloner.BuildCloneURL(repo.Provider, repo.ProviderOwner, repo.ProviderName)
	if urlErr != nil || cloneURL == "" {
		// Fall back to HTTPS URL if BuildCloneURL fails
		cloneURL = repositoryCloneURL(repo)
		if cloneURL == "" {
			return "", ErrNoCloneURL
		}
	}

	e.logger.Info("cloning provider-backed repository for local execution",
		zap.String("repository_id", repo.ID),
		zap.String("repo", repo.ProviderOwner+"/"+repo.ProviderName))

	localPath, err := e.repoCloner.EnsureCloned(ctx, cloneURL, repo.ProviderOwner, repo.ProviderName)
	if err != nil {
		e.logger.Error("failed to clone repository",
			zap.String("repository_id", repo.ID),
			zap.String("repo", repo.ProviderOwner+"/"+repo.ProviderName),
			zap.Error(err))
		return "", err
	}

	// Persist the local path so future launches skip cloning
	if e.repoUpdater != nil && localPath != "" {
		if updateErr := e.repoUpdater.UpdateRepositoryLocalPath(ctx, repo.ID, localPath); updateErr != nil {
			e.logger.Warn("failed to update repository local path after clone",
				zap.String("repository_id", repo.ID),
				zap.String("local_path", localPath),
				zap.Error(updateErr))
			// Non-fatal: the clone succeeded, we can use the path
		}
	}

	return localPath, nil
}

// persistLaunchState updates the session record after a successful agent launch.
// The executors_running row is now written by the lifecycle manager itself in
// lockstep with executionStore.Add (see lifecycle.persistExecutorRunning) — this
// function no longer touches it. The lifecycle manager also owns the columns that
// used to live on task_sessions (agent_execution_id, container_id); the orchestrator
// stops writing them so the only remaining source of truth is executors_running.
//
// What remains here: state transitions (e.g., STARTING) and prepare-result metadata
// merge, both of which are session-row concerns the lifecycle manager doesn't know
// about. The execCfg / existingRunning parameters are kept on the signature for
// historical call shape but are no longer used.
func (e *Executor) persistLaunchState(ctx context.Context, taskID, sessionID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool, now time.Time, execCfg executorConfig, existingRunning *models.ExecutorRunning) {
	_ = resp            // executors_running fields are written by lifecycle manager
	_ = execCfg         // ditto
	_ = existingRunning // ditto

	if startAgent {
		session.State = models.TaskSessionStateStarting
	}
	session.ErrorMessage = ""
	session.UpdatedAt = now

	// Merge prepare result into session metadata synchronously so it survives
	// the UpdateTaskSession write (which would otherwise clobber it if the async
	// handlePrepareCompleted event handler hasn't run yet).
	if resp.PrepareResult != nil && resp.PrepareResult.Success {
		if session.Metadata == nil {
			session.Metadata = make(map[string]interface{})
		}
		session.Metadata["prepare_result"] = buildPrepareResultMetadata(resp.PrepareResult)
	}

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update agent session after launch",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// buildPrepareResultMetadata serializes a prepare result for storage in session metadata.
// Uses lifecycle.SerializePrepareResult which is shared with the event handler.
func buildPrepareResultMetadata(result *lifecycle.EnvPrepareResult) map[string]interface{} {
	return lifecycle.SerializePrepareResult(result)
}

func (e *Executor) persistWorktreeAssociation(ctx context.Context, taskID string, session *models.TaskSession, repositoryID string, resp *LaunchAgentResponse) {
	// Multi-repo path: persist one TaskSessionWorktree row per per-repo worktree
	// returned by the preparer. Each row carries its own RepositoryID so
	// downstream lookups can scope by repo.
	if len(resp.Worktrees) > 0 {
		existingIDs := make(map[string]bool, len(session.Worktrees))
		for _, wt := range session.Worktrees {
			existingIDs[wt.WorktreeID] = true
		}
		for i, w := range resp.Worktrees {
			if w.WorktreeID == "" || existingIDs[w.WorktreeID] {
				continue
			}
			row := &models.TaskSessionWorktree{
				SessionID:      session.ID,
				WorktreeID:     w.WorktreeID,
				RepositoryID:   w.RepositoryID,
				Position:       i,
				WorktreePath:   w.WorktreePath,
				WorktreeBranch: w.WorktreeBranch,
			}
			if err := e.repo.CreateTaskSessionWorktree(ctx, row); err != nil {
				e.logger.Error("failed to persist session worktree association",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.String("worktree_id", w.WorktreeID),
					zap.String("repository_id", w.RepositoryID),
					zap.Error(err))
			}
		}
		return
	}

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
		e.logger.Error("failed to persist session worktree association",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
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

	// Force-cleanup any stale in-memory execution / agentctl state for terminal-state
	// sessions. Their agent process is dead by definition, so "already running" signals
	// from the execution store or agentctl's "starting" status are stale and would
	// otherwise block the relaunch.
	if isTerminalSessionState(session.State) {
		if cleanupErr := e.agentManager.CleanupStaleExecutionBySessionID(ctx, session.ID); cleanupErr != nil {
			e.logger.Warn("failed to force-cleanup stale execution before terminal-state resume",
				zap.String("session_id", session.ID),
				zap.Error(cleanupErr))
		}
	}

	req.Env = e.applyPreferredShellEnv(ctx, req.ExecutorType, req.Env)

	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil && isAgentAlreadyRunningError(err) {
		// "already has an agent running" fires both for live executions (a concurrent
		// resume raced us) and stale ones (agent never started or exited without
		// cleanup). Probe liveness before deciding what to do — otherwise we'd kill a
		// healthy agent mid-prompt. For terminal states the agent is dead by definition,
		// so skip the probe and go straight to cleanup+retry — this avoids a silent
		// regression to ErrExecutionAlreadyRunning if the preemptive cleanup above
		// failed and agentctl still reports a stale "starting" status.
		if !isTerminalSessionState(session.State) && e.agentManager.IsAgentRunningForSession(ctx, session.ID) {
			e.logger.Info("resume race: agent already running for session, returning ErrExecutionAlreadyRunning",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID))
			return nil, ErrExecutionAlreadyRunning
		}
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
	e.persistWorktreeAssociation(ctx, task.ID, session, repositoryID, resp)

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
	if taskModel.ArchivedAt != nil {
		unlock()
		return nil, func() {}, ErrTaskArchived
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

	// Re-read session state after acquiring the lock. The caller fetched the
	// session before the lock, so on concurrent resumes the state may be stale —
	// the first request could have already transitioned FAILED → STARTING, and
	// a stale FAILED state here would wrongly make isTerminalSessionState bypass
	// the live-execution guard and cleanup the live agent the first request just
	// registered, launching a duplicate. If the re-read fails, abort rather than
	// proceeding with uncertain state — silently falling back to the stale state
	// would reintroduce the exact race this re-read prevents.
	fresh, fetchErr := e.repo.GetTaskSession(ctx, session.ID)
	if fetchErr != nil {
		unlock()
		e.logger.Warn("failed to re-read session state inside lock; aborting resume to avoid duplicate agent",
			zap.String("session_id", session.ID),
			zap.Error(fetchErr))
		return nil, func() {}, fetchErr
	}
	if fresh != nil {
		session.State = fresh.State
	}

	// Skip the "already running" rejection for terminal-state sessions — the agent
	// process is dead by definition, and ResumeSession will force-cleanup stale
	// state before the relaunch.
	if !isTerminalSessionState(session.State) {
		if existing, ok := e.GetExecutionBySession(session.ID); ok && existing != nil {
			unlock()
			return nil, func() {}, ErrExecutionAlreadyRunning
		}
	}

	return task, unlock, nil
}

// buildResumeRequest constructs the LaunchAgentRequest for a session resume, resolving executor config,
// repository details, worktree settings, and ACP resume token.
// Returns the request, repository ID, executor config, existing ExecutorRunning record (may be nil), and error.
func (e *Executor) buildResumeRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, startAgent bool) (*LaunchAgentRequest, string, executorConfig, *models.ExecutorRunning, error) {
	req := &LaunchAgentRequest{
		TaskID:            task.ID,
		SessionID:         session.ID,
		TaskTitle:         task.Title,
		AgentProfileID:    session.AgentProfileID,
		TaskDescription:   task.Description,
		Priority:          task.Priority,
		IsEphemeral:       task.IsEphemeral,
		TaskEnvironmentID: session.TaskEnvironmentID,
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

	// Activate config-mode MCP tools when config_mode is set in session metadata.
	if isConfigModeSession(session) {
		req.McpMode = McpModeConfig
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

	if running.AgentExecutionID != "" {
		req.PreviousExecutionID = running.AgentExecutionID
	}

	// Carry forward only persistent metadata from the previous run.
	// Keys not in lifecycle.ShouldPersistMetadataKey() are launch-time-only
	// and are intentionally NOT carried forward (e.g., task_description).
	if running.Metadata != nil {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		for k, v := range running.Metadata {
			if _, exists := req.Metadata[k]; !exists && lifecycle.ShouldPersistMetadataKey(k) {
				req.Metadata[k] = v
			}
		}
	}

	if running.ResumeToken != "" && startAgent {
		req.ACPSessionID = running.ResumeToken
		// Clear TaskDescription so the agent doesn't receive an automatic prompt on resume.
		// The session context is restored via ACP session/load; sending a prompt here would
		// cause the agent to start working immediately instead of waiting for user input.
		req.TaskDescription = ""
		e.logger.Info("found resume token for session resumption",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Bool("has_resume_token", running.ResumeToken != ""))
	} else if startAgent && session.State == models.TaskSessionStateWaitingForInput {
		// Fresh-start resume (no resume token): don't auto-prompt with the task description.
		req.TaskDescription = ""
		e.logger.Info("fresh-start resume, clearing task description to avoid auto-prompt",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID))
	}

	return running
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

	if e.capabilities != nil && e.capabilities.RequiresCloneURL(req.ExecutorType) {
		cloneURL := repositoryCloneURL(repository)
		if cloneURL == "" {
			return "", ErrNoCloneURL
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
		// Carry forward CheckoutBranch from task repository (e.g. PR head branch)
		primaryTaskRepo, _ := e.repo.GetPrimaryTaskRepository(ctx, task.ID)
		if primaryTaskRepo != nil && primaryTaskRepo.CheckoutBranch != "" {
			req.CheckoutBranch = primaryTaskRepo.CheckoutBranch
		}
		req.WorktreeBranchPrefix = repository.WorktreeBranchPrefix
		req.PullBeforeWorktree = repository.PullBeforeWorktree
	}

	// Multi-repo: when the task has more than one repository, populate
	// req.Repositories so the lifecycle preparer can resume/recreate each
	// repo's worktree. The legacy top-level fields above stay populated
	// from the primary for backwards compat.
	if len(task.Repositories) > 1 {
		allRepos, allErr := e.resolveAllRepoInfo(ctx, task.ID)
		if allErr != nil {
			return repositoryID, allErr
		}
		if len(allRepos) > 1 {
			req.Repositories = buildRepoSpecs(allRepos)
			// Stamp the per-repo TaskDirName so the preparer reuses the same
			// task root that the original launch created.
			if env, _ := e.repo.GetTaskEnvironmentByTaskID(ctx, task.ID); env != nil && env.TaskDirName != "" {
				req.TaskDirName = env.TaskDirName
			}
		}
	}

	return repositoryID, nil
}

// persistResumeState updates the session row after a successful resume launch.
// Like persistLaunchState, executors_running is owned by the lifecycle manager
// and not touched here — see lifecycle.persistExecutorRunning. The orchestrator's
// only remaining responsibility is the session-row state machine (STARTING /
// CompletedAt-clear).
func (e *Executor) persistResumeState(ctx context.Context, taskID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool, execCfg executorConfig, existingRunning *models.ExecutorRunning) {
	_ = resp
	_ = execCfg
	_ = existingRunning

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
}

// startAgentProcessOnResume starts the agent process asynchronously after a session resume.
// Task state is managed by workflow triggers and stream handlers elsewhere; this callback
// just logs successful process start.
func (e *Executor) startAgentProcessOnResume(ctx context.Context, taskID string, session *models.TaskSession, agentExecutionID string) {
	e.runAgentProcessAsync(ctx, taskID, session.ID, agentExecutionID, func(updCtx context.Context) {
		e.logger.Debug("agent resumed successfully",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("session_state", string(session.State)))
	}, false, true)
}
