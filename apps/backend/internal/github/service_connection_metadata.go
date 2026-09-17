package github

import "context"

// WorkspaceConnectionMetadata reads stored health without resolving credentials
// or initiating a provider request.
func (s *Service) WorkspaceConnectionMetadata(ctx context.Context, workspaceID string) (*WorkspaceConnection, error) {
	if workspaceID == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, ErrGitHubNotConfigured
	}
	return s.store.GetWorkspaceConnection(ctx, workspaceID)
}
