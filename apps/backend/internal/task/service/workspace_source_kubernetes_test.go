package service

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceSourceRuntimeEntryNameKubernetesUsesBranchIdentity(t *testing.T) {
	repository := &models.Repository{ID: "repo-1", Name: "api"}
	taskRepository := &models.TaskRepository{
		RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/k8s",
	}

	got, err := WorkspaceSourceRuntimeEntryName(
		string(models.ExecutorTypeKubernetes), repository, taskRepository,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "api-feature-k8s" {
		t.Fatalf("runtime entry name = %q, want api-feature-k8s", got)
	}
}

func TestWorkspaceSourceRuntimeEntryNameRejectsUnknownExecutor(t *testing.T) {
	_, err := WorkspaceSourceRuntimeEntryName(
		"future_executor",
		&models.Repository{ID: "repo-1", Name: "api"},
		&models.TaskRepository{RepositoryID: "repo-1", BaseBranch: "main"},
	)
	if !errors.Is(err, ErrUnsupportedWorkspaceSource) {
		t.Fatalf("error = %v, want unsupported workspace source", err)
	}
}
