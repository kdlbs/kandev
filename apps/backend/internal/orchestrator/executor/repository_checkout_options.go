package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

func (e *Executor) ensureTaskCheckoutPath(ctx context.Context, taskID, sessionID string, repo *models.Repository, options *models.RepositoryCheckoutOptions) (repoclone.RemoteRefState, error) {
	if options == nil || (options.DownloadMode == models.DownloadStandard && len(options.SparseDirectories) == 0) {
		return e.ensureRepoLocalPathForSessionAndState(ctx, taskID, sessionID, repo)
	}
	policy, policyErr := e.resolveTaskGitCredentialPolicy(ctx, repo.WorkspaceID)
	if policyErr != nil {
		return repoclone.RemoteRefStateUnknown, policyErr
	}
	if policy.Mode != taskGitCredentialsModeManaged {
		return repoclone.RemoteRefStateUnknown, fmt.Errorf("repository checkout options require Kandev-managed Git credentials")
	}
	if repo.SourceType == sourceTypeLocal || !isGitHubRepository(repo) || e.repoCloner == nil {
		return repoclone.RemoteRefStateUnknown, fmt.Errorf("repository checkout options require a managed GitHub repository")
	}
	cloneURL := repositoryCloneURL(repo)
	if cloneURL == "" {
		return repoclone.RemoteRefStateUnknown, ErrNoCloneURL
	}
	request := repositoryGitCredentialRequest(taskID, sessionID, repo, cloneURL)
	request.CheckoutOptions = options
	cloner, ok := e.repoCloner.(remoteStateRepoCloner)
	if !ok {
		return repoclone.RemoteRefStateUnknown, fmt.Errorf("repository checkout options are unavailable")
	}
	path, state, err := cloner.EnsureWorkspaceClonedWithCredentialRequestAndState(ctx, request, "", "")
	if err == nil {
		repo.LocalPath = path
	}
	return state, err
}
