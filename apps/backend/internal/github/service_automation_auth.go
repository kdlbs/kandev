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
	return findPRByBranchInForkNetwork(ctx, resolved.Client, owner, repo, branch)
}

// findPRByBranchInForkNetwork first searches the requested repository. When
// that repository is a fork and no PR is found, it searches the parent for an
// open PR whose head is the exact fork owner and branch. This handles PRs that
// target the canonical repository while keeping same-named branches from
// another fork out of the association path.
func findPRByBranchInForkNetwork(
	ctx context.Context, client Client, owner, repo, branch string,
) (*PR, error) {
	pr, err := client.FindPRByBranch(ctx, owner, repo, branch)
	if err != nil || pr != nil {
		return pr, err
	}

	parentOwner, parentRepo, isFork, err := forkParentRepositoryForLookup(ctx, client, owner, repo)
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
	pr, err = finder.FindPRByHead(ctx, parentOwner, parentRepo, owner, repo, branch)
	if err != nil || pr == nil {
		return pr, err
	}
	if !sameRepositoryIdentity(pr.HeadRepoOwner, pr.HeadRepoName, owner, repo) {
		return nil, nil
	}
	return pr, nil
}

func forkParentRepositoryForLookup(
	ctx context.Context, client Client, owner, repo string,
) (parentOwner, parentRepo string, isFork bool, err error) {
	repositoryReader, ok := client.(repositoryDetailsReader)
	if !ok {
		return "", "", false, nil
	}
	repository, err := repositoryReader.GetRepository(ctx, owner, repo)
	if err != nil {
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("inspect repository %s/%s for parent PR lookup: %w", owner, repo, err)
	}
	if repository == nil || !repository.Fork {
		return "", "", false, nil
	}
	parentOwner, parentRepo, ok = parseForkParentFullName(repository.ParentFullName)
	if !ok || (strings.EqualFold(parentOwner, owner) && strings.EqualFold(parentRepo, repo)) {
		return "", "", false, nil
	}
	return parentOwner, parentRepo, true, nil
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
	mergeMethod string,
) error {
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
	return s.mergePRWithClient(
		ctx, resolved.Client, resolved.CacheScope, owner, repo, number, mergeMethod,
	)
}
