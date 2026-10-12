package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// exactHeadPRFinder is implemented by provider clients that can constrain a
// pull-request search to both the source repository and branch. A branch name
// is not unique across a fork network, so a parent-repository fallback must
// preserve the source repository identity.
type exactHeadPRFinder interface {
	FindPRByHead(ctx context.Context, owner, repo, headOwner, headRepo, branch string) (*PR, error)
}

type repositoryDetailsReader interface {
	GetRepository(ctx context.Context, owner, repo string) (*GitHubRepository, error)
}

// ResolveGitHubAutomationClient returns the workspace-owned automation client
// for non-repository GitHub operations such as Gist-backed task sharing.
func (s *Service) ResolveGitHubAutomationClient(ctx context.Context, workspaceID string) (Client, error) {
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		return nil, err
	}
	return resolved.Client, nil
}

func (s *Service) FindPRByBranchForWorkspace(
	ctx context.Context,
	workspaceID, owner, repo, branch string,
) (*PR, error) {
	return s.findPRByBranchForWorkspace(ctx, workspaceID, owner, repo, branch, nil)
}

// findPRByBranchForWatch is the watch-owned variant used by the legacy poller.
// Keeping the watch identity in the health admission lets a failed direct
// probe be pruned when the watch is deleted, even when GraphQL batching is not
// available.
func (s *Service) findPRByBranchForWatch(ctx context.Context, watch *PRWatch) (*PR, error) {
	if watch == nil {
		return nil, ErrGitHubWorkspaceRequired
	}
	return s.findPRByBranchForWorkspace(ctx, watch.WorkspaceID, watch.Owner, watch.Repo, watch.Branch, watch)
}

func (s *Service) findPRByBranchForWorkspace(
	ctx context.Context,
	workspaceID, owner, repo, branch string, watch *PRWatch,
) (*PR, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	credentialGeneration := int64(0)
	if resolved.credential != nil {
		credentialGeneration = resolved.credential.CredentialGeneration
	}
	tracked := watch != nil && watch.ID != ""
	var attempt prDiscoveryWatchAttempt
	if tracked {
		var admitted bool
		attempt, admitted = s.beginPRDiscoveryWatch(workspaceID, resolved.CacheScope, credentialGeneration, watch)
		if !admitted {
			return nil, nil
		}
	}
	pr, err := s.resolvePRByBranchAttempt(
		ctx, resolved.Client, resolved.CacheScope, owner, repo, branch, attempt,
	)
	if err != nil {
		if tracked {
			category := classifyPRDiscoveryError(err)
			s.completePRDiscoveryWatchAttempt(
				attempt, nil, false, true, err,
				category == PRDiscoveryHealthInvalidQuery || category == PRDiscoveryHealthRateLimited,
			)
			s.finishPRDiscoveryWatchFailure(workspaceID, resolved.CacheScope, credentialGeneration, attempt, err)
			s.forgetPRDiscoveryWatchAttempt(workspaceID, resolved.CacheScope, credentialGeneration, attempt)
		}
		return nil, err
	}
	if tracked {
		if !attempt.joined {
			s.completePRDiscoveryWatchAttempt(attempt, &PRStatus{PR: pr}, true, false, nil, false)
			s.finishPRDiscoveryWatchSuccess(workspaceID, resolved.CacheScope, credentialGeneration, attempt)
			s.forgetPRDiscoveryWatchAttempt(workspaceID, resolved.CacheScope, credentialGeneration, attempt)
		}
		if pr != nil {
			// Finish the immutable source attempt first, then move the live
			// consumer to the discovered repository/PR target. The caller still
			// owns persistence, but a successful fork lookup must not leave the
			// old fork key retaining the watch forever.
			rebound := *watch
			rebound.Owner = pr.RepoOwner
			rebound.Repo = pr.RepoName
			rebound.PRNumber = pr.Number
			s.trackPRDiscoveryWatchConsumer(
				workspaceID, resolved.CacheScope, credentialGeneration, &rebound,
			)
		}
	}
	return pr, nil
}

func (s *Service) resolvePRByBranchAttempt(
	ctx context.Context, client Client, cacheScope, owner, repo, branch string,
	attempt prDiscoveryWatchAttempt,
) (*PR, error) {
	if attempt.joined {
		return sharedPRDiscoveryResult(ctx, attempt)
	}
	return s.findPRByBranchInForkNetwork(ctx, client, cacheScope, owner, repo, branch)
}

// findPRByBranchInForkNetwork first searches the requested repository. When
// that repository is a fork and no PR is found, it searches the parent for an
// open PR whose head is the exact fork owner and branch. This handles PRs that
// target the canonical repository while keeping same-named branches from
// another fork out of the association path.
func (s *Service) findPRByBranchInForkNetwork(
	ctx context.Context, client Client, cacheScope, owner, repo, branch string,
) (*PR, error) {
	pr, err := client.FindPRByBranch(ctx, owner, repo, branch)
	if err != nil || pr != nil {
		return pr, err
	}
	return s.findPRInForkParent(ctx, client, cacheScope, owner, repo, branch)
}

// findPRInForkParent is the parent-repository half of the fork-network lookup.
// It is exported to the package for callers that already hold a definitive
// "no open PR in the requested repository" answer (see
// lookupSearchingWatchPR), so they can reach the parent without paying for a
// direct lookup whose result they already know.
func (s *Service) findPRInForkParent(
	ctx context.Context, client Client, cacheScope, owner, repo, branch string,
) (*PR, error) {
	parentOwner, parentRepo, isFork, err := s.forkParentRepositoryForLookup(ctx, client, cacheScope, owner, repo)
	if err != nil {
		return nil, err
	}
	if !isFork {
		return nil, nil
	}

	finder, ok := client.(exactHeadPRFinder)
	if !ok {
		return nil, nil
	}
	pr, err := finder.FindPRByHead(ctx, parentOwner, parentRepo, owner, repo, branch)
	if err != nil || pr == nil {
		return pr, err
	}
	if !sameRepositoryIdentity(pr.HeadRepoOwner, pr.HeadRepoName, owner, repo) {
		return nil, nil
	}
	return pr, nil
}

// forkParent is the cached outcome of one fork-parent resolution.
type forkParent struct {
	Owner  string
	Repo   string
	IsFork bool
}

// forkParentRepositoryForLookup resolves whether (owner, repo) is a fork and,
// if so, which repository it forked from.
//
// The answer is cached per (credential scope, owner, repo): fork status is
// effectively immutable, but this runs once per searching watch per sync cycle,
// which meant one `gh api /repos/...` subprocess per watch per minute against a
// constant answer. Keyed by scope like the sibling caches so a repository
// resolved under one principal's credential is never served to another's.
func (s *Service) forkParentRepositoryForLookup(
	ctx context.Context, client Client, cacheScope, owner, repo string,
) (parentOwner, parentRepo string, isFork bool, err error) {
	repositoryReader, ok := client.(repositoryDetailsReader)
	if !ok {
		return "", "", false, nil
	}
	if s == nil || s.forkParentCache == nil {
		resolved, resolveErr := resolveForkParent(ctx, repositoryReader, owner, repo)
		if resolveErr != nil {
			return "", "", false, resolveErr
		}
		return resolved.Owner, resolved.Repo, resolved.IsFork, nil
	}
	key := scopedCacheKey(cacheScope, repoErrorCacheKey(owner, repo))
	v, err := s.forkParentCache.doOrFetch(key, func() (any, error) {
		return resolveForkParent(ctx, repositoryReader, owner, repo)
	})
	if err != nil {
		return "", "", false, err
	}
	resolved, ok := v.(forkParent)
	if !ok {
		return "", "", false, nil
	}
	return resolved.Owner, resolved.Repo, resolved.IsFork, nil
}

func resolveForkParent(
	ctx context.Context, repositoryReader repositoryDetailsReader, owner, repo string,
) (forkParent, error) {
	repository, err := repositoryReader.GetRepository(ctx, owner, repo)
	if err != nil {
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return forkParent{}, nil
		}
		return forkParent{}, fmt.Errorf("inspect repository %s/%s for parent PR lookup: %w", owner, repo, err)
	}
	if repository == nil || !repository.Fork {
		return forkParent{}, nil
	}
	parentOwner, parentRepo, ok := parseForkParentFullName(repository.ParentFullName)
	if !ok || (strings.EqualFold(parentOwner, owner) && strings.EqualFold(parentRepo, repo)) {
		return forkParent{}, nil
	}
	return forkParent{Owner: parentOwner, Repo: parentRepo, IsFork: true}, nil
}

func parseForkParentFullName(fullName string) (owner, repo string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(fullName), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	owner, repo = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	return owner, repo, owner != "" && repo != ""
}

func sameRepositoryIdentity(ownerA, repoA, ownerB, repoB string) bool {
	return strings.EqualFold(strings.TrimSpace(ownerA), strings.TrimSpace(ownerB)) &&
		strings.EqualFold(strings.TrimSpace(repoA), strings.TrimSpace(repoB))
}

func (s *Service) GetPRFeedbackForAutomation(
	ctx context.Context,
	workspaceID, owner, repo string,
	number int,
) (*PRFeedback, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	return s.getPRFeedback(ctx, resolved.Client, resolved.CacheScope, workspaceID, owner, repo, number)
}

func (s *Service) MergePRForAutomation(
	ctx context.Context,
	workspaceID, owner, repo string,
	number int,
	mergeMethod, expectedHeadSHA string,
) error {
	if strings.TrimSpace(expectedHeadSHA) == "" {
		return fmt.Errorf("automatic merge requires an expected head SHA")
	}
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return err
	}
	if err := requireGitHubCapability(resolved, CapabilityPullRequestWrite); err != nil {
		return err
	}
	_, err = s.mergePRWithClient(
		ctx, resolved.Client, resolved.CacheScope, owner, repo, number, MergePRRequest{
			MergeMethod: mergeMethod, ExpectedHeadSHA: expectedHeadSHA,
		},
	)
	return err
}
