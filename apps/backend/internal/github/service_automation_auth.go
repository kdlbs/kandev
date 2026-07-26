package github

import "context"

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
	return resolved.Client.FindPRByBranch(ctx, owner, repo, branch)
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
	return s.getPRFeedback(ctx, resolved.Client, resolved.CacheScope, owner, repo, number)
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
