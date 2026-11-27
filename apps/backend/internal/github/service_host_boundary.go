package github

import (
	"context"

	"github.com/kandev/kandev/internal/common/githubhost"
)

// BindWorkspaceGitHubHost captures the connection identity for a multi-read operation.
// CommitWorkspaceGitHubHost must guard any local writes derived from those reads.
func (s *Service) BindWorkspaceGitHubHost(ctx context.Context, workspaceID, expectedHost string) (context.Context, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return ctx, err
	}
	if expectedHost == "" {
		expectedHost = defaultGitHubHost
	}
	expected, err := githubhost.Parse(expectedHost)
	if err != nil {
		return ctx, err
	}
	credential := &ResolvedCredential{GitHubHost: defaultGitHubHost}
	if s.resolver != nil && s.resolver.connections != nil {
		connection, readErr := s.resolver.connections.GetWorkspaceConnection(ctx, workspaceID)
		if readErr != nil {
			return ctx, readErr
		}
		if connection != nil {
			credential.CredentialGeneration = connection.CredentialGeneration
			credential.WorkspaceCredentialGeneration = connection.CredentialGeneration
			if connection.GitHubHost != "" {
				credential.GitHubHost = connection.GitHubHost
			}
		}
	}
	actual, err := githubhost.Parse(credential.GitHubHost)
	if err != nil || actual != expected {
		return ctx, ErrGitHubHostMismatch
	}
	ctx = withExpectedGitHubHost(ctx, expected.String())
	return withGitHubOperationFence(ctx, s, workspaceID, &resolvedServiceClient{credential: credential}), nil
}

// CommitWorkspaceGitHubHost serializes a captured operation's local writes with connection replacement.
func (s *Service) CommitWorkspaceGitHubHost(ctx context.Context, workspaceID string, apply func(context.Context) error) error {
	fence, ok := ctx.Value(githubOperationFenceKey{}).(githubOperationFence)
	if !ok || fence.service != s || fence.workspaceID != workspaceID {
		return ErrGitHubHostMismatch
	}
	ctx, unlock, err := s.lockResourceMutation(ctx, workspaceID)
	if err != nil {
		return err
	}
	defer unlock()
	return apply(ctx)
}
