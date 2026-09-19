package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func gitCredentialScopeTestRepository(remoteURL string) *fakeGitHubBrokerTaskRepository {
	repository := gitCredentialScopeRepository(remoteURL)
	repository.ProviderOwner = "acme"
	repository.ProviderName = "widgets"
	return &fakeGitHubBrokerTaskRepository{
		task:       &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		session:    &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		repository: repository,
		links:      []*taskmodels.TaskRepository{{TaskID: "task-1", RepositoryID: "repository-1"}},
	}
}

func gitCredentialScopeRepository(remoteURL string) *taskmodels.Repository {
	return &taskmodels.Repository{
		ID: "repository-1", WorkspaceID: "workspace-1", Provider: "github", RemoteURL: remoteURL,
	}
}

func gitCredentialScopeTaskRepository(repository *taskmodels.Repository) *fakeGitHubBrokerTaskRepository {
	return &fakeGitHubBrokerTaskRepository{
		task:       &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		session:    &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		repository: repository,
		links:      []*taskmodels.TaskRepository{{TaskID: "task-1", RepositoryID: "repository-1"}},
	}
}

func gitCredentialScopeForPath(path string) gitcredentials.Scope {
	return gitCredentialScopeForHostPath("github.com", path)
}

func gitCredentialScopeForHostPath(host, path string) gitcredentials.Scope {
	return gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: host, Path: path,
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
		repository *taskmodels.Repository
		scopePath  string
	}{
		"different repository": {
			repository: gitCredentialScopeTestRepository("git@github.com:acme/widgets.git").repository,
			scopePath:  "/acme/widgets-staging",
		},
		"ssh remote on another host, no provider identity": {
			repository: gitCredentialScopeRepository("git@github.example:acme/widgets.git"),
			scopePath:  "/acme/widgets",
		},
		"unsupported remote, no provider identity": {
			repository: gitCredentialScopeRepository("file:///tmp/widgets.git"),
			scopePath:  "/acme/widgets",
		},
		"https remote on another host wins over provider identity": {
			repository: gitCredentialScopeTestRepository("https://github.example/acme/widgets.git").repository,
			scopePath:  "/acme/widgets",
		},
	} {
		t.Run(name, func(t *testing.T) {
			authorizer := &githubBrokerScopeAuthorizer{repo: gitCredentialScopeTaskRepository(testCase.repository)}

			if err := authorizer.AuthorizeGitCredential(
				context.Background(), gitCredentialScopeForPath(testCase.scopePath),
			); err == nil {
				t.Fatal("AuthorizeGitCredential() error = nil, want scope denial")
			}
		})
	}
}

func TestGitHubBrokerScopeAuthorizerRechecksTaskScopeBeforeLegacyFallback(t *testing.T) {
	for name, mutate := range map[string]func(*fakeGitHubBrokerTaskRepository){
		"terminal session": func(repo *fakeGitHubBrokerTaskRepository) {
			repo.session.State = taskmodels.TaskSessionStateCompleted
		},
		"session for another task": func(repo *fakeGitHubBrokerTaskRepository) {
			repo.session.TaskID = "other-task"
		},
		"unlinked repository": func(repo *fakeGitHubBrokerTaskRepository) {
			repo.links = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := gitCredentialScopeTestRepository("https://github.com/acme/widgets.git")
			mutate(repo)
			authorizer := &githubBrokerScopeAuthorizer{repo: repo}

			if err := authorizer.AuthorizeGitCredential(
				context.Background(), gitCredentialScopeForPath("/acme/widgets.git"),
			); err == nil {
				t.Fatal("AuthorizeGitCredential() error = nil, want live task scope denial")
			}
		})
	}
}

func TestGitHubBrokerScopeAuthorizerDoesNotBypassProviderIdentityOnLegacyFallback(t *testing.T) {
	for name, identity := range map[string]gitcredentials.Scope{
		"mismatched provider identity": {
			IdentityProviderID: "999",
		},
		"unexpected parent identity": {
			IdentityProviderID: "100", ParentProviderID: "200",
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := gitCredentialScopeTestRepository("https://github.com/acme/widgets.git")
			repo.repository.ProviderHost = ""
			repo.repository.ProviderRepoID = "100"
			authorizer := &githubBrokerScopeAuthorizer{repo: repo}
			scope := gitCredentialScopeForPath("/acme/widgets.git")
			scope.IdentityProviderID = identity.IdentityProviderID
			scope.ParentProviderID = identity.ParentProviderID

			if err := authorizer.AuthorizeGitCredential(context.Background(), scope); err == nil {
				t.Fatal("AuthorizeGitCredential() error = nil, want provider identity denial")
			}
		})
	}
}

func TestGitHubBrokerScopeAuthorizerAcceptsPluginRepositoryScope(t *testing.T) {
	repository := &taskmodels.Repository{
		ID: "repository-1", WorkspaceID: "workspace-1", Provider: "bitbucket",
		ProviderHost: "https://bitbucket.example", RemoteURL: "https://bitbucket.example/scm/ENG/widgets.git",
	}
	authorizer := &githubBrokerScopeAuthorizer{repo: gitCredentialScopeTaskRepository(repository)}

	if err := authorizer.AuthorizeGitCredential(context.Background(), gitcredentials.Scope{
		ProviderID: "bitbucket", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	}); err != nil {
		t.Fatalf("AuthorizeGitCredential() error = %v", err)
	}
}

// An SSH URL cannot express the provider's HTTPS port, so the persisted
// provider identity, not a rewrite of the remote, decides the origin.
func TestGitHubBrokerScopeAuthorizerUsesProviderHostPortForSSHRemote(t *testing.T) {
	repository := gitCredentialScopeRepository("ssh://git@ghe.example:2222/acme/widgets.git")
	repository.ProviderHost = "https://ghe.example:8443"
	repository.ProviderOwner = "acme"
	repository.ProviderName = "widgets"
	authorizer := &githubBrokerScopeAuthorizer{repo: gitCredentialScopeTaskRepository(repository)}

	if err := authorizer.AuthorizeGitCredential(
		context.Background(), gitCredentialScopeForHostPath("ghe.example:8443", "/acme/widgets"),
	); err != nil {
		t.Fatalf("AuthorizeGitCredential() error = %v", err)
	}
	if err := authorizer.AuthorizeGitCredential(
		context.Background(), gitCredentialScopeForHostPath("ghe.example", "/acme/widgets"),
	); err == nil {
		t.Fatal("AuthorizeGitCredential() error = nil, want portless origin denial")
	}
}
