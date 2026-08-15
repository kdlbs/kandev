package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func gitCredentialScopeTestRepository(remoteURL string) *fakeGitHubBrokerTaskRepository {
	return &fakeGitHubBrokerTaskRepository{
		task:    &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		session: &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		repository: &taskmodels.Repository{
			ID: "repository-1", WorkspaceID: "workspace-1", Provider: "github",
			ProviderOwner: "acme", ProviderName: "widgets", RemoteURL: remoteURL,
		},
		links: []*taskmodels.TaskRepository{{TaskID: "task-1", RepositoryID: "repository-1"}},
	}
}

func gitCredentialScopeForPath(path string) gitcredentials.Scope {
	return gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: path,
	}
}

// The executor derives the lease scope from the repository's clone URL, so the
// authorizer must recognize the same repository however that column spells it:
// SSH or HTTPS, with or without the ".git" suffix the broker canonicalizes away.
func TestGitHubBrokerScopeAuthorizerAcceptsEquivalentRepositorySpellings(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		remoteURL string
		scopePath string
	}{
		{name: "scp style SSH remote", remoteURL: "git@github.com:acme/widgets.git", scopePath: "/acme/widgets"},
		{name: "ssh scheme remote", remoteURL: "ssh://git@github.com/acme/widgets.git", scopePath: "/acme/widgets"},
		{name: "https remote canonical scope", remoteURL: "https://github.com/acme/widgets.git", scopePath: "/acme/widgets"},
		{name: "https remote suffixed scope", remoteURL: "https://github.com/acme/widgets.git", scopePath: "/acme/widgets.git"},
		{name: "derived from provider columns", remoteURL: "", scopePath: "/acme/widgets"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			authorizer := &githubBrokerScopeAuthorizer{repo: gitCredentialScopeTestRepository(testCase.remoteURL)}

			if err := authorizer.AuthorizeGitCredential(
				context.Background(), gitCredentialScopeForPath(testCase.scopePath),
			); err != nil {
				t.Fatalf("AuthorizeGitCredential() error = %v", err)
			}
		})
	}
}

func TestGitHubBrokerScopeAuthorizerRejectsForeignRepositoryScope(t *testing.T) {
	for name, testCase := range map[string]struct {
		remoteURL string
		scopePath string
	}{
		"different repository": {remoteURL: "git@github.com:acme/widgets.git", scopePath: "/acme/widgets-staging"},
		"different host":       {remoteURL: "git@github.example:acme/widgets.git", scopePath: "/acme/widgets"},
		"unsupported remote":   {remoteURL: "file:///tmp/widgets.git", scopePath: "/acme/widgets"},
	} {
		t.Run(name, func(t *testing.T) {
			authorizer := &githubBrokerScopeAuthorizer{repo: gitCredentialScopeTestRepository(testCase.remoteURL)}

			if err := authorizer.AuthorizeGitCredential(
				context.Background(), gitCredentialScopeForPath(testCase.scopePath),
			); err == nil {
				t.Fatal("AuthorizeGitCredential() error = nil, want scope denial")
			}
		})
	}
}
