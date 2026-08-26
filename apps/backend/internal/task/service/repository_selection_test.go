package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type repositorySelectionResolverStub struct {
	calls   []TaskRepositoryInput
	resolve func(TaskRepositoryInput) (TaskRepositoryInput, error)
}

func (r *repositorySelectionResolverStub) ResolveRepositorySelection(
	_ context.Context, _ string, input TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	r.calls = append(r.calls, input)
	if r.resolve == nil {
		return input, nil
	}
	return r.resolve(input)
}

func TestCreateTaskPreflightsPluginRepositoryBeforePersistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "plugin repo",
		Repositories: []TaskRepositoryInput{{
			RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
			Provider:  "fixture-source-control", ProviderHost: "https://attacker.example.test",
			ProviderRepoID: "repo-42", ProviderOwner: "attacker", ProviderName: "wrong",
			DefaultBranch: "browser-default", BaseBranch: "main", CheckoutBranch: "feature/provider-contract",
		}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0].ProviderOwner != "attacker" {
		t.Fatalf("resolver calls = %+v, want one call with untrusted browser input", resolver.calls)
	}
	if result.Task == nil || len(result.Task.Repositories) != 1 {
		t.Fatalf("created task repositories = %+v", result.Task)
	}
	stored, err := repo.GetRepository(ctx, result.Task.Repositories[0].RepositoryID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.Provider != "fixture-source-control" || stored.ProviderHost != "https://bitbucket.example.test" ||
		stored.ProviderRepoID != "repo-42" || stored.ProviderOwner != "TEAM" || stored.ProviderName != "fixture" ||
		stored.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" || stored.DefaultBranch != "main" {
		t.Fatalf("stored repository = %+v, want authoritative plugin identity", stored)
	}
}

func TestCreateTaskPluginRepositoryResolutionFailureLeavesNoWrites(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: func(TaskRepositoryInput) (TaskRepositoryInput, error) {
		return TaskRepositoryInput{}, errors.New("upstream plugin response contains secret details")
	}}
	svc.SetRepositorySelectionResolver(resolver)

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "plugin repo failure",
		Repositories: []TaskRepositoryInput{{
			RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
			Provider:  "fixture-source-control", BaseBranch: "main",
		}},
	})
	assertRepositorySelectionError(t, err, RepositorySelectionErrorUnavailable, "secret")
	tasks, listErr := repo.ListTasks(ctx, "wf-1")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks after failed preflight = %d, want zero", len(tasks))
	}
	repositories, listErr := repo.ListRepositories(ctx, "ws-1")
	if listErr != nil {
		t.Fatalf("ListRepositories: %v", listErr)
	}
	if len(repositories) != 0 {
		t.Fatalf("repositories after failed preflight = %d, want zero", len(repositories))
	}
}

func TestCreateTaskPreflightsEveryPluginRepositoryBeforeAnyWrite(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: func(input TaskRepositoryInput) (TaskRepositoryInput, error) {
		if input.RemoteURL == "https://bitbucket.example.test/projects/TEAM/second" {
			return TaskRepositoryInput{}, errors.New("second provider unavailable")
		}
		return authoritativeRepositoryInput(input)
	}}
	svc.SetRepositorySelectionResolver(resolver)

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "multi plugin repos",
		Repositories: []TaskRepositoryInput{
			{RemoteURL: "https://bitbucket.example.test/projects/TEAM/first", Provider: "fixture-source-control"},
			{RemoteURL: "https://bitbucket.example.test/projects/TEAM/second", Provider: "fixture-source-control"},
		},
	})
	assertRepositorySelectionError(t, err, RepositorySelectionErrorUnavailable, "")
	if len(resolver.calls) != 2 {
		t.Fatalf("resolver calls = %d, want both repository selections preflighted", len(resolver.calls))
	}
	tasks, listErr := repo.ListTasks(ctx, "wf-1")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks after multi-repository preflight failure = %d, want zero", len(tasks))
	}
}

func TestCreateTaskSettledExternalIDSkipsPluginRepositoryInspection(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "first", ExternalID: "external-3066",
	})
	if err != nil {
		t.Fatalf("first CreateTask: %v", err)
	}
	if first.Task == nil {
		t.Fatal("first CreateTask returned no task")
	}
	resolver := &repositorySelectionResolverStub{resolve: func(TaskRepositoryInput) (TaskRepositoryInput, error) {
		return TaskRepositoryInput{}, errors.New("inspection must not run")
	}}
	svc.SetRepositorySelectionResolver(resolver)
	second, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "retry", ExternalID: "external-3066",
		Repositories: []TaskRepositoryInput{{RemoteURL: "https://bitbucket.example.test/TEAM/fixture", Provider: "fixture-source-control"}},
	})
	if err != nil {
		t.Fatalf("retry CreateTask: %v", err)
	}
	if second.Task == nil || second.Task.ID != first.Task.ID || len(resolver.calls) != 0 {
		t.Fatalf("retry result = %+v, resolver calls = %d", second, len(resolver.calls))
	}
}

func TestCreateTaskTrustedPluginDescriptorSkipsRepositoryInspection(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: func(TaskRepositoryInput) (TaskRepositoryInput, error) {
		return TaskRepositoryInput{}, errors.New("trusted descriptor must not be inspected again")
	}}
	svc.SetRepositorySelectionResolver(resolver)

	input, err := authoritativeRepositoryInput(TaskRepositoryInput{BaseBranch: "main"})
	if err != nil {
		t.Fatalf("authoritativeRepositoryInput: %v", err)
	}
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "trusted plugin task",
		Repositories: []TaskRepositoryInput{input},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if result.Task == nil || len(resolver.calls) != 0 {
		t.Fatalf("result = %+v, resolver calls = %d, want trusted path without inspection", result, len(resolver.calls))
	}
}

func createRepositorySelectionWorkspace(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
}

func authoritativeRepositoryInput(input TaskRepositoryInput) (TaskRepositoryInput, error) {
	return TaskRepositoryInput{
		RemoteURL: "https://bitbucket.example.test/scm/TEAM/fixture.git",
		Provider:  "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
		ProviderScope: "workspace-a", ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
		DefaultBranch: "main", BaseBranch: input.BaseBranch, CheckoutBranch: input.CheckoutBranch,
		TrustedProviderDescriptor: true,
	}, nil
}

func assertRepositorySelectionError(t *testing.T, err error, want RepositorySelectionErrorCode, leaked string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected repository selection error")
	}
	var selectionErr *RepositorySelectionError
	if !errors.As(err, &selectionErr) {
		t.Fatalf("error = %v, want RepositorySelectionError", err)
	}
	if selectionErr.Code != want {
		t.Fatalf("error code = %q, want %q", selectionErr.Code, want)
	}
	if leaked != "" && strings.Contains(err.Error(), leaked) {
		t.Fatalf("error leaked resolver detail: %v", err)
	}
}
