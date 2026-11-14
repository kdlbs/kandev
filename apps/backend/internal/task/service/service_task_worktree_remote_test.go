package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A plugin provider clones from the repository's recorded remote URL, so the
// provider repository that replaces a task-worktree row must keep it.
func TestService_CreateTask_TaskWorktreeReplacementKeepsTheRemoteURL(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const (
		workspaceID = "ws-1"
		workflowID  = "wf-1"
		badRepoID   = "repo-task-worktree"
		remoteURL   = "https://dev.azure.com/acme/Contoso/_git/api"
	)
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            badRepoID,
		WorkspaceID:   workspaceID,
		Name:          "Contoso/api",
		SourceType:    sourceTypeProvider,
		LocalPath:     "/home/u/.kandev/tasks/azure-task_abc/api",
		Provider:      "azure_devops",
		ProviderHost:  "https://dev.azure.com",
		ProviderOwner: "Contoso",
		ProviderName:  "api",
		RemoteURL:     remoteURL,
		DefaultBranch: "master",
	})

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-1",
		Title:          "Investigate",
		Repositories:   []TaskRepositoryInput{{RepositoryID: badRepoID, BaseBranch: "master"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	createdID := result.Task.Repositories[0].RepositoryID
	if createdID == badRepoID {
		t.Fatal("expected a provider repository, got the task worktree repository")
	}
	created, err := repo.GetRepository(ctx, createdID)
	if err != nil {
		t.Fatalf("GetRepository(%q): %v", createdID, err)
	}
	if created.RemoteURL != remoteURL || created.ProviderHost != "https://dev.azure.com" {
		t.Fatalf("replacement = remote %q host %q, want %q and https://dev.azure.com",
			created.RemoteURL, created.ProviderHost, remoteURL)
	}
}

// An earlier replacement recorded without a clone URL is reused, so the task
// would otherwise clone from a URL derived from owner and name.
func TestService_CreateTask_TaskWorktreeReplacementReuseBackfillsTheRemoteURL(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const remoteURL = "https://dev.azure.com/acme/Contoso/_git/api"
	seedTaskWorktreeReplacementFixture(t, repo, remoteURL, "")

	created := createTaskOnTaskWorktreeRepository(t, svc)
	if created != "repo-replacement" {
		t.Fatalf("replacement = %q, want the existing repo-replacement", created)
	}
	got, err := repo.GetRepository(ctx, created)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if got.RemoteURL != remoteURL {
		t.Fatalf("reused replacement remote = %q, want %q", got.RemoteURL, remoteURL)
	}
}

func TestService_CreateTask_TaskWorktreeReplacementReuseKeepsItsOwnRemoteURL(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const kept = "https://dev.azure.com/acme/Contoso/_git/api-kept"
	seedTaskWorktreeReplacementFixture(t, repo, "https://dev.azure.com/acme/Contoso/_git/api", kept)

	created := createTaskOnTaskWorktreeRepository(t, svc)
	got, err := repo.GetRepository(ctx, created)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if got.RemoteURL != kept {
		t.Fatalf("reused replacement remote = %q, want it kept as %q", got.RemoteURL, kept)
	}
}

func seedTaskWorktreeReplacementFixture(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateRepository(context.Context, *models.Repository) error
}, sourceRemote, replacementRemote string) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	identity := func(id, localPath, remote string) *models.Repository {
		return &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: "Contoso/api", SourceType: sourceTypeProvider,
			LocalPath: localPath, Provider: "azure_devops", ProviderHost: "https://dev.azure.com",
			ProviderOwner: "Contoso", ProviderName: "api", RemoteURL: remote, DefaultBranch: "master",
		}
	}
	if err := repo.CreateRepository(ctx, identity("repo-task-worktree", "/home/u/.kandev/tasks/azure-task_abc/api", sourceRemote)); err != nil {
		t.Fatalf("CreateRepository(task worktree): %v", err)
	}
	if err := repo.CreateRepository(ctx, identity("repo-replacement", "", replacementRemote)); err != nil {
		t.Fatalf("CreateRepository(replacement): %v", err)
	}
}

func createTaskOnTaskWorktreeRepository(t *testing.T, svc *Service) string {
	t.Helper()
	result, err := svc.CreateTask(context.Background(), &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Investigate",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-task-worktree", BaseBranch: "master"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return result.Task.Repositories[0].RepositoryID
}
