package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/gitcredentials"
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

// ResolveGitCredentialForHost resolves credentials for an explicit clone authority.
func (s *Service) ResolveGitCredentialForHost(ctx context.Context, workspaceID, provider, host, owner, repo string) (string, string, error) {
	if s == nil || s.resolver == nil || !strings.EqualFold(provider, githubProviderName) {
		return "", "", ErrGitHubNotConfigured
	}
	scope := gitcredentials.Scope{ProviderID: githubProviderName, WorkspaceID: workspaceID, Host: host, Path: githubCredentialPath(owner, repo)}
	resolver := &gitHubCredentialProvider{connections: s.resolver.connections, resolver: s.resolver}
	before, err := resolver.Binding(ctx, scope)
	if err != nil {
		return "", "", err
	}
	credential, err := resolver.Resolve(ctx, scope)
	if err != nil {
		return "", "", err
	}
	after, err := resolver.Binding(ctx, scope)
	if err != nil {
		return "", "", err
	}
	if before != after {
		return "", "", ErrCredentialLeaseRevoked
	}
	return credential.Username, credential.Password, nil
}
