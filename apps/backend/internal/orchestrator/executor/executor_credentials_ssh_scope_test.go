package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/task/models"
)

// sshRemoteScopeCase is one spelling a checkout cloned over SSH can leave in
// repositories.remote_url. All four name the same repository, so all four must
// resolve the same managed-credential scope. The `.git` suffix is deliberately
// mixed: the persisted provider identity is authoritative, so the scope path is
// the provider's canonical one regardless of how the remote was spelled. That
// is what keeps the bare/suffixed split (#2675) out of the credential scope.
type sshRemoteScopeCase struct {
	name      string
	remoteURL string
}

const sshRemoteScopePath = "/acme/widgets.git"

func sshRemoteScopeCases() []sshRemoteScopeCase {
	return []sshRemoteScopeCase{
		{name: "scp style suffixed", remoteURL: "git@github.com:acme/widgets.git"},
		{name: "scp style bare", remoteURL: "git@github.com:acme/widgets"},
		{name: "ssh scheme suffixed", remoteURL: "ssh://git@github.com/acme/widgets.git"},
		{name: "ssh scheme bare", remoteURL: "ssh://git@github.com/acme/widgets"},
	}
}

// The regression in #2673 was not that a helper returned the wrong string: it
// was that a repository whose remote_url holds an SSH URL, but which carries a
// valid provider owner/name, stopped being issued a managed credential scope at
// all. Assert the issued lease, not the URL rewrite, so a future refactor that
// re-tightens the scheme check fails here rather than in production.
func TestConfigureGitCredentialBrokerIssuesScopeForEverySSHRemoteSpelling(t *testing.T) {
	for _, testCase := range sshRemoteScopeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
			exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
			req := &LaunchAgentRequest{
				TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
				ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
			}
			info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
				SourceType: sourceTypeLocal, Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
				RemoteURL: testCase.remoteURL,
			}}

			if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
				t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
			}

			if issuer.calls != 1 {
				t.Fatalf("Issue calls = %d, want 1 lease issued for remote %q", issuer.calls, testCase.remoteURL)
			}
			if got := issuer.request; got.Host != "github.com" || got.Path != sshRemoteScopePath {
				t.Fatalf("lease scope host/path = %q %q, want %q %q", got.Host, got.Path, "github.com", sshRemoteScopePath)
			}
			if got := req.Env[envGitHubCredentialOwner]; got != "acme" {
				t.Fatalf("credential owner = %q, want acme", got)
			}
			if got := req.Env[envGitHubCredentialScopes]; !strings.Contains(got, `"path":"`+sshRemoteScopePath+`"`) {
				t.Fatalf("credential scopes = %q, want HTTPS scope path %q", got, sshRemoteScopePath)
			}
		})
	}
}

// The provider columns are the identity of record. #2117 moved this path onto
// repositoryCloneURL, which reads remote_url first, and an SSH value there then
// failed the HTTPS assertion. Pin that an SSH remote and its HTTPS twin produce
// byte-identical scopes, so reading the transport-bearing column can never
// again change the credential the launch receives.
func TestGitCredentialScopeIsIndependentOfRemoteTransport(t *testing.T) {
	scopeFor := func(t *testing.T, remoteURL string) string {
		t.Helper()
		issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
		exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
		exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
		req := &LaunchAgentRequest{
			TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
			ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
		}
		info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
			SourceType: sourceTypeLocal, Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
			RemoteURL: remoteURL,
		}}
		if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
			t.Fatalf("configureGitCredentialBrokerForRepositories(%q) error = %v", remoteURL, err)
		}
		return req.Env[envGitHubCredentialScopes]
	}

	https := scopeFor(t, "https://github.com/acme/widgets.git")
	// Driven from sshRemoteScopeCases() rather than a hand-listed subset. All
	// four spellings agree today only because the provider identity wins; the
	// fallback that rewrites the remote in place does not collapse the `.git`
	// suffix, so a regression demoting the provider columns makes the bare forms
	// diverge from the suffixed ones. A hand-listed pair of suffixed remotes
	// cannot see that.
	for _, testCase := range sshRemoteScopeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			if got := scopeFor(t, testCase.remoteURL); got != https {
				t.Fatalf("scopes for %q = %q, want the HTTPS-remote scopes %q", testCase.remoteURL, got, https)
			}
		})
	}
}

// Accepting SSH remotes must not have widened which hosts a managed credential
// can be minted for. A repository with no provider owner/name falls back to
// rewriting its remote, and that rewrite is still checked against the provider
// origin: an SSH remote pointing somewhere else is rejected before any lease is
// issued, so a mis-backfilled remote_url cannot redirect a GitHub credential.
func TestConfigureGitCredentialBrokerRejectsSSHRemoteFromForeignOrigin(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		SourceType: sourceTypeLocal, Provider: "github",
		RemoteURL: "git@evil.example:acme/widgets.git",
	}}

	err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info})
	if err == nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = nil, want rejection of a foreign SSH origin")
	}
	if issuer.calls != 0 {
		t.Fatalf("Issue calls = %d, want no lease issued for a foreign origin", issuer.calls)
	}
}

// An SSH remote can carry a non-default SSH port, which names the SSH daemon
// and not the HTTPS origin. The rewrite drops it, so a repository reachable at
// ssh://…:2222 is still scoped to the provider's ordinary HTTPS origin rather
// than to a port that would never match a credential helper.
func TestConfigureGitCredentialBrokerDropsSSHPortFromScope(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		SourceType: sourceTypeLocal, Provider: "github",
		RemoteURL: "ssh://git@github.com:2222/acme/widgets.git",
	}}

	if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	if got := issuer.request; got.Host != "github.com" || got.Path != sshRemoteScopePath {
		t.Fatalf("lease scope host/path = %q %q, want %q %q", got.Host, got.Path, "github.com", sshRemoteScopePath)
	}
}
