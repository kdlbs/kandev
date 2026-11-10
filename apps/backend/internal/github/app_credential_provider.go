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
	if p.tokens.registrationID != "" && connection.AppRegistrationID != p.tokens.registrationID {
		return nil, ErrGitHubNotConfigured
	}
	host, err := connectionHost(connection.GitHubHost)
	if err != nil {
		return nil, err
	}
	if app, ok := p.tokens.minter.(*AppClient); ok {
		if app == nil {
			return nil, ErrGitHubNotConfigured
		}
		if app.host != host {
			return nil, ErrGitHubHostMismatch
		}
	}
	var repositories []string
	if req.RepoName != "" {
		repositories = []string{req.RepoName}
	}
	token, err := p.tokens.GetForWorkspace(
		ctx, connection.WorkspaceID, *connection.InstallationID, nil, repositories,
	)
	if err != nil {
		return nil, err
	}
	tracker := NewRateTracker(nil, nil)
	client := NewTokenClientForHost(token.Token, token.Principal, host).WithRateTracker(tracker)
	return &ResolvedCredential{
		Client:     client,
		GitHubHost: host.String(),
		Principal: AuthPrincipal{
			GitHubHost: host.String(),
			Kind:       AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			Login: connection.InstallationAccountLogin, InstallationID: *connection.InstallationID,
			AppRegistrationID:       p.tokens.registrationID,
			AppCredentialGeneration: p.tokens.credentialGeneration,
		},
		Capabilities: CapabilitiesForPermissions(token.Permissions), ExpiresAt: token.ExpiresAt,
		AppRegistrationID:       p.tokens.registrationID,
		AppCredentialGeneration: p.tokens.credentialGeneration,
		RateTracker:             tracker, credential: token.Token,
	}, nil
}
