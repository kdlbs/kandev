package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestWorkspaceSourceMaterializerKubernetesUsesRemoteMaterialization(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	seedRemoteMaterializerRepositories(t, repo)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = string(models.ExecutorTypeKubernetes)
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	remote := &remoteWorkspaceMaterializerStub{ids: []string{"session-1"}}
	materializer := &workspaceSourceMaterializer{
		repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")),
		remoteMaterializer: remote, logger: newTestLogger(),
	}

	result, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{
		TaskID: "task-1",
		Sources: []models.WorkspaceSource{{Repository: &models.TaskRepository{
			RepositoryID: "repo-added", BaseBranch: "main",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SessionIDs) != 1 || result.SessionIDs[0] != "session-1" {
		t.Fatalf("materialization result = %#v", result)
	}
	if len(remote.calls) != 1 || len(remote.calls[0]) != 1 || remote.calls[0][0].Destination != "added-main" {
		t.Fatalf("remote projection = %+v, want added-main", remote.calls)
	}
}

func TestWorkspaceSourceMaterializerRejectsUnknownExecutor(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = "future_executor"
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	remote := &remoteWorkspaceMaterializerStub{}
	materializer := &workspaceSourceMaterializer{
		repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")),
		remoteMaterializer: remote, logger: newTestLogger(),
	}

	_, err = materializer.MaterializeWorkspaceSources(
		ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1"},
	)
	if !errors.Is(err, taskservice.ErrUnsupportedWorkspaceSource) {
		t.Fatalf("error = %v, want unsupported workspace source", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote materializer called for unknown executor: %+v", remote.calls)
	}
}
