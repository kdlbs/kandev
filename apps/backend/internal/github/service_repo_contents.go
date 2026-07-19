package github

import "context"

func (s *Service) ListRepoDirectoryForWorkspace(
	ctx context.Context, workspaceID, owner, repo, path, ref string,
) ([]RepoContentEntry, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.ListRepoDirectory(ctx, owner, repo, path, ref)
}

func (s *Service) GetRepoFileContentForWorkspace(
	ctx context.Context, workspaceID, owner, repo, path, ref string,
) ([]byte, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.GetRepoFileContent(ctx, owner, repo, path, ref)
}
