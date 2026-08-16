package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/task/models"
)

// A repository cloned over SSH stores an SSH remote_url, which names the same
// repository as its HTTPS form. Managed credentials derive their scope from
// that column, so rejecting the SSH spelling failed every launch for local
// repositories kandev itself backfilled from an SSH checkout.
func TestGitCredentialCloneIdentityAcceptsSSHRemoteURL(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		remoteURL string
		wantHost  string
		wantPath  string
	}{
		{
			name:      "scp style",
			remoteURL: "git@github.com:acme/widgets.git",
			wantHost:  "github.com",
			wantPath:  "/acme/widgets.git",
		},
		{
			name:      "ssh scheme",
			remoteURL: "ssh://git@github.com/acme/widgets.git",
			wantHost:  "github.com",
			wantPath:  "/acme/widgets.git",
		},
		{
			name:      "https stays untouched",
			remoteURL: "https://github.com/acme/widgets.git",
			wantHost:  "github.com",
			wantPath:  "/acme/widgets.git",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &models.Repository{
				SourceType: sourceTypeLocal, Provider: "github",
				ProviderOwner: "acme", ProviderName: "widgets", RemoteURL: testCase.remoteURL,
			}

			host, path, owner, repo, err := gitCredentialCloneIdentity(repository, "repo-1")
			if err != nil {
				t.Fatalf("gitCredentialCloneIdentity() error = %v", err)
			}
			if host != testCase.wantHost || path != testCase.wantPath {
				t.Fatalf("clone identity = %q %q, want %q %q", host, path, testCase.wantHost, testCase.wantPath)
			}
			if owner != "acme" || repo != "widgets" {
				t.Fatalf("clone identity owner/repo = %q/%q, want acme/widgets", owner, repo)
			}
		})
	}
}

// An SSH URL cannot express the provider's HTTPS port, so a repository whose
// provider runs HTTPS on a non-default port must take its credential identity
// from the persisted provider host rather than from a rewrite of the remote.
func TestGitCredentialCloneIdentityPrefersProviderHostOverSSHRemote(t *testing.T) {
	repository := &models.Repository{
		Provider: "github", ProviderHost: "https://ghe.example:8443",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "ssh://git@ghe.example:2222/acme/widgets.git",
	}

	host, path, owner, repo, err := gitCredentialCloneIdentity(repository, "repo-1")
	if err != nil {
		t.Fatalf("gitCredentialCloneIdentity() error = %v", err)
	}
	if host != "ghe.example:8443" || path != "/acme/widgets.git" {
		t.Fatalf("clone identity = %q %q, want ghe.example:8443 /acme/widgets.git", host, path)
	}
	if owner != "acme" || repo != "widgets" {
		t.Fatalf("clone identity owner/repo = %q/%q, want acme/widgets", owner, repo)
	}
}

func TestGitCredentialCloneIdentityRejectsUnsupportedRemoteURL(t *testing.T) {
	for name, remoteURL := range map[string]string{
		"local file":     "file:///tmp/repository.git",
		"plain http":     "http://github.com/acme/widgets.git",
		"embedded token": "https://token@github.com/acme/widgets.git",
	} {
		t.Run(name, func(t *testing.T) {
			repository := &models.Repository{Provider: "github", RemoteURL: remoteURL}
			if _, _, _, _, err := gitCredentialCloneIdentity(repository, "repo-1"); err == nil {
				t.Fatalf("gitCredentialCloneIdentity(%q) error = nil, want rejection", remoteURL)
			}
		})
	}
}

func TestConfigureGitCredentialBrokerIssuesLeaseForSSHRemoteURL(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		SourceType: sourceTypeLocal, Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "git@github.com:acme/widgets.git",
	}}

	if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("Issue calls = %d, want 1", issuer.calls)
	}
	if got := issuer.request; got.Host != "github.com" || got.Path != "/acme/widgets.git" {
		t.Fatalf("lease scope = %#v", got)
	}
	if got := req.Env[envGitHubCredentialOwner]; got != "acme" {
		t.Fatalf("credential owner = %q, want acme", got)
	}
	if got := req.Env[envGitHubCredentialScopes]; !strings.Contains(got, `"path":"/acme/widgets.git"`) {
		t.Fatalf("credential scopes = %q, want HTTPS scope path", got)
	}
}
