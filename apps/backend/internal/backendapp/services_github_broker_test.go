package backendapp

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	githubpkg "github.com/kandev/kandev/internal/github"
	executorpkg "github.com/kandev/kandev/internal/orchestrator/executor"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeGitHubCredentialLeaseService struct {
	request githubpkg.CredentialLeaseRequest
}

func (s *fakeGitHubCredentialLeaseService) IssueGitHubCredentialLease(
	_ context.Context,
	request githubpkg.CredentialLeaseRequest,
) (*githubpkg.CredentialLease, error) {
	s.request = request
	return &githubpkg.CredentialLease{Token: "lease-token"}, nil
}

func TestGitHubExecutorCredentialLeaseAdapterMapsScope(t *testing.T) {
	service := &fakeGitHubCredentialLeaseService{}
	adapter := githubExecutorCredentialLeaseAdapter{service: service}
	lease, err := adapter.IssueGitHubCredentialLease(context.Background(), executorpkg.GitHubCredentialLeaseRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Owner: "kdlbs", Repo: "kandev", Host: "github.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if lease.Token != "lease-token" || service.request.SessionID != "session-1" || service.request.RepositoryID != "repository-1" {
		t.Fatalf("lease = %+v, request = %+v", lease, service.request)
	}
}

func TestGitHubCredentialBrokerEndpoint(t *testing.T) {
	dedicated := &config.Config{
		GitHubCredentialBroker: config.GitHubCredentialBrokerConfig{PublicBaseURL: "https://broker.example/"},
		GitHubApp:              config.GitHubAppConfig{PublicBaseURL: "https://app.example/"},
	}
	if got, want := githubCredentialBrokerEndpoint(dedicated), "https://broker.example/api/v1/github/credentials/resolve"; got != want {
		t.Fatalf("dedicated endpoint = %q, want %q", got, want)
	}
	public := &config.Config{GitHubApp: config.GitHubAppConfig{PublicBaseURL: "https://kandev.example/"}}
	if got, want := githubCredentialBrokerEndpoint(public), "https://kandev.example/api/v1/github/credentials/resolve"; got != want {
		t.Fatalf("public endpoint = %q, want %q", got, want)
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
