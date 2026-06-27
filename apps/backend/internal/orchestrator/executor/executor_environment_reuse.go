package executor

import (
	"context"
	"sort"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

// reuseExistingEnvironment carries forward worktree, container, sandbox, and
// runtime metadata from an existing TaskEnvironment into the launch request
// so that executor backends can reuse the prior execution.
//
// Reuse is gated on the env's executor_type matching the launch request: if
// the user switched the task's executor profile to a different type, we must
// NOT pass stale PreviousExecutionID / container_id / sprite_name to the
// wrong backend (it would either fail loudly or, worse, overwrite the
// persisted env with mixed resource IDs on the next save).
//
// Two layers of metadata feed in:
//   - env-level (stable IDs: worktree id, container id, sandbox id, branch)
//   - the latest matching ExecutorRunning row (live runtime metadata: agent
//     execution id, secret references, anything in persistentMetadataKeys)
//
// applyExecutorRunningMetadata overwrites container_id with running.ContainerID
// (running wins for live runtime values) but only adds keys that don't already
// exist for the rest (env wins for sprite_name and other stable IDs).
func (e *Executor) reuseExistingEnvironment(ctx context.Context, req *LaunchAgentRequest, env *models.TaskEnvironment) {
	if env == nil {
		return
	}
	if env.ExecutorType != "" && env.ExecutorType != req.ExecutorType {
		e.logger.Info("skipping task environment reuse: executor type changed",
			zap.String("task_id", req.TaskID),
			zap.String("env_executor_type", env.ExecutorType),
			zap.String("req_executor_type", req.ExecutorType))
		return
	}

	if env.TaskDirName != "" && req.UseWorktree {
		req.TaskDirName = env.TaskDirName
	}

	if req.UseWorktree {
		e.reuseExistingRepositoryWorktrees(ctx, req, env)
	}
	if req.WorktreeID == "" && env.WorktreeID != "" && req.UseWorktree && !hasBranchScopedEnvironmentWorktrees(env) {
		req.WorktreeID = env.WorktreeID
		e.logger.Info("reusing existing task environment worktree",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", env.WorktreeID))
	}

	if env.ContainerID != "" || env.SandboxID != "" {
		metadata := ensureLaunchMetadata(req)
		if env.ContainerID != "" {
			metadata[lifecycle.MetadataKeyContainerID] = env.ContainerID
		}
		if env.SandboxID != "" {
			metadata["sprite_name"] = env.SandboxID
		}
	}

	// Forward the persisted feature branch so the in-sandbox prepare script
	// can re-create or reuse it. Applies to every clone-based remote executor
	// (the preparer is responsible for stamping env.WorktreeBranch in the
	// first place); the host-side worktree path uses req.WorktreeID instead.
	if env.WorktreeBranch != "" && isContainerizedExecutor(req.ExecutorType) {
		ensureLaunchMetadata(req)[lifecycle.MetadataKeyWorktreeBranch] = env.WorktreeBranch
	}

	if env.ID == "" {
		return
	}
	if running := e.latestExecutorRunningForEnvironment(ctx, req.TaskID, env); running != nil {
		applyExecutorRunningMetadata(req, running)
	}
}

type repositoryWorktreeKey struct {
	repositoryID string
	branchSlug   string
}

func (e *Executor) reuseExistingRepositoryWorktrees(ctx context.Context, req *LaunchAgentRequest, env *models.TaskEnvironment) {
	if !req.UseWorktree {
		return
	}
	repoSpecs := req.Repositories
	usingTopLevelRepo := false
	if len(repoSpecs) == 0 {
		spec, ok := topLevelLaunchRepoSpec(req)
		if !ok {
			return
		}
		repoSpecs = []RepoSpec{spec}
		usingTopLevelRepo = true
	}

	if len(repoSpecs) == 0 {
		return
	}

	worktreeIDs := e.environmentRepoWorktreeIDs(req, env)
	for key, id := range e.latestSessionWorktreeIDsForEnvironment(ctx, req.TaskID, env.ID) {
		if _, exists := worktreeIDs[key]; !exists {
			worktreeIDs[key] = id
		}
	}
	if len(worktreeIDs) == 0 {
		return
	}

	allowLegacyEmptyBranchFallback := !hasBranchScopedEnvironmentWorktrees(env)
	for i := range repoSpecs {
		spec := &repoSpecs[i]
		key := repositoryWorktreeKey{
			repositoryID: spec.RepositoryID,
			branchSlug:   launchRepoBranchIdentitySlug(*spec),
		}
		if id := worktreeIDs[key]; id != "" {
			spec.WorktreeID = id
			continue
		}
		if spec.BranchSlug == "" && allowLegacyEmptyBranchFallback {
			legacyKey := repositoryWorktreeKey{
				repositoryID: spec.RepositoryID,
				branchSlug:   "",
			}
			if id := worktreeIDs[legacyKey]; id != "" {
				spec.WorktreeID = id
			}
		}
	}
	if repoSpecs[0].WorktreeID == "" {
		return
	}
	req.WorktreeID = repoSpecs[0].WorktreeID
	if !usingTopLevelRepo {
		req.Repositories = repoSpecs
	}
}

func topLevelLaunchRepoSpec(req *LaunchAgentRequest) (RepoSpec, bool) {
	if req.RepositoryID == "" {
		return RepoSpec{}, false
	}
	return RepoSpec{
		RepositoryID:       req.RepositoryID,
		BranchIdentitySlug: topLevelBranchIdentitySlug(req),
	}, true
}

func topLevelBranchIdentitySlug(req *LaunchAgentRequest) string {
	branch := req.CheckoutBranch
	if branch == "" {
		branch = req.BaseBranch
	}
	if branch == "" {
		branch = req.DefaultBranch
	}
	return worktree.SanitizeBranchSlug(branch)
}

func launchRepoBranchIdentitySlug(spec RepoSpec) string {
	if spec.BranchIdentitySlug != "" {
		return worktree.SanitizeBranchSlug(spec.BranchIdentitySlug)
	}
	return worktree.SanitizeBranchSlug(spec.BranchSlug)
}

func (e *Executor) environmentRepoWorktreeIDs(req *LaunchAgentRequest, env *models.TaskEnvironment) map[repositoryWorktreeKey]string {
	result := make(map[repositoryWorktreeKey]string)
	if env.WorktreeID != "" && !hasBranchScopedEnvironmentWorktrees(env) {
		repoID := env.RepositoryID
		if repoID == "" {
			repoID = req.RepositoryID
		}
		if repoID != "" {
			result[repositoryWorktreeKey{
				repositoryID: repoID,
				branchSlug:   "",
			}] = env.WorktreeID
		}
	}
	for _, repo := range env.Repos {
		if repo.RepositoryID == "" || repo.WorktreeID == "" {
			continue
		}
		result[repositoryWorktreeKey{
			repositoryID: repo.RepositoryID,
			branchSlug:   worktree.SanitizeBranchSlug(repo.BranchSlug),
		}] = repo.WorktreeID
	}
	return result
}

func hasBranchScopedEnvironmentWorktrees(env *models.TaskEnvironment) bool {
	for _, repo := range env.Repos {
		if repo.RepositoryID != "" && repo.WorktreeID != "" && worktree.SanitizeBranchSlug(repo.BranchSlug) != "" {
			return true
		}
	}
	return false
}

func (e *Executor) latestSessionWorktreeIDsForEnvironment(ctx context.Context, taskID, envID string) map[repositoryWorktreeKey]string {
	sessions, err := e.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list sessions for per-repo worktree reuse",
			zap.String("task_id", taskID),
			zap.String("task_environment_id", envID),
			zap.Error(err))
		return nil
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if !sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].StartedAt.After(sessions[j].StartedAt)
		}
		if !sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		}
		return sessions[i].ID > sessions[j].ID
	})
	for _, session := range sessions {
		if envID != "" && session.TaskEnvironmentID != "" && session.TaskEnvironmentID != envID {
			continue
		}
		worktrees, err := e.repo.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			e.logger.Warn("failed to list session worktrees for reuse",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			continue
		}
		result := sessionWorktreeIDsByKey(worktrees)
		if len(result) > 0 {
			return result
		}
	}
	return nil
}

func sessionWorktreeIDsByKey(worktrees []*models.TaskSessionWorktree) map[repositoryWorktreeKey]string {
	result := make(map[repositoryWorktreeKey]string, len(worktrees))
	for _, wt := range worktrees {
		if wt.RepositoryID == "" || wt.WorktreeID == "" {
			continue
		}
		key := repositoryWorktreeKey{
			repositoryID: wt.RepositoryID,
			branchSlug:   worktree.SanitizeBranchSlug(wt.BranchSlug),
		}
		result[key] = wt.WorktreeID
	}
	return result
}

func (e *Executor) latestExecutorRunningForEnvironment(ctx context.Context, taskID string, env *models.TaskEnvironment) *models.ExecutorRunning {
	sessions, err := e.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list sessions for task environment metadata reuse",
			zap.String("task_id", taskID),
			zap.String("task_environment_id", env.ID),
			zap.Error(err))
		return nil
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if !sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].StartedAt.After(sessions[j].StartedAt)
		}
		if !sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		}
		return sessions[i].ID > sessions[j].ID
	})

	var fallback *models.ExecutorRunning
	for _, s := range sessions {
		running, runErr := e.repo.GetExecutorRunningBySessionID(ctx, s.ID)
		if runErr != nil || running == nil {
			continue
		}
		if s.TaskEnvironmentID == env.ID {
			return running
		}
		if fallback == nil && executorRunningMatchesEnvironment(running, env) {
			fallback = running
		}
	}
	return fallback
}

func executorRunningMatchesEnvironment(running *models.ExecutorRunning, env *models.TaskEnvironment) bool {
	if running == nil || env == nil {
		return false
	}
	if env.ContainerID != "" && running.ContainerID == env.ContainerID {
		return true
	}
	if env.SandboxID != "" && running.Metadata != nil && running.Metadata["sprite_name"] == env.SandboxID {
		return true
	}
	return false
}

func applyExecutorRunningMetadata(req *LaunchAgentRequest, running *models.ExecutorRunning) {
	if running.AgentExecutionID != "" && req.PreviousExecutionID == "" {
		req.PreviousExecutionID = running.AgentExecutionID
	}
	var metadata map[string]interface{}
	if running.ContainerID != "" {
		metadata = ensureLaunchMetadata(req)
		metadata[lifecycle.MetadataKeyContainerID] = running.ContainerID
	}
	for k, v := range running.Metadata {
		if metadata == nil {
			metadata = ensureLaunchMetadata(req)
		}
		if _, exists := metadata[k]; exists {
			continue
		}
		if !lifecycle.ShouldPersistMetadataKey(k) {
			continue
		}
		// Skip session-scoped runtime resources (PIDs, ports, session dirs).
		// Carrying these across sibling sessions on the same task makes a
		// fresh launch look like a same-session resume — see SSH executor's
		// ResumeRemoteInstance — and the new session ends up sharing the
		// previous session's agentctl process.
		if lifecycle.IsSessionScopedMetadataKey(k) {
			continue
		}
		metadata[k] = v
	}
}

func ensureLaunchMetadata(req *LaunchAgentRequest) map[string]interface{} {
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	return req.Metadata
}
