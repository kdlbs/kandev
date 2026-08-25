package executor

import (
	"context"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

const (
	taskEnvironmentRepoStatusFailed  = "failed"
	taskEnvironmentRepoStatusDeleted = "deleted"
)

// validateReuseEnvironmentInventory proves that every repository/branch slot
// selected for this launch has one active canonical environment row before any
// lifecycle preparer can touch the workspace. It intentionally consults the
// durable environment inventory rather than a sibling session projection.
func (e *Executor) validateReuseEnvironmentInventory(ctx context.Context, req *LaunchAgentRequest, env *models.TaskEnvironment) error {
	if !req.WorkspaceReuseRequired || env == nil || env.ID == "" || e.repo == nil {
		return nil
	}
	specs := req.Repositories
	if len(specs) == 0 {
		spec, ok := topLevelLaunchRepoSpec(req)
		if !ok {
			return nil
		}
		specs = []RepoSpec{spec}
	}
	rows, err := e.repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		return fmt.Errorf("%w: load canonical workspace inventory", models.ErrWorkspaceReuseUnsafe)
	}
	// Hydrate the launch-local environment once with the authoritative rows.
	// Reuse setup below consumes this same inventory, avoiding a second query
	// and keeping cancellation attached to the caller's context.
	env.Repos = rows
	// Zero recorded rows means the canonical inventory was never captured at
	// all (for example: a launch whose prepare step failed before writing
	// any repo rows). That is recoverable — letting this launch through lets
	// reuseExistingRepositoryWorktrees fall through its own empty-inventory
	// check and rebuild fresh worktree/repo specs. A non-empty but wrong
	// inventory below is the guard's actual purpose and must still refuse.
	if len(rows) == 0 {
		req.WorkspaceReuseRequired = workspaceReuseAllowed(
			env,
			req.ExecutorType,
			req.WorkspaceReuseRequired,
			e.taskIsRepoBacked(ctx, req.TaskID),
		)
		return nil
	}
	for _, spec := range specs {
		if canonicalInventoryMatches(spec, rows, req.UseWorktree) != 1 {
			return fmt.Errorf("%w: canonical workspace repository inventory has no matching entry for repository %q branch %q",
				models.ErrWorkspaceReuseUnsafe, spec.RepositoryID, launchRepoBranchIdentitySlug(spec))
		}
	}
	return nil
}

func canonicalInventoryMatches(spec RepoSpec, rows []*models.TaskEnvironmentRepo, useWorktree bool) int {
	matches := 0
	expectedBranchSlug := launchRepoBranchIdentitySlug(spec)
	allowLegacyEmptyBranch := expectedBranchSlug != "" && !hasBranchScopedEnvironmentRepoRows(rows)
	for _, row := range rows {
		branchMatches := worktree.SanitizeBranchSlug(row.BranchSlug) == expectedBranchSlug
		if allowLegacyEmptyBranch && row.BranchSlug == "" {
			branchMatches = true
		}
		if row.RepositoryID != spec.RepositoryID || !branchMatches {
			continue
		}
		if row.DeletedAt != nil || row.Status == taskEnvironmentRepoStatusFailed || row.Status == taskEnvironmentRepoStatusDeleted || (useWorktree && row.WorktreeID == "") {
			continue
		}
		matches++
	}
	return matches
}

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
	// A repo-backed environment with no canonical rows is being freshly
	// materialized. Do not forward any environment or sibling runtime handle:
	// those handles could reconnect to the incomplete workspace, and
	// PreviousExecutionID would route the new session through the resume path.
	if !req.WorkspaceReuseRequired && e.taskIsRepoBacked(ctx, req.TaskID) && len(env.Repos) == 0 {
		return
	}

	if env.TaskDirName != "" && req.UseWorktree {
		req.TaskDirName = env.TaskDirName
	}
	// SSH uses the remote task directory as an environment-scoped attachment
	// handle. It is distinct from the per-session agentctl directory and must
	// therefore be forwarded for a sibling session without adopting its
	// runtime. Only forward it when reuse is actually required: a launch that
	// fell through to full materialization (see workspaceReuseAllowed) must
	// not see the old, possibly incomplete or stale, workspace path.
	if req.ExecutorType == string(models.ExecutorTypeSSH) && env.WorkspacePath != "" && req.WorkspaceReuseRequired {
		ensureLaunchMetadata(req)[lifecycle.MetadataKeySSHRemoteTaskDir] = env.WorkspacePath
	}

	if req.UseWorktree {
		e.reuseExistingRepositoryWorktrees(ctx, req, env)
	}

	if env.ContainerID != "" || env.SandboxID != "" {
		metadata := ensureLaunchMetadata(req)
		if env.ContainerID != "" {
			metadata[lifecycle.MetadataKeyContainerID] = env.ContainerID
		}
		// The bootstrap nonce is an environment control-plane capability, not a
		// session runtime credential. It lets a new session authenticate to the
		// canonical container's agentctl and create its own instance; never copy
		// a sibling's auth token, execution ID, port, or session directory.
		if env.ContainerBootstrapNonceSecretID != "" {
			metadata[lifecycle.MetadataKeyBootstrapNonceSecret] = env.ContainerBootstrapNonceSecretID
		}
		if env.ContainerControlAuthTokenSecretID != "" {
			metadata[lifecycle.MetadataKeyContainerControlAuthSecret] = env.ContainerControlAuthTokenSecretID
		}
		if env.SandboxID != "" {
			metadata["sprite_name"] = env.SandboxID
		}
	}

	// Forward the persisted feature branch so the in-sandbox prepare script
	// can re-create or reuse it. Applies to every clone-based remote executor
	// (the preparer is responsible for stamping the environment repository
	// row's worktree_branch in the first place); the host-side worktree path
	// uses req.WorktreeID instead.
	if branch := environmentWorktreeBranch(env); branch != "" && isContainerizedExecutor(req.ExecutorType) {
		ensureLaunchMetadata(req)[lifecycle.MetadataKeyWorktreeBranch] = branch
	}

	if env.ID == "" {
		return
	}
	if !req.WorkspaceReuseRequired {
		if running := e.latestExecutorRunningForEnvironment(ctx, req.TaskID, env); running != nil {
			applyExecutorRunningMetadata(req, running)
		}
	}
}

func extractContainerBootstrapNonceSecretID(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	secretID, _ := metadata[lifecycle.MetadataKeyBootstrapNonceSecret].(string)
	return secretID
}

func extractContainerControlAuthTokenSecretID(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	secretID, _ := metadata[lifecycle.MetadataKeyContainerControlAuthSecret].(string)
	return secretID
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

	envWorktreeIDs := environmentRepoWorktreeIDs(env)
	var sessionWorktreeIDs map[repositoryWorktreeKey]string
	if !req.WorkspaceReuseRequired {
		sessionWorktreeIDs = e.latestSessionWorktreeIDsForEnvironment(ctx, req.TaskID, env.ID)
	}
	if len(envWorktreeIDs) == 0 && len(sessionWorktreeIDs) == 0 {
		return
	}
	hasScopedWorktrees := hasBranchScopedEnvironmentWorktrees(env)
	allowLegacyEmptyBranchFallback := !hasScopedWorktrees
	for i := range repoSpecs {
		spec := &repoSpecs[i]
		if id := reusableWorktreeIDForSpec(*spec, envWorktreeIDs, sessionWorktreeIDs, allowLegacyEmptyBranchFallback); id != "" {
			spec.WorktreeID = id
		}
	}
	if repoSpecs[0].WorktreeID == "" {
		routeUnmatchedTopLevelRepoToScopedPath(req, repoSpecs[0], usingTopLevelRepo, hasScopedWorktrees)
		return
	}
	req.WorktreeID = repoSpecs[0].WorktreeID
	if usingTopLevelRepo {
		routeMatchedTopLevelRepoToScopedIdentity(req, repoSpecs[0], hasScopedWorktrees)
	} else {
		req.Repositories = repoSpecs
	}
}

func reusableWorktreeIDForSpec(
	spec RepoSpec,
	envWorktreeIDs map[repositoryWorktreeKey]string,
	sessionWorktreeIDs map[repositoryWorktreeKey]string,
	allowLegacyEmptyBranchFallback bool,
) string {
	branchIdentity := launchRepoBranchIdentitySlug(spec)
	key := repositoryWorktreeKey{
		repositoryID: spec.RepositoryID,
		branchSlug:   branchIdentity,
	}
	if id := envWorktreeIDs[key]; id != "" {
		return id
	}
	if id := sessionWorktreeIDs[key]; id != "" {
		return id
	}
	if spec.BranchSlug != "" || !allowLegacyEmptyBranchFallback {
		return ""
	}
	legacyKey := repositoryWorktreeKey{
		repositoryID: spec.RepositoryID,
		branchSlug:   "",
	}
	if id := envWorktreeIDs[legacyKey]; id != "" {
		return id
	}
	if branchIdentity == "" {
		if id := sessionWorktreeIDs[legacyKey]; id != "" {
			return id
		}
	}
	return ""
}

func routeUnmatchedTopLevelRepoToScopedPath(
	req *LaunchAgentRequest,
	spec RepoSpec,
	usingTopLevelRepo bool,
	hasScopedWorktrees bool,
) {
	if !usingTopLevelRepo || !hasScopedWorktrees {
		return
	}
	pathSlug := launchRepoBranchIdentitySlug(spec)
	if pathSlug == "" {
		return
	}
	spec.BranchIdentitySlug = pathSlug
	spec.BranchSlug = pathSlug
	req.BranchIdentitySlug = pathSlug
	req.BranchSlug = pathSlug
	req.Repositories = []RepoSpec{spec}
}

func routeMatchedTopLevelRepoToScopedIdentity(req *LaunchAgentRequest, spec RepoSpec, hasScopedWorktrees bool) {
	if !hasScopedWorktrees {
		return
	}
	identitySlug := launchRepoBranchIdentitySlug(spec)
	if identitySlug == "" {
		return
	}
	spec.BranchIdentitySlug = identitySlug
	req.BranchIdentitySlug = identitySlug
	req.Repositories = []RepoSpec{spec}
}

func topLevelLaunchRepoSpec(req *LaunchAgentRequest) (RepoSpec, bool) {
	if req.RepositoryID == "" {
		return RepoSpec{}, false
	}
	return RepoSpec{
		TaskRepositoryID:       req.TaskRepositoryID,
		RepositoryID:           req.RepositoryID,
		RepositoryPath:         req.RepositoryPath,
		RepositoryURL:          req.RepositoryURL,
		RepoName:               req.RepoName,
		BaseBranch:             req.BaseBranch,
		DefaultBranch:          req.DefaultBranch,
		CheckoutBranch:         req.CheckoutBranch,
		PRNumber:               req.PRNumber,
		WorktreeID:             req.WorktreeID,
		WorktreeBranchPrefix:   req.WorktreeBranchPrefix,
		WorktreeBranchTemplate: req.WorktreeBranchTemplate,
		WorktreeBranchTicket:   req.WorktreeBranchTicket,
		PullBeforeWorktree:     req.PullBeforeWorktree,
		RemoteSyncHandled:      req.RemoteSyncHandled,
		RefreshRepository:      req.RefreshRepository,
		CopyFiles:              req.CopyFiles,
		BranchIdentitySlug:     topLevelBranchIdentitySlug(req),
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

// environmentWorktreeBranch returns the first non-empty worktree branch
// recorded on the environment's repository rows.
func environmentWorktreeBranch(env *models.TaskEnvironment) string {
	for _, repo := range env.Repos {
		if repo != nil && repo.WorktreeBranch != "" {
			return repo.WorktreeBranch
		}
	}
	return ""
}

func environmentRepoWorktreeIDs(env *models.TaskEnvironment) map[repositoryWorktreeKey]string {
	result := make(map[repositoryWorktreeKey]string)
	repos := env.Repos
	for _, repo := range repos {
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
	if env == nil {
		return false
	}
	return hasBranchScopedEnvironmentRepoRows(env.Repos)
}

func hasBranchScopedEnvironmentRepoRows(repos []*models.TaskEnvironmentRepo) bool {
	for _, repo := range repos {
		if repo.RepositoryID != "" && repo.WorktreeID != "" && worktree.SanitizeBranchSlug(repo.BranchSlug) != "" {
			return true
		}
	}
	return false
}

func (e *Executor) latestSessionWorktreeIDsForEnvironment(ctx context.Context, taskID, envID string) map[repositoryWorktreeKey]string {
	if e.repo == nil {
		return nil
	}
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

func sessionWorktreeIDsByKey(worktrees []*models.TaskEnvironmentRepo) map[repositoryWorktreeKey]string {
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
