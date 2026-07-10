package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
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
	return err != nil && errors.Is(err, lifecycle.ErrAgentAlreadyRunning)
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
	RepositoryID           string
	RepositoryPath         string
	BaseBranch             string
	CheckoutBranch         string
	PRNumber               int // GitHub PR number when CheckoutBranch is a PR head; sourced from task_repositories.metadata["pr_number"].
	Position               int
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	PullBeforeWorktree     bool
	Repository             *models.Repository
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
		PRNumber:       prNumberFromMetadata(tr.Metadata),
		Position:       tr.Position,
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

	// Backfill default_branch from the local clone when missing. This fires for
	// two cases: (1) a freshly cloned provider-backed repo whose row was created
	// without an upstream-derived value (e.g. the MCP create_task path that
	// takes a bare github URL), and (2) an already-cloned row that escaped a
	// prior backfill (e.g. a launch that ran before this code existed). Without
	// this, the BaseBranch fallback below stays empty and surfaces to the user
	// as "base branch does not exist" from worktree.Manager.Create.
	e.backfillRepoDefaultBranch(ctx, repo, repo.LocalPath)

	info.Repository = repo
	info.RepositoryPath = repo.LocalPath
	info.WorktreeBranchPrefix = repo.WorktreeBranchPrefix
	info.WorktreeBranchTemplate = repo.WorktreeBranchTemplate
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

	// Note: default_branch backfill is intentionally driven from
	// resolveTaskRepoInfo (the caller), not here. That way it also runs for
	// rows whose local_path was already populated by a prior launch but whose
	// default_branch was never persisted (e.g. rows created before the
	// backfill existed).

	return localPath, nil
}

// backfillRepoDefaultBranch populates repo.DefaultBranch from the local clone
// (in memory + DB) when it's empty. Best-effort: on any failure we log and
// continue, since the launch still has the legacy worktree-manager fallback
// to fall back on if it can find a branch by another route.
func (e *Executor) backfillRepoDefaultBranch(ctx context.Context, repo *models.Repository, localPath string) {
	if repo.DefaultBranch != "" || localPath == "" {
		return
	}
	detected, err := gitref.DefaultBranchOrEmpty(localPath)
	if err != nil || detected == "" {
		e.logger.Debug("could not detect default branch from clone; leaving empty",
			zap.String("repository_id", repo.ID),
			zap.String("local_path", localPath),
			zap.Error(err))
		return
	}
	repo.DefaultBranch = detected
	if e.repoUpdater == nil {
		return
	}
	if updateErr := e.repoUpdater.UpdateRepositoryDefaultBranch(ctx, repo.ID, detected); updateErr != nil {
		e.logger.Warn("failed to persist detected default branch after clone",
			zap.String("repository_id", repo.ID),
			zap.String("default_branch", detected),
			zap.Error(updateErr))
	}
}

// persistLaunchState updates the session record after a successful agent launch.
// The executors_running row is now written by the lifecycle manager itself in
// lockstep with executionStore.Add (see lifecycle.persistExecutorRunning) — this
// function no longer touches it. The lifecycle manager also owns the columns
// that used to live on task_sessions (agent_execution_id, container_id); the
// orchestrator stops writing them so the only remaining source of truth is
// executors_running.
//
// What remains here: state transitions (e.g., STARTING) and prepare-result
// metadata merge, both of which are session-row concerns the lifecycle manager
// doesn't know about.
func (e *Executor) persistLaunchState(ctx context.Context, taskID, sessionID string, session *models.TaskSession, resp *LaunchAgentResponse, startAgent bool, now time.Time) error {
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

	var updateErr error
	if startAgent {
		updateErr = e.updateSessionStarting(ctx, taskID, session, true)
	} else {
		updateErr = e.repo.UpdateTaskSession(ctx, session)
	}
	if updateErr != nil {
		e.logger.Error("failed to update agent session after launch",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
		return updateErr
	}
	return nil
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
				BranchSlug:     w.BranchSlug,
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

	req, repositoryID, execCfg, existingEnv, _, err := e.buildResumeRequest(ctx, task, session, startAgent)
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

	if err := e.persistResumeState(ctx, task.ID, session, startAgent); err != nil {
		e.stopUnstartedExecution(ctx, session.ID, resp.AgentExecutionID)
		return nil, err
	}
	e.persistWorktreeAssociation(ctx, task.ID, session, repositoryID, resp)
	// Refresh task_environments after a successful resume so the row reflects
	// the new worktree, container, and ready status. Without this, sessions
	// that failed on initial launch (empty task_dir_name / worktree_path,
	// status=stopped) stay stuck on those stale values even though the resume
	// just prepared a fresh worktree — the frontend keeps showing
	// "Executor environment is unavailable (stopped)" and disables chat input.
	// existingEnv is captured from buildResumeRequest above (resolveResumeTaskEnvironment
	// already aborts on real DB errors); when nil, persistTaskEnvironment will
	// re-fetch under the per-task lock and create a fresh row if needed.
	e.persistTaskEnvironment(ctx, task.ID, session, existingEnv, req, resp, execCfg)

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
func (e *Executor) buildResumeRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, startAgent bool) (*LaunchAgentRequest, string, executorConfig, *models.TaskEnvironment, *models.ExecutorRunning, error) {
	req := &LaunchAgentRequest{
		TaskID:            task.ID,
		SessionID:         session.ID,
		TaskTitle:         task.Title,
		AgentProfileID:    session.AgentProfileID,
		TaskDescription:   task.Description,
		Priority:          task.Priority,
		IsEphemeral:       task.IsEphemeral,
		IsPassthrough:     session.IsPassthrough,
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
	req.WorktreeBranchTicket = worktree.TicketForBranchName(task.Identifier, metadata)

	execConfig := e.applyExecutorConfigToResumeRequest(ctx, req, task, session, metadata)

	existingEnv, err := e.resolveResumeTaskEnvironment(ctx, task.ID, session)
	if err != nil {
		return nil, "", execConfig, nil, nil, err
	}
	if session.TaskEnvironmentID != "" {
		req.TaskEnvironmentID = session.TaskEnvironmentID
	}

	repositoryID, err := e.applyResumeRepoConfig(ctx, task, session, req, existingEnv)
	if err != nil {
		return nil, "", execConfig, nil, nil, err
	}

	e.reuseExistingEnvironment(ctx, req, existingEnv)

	// Activate config-mode MCP tools when config_mode is set in session metadata.
	if isConfigModeSession(session) {
		req.McpMode = McpModeConfig
	}

	existingRunning := e.applyRunningRecordToResumeRequest(ctx, req, task, session, startAgent)

	return req, repositoryID, execConfig, existingEnv, existingRunning, nil
}

func (e *Executor) resolveResumeTaskEnvironment(ctx context.Context, taskID string, session *models.TaskSession) (*models.TaskEnvironment, error) {
	env, err := e.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("lookup existing task environment: %w", err)
	}
	if env == nil {
		return nil, nil
	}
	if session.TaskEnvironmentID != env.ID {
		session.TaskEnvironmentID = env.ID
	}
	return env, nil
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
// Returns the resolved repositoryID. existingEnv is the task_environments row
// resolved upstream in buildResumeRequest; it is passed in so we can derive
// TaskDirName without an extra DB round-trip and so both the single-repo and
// multi-repo branches agree on the same fallback name (the fallback uses a
// random suffix, so two independent calls would produce different names).
func (e *Executor) applyResumeRepoConfig(ctx context.Context, task *v1.Task, session *models.TaskSession, req *LaunchAgentRequest, existingEnv *models.TaskEnvironment) (string, error) {
	repositoryID, baseBranch := resolveResumeRepoIDAndBranch(task, session)
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
	applyResumeRepoBasics(req, repository, repositoryPath)

	if err := e.applyResumeCloneURL(req, repository, baseBranch); err != nil {
		return "", err
	}

	if shouldUseWorktree(req.ExecutorType) && repositoryPath != "" {
		e.applyResumeWorktreeConfig(ctx, task, req, repository, repositoryID, repositoryPath, baseBranch, existingEnv)
	}

	if err := e.applyResumeMultiRepoConfig(ctx, task, req, existingEnv); err != nil {
		return repositoryID, err
	}

	return repositoryID, nil
}

// resolveResumeRepoIDAndBranch picks the primary repositoryID and baseBranch
// for a resume, preferring the session's persisted values and falling back to
// the task's primary repository when the session row was created before those
// fields existed.
func resolveResumeRepoIDAndBranch(task *v1.Task, session *models.TaskSession) (string, string) {
	repositoryID := session.RepositoryID
	if repositoryID == "" && len(task.Repositories) > 0 {
		repositoryID = task.Repositories[0].RepositoryID
	}
	baseBranch := session.BaseBranch
	if baseBranch == "" && len(task.Repositories) > 0 && task.Repositories[0].BaseBranch != "" {
		baseBranch = task.Repositories[0].BaseBranch
	}
	return repositoryID, baseBranch
}

// applyResumeRepoBasics copies the repository's local path and setup script
// onto the request. Pulled out of applyResumeRepoConfig so the parent's
// cyclomatic complexity stays inside the lint budget.
func applyResumeRepoBasics(req *LaunchAgentRequest, repository *models.Repository, repositoryPath string) {
	if repositoryPath != "" {
		req.RepositoryURL = repositoryPath
	}
	if repository.SetupScript != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata[lifecycle.MetadataKeyRepoSetupScript] = repository.SetupScript
	}
}

// applyResumeCloneURL handles clone-based remote executors: it stamps the
// clone URL on the request and propagates BaseBranch into launch metadata so
// the prepare script's `git clone --branch <X>` resolves. Local executors skip
// this path so LocalPreparer doesn't clobber the "use current state" UX.
func (e *Executor) applyResumeCloneURL(req *LaunchAgentRequest, repository *models.Repository, baseBranch string) error {
	if e.capabilities == nil || !e.capabilities.RequiresCloneURL(req.ExecutorType) {
		return nil
	}
	cloneURL := repositoryCloneURL(repository)
	if cloneURL == "" {
		return ErrNoCloneURL
	}
	req.RepositoryURL = cloneURL
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata["repository_clone_url"] = cloneURL
	if baseBranch != "" {
		req.BaseBranch = baseBranch
	}
	return nil
}

// applyResumeMultiRepoConfig populates req.Repositories for multi-repo tasks
// so the lifecycle preparer can resume/recreate each repo's worktree. The
// legacy top-level fields stay populated from the primary for backwards
// compat.
func (e *Executor) applyResumeMultiRepoConfig(ctx context.Context, task *v1.Task, req *LaunchAgentRequest, existingEnv *models.TaskEnvironment) error {
	if len(task.Repositories) <= 1 {
		return nil
	}
	allRepos, err := e.resolveAllRepoInfo(ctx, task.ID)
	if err != nil {
		return err
	}
	if len(allRepos) > 1 {
		req.Repositories = buildRepoSpecs(allRepos)
		for i := range req.Repositories {
			req.Repositories[i].WorktreeBranchTicket = req.WorktreeBranchTicket
		}
		req.TaskDirName = resolveResumeTaskDirName(existingEnv, task)
	}
	return nil
}

// applyResumeWorktreeConfig stamps the worktree-related fields on req for a
// single-repo worktree resume. Extracted from applyResumeRepoConfig to keep
// cognitive complexity within the lint budget.
func (e *Executor) applyResumeWorktreeConfig(
	ctx context.Context,
	task *v1.Task,
	req *LaunchAgentRequest,
	repository *models.Repository,
	repositoryID, repositoryPath, baseBranch string,
	existingEnv *models.TaskEnvironment,
) {
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
		req.PRNumber = prNumberFromMetadata(primaryTaskRepo.Metadata)
	}
	req.WorktreeBranchPrefix = repository.WorktreeBranchPrefix
	req.WorktreeBranchTemplate = repository.WorktreeBranchTemplate
	req.PullBeforeWorktree = repository.PullBeforeWorktree
	// Worktree manager requires TaskDirName and RepoName. Mirror the
	// initial-launch path (applyRepositoryConfig) so resumes of single-repo
	// tasks don't fail with ErrTaskDirRequired. Prefer a persisted
	// TaskDirName so we reuse the same on-disk task root; fall back to a
	// freshly generated one when the original launch failed before the
	// environment was stamped.
	if repository.Name != "" {
		req.RepoName = repository.Name
	}
	req.TaskDirName = resolveResumeTaskDirName(existingEnv, task)
}

// resolveResumeTaskDirName returns the per-task directory name to use when
// resuming. It prefers the value persisted on task_environments (so we reuse
// the same ~/.kandev/tasks/{name}/ root the initial launch created) and falls
// back to a fresh semantic name when the original launch failed before any
// environment was stamped. That fallback is what lets a previously failed
// session recover instead of looping on ErrTaskDirRequired.
func resolveResumeTaskDirName(existingEnv *models.TaskEnvironment, task *v1.Task) string {
	if existingEnv != nil && existingEnv.TaskDirName != "" {
		return existingEnv.TaskDirName
	}
	return worktree.SemanticWorktreeName(task.Title, worktree.SmallSuffix(3))
}

// persistResumeState updates the session row after a successful resume launch.
// Like persistLaunchState, executors_running is owned by the lifecycle manager
// and not touched here — see lifecycle.persistExecutorRunning. The
// orchestrator's only remaining responsibility is the session-row state
// machine (STARTING / CompletedAt-clear).
func (e *Executor) persistResumeState(ctx context.Context, taskID string, session *models.TaskSession, startAgent bool) error {
	session.ErrorMessage = ""
	if startAgent {
		session.State = models.TaskSessionStateStarting
		session.CompletedAt = nil
	}

	var updateErr error
	if startAgent {
		updateErr = e.updateSessionStarting(ctx, taskID, session, false)
	} else {
		updateErr = e.repo.UpdateTaskSession(ctx, session)
	}
	if updateErr != nil {
		e.logger.Error("failed to update task session for resume",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(updateErr))
		return updateErr
	}
	return nil
}

// prNumberFromMetadata extracts a GitHub PR number from a task_repository's
// metadata bag. Stored as JSON, so on retrieval the value can decode as either
// float64 (default for json.Unmarshal into interface{}) or int. Returns 0 when
// the key is absent, malformed, or non-positive.
func prNumberFromMetadata(metadata map[string]interface{}) int {
	if metadata == nil {
		return 0
	}
	raw, ok := metadata["pr_number"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	}
	return 0
}

// startAgentProcessOnResume starts the agent process asynchronously after a session resume.
// Task state is managed by workflow triggers and stream handlers elsewhere; this callback
// just logs successful process start.
func (e *Executor) startAgentProcessOnResume(ctx context.Context, taskID string, session *models.TaskSession, agentExecutionID string) {
	e.runAgentProcessAsync(ctx, taskID, session.ID, agentExecutionID, func(updCtx context.Context) {
		if updateErr := e.writeTaskInProgressForRuntime(updCtx, taskID, session.ID); updateErr != nil {
			e.logger.Warn("failed to update task state to IN_PROGRESS after resume start",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(updateErr))
		}
		e.logger.Debug("agent resumed successfully",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("session_state", string(session.State)))
	}, false, true)
}

func (e *Executor) writeTaskInProgressForRuntime(ctx context.Context, taskID, sessionID string) error {
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task != nil && task.ArchivedAt != nil {
		e.logger.Debug("skipping IN_PROGRESS transition for archived task",
			zap.String("task_id", taskID))
		return nil
	}
	if task != nil && task.AssigneeAgentProfileID != "" {
		e.logger.Debug("skipping IN_PROGRESS transition for office task",
			zap.String("task_id", taskID))
		return nil
	}
	if sessionID != "" {
		session, err := e.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if session == nil || !isRuntimeWorkingSessionState(session.State) {
			state := ""
			if session != nil {
				state = string(session.State)
			}
			e.logger.Debug("skipping IN_PROGRESS transition because resumed session is no longer active",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("session_state", state))
			return nil
		}
	}
	return e.updateTaskState(ctx, taskID, v1.TaskStateInProgress)
}
