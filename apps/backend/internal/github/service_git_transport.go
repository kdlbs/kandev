package github

import (
	"context"
	"fmt"
	"strings"
)

// ResolveGitCredential resolves the workspace automation credential for a
// repository transport operation. Personal credentials are never considered.
func (s *Service) ResolveGitCredential(
	ctx context.Context,
	workspaceID, provider, owner, repo string,
) (string, string, error) {
	if !strings.EqualFold(strings.TrimSpace(provider), "github") {
		return "", "", fmt.Errorf("unsupported Git credential provider %q", provider)
	}
	if s == nil || s.resolver == nil {
		return "", "", ErrGitHubNotConfigured
	}
	resolved, err := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID,
		Purpose:     CredentialPurposeGitTransport,
		RepoOwner:   owner,
		RepoName:    repo,
	})
	if err != nil {
		return "", "", err
	}
	if resolved == nil || strings.TrimSpace(resolved.credential) == "" {
		return "", "", ErrGitHubNotConfigured
	}
	if resolved.Principal.Kind == AuthPrincipalApp && !resolved.Capabilities[CapabilityGitRead] {
		return "", "", fmt.Errorf("%w: %s", ErrGitHubCapabilityDenied, CapabilityGitRead)
	}
	return gitHubTokenUsername, resolved.credential, nil
}
