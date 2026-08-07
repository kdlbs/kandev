package backendapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/repoclone"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type fakePluginCredentialService struct {
	found  bool
	remote pluginCredentialRemote
}

func (s fakePluginCredentialService) Provider(string) (string, string, pluginCredentialRemote, bool) {
	return "kandev-plugin-bitbucket", "1.2.3", s.remote, s.found
}

type recordingPluginCredentialRemote struct {
	request        *pluginsdk.ResolveGitCredentialRequest
	bindingRequest *pluginsdk.GitCredentialBindingRequest
	calls          int
	binding        string
}

func (r *recordingPluginCredentialRemote) GetGitCredentialBinding(
	_ context.Context,
	request *pluginsdk.GitCredentialBindingRequest,
) (*pluginsdk.GitCredentialBindingResponse, error) {
	r.bindingRequest = request
	return &pluginsdk.GitCredentialBindingResponse{Binding: r.binding}, nil
}

func (r *recordingPluginCredentialRemote) ResolveGitCredential(
	_ context.Context,
	request *pluginsdk.ResolveGitCredentialRequest,
) (*pluginsdk.ResolveGitCredentialResponse, error) {
	r.calls++
	r.request = request
	return &pluginsdk.ResolveGitCredentialResponse{
		Username: "x-token-auth", Secret: "transient", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}, nil
}

func TestPluginGitCredentialResolverCallsLiveProviderWithExactScope(t *testing.T) {
	remote := &recordingPluginCredentialRemote{binding: "connection:7"}
	resolver := pluginGitCredentialResolver{service: fakePluginCredentialService{found: true, remote: remote}}
	scope := gitcredentials.Scope{
		ProviderID: "bitbucket", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	}
	credential, err := resolver.Resolve(context.Background(), scope)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if credential.Username != "x-token-auth" || credential.Password != "transient" {
		t.Fatalf("credential = %#v", credential)
	}
	if remote.calls != 1 || remote.request == nil || remote.request.Path != scope.Path || remote.request.Host != scope.Host ||
		remote.request.SessionID != scope.SessionID || remote.request.ProviderID != scope.ProviderID {
		t.Fatalf("live RPC request = %#v", remote.request)
	}
}

func TestRepositoryCloneCredentialProviderResolvesPrivatePluginRepository(t *testing.T) {
	remote := &recordingPluginCredentialRemote{}
	provider := repositoryCloneCredentialProvider{
		plugins: fakePluginCredentialService{found: true, remote: remote},
	}
	username, password, err := provider.ResolveGitCredential(t.Context(), repoclone.GitCredentialRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Provider:     "bitbucket",
		ProviderHost: "https://bitbucket.example/context",
		CloneURL:     "https://bitbucket.example/context/scm/ENG/widgets.git",
		Owner:        "ENG", Name: "widgets",
	})
	if err != nil {
		t.Fatalf("ResolveGitCredential(): %v", err)
	}
	if username != "x-token-auth" || password != "transient" {
		t.Fatalf("clone credential = %q/%q", username, password)
	}
	if remote.request == nil || remote.request.WorkspaceID != "workspace-1" ||
		remote.request.ProviderID != "bitbucket" || remote.request.Host != "bitbucket.example" ||
		remote.request.Path != "/context/scm/ENG/widgets.git" || remote.request.TaskID != "task-1" ||
		remote.request.SessionID != "session-1" || remote.request.RepositoryID != "repository-1" {
		t.Fatalf("plugin clone credential request = %#v", remote.request)
	}
}

func TestRepositoryCloneCredentialProviderRejectsMismatchedPluginOrigin(t *testing.T) {
	remote := &recordingPluginCredentialRemote{}
	provider := repositoryCloneCredentialProvider{
		plugins: fakePluginCredentialService{found: true, remote: remote},
	}
	_, _, err := provider.ResolveGitCredential(t.Context(), repoclone.GitCredentialRequest{
		WorkspaceID: "workspace-1", Provider: "bitbucket",
		ProviderHost: "https://bitbucket.example",
		CloneURL:     "https://attacker.example/ENG/widgets.git",
	})
	if err == nil {
		t.Fatal("ResolveGitCredential() accepted a clone URL outside the provider origin")
	}
	if remote.calls != 0 {
		t.Fatalf("plugin credential RPC calls = %d, want 0", remote.calls)
	}
}

func TestPluginGitCredentialResolverUsesLiveCredentialBindingNotPluginVersion(t *testing.T) {
	remote := &recordingPluginCredentialRemote{binding: "connection:7"}
	resolver := pluginGitCredentialResolver{service: fakePluginCredentialService{found: true, remote: remote}}
	scope := gitcredentials.Scope{ProviderID: "bitbucket", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1", Host: "bitbucket.example", Path: "/scm/ENG/widgets.git"}

	binding, err := resolver.Binding(context.Background(), scope)

	if err != nil {
		t.Fatalf("Binding() error = %v", err)
	}
	if binding != "connection:7" {
		t.Fatalf("Binding() = %q, want live connection generation", binding)
	}
	if remote.bindingRequest == nil || remote.bindingRequest.Path != scope.Path || remote.bindingRequest.RepositoryID != scope.RepositoryID {
		t.Fatalf("binding request = %#v, want exact host-verified scope", remote.bindingRequest)
	}
}

type legacyPluginCredentialRemote struct{}

func (legacyPluginCredentialRemote) ResolveGitCredential(context.Context, *pluginsdk.ResolveGitCredentialRequest) (*pluginsdk.ResolveGitCredentialResponse, error) {
	return &pluginsdk.ResolveGitCredentialResponse{Username: "x-token-auth", Secret: "transient"}, nil
}

func TestPluginGitCredentialResolverFailsClosedWithoutCredentialBinding(t *testing.T) {
	resolver := pluginGitCredentialResolver{service: fakePluginCredentialService{found: true, remote: legacyPluginCredentialRemote{}}}
	_, err := resolver.Binding(context.Background(), gitcredentials.Scope{ProviderID: "bitbucket", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1", Host: "bitbucket.example", Path: "/scm/ENG/widgets.git"})
	if !errors.Is(err, gitcredentials.ErrUnsupported) {
		t.Fatalf("Binding() error = %v, want ErrUnsupported", err)
	}
}

func TestPluginGitCredentialResolverFailsClosedWhenProviderDisabled(t *testing.T) {
	resolver := pluginGitCredentialResolver{service: fakePluginCredentialService{found: false}}
	if resolver.Supports("bitbucket") {
		t.Fatal("disabled provider unexpectedly supported")
	}
	if _, err := resolver.Resolve(context.Background(), gitcredentials.Scope{ProviderID: "bitbucket"}); err == nil ||
		!strings.Contains(err.Error(), gitcredentials.ErrUnsupported.Error()) {
		t.Fatalf("disabled provider error = %v", err)
	}
}

func TestGitHubCredentialBrokerEndpoint(t *testing.T) {
	dedicated := &config.Config{
		GitHubCredentialBroker: config.GitHubCredentialBrokerConfig{PublicBaseURL: "https://broker.example/"},
	}
	if got, want := githubCredentialBrokerEndpoint(dedicated), "https://broker.example/api/v1/github/credentials/resolve"; got != want {
		t.Fatalf("dedicated endpoint = %q, want %q", got, want)
	}
	local := &config.Config{Server: config.ServerConfig{Port: 49123}}
	if got, want := githubCredentialBrokerEndpoint(local), "http://localhost:49123/api/v1/github/credentials/resolve"; got != want {
		t.Fatalf("local endpoint = %q, want %q", got, want)
	}
}

type fakeGitHubBrokerTaskRepository struct {
	task       *taskmodels.Task
	session    *taskmodels.TaskSession
	repository *taskmodels.Repository
	links      []*taskmodels.TaskRepository
}

func (r *fakeGitHubBrokerTaskRepository) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return r.task, nil
}

func (r *fakeGitHubBrokerTaskRepository) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return r.session, nil
}

func (r *fakeGitHubBrokerTaskRepository) GetRepository(context.Context, string) (*taskmodels.Repository, error) {
	return r.repository, nil
}

func (r *fakeGitHubBrokerTaskRepository) ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error) {
	return r.links, nil
}

func TestGitHubBrokerScopeAuthorizerValidatesSessionOwnershipAndState(t *testing.T) {
	repo := &fakeGitHubBrokerTaskRepository{
		task:    &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		session: &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		repository: &taskmodels.Repository{
			ID: "repository-1", WorkspaceID: "workspace-1", Provider: "github",
			ProviderOwner: "kdlbs", ProviderName: "kandev",
		},
		links: []*taskmodels.TaskRepository{{TaskID: "task-1", RepositoryID: "repository-1"}},
	}
	authorizer := &githubBrokerScopeAuthorizer{repo: repo}

	if err := authorizer.AuthorizeGitHubRepository(
		context.Background(), "workspace-1", "task-1", "session-1", "repository-1", "kdlbs", "kandev",
	); err != nil {
		t.Fatalf("AuthorizeGitHubRepository() error = %v", err)
	}

	repo.session.TaskID = "another-task"
	if err := authorizer.AuthorizeGitHubRepository(
		context.Background(), "workspace-1", "task-1", "session-1", "repository-1", "kdlbs", "kandev",
	); err == nil || !strings.Contains(err.Error(), "session does not belong") {
		t.Fatalf("session mismatch error = %v", err)
	}

	repo.session.TaskID = "task-1"
	repo.session.State = taskmodels.TaskSessionStateCompleted
	if err := authorizer.AuthorizeGitHubRepository(
		context.Background(), "workspace-1", "task-1", "session-1", "repository-1", "kdlbs", "kandev",
	); err == nil || !strings.Contains(err.Error(), "session is terminal") {
		t.Fatalf("terminal session error = %v", err)
	}
}

func TestGitHubBrokerScopeAuthorizerAllowsOnlyTheBoundContributionFork(t *testing.T) {
	binding := taskmodels.RemoteContribution{
		Version:              taskmodels.RemoteContributionVersion,
		Provider:             taskmodels.RemoteContributionProviderGitHub,
		Kind:                 taskmodels.RemoteContributionKindPullRequest,
		CanonicalURL:         "https://github.com/acme/widget/pull/7",
		Number:               7,
		State:                taskmodels.RemoteContributionStateOpen,
		BaseBranch:           "main",
		HeadBranch:           "feature/remote",
		HeadSHA:              strings.Repeat("a", 40),
		CollaborationAllowed: true,
		SourceRepository: taskmodels.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget-fork", RemoteURL: "https://github.com/contributor/widget-fork.git",
		},
	}
	metadata := map[string]interface{}{}
	if err := taskmodels.PutRemoteContribution(metadata, &binding); err != nil {
		t.Fatalf("PutRemoteContribution() error = %v", err)
	}
	repo := &fakeGitHubBrokerTaskRepository{
		task:       &taskmodels.Task{ID: "task-fork", WorkspaceID: "workspace-fork"},
		session:    &taskmodels.TaskSession{ID: "session-fork", TaskID: "task-fork", State: taskmodels.TaskSessionStateRunning},
		repository: &taskmodels.Repository{ID: "repository-fork", WorkspaceID: "workspace-fork", Provider: "github", ProviderOwner: "acme", ProviderName: "widget"},
		links:      []*taskmodels.TaskRepository{{TaskID: "task-fork", RepositoryID: "repository-fork", Metadata: metadata}},
	}
	authorizer := &githubBrokerScopeAuthorizer{repo: repo}

	if err := authorizer.AuthorizeGitHubRepository(
		context.Background(), "workspace-fork", "task-fork", "session-fork", "repository-fork", "contributor", "widget-fork",
	); err != nil {
		t.Fatalf("bound fork was denied: %v", err)
	}
	if err := authorizer.AuthorizeGitHubRepository(
		context.Background(), "workspace-fork", "task-fork", "session-fork", "repository-fork", "contributor", "other-fork",
	); err == nil || !strings.Contains(err.Error(), "identity does not match") {
		t.Fatalf("unbound fork error = %v, want identity denial", err)
	}

	binding.CollaborationAllowed = false
	if err := taskmodels.PutRemoteContribution(metadata, &binding); err != nil {
		t.Fatalf("update remote contribution: %v", err)
	}
	if err := authorizer.AuthorizeGitHubRepository(
		context.Background(), "workspace-fork", "task-fork", "session-fork", "repository-fork", "contributor", "widget-fork",
	); err == nil || !strings.Contains(err.Error(), "identity does not match") {
		t.Fatalf("non-collaborative fork error = %v, want identity denial", err)
	}
}
