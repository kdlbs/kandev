package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// These cases document existing fail-closed behavior for Kubernetes-specific
// capability boundaries; they are parity contract coverage, not fix tests.
func TestKubernetesWorkspaceSourceCapabilities(t *testing.T) {
	t.Run("folder sources are rejected before host path resolution", func(t *testing.T) {
		svc, taskID := newKubernetesWorkspaceSourceTestService(t)
		_, err := svc.AttachWorkspaceSources(context.Background(), AttachWorkspaceSourcesRequest{
			TaskID: taskID,
			Sources: []WorkspaceSourceInput{{
				Kind: WorkspaceSourceFolder, LocalPath: filepath.Join(t.TempDir(), "missing"), DisplayName: "docs",
			}},
		})
		if !errors.Is(err, ErrUnsupportedWorkspaceSource) {
			t.Fatalf("error = %v, want unsupported workspace source", err)
		}
	})

	t.Run("host sibling worktree branch add is rejected", func(t *testing.T) {
		svc, taskID := newKubernetesWorkspaceSourceTestService(t)
		_, err := svc.AddBranchToTask(context.Background(), AddBranchToTaskRequest{
			TaskID: taskID, CheckoutBranch: "feature/other",
		})
		if err == nil || !strings.Contains(err.Error(), "worktree executor") {
			t.Fatalf("error = %v, want worktree executor rejection", err)
		}
	})
}

func newKubernetesWorkspaceSourceTestService(t *testing.T) (*Service, string) {
	t.Helper()
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-k8s", Name: "Kubernetes"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-k8s", WorkspaceID: "ws-k8s", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-k8s", WorkspaceID: "ws-k8s", Name: "api", DefaultBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-k8s", WorkflowID: "wf-k8s", WorkflowStepID: "step-k8s", Title: "Task",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-k8s", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-k8s", TaskID: result.Task.ID, ExecutorType: string(models.ExecutorTypeKubernetes),
		Status: models.TaskEnvironmentStatusReady, CreatedAt: now, UpdatedAt: now,
		Repos: []*models.TaskEnvironmentRepo{{
			RepositoryID: "repo-k8s",
			Position:     0,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return svc, result.Task.ID
}
