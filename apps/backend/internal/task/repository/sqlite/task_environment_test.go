package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeleteTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.DeleteTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("DeleteTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestTaskEnvironmentRuntimeSecretRefsRoundTripAndRotateIndependently(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-runtime-secrets")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-runtime-secrets", WorkspaceID: "workspace-runtime-secrets", Title: "Task",
	}); err != nil {
		t.Fatal(err)
	}
	environment := &models.TaskEnvironment{
		ID: "env-runtime-secrets", TaskID: "task-runtime-secrets",
		ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
		AgentctlAuthSecretID: "auth-old", AgentctlBootstrapSecretID: "nonce-stable",
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskEnvironmentRuntimeSecretRefs(ctx, environment.ID, "auth-new", ""); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AgentctlAuthSecretID != "auth-new" || stored.AgentctlBootstrapSecretID != "nonce-stable" {
		t.Fatalf("runtime secret refs = %#v", stored)
	}
	stored.AgentctlAuthSecretID = ""
	stored.AgentctlBootstrapSecretID = ""
	stored.Status = models.TaskEnvironmentStatusStopped
	if err := repo.UpdateTaskEnvironment(ctx, stored); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AgentctlAuthSecretID != "auth-new" || stored.AgentctlBootstrapSecretID != "nonce-stable" {
		t.Fatalf("general environment update clobbered runtime secret refs: %#v", stored)
	}
}

func TestTaskEnvironmentRepoUpdateDeleteAndBulkCleanup(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-task-environment-crud")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-environment-crud", WorkspaceID: "workspace-task-environment-crud", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	for _, repositoryID := range []string{"environment-repo-one", "environment-repo-two"} {
		if err := repo.CreateRepository(ctx, &models.Repository{ID: repositoryID, WorkspaceID: "workspace-task-environment-crud", Name: repositoryID}); err != nil {
			t.Fatal(err)
		}
	}
	environment := &models.TaskEnvironment{ID: "task-environment-crud", TaskID: "task-environment-crud", ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	seedForMsgTest(t, repo, "task-environment-crud", "session-environment-crud", "turn-environment-crud")
	if _, err := repo.db.Exec(`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`, environment.ID, "session-environment-crud"); err != nil {
		t.Fatal(err)
	}
	for index, repositoryID := range []string{"environment-repo-one", "environment-repo-two"} {
		if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{ID: "env-link-" + repositoryID, TaskEnvironmentID: environment.ID, RepositoryID: repositoryID, Position: index, WorktreeBranch: "main"}); err != nil {
			t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
		}
	}
	links, err := repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("ListTaskEnvironmentRepos = %+v, %v", links, err)
	}
	mergedAt := time.Date(2026, time.July, 8, 9, 10, 11, 0, time.UTC)
	links[0].BranchSlug = "feature"
	links[0].WorktreeBranch = "feature/updated"
	links[0].Status = "merged"
	links[0].MergedAt = &mergedAt
	if err := repo.UpdateTaskEnvironmentRepo(ctx, links[0]); err != nil {
		t.Fatalf("UpdateTaskEnvironmentRepo: %v", err)
	}
	if err := repo.UpdateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{ID: "missing"}); err == nil {
		t.Fatal("UpdateTaskEnvironmentRepo accepted missing row")
	}
	links, err = repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil || links[0].WorktreeBranch != "feature/updated" || links[0].MergedAt == nil {
		t.Fatalf("updated environment repo = %+v, %v", links, err)
	}
	if err := repo.UpdateTaskSessionWorktreeBranch(ctx, "session-environment-crud", "feature/all"); err != nil {
		t.Fatalf("UpdateTaskSessionWorktreeBranch: %v", err)
	}
	branches, err := repo.ListSessionsWithBranches(ctx)
	if err != nil || len(branches) != 1 || branches[0].SessionID != "session-environment-crud" || branches[0].Branch != "feature/all" {
		t.Fatalf("ListSessionsWithBranches = %+v, %v", branches, err)
	}
	if err := repo.DeleteTaskEnvironmentRepo(ctx, links[0].ID); err != nil {
		t.Fatalf("DeleteTaskEnvironmentRepo: %v", err)
	}
	if err := repo.DeleteTaskEnvironmentRepo(ctx, links[0].ID); err == nil {
		t.Fatal("second DeleteTaskEnvironmentRepo returned nil")
	}
	if err := repo.DeleteTaskEnvironmentReposByEnv(ctx, environment.ID); err != nil {
		t.Fatalf("DeleteTaskEnvironmentReposByEnv: %v", err)
	}
	links, err = repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil || len(links) != 0 {
		t.Fatalf("links after bulk cleanup = %+v, %v", links, err)
	}
	if err := repo.DeleteTaskEnvironmentsByTask(ctx, environment.TaskID); err != nil {
		t.Fatalf("DeleteTaskEnvironmentsByTask: %v", err)
	}
	if got, err := repo.GetTaskEnvironmentByTaskID(ctx, environment.TaskID); err != nil || got != nil {
		t.Fatalf("environment after bulk delete = %+v, %v", got, err)
	}
}

func TestGetTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	_, err := repo.GetTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("GetTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestUpdateTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.UpdateTaskEnvironment(context.Background(), &models.TaskEnvironment{ID: "missing-environment"})

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("UpdateTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestTransferTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.TransferTaskEnvironmentToTask(context.Background(), "missing-environment", "task-1")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("TransferTaskEnvironmentToTask error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}
