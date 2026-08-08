package gitlab

import "context"

// ListRepoTreeForWorkspace lists one repository directory for the workspace's
// configured GitLab connection, resolving host, auth method, and credential
// through ClientForWorkspace — including self-managed hosts, with no
// additional configuration needed here.
func (s *Service) ListRepoTreeForWorkspace(
	ctx context.Context, workspaceID, projectPath, path, ref string,
) ([]RepoTreeEntry, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListRepoTree(ctx, projectPath, path, ref)
}

// GetRepoFileContentForWorkspace returns the raw bytes of a repository file
// for the workspace's configured GitLab connection.
func (s *Service) GetRepoFileContentForWorkspace(
	ctx context.Context, workspaceID, projectPath, path, ref string,
) ([]byte, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetRepoFileContent(ctx, projectPath, path, ref)
}
