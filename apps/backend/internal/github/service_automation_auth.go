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
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	return s.findPRByBranchInForkNetwork(ctx, resolved.Client, resolved.CacheScope, owner, repo, branch)
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
