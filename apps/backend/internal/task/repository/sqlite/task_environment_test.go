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

func TestCreateTaskSessionWithWorkspaceBindingElectsAndAttaches(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding", WorkspaceID: "workspace-binding", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	first := &models.TaskSession{ID: "session-owner", TaskID: "task-binding"}
	candidate := &models.TaskEnvironment{TaskID: "task-binding", ExecutorType: string(models.ExecutorTypeLocal)}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, first, candidate); err != nil {
		t.Fatalf("bind first session: %v", err)
	}
	if first.TaskEnvironmentID == "" || first.TaskEnvironmentID != candidate.ID {
		t.Fatalf("first binding = %q, candidate = %q", first.TaskEnvironmentID, candidate.ID)
	}
	env, err := repo.GetTaskEnvironment(ctx, first.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != first.ID {
		t.Fatalf("creating environment = %+v", env)
	}

	blocked := &models.TaskSession{ID: "session-blocked", TaskID: "task-binding"}
	err = repo.CreateTaskSessionWithWorkspaceBinding(ctx, blocked, &models.TaskEnvironment{TaskID: "task-binding"})
	if !errors.Is(err, models.ErrWorkspacePreparing) {
		t.Fatalf("second bind error = %v, want preparing", err)
	}
	sessions, err := repo.ListTaskSessions(ctx, "task-binding")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions after blocked bind = %+v, %v", sessions, err)
	}

	env.Status = models.TaskEnvironmentStatusReady
	env.MaterializationSessionID = ""
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	attached := &models.TaskSession{ID: "session-attached", TaskID: "task-binding"}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, attached, &models.TaskEnvironment{TaskID: "task-binding"}); err != nil {
		t.Fatalf("attach ready environment: %v", err)
	}
	if attached.TaskEnvironmentID != env.ID {
		t.Fatalf("attached environment = %q, want %q", attached.TaskEnvironmentID, env.ID)
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
