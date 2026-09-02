package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestKubernetesExecutorUsesRemoteContainerCredentialPolicy(t *testing.T) {
	executorType := string(models.ExecutorTypeKubernetes)
	if !isContainerizedExecutor(executorType) {
		t.Fatal("Kubernetes executor must be classified as containerized")
	}
	if !executorNeedsResolvedCredentials(executorType) {
		t.Fatal("Kubernetes executor must resolve selected profile credentials")
	}
	if shouldUseWorktree(executorType) {
		t.Fatal("Kubernetes executor must not use host git worktrees")
	}
}

func TestKubernetesCredentialBrokerRequiresHTTPS(t *testing.T) {
	err := validateGitHubCredentialBrokerURL(
		"http://127.0.0.1:8080/api/github/credentials/resolve",
		string(models.ExecutorTypeKubernetes),
	)
	if !errors.Is(err, ErrGitHubCredentialBrokerURL) {
		t.Fatalf("validateGitHubCredentialBrokerURL() error = %v, want broker URL error", err)
	}
}

func TestKubernetesProfileTokenSkipsManagedCredentialPreflight(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{
		ID: "repo-1", SourceType: sourceTypeLocal, Provider: "acme-forge",
		RemoteURL: "https://forge.example/acme/widgets.git",
	})
	repo.executors["exec-k8s"] = &models.Executor{
		ID: "exec-k8s", Type: models.ExecutorTypeKubernetes,
	}
	repo.executorProfiles["profile-token"] = &models.ExecutorProfile{
		ID: "profile-token", ExecutorID: "exec-k8s",
		Config: map[string]string{profileKeyRemoteAuthSecrets: `{"gh_cli_env":"secret-gh"}`},
	}
	exec := newPreflightTestExecutor(t, repo)

	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}
	if _, err := exec.PrepareSession(
		context.Background(), task, "agent-profile", "exec-k8s", "profile-token", "",
	); err != nil {
		t.Fatalf("PrepareSession() error = %v, want Kubernetes profile token override", err)
	}
	if len(repo.createTaskSessionCalls) != 1 {
		t.Fatalf("CreateTaskSession calls = %d, want 1", len(repo.createTaskSessionCalls))
	}
}

func TestKubernetesEnvironmentReuseCarriesFeatureBranch(t *testing.T) {
	exec := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeKubernetes),
	}
	env := &models.TaskEnvironment{
		ExecutorType: string(models.ExecutorTypeKubernetes),
		Repos: []*models.TaskEnvironmentRepo{
			{WorktreeBranch: "feature/existing-task-abc"},
		},
	}

	exec.reuseExistingEnvironment(context.Background(), req, env)

	if got := req.Metadata[lifecycle.MetadataKeyWorktreeBranch]; got != "feature/existing-task-abc" {
		t.Fatalf("metadata worktree_branch = %v, want existing branch", got)
	}
}
