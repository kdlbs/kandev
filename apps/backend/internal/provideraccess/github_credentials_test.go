package provideraccess

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
)

type fakeGitHubTokenClient struct {
	token        github.InstallationToken
	mintErr      error
	revokeErr    error
	installation int64
	permissions  github.InstallationPermissions
	repositories []string
	revoked      string
}

func (f *fakeGitHubTokenClient) MintInstallationToken(
	_ context.Context, installation int64, permissions github.InstallationPermissions,
	repositories []string,
) (github.InstallationToken, error) {
	f.installation = installation
	f.permissions = permissions
	f.repositories = repositories
	return f.token, f.mintErr
}

func (f *fakeGitHubTokenClient) RevokeInstallationToken(_ context.Context, token string) error {
	f.revoked = token
	return f.revokeErr
}

func TestGitHubRerunCredentialsMintsExactUncachedScope(t *testing.T) {
	client := &fakeGitHubTokenClient{token: github.InstallationToken{
		Token: "secret", ExpiresAt: time.Now().Add(45 * time.Minute),
		Permissions: github.InstallationPermissions{
			"actions": github.PermissionWrite, "pull_requests": github.PermissionRead,
			"metadata": github.PermissionRead},
		Repositories: []github.InstallationTokenRepository{{FullName: "owner/base"}},
		Principal:    github.TokenPrincipal{InstallationID: 42},
	}}
	adapter := GitHubRerunCredentials{Client: client}
	if _, err := adapter.Mint(context.Background(), 42, "owner/base"); err != nil {
		t.Fatal(err)
	}
	if client.installation != 42 || !reflect.DeepEqual(client.repositories, []string{"base"}) ||
		!reflect.DeepEqual(client.permissions, client.token.Permissions) || client.revoked != "" {
		t.Fatalf("mint scope = installation %d, repos %+v, permissions %+v, revoked %q",
			client.installation, client.repositories, client.permissions, client.revoked)
	}
}

func TestGitHubRerunCredentialsRevokesOverbroadResponse(t *testing.T) {
	client := &fakeGitHubTokenClient{token: github.InstallationToken{
		Token: "secret", ExpiresAt: time.Now().Add(45 * time.Minute),
		Permissions: github.InstallationPermissions{
			"actions": github.PermissionWrite, "pull_requests": github.PermissionRead,
			"metadata": github.PermissionRead, "contents": github.PermissionWrite},
		Repositories: []github.InstallationTokenRepository{{FullName: "owner/base"}},
		Principal:    github.TokenPrincipal{InstallationID: 42},
	}}
	adapter := GitHubRerunCredentials{Client: client}
	if _, err := adapter.Mint(context.Background(), 42, "owner/base"); !errors.Is(err, ErrProviderTokenScope) ||
		client.revoked != "secret" {
		t.Fatalf("overbroad response = %v, revoked = %q", err, client.revoked)
	}
	client.revokeErr = errors.New("provider echoed secret")
	if _, err := adapter.Mint(context.Background(), 42, "owner/base"); !errors.Is(err, ErrRevocationUnconfirmed) {
		t.Fatalf("failed revoke = %v", err)
	} else if containsSecret(err.Error()) {
		t.Fatalf("revoke error leaked secret: %v", err)
	}
}

func containsSecret(text string) bool { return strings.Contains(text, "secret") }
