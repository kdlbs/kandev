package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/integrations/cloneauth"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// isLocalGitRepo reports whether path looks like a valid local git checkout,
// i.e. it has a ".git" entry that is either a directory (regular repo) or a
// regular file (worktree gitdir pointer). Mirrors worktree.Manager.isGitRepo
// in internal/worktree/manager_git.go; kept local since that method is
// unexported on a different package's type.
func isLocalGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

// sourceTypeLocal is the Repository.SourceType value for on-machine repos.
// Mirrors the constant of the same name in internal/task/service (unexported
// there too) — a repo can carry a ProviderOwner/ProviderName (the origin it
// was imported from) while still being SourceType "local", and such a repo's
// on-disk checkout is the user's, not a managed clone we're allowed to
// silently replace.
const sourceTypeLocal = "local"

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
	TaskRepositoryID           string
	RepositoryID               string
	RepositoryPath             string
	BaseBranch                 string
	CheckoutBranch             string
	PRNumber                   int // GitHub PR number when CheckoutBranch is a PR head; sourced from task_repositories.metadata["pr_number"].
	RemoteContribution         *models.RemoteContribution
	ContributionDestination    *models.ContributionDestination
	ComparisonTarget           *models.ComparisonTarget
	Position                   int
	WorktreeBranchPrefix       string
	WorktreeBranchTemplate     string
	PullBeforeWorktree         bool
	RemoteSyncHandled          bool
	RefreshRepository          func(context.Context) error
	RefreshRepositoryWithState func(context.Context) (repoclone.RemoteRefState, error)
	RemoteRefState             repoclone.RemoteRefState
	Repository                 *models.Repository
}

// resumeCredentialSnapshotBackup holds the non-secret routing metadata that
// was persisted before a resume attempt. A failed launch must restore this
// value so the session never advertises credentials that did not successfully
// start an agent.
type resumeCredentialSnapshotBackup struct {
	value   interface{}
	present bool
}

func captureResumeCredentialSnapshot(session *models.TaskSession) resumeCredentialSnapshotBackup {
	if session == nil || session.Metadata == nil {
		return resumeCredentialSnapshotBackup{}
	}
	value, present := session.Metadata[models.SessionMetaKeyGitCredentialSnapshot]
	return resumeCredentialSnapshotBackup{value: value, present: present}
}

func resumeCredentialSnapshotChanged(session *models.TaskSession, backup resumeCredentialSnapshotBackup) bool {
	if session == nil || session.Metadata == nil {
		return backup.present
	}
	value, present := session.Metadata[models.SessionMetaKeyGitCredentialSnapshot]
	if present != backup.present {
		return true
	}
	if !present {
		return false
	}
	return !reflect.DeepEqual(value, backup.value)
}

func resumeCredentialSnapshotBackupIfPersisted(
	persisted bool,
	backup resumeCredentialSnapshotBackup,
) *resumeCredentialSnapshotBackup {
	if !persisted {
		return nil
	}
	return &backup
}

// resolveAllRepoInfo returns the resolved repository info for every repository
// linked to the task, ordered by Position. Returns a single-element slice for
// single-repo tasks and an empty slice for repo-less tasks (e.g. quick chat).
// Each entry has LocalPath populated, cloning provider-backed repos on demand.
func (e *Executor) resolveAllRepoInfo(ctx context.Context, taskID string) ([]*repoInfo, error) {
	return e.resolveAllRepoInfoForSession(ctx, taskID, "")
}

func (e *Executor) resolveAllRepoInfoForSession(
	ctx context.Context, taskID, sessionID string,
) ([]*repoInfo, error) {
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
		info, resolveErr := e.resolveTaskRepoInfoForSession(ctx, sessionID, tr)
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
	return e.resolveTaskRepoInfoForSession(ctx, "", tr)
}

func (e *Executor) resolveTaskRepoInfoForSession(
	ctx context.Context, sessionID string, tr *models.TaskRepository,
) (*repoInfo, error) {
	info := &repoInfo{
		TaskRepositoryID: tr.ID,
		RepositoryID:     tr.RepositoryID,
		BaseBranch:       tr.BaseBranch,
		CheckoutBranch:   tr.CheckoutBranch,
		PRNumber:         prNumberFromMetadata(tr.Metadata),
		Position:         tr.Position,
	}
	if binding, found, err := models.LoadRemoteContribution(tr.Metadata); err != nil {
		return nil, fmt.Errorf("load remote contribution for task repository %q: %w", tr.ID, err)
	} else if found {
		if tr.BaseBranch != "" && tr.BaseBranch != binding.BaseBranch {
			e.logger.Debug("reconciling remote contribution base branch with task repository",
				zap.String("task_repository_id", tr.ID),
				zap.String("old_base_branch", binding.BaseBranch),
				zap.String("new_base_branch", tr.BaseBranch))
			binding.BaseBranch = tr.BaseBranch
		}
		if tr.CheckoutBranch != "" && tr.CheckoutBranch != binding.HeadBranch {
			return nil, fmt.Errorf("task repository %q checkout branch does not match remote contribution", tr.ID)
		}
		info.RemoteContribution = &binding
		info.BaseBranch = binding.BaseBranch
		info.CheckoutBranch = binding.HeadBranch
	}
	if destination, found, err := models.LoadContributionDestination(tr.Metadata); err != nil {
		return nil, fmt.Errorf("load contribution destination for task repository %q: %w", tr.ID, err)
	} else if found {
		info.ContributionDestination = &destination
	}
	if target, found, err := models.LoadComparisonTarget(tr.Metadata); err != nil {
		return nil, fmt.Errorf("load comparison target for task repository %q: %w", tr.ID, err)
	} else if found {
		info.ComparisonTarget = &target
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
	e.resolvePRBaseForLaunch(ctx, tr, repo, info)

	remoteRefState, err := e.ensureRepoLocalPathForSessionAndState(ctx, tr.TaskID, sessionID, repo)
	if err != nil {
		return nil, err
	}
	info.RemoteRefState = remoteRefState

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
	if tr.BranchPolicyBranchTemplate != "" {
		info.WorktreeBranchTemplate = tr.BranchPolicyBranchTemplate
	}
	info.PullBeforeWorktree = repo.PullBeforeWorktree
	if info.BaseBranch == "" && repo.DefaultBranch != "" {
		info.BaseBranch = repo.DefaultBranch
	}
	if info.PullBeforeWorktree {
		refreshRequired, refreshErr := e.shouldRefreshRepositoryForSession(ctx, repo)
		if refreshErr != nil {
			return nil, refreshErr
		}
		if !refreshRequired {
			return info, nil
		}
		prNumber, checkoutBranch := info.PRNumber, info.CheckoutBranch
		info.RefreshRepository = func(refreshCtx context.Context) error {
			return e.refreshManagedRepositoryForSession(
				refreshCtx, tr.TaskID, sessionID, repo, prNumber, checkoutBranch,
			)
		}
		info.RefreshRepositoryWithState = func(refreshCtx context.Context) (repoclone.RemoteRefState, error) {
			return e.refreshManagedRepositoryForSessionWithState(
				refreshCtx, tr.TaskID, sessionID, repo, prNumber, checkoutBranch,
			)
		}
	}
	return info, nil
}

func (e *Executor) resolvePRBaseForLaunch(
	ctx context.Context, tr *models.TaskRepository, repo *models.Repository, info *repoInfo,
) {
	if e.prBaseResolver == nil || info.PRNumber <= 0 || !isGitHubRepository(repo) {
		return
	}
	baseBranch, err := e.prBaseResolver.ResolvePRBaseBranch(
		ctx, repo.WorkspaceID, repo.ProviderOwner, repo.ProviderName, info.PRNumber,
	)
	baseBranch = strings.TrimSpace(baseBranch)
	if err != nil || baseBranch == "" {
		e.logger.Debug("could not resolve live pull request base branch",
			zap.String("task_id", tr.TaskID),
			zap.Int("pr_number", info.PRNumber),
			zap.Error(err))
		return
	}
	if baseBranch != tr.BaseBranch {
		e.logger.Info("pull request base branch changed since task creation",
			zap.String("task_id", tr.TaskID),
			zap.Int("pr_number", info.PRNumber),
			zap.String("old_base_branch", tr.BaseBranch),
			zap.String("new_base_branch", baseBranch))
	}
	info.BaseBranch = baseBranch
	if info.RemoteContribution != nil {
		info.RemoteContribution.BaseBranch = baseBranch
	}
}

func hasProviderRepositoryIdentity(repo *models.Repository) bool {
	if repo == nil {
		return false
	}
	return (strings.TrimSpace(repo.ProviderOwner) != "" && strings.TrimSpace(repo.ProviderName) != "") ||
		(strings.TrimSpace(repo.ProviderScope) != "" && strings.TrimSpace(repo.ProviderRepoID) != "")
}

func (e *Executor) shouldRefreshRepositoryForSession(
	ctx context.Context, repo *models.Repository,
) (bool, error) {
	if repo == nil || repo.SourceType == sourceTypeLocal {
		return false, nil
	}
	provider := strings.ToLower(strings.TrimSpace(repo.Provider))
	if provider == gitLabProviderID {
		return hasProviderRepositoryIdentity(repo), nil
	}
	if provider == providerAzureDevOps {
		return strings.TrimSpace(repo.ProviderOwner) != "" && strings.TrimSpace(repo.ProviderName) != "", nil
	}
	if !isGitHubRepository(repo) {
		return isPluginManagedRepository(repo), nil
	}
	policy := TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged}
	if e.githubCredentialPolicyResolver != nil {
		resolved, err := e.githubCredentialPolicyResolver.ResolveTaskGitCredentialPolicy(ctx, repo.WorkspaceID)
		if err != nil {
			return false, fmt.Errorf("resolve task Git credential policy: %w", err)
		}
		policy = resolved
	}
	return policy.Mode != taskGitCredentialsModeExecutor, nil
}

func isPluginManagedRepository(repo *models.Repository) bool {
	if repo == nil || repo.SourceType == sourceTypeLocal || !hasProviderRepositoryIdentity(repo) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(repo.Provider)) {
	case "", gitHubProviderID, gitLabProviderID, providerAzureDevOps:
		return false
	default:
		return true
	}
}

func (e *Executor) refreshManagedRepositoryForSession(
	ctx context.Context, taskID, sessionID string, repo *models.Repository, prNumber int, checkoutBranch string,
) error {
	_, err := e.refreshManagedRepositoryForSessionWithState(
		ctx, taskID, sessionID, repo, prNumber, checkoutBranch,
	)
	return err
}

func (e *Executor) refreshManagedRepositoryForSessionWithState(
	ctx context.Context, taskID, sessionID string, repo *models.Repository, prNumber int, checkoutBranch string,
) (repoclone.RemoteRefState, error) {
	if e.repoCloner == nil || repo.LocalPath == "" {
		return repoclone.RemoteRefStateUnknown, errors.New("managed repository refresh is unavailable")
	}
	cloneURL := repositoryCloneURL(repo)
	if cloneURL == "" {
		var err error
		cloneURL, err = e.repoCloner.BuildCloneURLWithHost(
			ctx, repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName,
		)
		if err != nil || cloneURL == "" {
			return repoclone.RemoteRefStateUnknown, ErrNoCloneURL
		}
	}
	credentialOrigin, token := "", ""
	provider := strings.ToLower(strings.TrimSpace(repo.Provider))
	if provider == gitLabProviderID && e.gitlabCredentials != nil {
		var err error
		credentialOrigin, token, err = e.gitlabCredentials.ResolveGitLabExecutionCredentials(ctx, repo.WorkspaceID)
		if err != nil {
			return repoclone.RemoteRefStateUnknown, fmt.Errorf("resolve GitLab refresh credentials: %w", err)
		}
	}
	if provider == providerAzureDevOps && strings.HasPrefix(strings.ToLower(cloneURL), "https://") {
		return e.refreshAzureDevOpsRepositoryForSessionWithState(
			ctx, repo, cloneURL,
		)
	}
	request := repositoryGitCredentialRequest(taskID, sessionID, repo, cloneURL)
	if isGitHubRepository(repo) {
		request.PRNumber = prNumber
		request.CheckoutBranch = checkoutBranch
	}
	if stateCloner, ok := e.repoCloner.(remoteStateRepoCloner); ok {
		state, err := stateCloner.RefreshWorkspaceRepositoryWithCredentialRequestAndState(
			ctx, request, repo.LocalPath, credentialOrigin, token,
		)
		if err != nil {
			return repoclone.RemoteRefStateUnknown, fmt.Errorf("refresh repository before worktree: %w", err)
		}
		return state, nil
	}
	if err := e.repoCloner.RefreshWorkspaceRepositoryWithCredentialRequest(
		ctx, request, repo.LocalPath, credentialOrigin, token,
	); err != nil {
		return repoclone.RemoteRefStateUnknown, fmt.Errorf("refresh repository before worktree: %w", err)
	}
	return repoclone.RemoteRefStateHasRefs, nil
}

func (e *Executor) refreshAzureDevOpsRepositoryForSession(
	ctx context.Context, repo *models.Repository, cloneURL string,
) error {
	_, err := e.refreshAzureDevOpsRepositoryForSessionWithState(ctx, repo, cloneURL)
	return err
}

func (e *Executor) refreshAzureDevOpsRepositoryForSessionWithState(
	ctx context.Context, repo *models.Repository, cloneURL string,
) (repoclone.RemoteRefState, error) {
	authCloner, ok := e.repoCloner.(strictAuthenticatedRepoCloner)
	if !ok || e.secretStore == nil {
		return repoclone.RemoteRefStateUnknown, errors.New("azure DevOps repository refresh authentication is unavailable")
	}
	pat, err := e.secretStore.Reveal(ctx, cloneauth.AzureDevOpsPATKey(repo.WorkspaceID))
	if err != nil {
		return repoclone.RemoteRefStateUnknown, fmt.Errorf("read Azure DevOps refresh credential: %w", err)
	}
	if stateCloner, ok := e.repoCloner.(remoteStateAuthenticatedRepoCloner); ok {
		state, err := stateCloner.RefreshWorkspaceRepositoryWithBasicAuthAndState(
			ctx, repo.WorkspaceID, repo.Provider, repo.ProviderHost, cloneURL,
			repo.ProviderOwner, repo.ProviderName, repo.LocalPath, "kandev", pat,
		)
		if err != nil {
			return repoclone.RemoteRefStateUnknown, fmt.Errorf("refresh Azure DevOps repository before worktree: %w", err)
		}
		return state, nil
	}
	if err := authCloner.RefreshWorkspaceRepositoryWithBasicAuth(
		ctx, repo.WorkspaceID, repo.Provider, repo.ProviderHost, cloneURL,
		repo.ProviderOwner, repo.ProviderName, repo.LocalPath, "kandev", pat,
	); err != nil {
		return repoclone.RemoteRefStateUnknown, fmt.Errorf("refresh Azure DevOps repository before worktree: %w", err)
	}
	return repoclone.RemoteRefStateHasRefs, nil
}

// ensureRepoLocalPath re-clones a repo's local checkout in place when it's
// missing or has gone stale (moved, deleted, or never actually a git repo —
// e.g. a leftover placeholder directory), but ONLY for genuinely
// provider-backed repositories: SourceType must not be "local" and both
// ProviderOwner/ProviderName must be set. A repo can carry those provider
// fields (the origin it was imported from) while still being a local
// checkout the user manages themselves; re-cloning over such a repo would
// silently redirect future launches away from the user's saved path. Mutates
// repo.LocalPath only when the clone actually returns a path — never blanks
// an already-set one.
func (e *Executor) ensureRepoLocalPath(ctx context.Context, repo *models.Repository) error {
	return e.ensureRepoLocalPathForSession(ctx, "", "", repo)
}

func (e *Executor) ensureRepoLocalPathForSession(
	ctx context.Context, taskID, sessionID string, repo *models.Repository,
) error {
	_, err := e.ensureRepoLocalPathForSessionAndState(ctx, taskID, sessionID, repo)
	return err
}

func (e *Executor) ensureRepoLocalPathForSessionAndState(
	ctx context.Context, taskID, sessionID string, repo *models.Repository,
) (repoclone.RemoteRefState, error) {
	if repo.SourceType == sourceTypeLocal {
		if repo.LocalPath == "" || e.repoCloner == nil {
			return repoclone.RemoteRefStateUnknown, nil
		}
		stateCloner, ok := e.repoCloner.(localRemoteStateRepoCloner)
		if !ok {
			return repoclone.RemoteRefStateUnknown, nil
		}
		state, err := stateCloner.InspectLocalRepositoryRemoteRefState(ctx, repo.LocalPath)
		if err != nil {
			// Local repositories historically support offline launch. Preserve
			// that behavior when an origin is missing or temporarily
			// unreachable; unknown state never authorizes empty-remote bootstrap.
			e.logger.Debug("could not inspect local repository remote refs",
				zap.String("repository_id", repo.ID),
				zap.String("local_path", repo.LocalPath),
				zap.Error(err))
			return repoclone.RemoteRefStateUnknown, nil
		}
		return state, nil
	}
	if !hasProviderRepositoryIdentity(repo) {
		return repoclone.RemoteRefStateUnknown, nil
	}
	if repo.LocalPath != "" && isLocalGitRepo(repo.LocalPath) &&
		(e.repoCloner == nil || !e.repoCloner.ShouldRecloneForWorkspace(repo.WorkspaceID, repo.LocalPath)) {
		return repoclone.RemoteRefStateUnknown, e.reconcileGitHubCheckoutOrigin(ctx, repo, repo.LocalPath)
	}
	localPath, state, cloneErr := e.ensureRepoClonedForSessionAndState(ctx, taskID, sessionID, repo)
	if cloneErr != nil {
		return state, cloneErr
	}
	if localPath != "" {
		repo.LocalPath = localPath
	}
	return state, nil
}

// ensureRepoCloned clones a provider-backed repository to local disk and updates its local path in the database.
// Returns the local path on success, or empty string if no cloner is configured.
func (e *Executor) ensureRepoCloned(ctx context.Context, repo *models.Repository) (string, error) {
	return e.ensureRepoClonedForSession(ctx, "", "", repo)
}

func (e *Executor) ensureRepoClonedForSession(
	ctx context.Context, taskID, sessionID string, repo *models.Repository,
) (string, error) {
	path, _, err := e.ensureRepoClonedForSessionAndState(ctx, taskID, sessionID, repo)
	return path, err
}

func (e *Executor) ensureRepoClonedForSessionAndState(
	ctx context.Context, taskID, sessionID string, repo *models.Repository,
) (string, repoclone.RemoteRefState, error) {
	if e.repoCloner == nil {
		e.logger.Warn("repository has no local path and no cloner configured",
			zap.String("repository_id", repo.ID),
			zap.String("provider", repo.Provider),
			zap.String("owner", repo.ProviderOwner),
			zap.String("name", repo.ProviderName))
		return "", repoclone.RemoteRefStateUnknown, nil
	}

	// RemoteURL is the canonical provider-declared transport. In particular,
	// plugin providers may use a path shape that cannot be reconstructed from
	// owner/name. Only derive a URL for legacy rows that do not have one.
	cloneURL := strings.TrimSpace(repo.RemoteURL)
	if cloneURL == "" {
		var urlErr error
		cloneURL, urlErr = e.repoCloner.BuildCloneURLWithHost(
			ctx, repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName,
		)
		if urlErr != nil || cloneURL == "" {
			return "", repoclone.RemoteRefStateUnknown, ErrNoCloneURL
		}
	}

	e.logger.Info("cloning provider-backed repository for local execution",
		zap.String("repository_id", repo.ID),
		zap.String("repo", repo.ProviderOwner+"/"+repo.ProviderName))

	localPath, state, err := e.ensureClonedWithWorkspaceAuthForSessionAndState(ctx, taskID, sessionID, repo, cloneURL)
	if err != nil {
		e.logger.Error("failed to clone repository",
			zap.String("repository_id", repo.ID),
			zap.String("repo", repo.ProviderOwner+"/"+repo.ProviderName),
			zap.Error(err))
		return "", state, err
	}

	// Persist the local path before reconciliation so a remote-update failure
	// does not discard a completed clone and force a re-clone on every retry.
	if e.repoUpdater != nil && localPath != "" {
		if updateErr := e.repoUpdater.UpdateRepositoryLocalPath(ctx, repo.ID, localPath); updateErr != nil {
			e.logger.Warn("failed to update repository local path after clone",
				zap.String("repository_id", repo.ID),
				zap.String("local_path", localPath),
				zap.Error(updateErr))
			// Non-fatal: the clone succeeded, we can use the path
		}
	}
	if err := e.reconcileGitHubCheckoutOrigin(ctx, repo, localPath); err != nil {
		return "", state, err
	}

	// Note: default_branch backfill is intentionally driven from
	// resolveTaskRepoInfo (the caller), not here. That way it also runs for
	// rows whose local_path was already populated by a prior launch but whose
	// default_branch was never persisted (e.g. rows created before the
	// backfill existed).

	return localPath, state, nil
}

func (e *Executor) reconcileGitHubCheckoutOrigin(
	ctx context.Context, repo *models.Repository, localPath string,
) error {
	if e.repoCloner == nil || localPath == "" || !isGitHubRepository(repo) {
		return nil
	}
	policy := TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged}
	if e.githubCredentialPolicyResolver != nil {
		resolved, err := e.githubCredentialPolicyResolver.ResolveTaskGitCredentialPolicy(ctx, repo.WorkspaceID)
		if err != nil {
			return fmt.Errorf("resolve task Git credential policy: %w", err)
		}
		policy = resolved
	}
	originURL, err := gitHubCheckoutOriginURL(ctx, repo, policy, e.repoCloner)
	if err != nil {
		return err
	}
	if err := e.repoCloner.SetOriginURL(ctx, localPath, originURL); err != nil {
		return fmt.Errorf("set GitHub checkout origin: %w", err)
	}
	return nil
}

func isGitHubRepository(repo *models.Repository) bool {
	if repo == nil || repo.ProviderOwner == "" || repo.ProviderName == "" {
		return false
	}
	// Provider == "" is treated as GitHub for legacy rows imported before
	// provider tagging; this matches the clone URL fallback convention.
	return repo.Provider == "" || strings.EqualFold(repo.Provider, "github")
}

func gitHubCheckoutOriginURL(
	ctx context.Context, repo *models.Repository, policy TaskGitCredentialPolicy, cloner RepoCloner,
) (string, error) {
	if policy.Mode == taskGitCredentialsModeExecutor {
		originURL, err := cloner.BuildCloneURLWithHost(
			ctx, repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName,
		)
		if err != nil {
			return "", fmt.Errorf("build executor GitHub checkout origin: %w", err)
		}
		if originURL == "" {
			return "", errors.New("build executor GitHub checkout origin: empty URL")
		}
		return originURL, nil
	}
	originURL, err := repoclone.CloneURLWithHost(
		repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, repoclone.ProtocolHTTPS,
	)
	if err != nil {
		return "", fmt.Errorf("build managed GitHub checkout origin: %w", err)
	}
	return originURL, nil
}

// EnsureRepositoryCloned is the host-materialization seam for provider-backed
// repositories. Authentication and clone-url construction deliberately remain
// inside Executor so callers never receive credentials or construct git calls.
func (e *Executor) EnsureRepositoryCloned(ctx context.Context, repo *models.Repository) (string, error) {
	return e.EnsureRepositoryClonedForSession(ctx, "", "", repo)
}

// EnsureRepositoryClonedForSession materializes a provider repository with
// exact task/session scope for plugin credential resolution.
func (e *Executor) EnsureRepositoryClonedForSession(
	ctx context.Context, taskID, sessionID string, repo *models.Repository,
) (string, error) {
	if repo == nil {
		return "", errors.New("repository is required")
	}
	if repo.LocalPath != "" {
		return repo.LocalPath, nil
	}
	path, err := e.ensureRepoClonedForSession(ctx, taskID, sessionID, repo)
	if err != nil {
		return "", err
	}
	if path != "" {
		repo.LocalPath = path
	}
	return path, nil
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
	expectedState := session.State
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
		updateErr = e.updateSessionStarting(ctx, taskID, session, expectedState, true)
	} else {
		updateErr = e.persistSessionFullRowIfCurrentState(ctx, session, expectedState)
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

// ResumeOptions controls explicit recovery behavior for a session resume.
// Branch replacement is intentionally opt-in; ordinary resume preserves the
// original worktree branch and reports when it is unrecoverable.
type ResumeOptions struct {
	AllowBranchReplacement bool
}

// ResumeSession restarts an existing task session using its stored worktree.
// When startAgent is false, only the executor runtime is started (agent process is not launched).
func (e *Executor) ResumeSession(ctx context.Context, session *models.TaskSession, startAgent bool) (*TaskExecution, error) {
	return e.resumeSession(ctx, session, startAgent, ResumeOptions{})
}

// ResumeSessionWithOptions restarts an existing task session with an explicit
// recovery permission. It keeps the same session and provider resume identity.
func (e *Executor) ResumeSessionWithOptions(
	ctx context.Context,
	session *models.TaskSession,
	startAgent bool,
	options ResumeOptions,
) (*TaskExecution, error) {
	return e.resumeSession(ctx, session, startAgent, options)
}

//nolint:cyclop,gocognit,funlen // Resume coordinates the established launch, rollback, and stale-execution recovery sequence.
func (e *Executor) resumeSession(
	ctx context.Context,
	session *models.TaskSession,
	startAgent bool,
	options ResumeOptions,
) (*TaskExecution, error) {
	if session != nil {
		resumeSnapshot := *session
		resumeSnapshot.Metadata = cloneMetadata(session.Metadata)
		session = &resumeSnapshot
	}
	task, unlock, err := e.validateAndLockResume(ctx, session)
	if err != nil {
		return nil, err
	}
	defer unlock()

	resumeInitialState := session.State
	previousCredentialSnapshot := captureResumeCredentialSnapshot(session)
	wasTerminalResume := isTerminalSessionState(resumeInitialState)
	// Force-cleanup any stale in-memory execution / agentctl state for terminal-state
	// sessions. Their agent process is dead by definition, so "already running" signals
	// from the execution store or agentctl's "starting" status are stale and would
	// otherwise block the relaunch.
	if wasTerminalResume {
		if cleanupErr := e.agentManager.CleanupStaleExecutionBySessionID(ctx, session.ID); cleanupErr != nil {
			e.logger.Warn("failed to force-cleanup stale execution before terminal-state resume",
				zap.String("session_id", session.ID),
				zap.Error(cleanupErr))
		}
	}

	resumeStatePersisted := false
	var beforeCredentialLease func() error
	if startAgent {
		beforeCredentialLease = func() error {
			if persistErr := e.persistResumeState(ctx, task.ID, session, true); persistErr != nil {
				return persistErr
			}
			resumeStatePersisted = true
			return nil
		}
	}
	req, _, execCfg, existingEnv, _, err := e.buildResumeRequestAtCredentialBoundaryWithOptions(
		ctx, task, session, startAgent, beforeCredentialLease, options,
	)
	if err != nil {
		if resumeStatePersisted {
			e.rollbackResumeStateAfterFailure(ctx, task.ID, session.ID, resumeInitialState, err, nil)
		}
		return nil, err
	}
	credentialSnapshotPersisted := false
	if startAgent {
		// Credential setup records the non-secret routing snapshot after the
		// lease issuer has observed STARTING. Persist that metadata with the
		// same expected-state guard before launching, so a concurrent terminal
		// transition cannot be overwritten by a stale resume.
		if err := e.persistSessionFullRowIfCurrentState(ctx, session, models.TaskSessionStateStarting); err != nil {
			if resumeStatePersisted {
				e.rollbackResumeStateAfterFailure(ctx, task.ID, session.ID, resumeInitialState, err, nil)
			}
			return nil, err
		}
		credentialSnapshotPersisted = resumeCredentialSnapshotChanged(session, previousCredentialSnapshot)
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
	if err != nil && isAgentAlreadyRunningError(err) {
		// "already has an agent running" fires both for live executions (a concurrent
		// resume raced us) and stale ones (agent never started or exited without
		// cleanup). Probe liveness before deciding what to do — otherwise we'd kill a
		// healthy agent mid-prompt. For terminal states the agent is dead by definition,
		// so skip the probe and go straight to cleanup+retry — this avoids a silent
		// regression to ErrExecutionAlreadyRunning if the preemptive cleanup above
		// failed and agentctl still reports a stale "starting" status.
		if !wasTerminalResume && e.agentManager.IsAgentRunningForSession(ctx, session.ID) {
			e.logger.Info("resume race: agent already running for session, returning ErrExecutionAlreadyRunning",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID))
			if startAgent {
				e.rollbackResumeStateAfterFailure(
					ctx, task.ID, session.ID, resumeInitialState, err,
					resumeCredentialSnapshotBackupIfPersisted(credentialSnapshotPersisted, previousCredentialSnapshot),
				)
			}
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
		if startAgent {
			e.rollbackResumeStateAfterFailure(
				ctx, task.ID, session.ID, resumeInitialState, err,
				resumeCredentialSnapshotBackupIfPersisted(credentialSnapshotPersisted, previousCredentialSnapshot),
			)
		}
		e.logger.Error("failed to relaunch agent for session",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, err
	}

	if !startAgent {
		if err := e.persistResumeState(ctx, task.ID, session, false); err != nil {
			e.cleanupUnstartedExecutionAfterPersistError(ctx, session.ID, resp.AgentExecutionID, err)
			return nil, err
		}
	}
	if err := e.persistTaskEnvironment(ctx, task.ID, session, existingEnv, req, resp, execCfg); err != nil {
		e.cleanupUnstartedExecutionAfterPersistError(ctx, session.ID, resp.AgentExecutionID, err)
		e.markTaskEnvironmentMaterializationFailed(ctx, existingEnv, session.ID)
		if startAgent {
			e.rollbackResumeStateAfterFailure(ctx, task.ID, session.ID, resumeInitialState, err,
				resumeCredentialSnapshotBackupIfPersisted(credentialSnapshotPersisted, previousCredentialSnapshot))
		}
		return nil, err
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
		e.startAgentProcessOnResume(ctx, task.ID, session, resp.AgentExecutionID)
	}

	return execution, nil
}

// restoreResumeCredentialSnapshotIfStarting restores the prior non-secret Git
// credential routing metadata only while the session is still STARTING. The
// state guard ensures a concurrent terminal transition cannot be overwritten by
// a stale resume rollback.
func (e *Executor) restoreResumeCredentialSnapshotIfStarting(
	ctx context.Context,
	sessionID string,
	backup *resumeCredentialSnapshotBackup,
) {
	if backup == nil {
		return
	}
	current, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil || current == nil || current.State != models.TaskSessionStateStarting {
		return
	}
	if !resumeCredentialSnapshotChanged(current, *backup) {
		return
	}

	metadata := cloneMetadata(current.Metadata)
	if backup.present {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata[models.SessionMetaKeyGitCredentialSnapshot] = backup.value
	} else {
		delete(metadata, models.SessionMetaKeyGitCredentialSnapshot)
	}
	current.Metadata = metadata
	changed, err := e.repo.UpdateTaskSessionIfCurrentState(ctx, current, models.TaskSessionStateStarting)
	if err != nil {
		e.logger.Warn("failed to restore Git credential snapshot after resume failure",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if !changed {
		e.logger.Debug("skipped Git credential snapshot rollback after session state changed",
			zap.String("session_id", sessionID))
	}
}

// rollbackResumeStateAfterFailure restores the state observed before a resume
// attempt only while the session is still STARTING. A concurrent terminal
// transition wins and is left untouched by transitionSessionState.
func (e *Executor) rollbackResumeStateAfterFailure(
	ctx context.Context,
	taskID, sessionID string,
	priorState models.TaskSessionState,
	resumeErr error,
	credentialSnapshot *resumeCredentialSnapshotBackup,
) {
	e.restoreResumeCredentialSnapshotIfStarting(ctx, sessionID, credentialSnapshot)
	if e.onSessionStateTransition != nil {
		current, err := e.repo.GetTaskSession(ctx, sessionID)
		if err != nil || current == nil || current.State != models.TaskSessionStateStarting {
			return
		}
		_, _, rollbackErr := e.transitionSessionState(ctx, taskID, sessionID, priorState, resumeErr.Error())
		if rollbackErr != nil {
			e.logger.Warn("failed to roll back session state after resume failure",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(rollbackErr))
		}
		return
	}
	if updater, ok := e.repo.(interface {
		UpdateTaskSessionStateIfCurrent(context.Context, string, models.TaskSessionState, models.TaskSessionState, string) (bool, time.Time, error)
	}); ok {
		if _, _, err := updater.UpdateTaskSessionStateIfCurrent(
			ctx,
			sessionID,
			models.TaskSessionStateStarting,
			priorState,
			resumeErr.Error(),
		); err != nil {
			e.logger.Warn("failed to roll back session state after resume failure",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return
	}
	current, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil || current == nil || current.State != models.TaskSessionStateStarting {
		return
	}
	_, _, rollbackErr := e.transitionSessionState(ctx, taskID, sessionID, priorState, resumeErr.Error())
	if rollbackErr != nil {
		e.logger.Warn("failed to roll back session state after resume failure",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(rollbackErr))
	}
}

// validateAndLockResume validates the session is resumable, acquires the per-session lock,
// and loads the associated task. Returns the task, an unlock function, and any error.
// The caller must call unlock() when the critical section is complete.
func (e *Executor) validateAndLockResume(ctx context.Context, session *models.TaskSession) (*v1.Task, func(), error) {
	if session == nil {
		return nil, func() {}, ErrExecutionNotFound
	}
	requestedState := session.State

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
		if isTerminalSessionState(fresh.State) && fresh.State != requestedState {
			unlock()
			return nil, func() {}, &SessionStateSupersededError{
				SessionID: session.ID,
				State:     fresh.State,
			}
		}
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
	return e.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, task, session, startAgent, nil, ResumeOptions{})
}

// buildResumeRequestAtCredentialBoundary prepares a resume request and invokes
// beforeCredentialLease after all prior-state-dependent preparation succeeds,
// immediately before managed GitHub credential issuance. Production resume
// uses this boundary to persist STARTING; focused request-builder tests omit it.
func (e *Executor) buildResumeRequestAtCredentialBoundary(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	startAgent bool,
	beforeCredentialLease func() error,
) (*LaunchAgentRequest, string, executorConfig, *models.TaskEnvironment, *models.ExecutorRunning, error) {
	return e.buildResumeRequestAtCredentialBoundaryWithOptions(
		ctx, task, session, startAgent, beforeCredentialLease, ResumeOptions{},
	)
}

func (e *Executor) buildResumeRequestAtCredentialBoundaryWithOptions(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	startAgent bool,
	beforeCredentialLease func() error,
	options ResumeOptions,
) (*LaunchAgentRequest, string, executorConfig, *models.TaskEnvironment, *models.ExecutorRunning, error) {
	req, metadata := newResumeLaunchRequest(task, session, startAgent, options)
	execConfig := e.applyExecutorConfigToResumeRequest(ctx, req, task, session, metadata)
	repositoryID, existingEnv, allRepos, err := e.prepareResumeRepositorySettings(
		ctx, task, session, req,
	)
	if err != nil {
		return nil, "", execConfig, existingEnv, nil, err
	}
	if err := e.applyResumeMCPSettings(ctx, task.ID, session, req); err != nil {
		return nil, "", execConfig, existingEnv, nil, err
	}

	existingRunning := e.applyRunningRecordToResumeRequest(ctx, req, task, session, startAgent)
	if err := e.applyResumeWorkspaceFolders(ctx, task.ID, req); err != nil {
		return nil, "", execConfig, existingEnv, existingRunning, err
	}
	if err := e.configureResumeGitHubCredentials(
		ctx, req, session, allRepos, beforeCredentialLease,
	); err != nil {
		return nil, "", execConfig, existingEnv, existingRunning, err
	}
	e.injectGitLabWorkspaceCredentials(ctx, req)
	if err := e.resolveLaunchEnvironment(ctx, req, execConfig.ProfileEnvVars, allRepos); err != nil {
		return nil, "", execConfig, existingEnv, existingRunning, err
	}

	return req, repositoryID, execConfig, existingEnv, existingRunning, nil
}

func newResumeLaunchRequest(
	task *v1.Task,
	session *models.TaskSession,
	startAgent bool,
	options ResumeOptions,
) (*LaunchAgentRequest, map[string]interface{}) {
	executionProfileID := session.ExecutionProfileID
	if executionProfileID == "" {
		executionProfileID = session.AgentProfileID
	}
	req := &LaunchAgentRequest{
		TaskID:                 task.ID,
		WorkspaceID:            task.WorkspaceID,
		SessionID:              session.ID,
		TaskTitle:              task.Title,
		AgentProfileID:         executionProfileID,
		OfficeAgentProfileID:   session.AgentProfileID,
		StartAgent:             startAgent,
		TaskDescription:        task.Description,
		Priority:               task.Priority,
		IsEphemeral:            task.IsEphemeral,
		IsPassthrough:          session.IsPassthrough,
		TaskEnvironmentID:      session.TaskEnvironmentID,
		AllowBranchReplacement: options.AllowBranchReplacement,
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
	return req, metadata
}

func (e *Executor) prepareResumeRepositorySettings(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
) (string, *models.TaskEnvironment, []*repoInfo, error) {
	existingEnv, err := e.resolveResumeTaskEnvironment(ctx, task.ID, session)
	if err != nil {
		return "", nil, nil, err
	}
	if session.TaskEnvironmentID != "" {
		req.TaskEnvironmentID = session.TaskEnvironmentID
	}
	req.WorkspaceReuseRequired = workspaceReuseAllowed(
		existingEnv,
		req.ExecutorType,
		existingEnv != nil && existingEnv.MaterializationSessionID != session.ID,
		e.taskIsRepoBacked(ctx, task.ID),
	)

	allRepos, err := e.resolveAllRepoInfoForSession(ctx, task.ID, session.ID)
	if err != nil {
		return "", nil, nil, err
	}
	req.McpProviders = deriveMCPProviders(allRepos)
	repositoryID, err := e.applyResumeRepoConfig(ctx, task, session, req, existingEnv, allRepos)
	if err != nil {
		return "", nil, nil, err
	}
	applyResumeRepositoryFlags(req, allRepos)
	if err := e.validateReuseEnvironmentInventory(ctx, req, existingEnv); err != nil {
		return "", existingEnv, nil, err
	}

	e.reuseExistingEnvironment(ctx, req, existingEnv)
	return repositoryID, existingEnv, allRepos, nil
}

func applyResumeRepositoryFlags(req *LaunchAgentRequest, repositories []*repoInfo) {
	if len(repositories) == 0 {
		return
	}
	first := repositories[0]
	req.PullBeforeWorktree = first.PullBeforeWorktree
	req.RemoteSyncHandled = first.RemoteSyncHandled
	req.RefreshRepository = first.RefreshRepository
	req.RefreshRepositoryWithState = first.RefreshRepositoryWithState
	req.RemoteRefState = first.RemoteRefState
}

func (e *Executor) applyResumeMCPSettings(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	req *LaunchAgentRequest,
) error {
	mode, err := e.resolveTaskSessionMCPMode(ctx, taskID, session, true)
	if err != nil {
		return err
	}
	profileContext, err := e.resolveTaskSessionMCPProfile(ctx, taskID, session, true)
	if err != nil {
		return err
	}
	req.McpMode = mode
	profileContext.Providers = req.McpProviders
	req.McpProfile = &profileContext
	return nil
}

func (e *Executor) applyResumeWorkspaceFolders(
	ctx context.Context,
	taskID string,
	req *LaunchAgentRequest,
) error {
	folders, err := e.repo.ListTaskWorkspaceFolders(ctx, taskID)
	if err != nil {
		return err
	}
	for _, folder := range folders {
		if folder != nil {
			req.WorkspaceFolders = append(req.WorkspaceFolders, WorkspaceFolderSpec{
				Name: folder.DisplayName, LocalPath: folder.LocalPath,
			})
		}
	}
	return nil
}

func (e *Executor) configureResumeGitHubCredentials(
	ctx context.Context,
	req *LaunchAgentRequest,
	session *models.TaskSession,
	repositories []*repoInfo,
	beforeCredentialLease func() error,
) error {
	if beforeCredentialLease != nil {
		if err := beforeCredentialLease(); err != nil {
			return err
		}
	}
	if err := e.configureGitCredentialBrokerForRepositories(ctx, req, repositories); err != nil {
		return err
	}
	return e.applyGitCredentialSnapshot(ctx, req, session)
}

func (e *Executor) resolveResumeTaskEnvironment(ctx context.Context, taskID string, session *models.TaskSession) (*models.TaskEnvironment, error) {
	env, err := e.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("lookup existing task environment: %w", err)
	}
	if env == nil {
		// Inherited-environment fallback: a child task created by an office
		// task-handoff may have had session.TaskEnvironmentID rewritten to point
		// at the parent's / shared group's env row, which is owned by a
		// *different* task. GetTaskEnvironmentByTaskID misses that row because it
		// indexes by the child task id, so without this lookup the resume path
		// treats the env as absent and persistTaskEnvironment tries to CREATE a
		// new row using the inherited ID — which already exists, producing
		// "UNIQUE constraint failed: task_environments.id".
		//
		// Unlike the launch path (executor_execute.go), which hard-errors with
		// ErrWorkspaceReuseUnsafe when the referenced row is absent, resume
		// falls through to (nil, nil): the ID is then free, so the create path
		// produces a valid fresh environment rather than a failing resume. A
		// missing row surfaces as ErrTaskEnvironmentNotFound, which is a miss,
		// not a fatal error; any other lookup error still propagates.
		if session.TaskEnvironmentID != "" {
			inherited, inhErr := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
			if inhErr != nil && !errors.Is(inhErr, repoerrors.ErrTaskEnvironmentNotFound) {
				return nil, fmt.Errorf("lookup inherited task environment: %w", inhErr)
			}
			if inherited != nil {
				return inherited, nil
			}
		}
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

// isArchiveCancelledResumeSession reports whether session was cancelled by an
// archive (Service.ArchiveTask's single-task path or HandoffService's cascade
// archive) rather than an explicit user/coordinator stop. Mirrors
// orchestrator.isArchiveCancelledSession — kept local to this package since
// the two live on opposite sides of the executor/orchestrator boundary and
// the check is two lines over already-exported models helpers.
func isArchiveCancelledResumeSession(session *models.TaskSession) bool {
	return session != nil &&
		session.State == models.TaskSessionStateCancelled &&
		models.IsArchiveCancelReason(session.ErrorMessage)
}

// applyRunningRecordToResumeRequest loads the ExecutorRunning record and applies
// resume-related fields (remote reconnect, resume token) to the request.
func (e *Executor) applyRunningRecordToResumeRequest(ctx context.Context, req *LaunchAgentRequest, task *v1.Task, session *models.TaskSession, startAgent bool) *models.ExecutorRunning {
	running, runErr := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if runErr != nil || running == nil {
		// Archive cleanup tears down the executors_running row entirely, so an
		// archive-cancelled session reaches this point with running == nil —
		// exactly the shape GetTaskSessionStatus's resumeReasonArchiveCancelledResumable
		// auto-resumes once the task is unarchived. There is no resume token or
		// running record to fall back on here, but the launch still must not
		// replay task.Description as a fresh prompt — see the else-if branch
		// below for the same guard on the running-record path.
		if startAgent && isArchiveCancelledResumeSession(session) {
			req.TaskDescription = ""
		}
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

	if token := resumeTokenForExecutionProfile(running, req.AgentProfileID); token != "" && startAgent {
		req.ACPSessionID = token
		// Clear TaskDescription so the agent doesn't receive an automatic prompt on resume.
		// The session context is restored via ACP session/load; sending a prompt here would
		// cause the agent to start working immediately instead of waiting for user input.
		req.TaskDescription = ""
		e.logger.Info("found resume token for session resumption",
			zap.String("task_id", task.ID),
			zap.String("session_id", session.ID),
			zap.Bool("has_resume_token", running.ResumeToken != ""))
	} else if startAgent && (session.State == models.TaskSessionStateWaitingForInput || isArchiveCancelledResumeSession(session)) {
		// Fresh-start resume (no resume token): don't auto-prompt with the task
		// description. Also covers an archive-cancelled session whose running
		// record survived cleanup but carries no token — same auto-resume shape
		// as the running==nil branch above.
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
// resolvedRepos is supplied by buildResumeRequest so primary, multi-repository,
// and credential configuration share one materialization pass. Legacy callers
// may omit it; those calls resolve the set once locally.
func (e *Executor) applyResumeRepoConfig(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
	existingEnv *models.TaskEnvironment,
	resolved ...[]*repoInfo,
) (string, error) {
	allRepos, err := e.resumeRepoSet(ctx, task.ID, resolved...)
	if err != nil {
		return "", err
	}
	repositoryID, baseBranch := resolveResumeRepoIDAndBranch(task, session)
	baseBranch = resolveResumeBaseBranch(repositoryID, baseBranch, allRepos)
	if baseBranch != "" {
		req.Branch = baseBranch
	}
	if repositoryID == "" {
		return "", nil
	}

	var repository *models.Repository
	for _, info := range allRepos {
		if info != nil && info.RepositoryID == repositoryID {
			repository = info.Repository
			break
		}
	}
	if repository == nil {
		repository, err = e.repo.GetRepository(ctx, repositoryID)
		if err != nil {
			e.logger.Error("failed to load repository for task session resume",
				zap.String("task_id", task.ID),
				zap.String("repository_id", repositoryID),
				zap.Error(err))
			return "", err
		}
		// A persisted session repository may legitimately be absent from the
		// current task attachment set. Resolve that one exceptional repository,
		// but never re-resolve the complete task set.
		if err := e.ensureRepoLocalPathForSession(ctx, task.ID, session.ID, repository); err != nil {
			return "", err
		}
	}

	repositoryPath := repository.LocalPath
	applyResumeRepoBasics(req, repository, repositoryPath)
	for _, info := range allRepos {
		if info != nil && info.RepositoryID == repositoryID {
			req.ContributionDestination = info.ContributionDestination
			break
		}
	}

	if err := e.applyResumeCloneURL(req, repository, baseBranch); err != nil {
		return "", err
	}

	if shouldUseWorktree(req.ExecutorType) && repositoryPath != "" {
		e.applyResumeWorktreeConfig(ctx, task, req, repository, repositoryID, repositoryPath, baseBranch, existingEnv)
	}

	if err := e.applyResumeMultiRepoConfig(task, req, existingEnv, allRepos); err != nil {
		return repositoryID, err
	}

	return repositoryID, nil
}

func (e *Executor) resumeRepoSet(ctx context.Context, taskID string, resolved ...[]*repoInfo) ([]*repoInfo, error) {
	if len(resolved) > 0 {
		return resolved[0], nil
	}
	return e.resolveAllRepoInfo(ctx, taskID)
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

// resolveResumeBaseBranch prefers the current task-repository row when a
// recovery action changed it after the session was created. A session keeps a
// legacy primary base snapshot, but that snapshot must not overwrite an exact
// task-repository recovery choice on the next launch. If the task has multiple
// rows for the same repository, the legacy session value remains the only
// unambiguous choice because TaskSession predates exact row identity.
func resolveResumeBaseBranch(repositoryID, sessionBaseBranch string, repos []*repoInfo) string {
	var match *repoInfo
	for _, info := range repos {
		if info == nil || info.RepositoryID != repositoryID {
			continue
		}
		if match != nil {
			return sessionBaseBranch
		}
		match = info
	}
	if match != nil && strings.TrimSpace(match.BaseBranch) != "" {
		return match.BaseBranch
	}
	return sessionBaseBranch
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
	if req.WorkspaceReuseRequired || e.capabilities == nil || !e.capabilities.RequiresCloneURL(req.ExecutorType) {
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
//
// Gates on the resolved repository set's DB-backed count, not task.Repositories:
// task here is loaded via the raw repository GetTask (validateAndLockResume),
// which never attaches the one-to-many task_repositories rows — that field is
// always empty on this path. Gating on it silently dropped every repo but the
// primary on any resume of a multi-repo task.
func (e *Executor) applyResumeMultiRepoConfig(task *v1.Task, req *LaunchAgentRequest, existingEnv *models.TaskEnvironment, allRepos []*repoInfo) error {
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
	if primaryTaskRepo != nil && primaryTaskRepo.RepositoryID == repositoryID {
		req.TaskRepositoryID = primaryTaskRepo.ID
	}
	if primaryTaskRepo != nil && primaryTaskRepo.CheckoutBranch != "" {
		req.CheckoutBranch = primaryTaskRepo.CheckoutBranch
		req.PRNumber = prNumberFromMetadata(primaryTaskRepo.Metadata)
	}
	if primaryTaskRepo != nil && primaryTaskRepo.RepositoryID == repositoryID && req.ContributionDestination == nil {
		if destination, found, err := models.LoadContributionDestination(primaryTaskRepo.Metadata); err == nil && found {
			req.ContributionDestination = &destination
		}
	}
	req.WorktreeBranchPrefix = repository.WorktreeBranchPrefix
	req.WorktreeBranchTemplate = repository.WorktreeBranchTemplate
	if primaryTaskRepo != nil && primaryTaskRepo.RepositoryID == repositoryID && primaryTaskRepo.BranchPolicyBranchTemplate != "" {
		req.WorktreeBranchTemplate = primaryTaskRepo.BranchPolicyBranchTemplate
	}
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
	return worktree.SemanticWorktreeName(task.Title, worktree.TaskDirSuffix(task.ID))
}

// persistResumeState updates the session row for a resume launch. For an agent
// launch this is called before LaunchAgent so credential issuance observes the
// guarded STARTING state. Like persistLaunchState, executors_running is owned
// by the lifecycle manager and not touched here — see
// lifecycle.persistExecutorRunning. The orchestrator's remaining
// responsibility is the session-row state machine (STARTING / CompletedAt-clear).
func (e *Executor) persistResumeState(ctx context.Context, taskID string, session *models.TaskSession, startAgent bool) error {
	expectedState := session.State
	session.ErrorMessage = ""
	if startAgent {
		session.State = models.TaskSessionStateStarting
		session.CompletedAt = nil
	}

	var updateErr error
	if startAgent {
		updateErr = e.updateSessionStarting(ctx, taskID, session, expectedState, false)
	} else {
		updateErr = e.persistSessionFullRowIfCurrentState(ctx, session, expectedState)
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
	if e.onTaskRuntimeStateReconcile != nil {
		return e.onTaskRuntimeStateReconcile(ctx, taskID, sessionID, v1.TaskStateInProgress)
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task != nil && task.ArchivedAt != nil {
		e.logger.Debug("skipping IN_PROGRESS transition for archived task",
			zap.String("task_id", taskID))
		return nil
	}
	if task != nil && task.IsFromOffice {
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

func (e *Executor) writeTaskFailedForRuntime(ctx context.Context, taskID, sessionID string) error {
	if e.onTaskRuntimeStateReconcile != nil {
		return e.onTaskRuntimeStateReconcile(ctx, taskID, sessionID, v1.TaskStateFailed)
	}
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.State != models.TaskSessionStateFailed {
		return nil
	}
	return e.updateTaskState(ctx, taskID, v1.TaskStateFailed)
}
