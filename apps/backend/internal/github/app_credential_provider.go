package github

import (
	"context"
	"errors"
)

// CachedInstallationCredentialProvider resolves short-lived installation
// tokens without exposing App private-key material to callers or executors.
type CachedInstallationCredentialProvider struct {
	tokens *InstallationTokenCache
}

func NewCachedInstallationCredentialProvider(tokens *InstallationTokenCache) *CachedInstallationCredentialProvider {
	return &CachedInstallationCredentialProvider{tokens: tokens}
}

func (p *CachedInstallationCredentialProvider) ResolveInstallation(
	ctx context.Context,
	connection *WorkspaceConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if p == nil || p.tokens == nil {
		return nil, ErrGitHubNotConfigured
	}
	if connection == nil || connection.InstallationID == nil || *connection.InstallationID <= 0 {
		return nil, errors.New("GitHub App installation connection is missing an installation ID")
	}
	var repositories []string
	if req.RepoName != "" {
		repositories = []string{req.RepoName}
	}
	token, err := p.tokens.Get(ctx, *connection.InstallationID, nil, repositories)
	if err != nil {
		return nil, err
	}
	tracker := NewRateTracker(nil, nil)
	client := NewTokenClient(token.Token, token.Principal).WithRateTracker(tracker)
	return &ResolvedCredential{
		Client: client,
		Principal: AuthPrincipal{
			Kind:           AuthPrincipalApp,
			Source:         ConnectionSourceGitHubAppInstallation,
			Login:          connection.InstallationAccountLogin,
			InstallationID: *connection.InstallationID,
		},
		Capabilities: CapabilitiesForPermissions(token.Permissions),
		ExpiresAt:    token.ExpiresAt,
		RateTracker:  tracker,
		credential:   token.Token,
	}, nil
}
