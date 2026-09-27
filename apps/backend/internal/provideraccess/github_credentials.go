package provideraccess

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/github"
)

var (
	ErrProviderMintUncertain = errors.New("provider access token mint outcome uncertain")
	ErrProviderTokenScope    = errors.New("provider access token scope mismatch")
)

// GitHubTokenClient is the uncached App-authenticated minter and exact-token
// revoker. A shared installation token cache cannot satisfy lease isolation.
type GitHubTokenClient interface {
	MintInstallationToken(context.Context, int64, github.InstallationPermissions, []string) (github.InstallationToken, error)
	RevokeInstallationToken(context.Context, string) error
}

// GitHubRerunCredentials requests only the installation permissions needed
// for direct plugin-side GitHub Actions reruns in one canonical base repository.
type GitHubRerunCredentials struct{ Client GitHubTokenClient }

func (g GitHubRerunCredentials) Mint(
	ctx context.Context, installationID int64, canonicalRepository string,
) (github.InstallationToken, error) {
	parts := strings.Split(canonicalRepository, "/")
	if g.Client == nil || installationID <= 0 || len(parts) != 2 ||
		parts[0] == "" || parts[1] == "" || canonicalRepository != strings.TrimSpace(canonicalRepository) {
		return github.InstallationToken{}, ErrProviderTokenScope
	}
	permissions := github.InstallationPermissions{
		"actions": github.PermissionWrite, "pull_requests": github.PermissionRead,
		"metadata": github.PermissionRead,
	}
	token, err := g.Client.MintInstallationToken(ctx, installationID, permissions, []string{parts[1]})
	if err != nil {
		return github.InstallationToken{}, ErrProviderMintUncertain
	}
	if validRerunToken(token, installationID, canonicalRepository, permissions) {
		return token, nil
	}
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if token.Token != "" && g.Client.RevokeInstallationToken(revokeCtx, token.Token) != nil {
		return github.InstallationToken{}, errors.Join(ErrProviderTokenScope, ErrRevocationUnconfirmed)
	}
	return github.InstallationToken{}, ErrProviderTokenScope
}

func validRerunToken(
	token github.InstallationToken, installationID int64, repository string,
	permissions github.InstallationPermissions,
) bool {
	if token.Token == "" || token.Principal.InstallationID != installationID ||
		len(token.Repositories) != 1 || token.Repositories[0].FullName != repository ||
		len(token.Permissions) != len(permissions) {
		return false
	}
	for name, level := range permissions {
		if token.Permissions[name] != level {
			return false
		}
	}
	now := time.Now()
	return token.ExpiresAt.After(now) && !token.ExpiresAt.After(now.Add(time.Hour+time.Minute))
}

func (g GitHubRerunCredentials) Revoke(ctx context.Context, token string) error {
	if g.Client == nil || token == "" {
		return ErrRevocationUnconfirmed
	}
	if err := g.Client.RevokeInstallationToken(ctx, token); err != nil {
		return ErrRevocationUnconfirmed
	}
	return nil
}
