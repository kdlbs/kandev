package sqlite

import (
	"context"
	"errors"
	"sync"
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

func TestCreateTaskSessionWithWorkspaceBindingConcurrentElectionHasOneOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-race")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-race", WorkspaceID: "workspace-binding-race", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"session-race-a", "session-race-b"} {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			<-start
			errs <- repo.CreateTaskSessionWithWorkspaceBinding(ctx,
				&models.TaskSession{ID: sessionID, TaskID: "task-binding-race"},
				&models.TaskEnvironment{TaskID: "task-binding-race", ExecutorType: string(models.ExecutorTypeLocal)})
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)

	var succeeded, preparing int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, models.ErrWorkspacePreparing):
			preparing++
		default:
			t.Fatalf("concurrent binding error = %v", err)
		}
	}
	if succeeded != 1 || preparing != 1 {
		t.Fatalf("election results: success=%d preparing=%d, want one each", succeeded, preparing)
	}
	sessions, err := repo.ListTaskSessions(ctx, "task-binding-race")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("persisted sessions = %+v, %v", sessions, err)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingFailsClosedForAbandonedCreatingClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-abandoned")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-abandoned", WorkspaceID: "workspace-binding-abandoned", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	environment := &models.TaskEnvironment{
		ID:                       "environment-abandoned",
		TaskID:                   "task-binding-abandoned",
		ExecutorType:             string(models.ExecutorTypeLocal),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "missing-owner",
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("create abandoned environment: %v", err)
	}

	err := repo.CreateTaskSessionWithWorkspaceBinding(ctx,
		&models.TaskSession{ID: "session-retry", TaskID: "task-binding-abandoned"},
		&models.TaskEnvironment{TaskID: "task-binding-abandoned", ExecutorType: string(models.ExecutorTypeLocal)})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind after abandoned claim error = %v, want reuse unsafe", err)
	}

	got, err := repo.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TaskEnvironmentStatusFailed || got.MaterializationSessionID != "" {
		t.Fatalf("abandoned environment = %+v, want failed without owner", got)
	}
	sessions, err := repo.ListTaskSessions(ctx, environment.TaskID)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("sessions after failed recovery = %+v, %v", sessions, err)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingFailsClosedForOwnerlessCreatingClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-ownerless")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-ownerless", WorkspaceID: "workspace-binding-ownerless", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	environment := &models.TaskEnvironment{
		ID:           "environment-ownerless",
		TaskID:       "task-binding-ownerless",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusCreating,
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("create ownerless environment: %v", err)
	}

	err := repo.CreateTaskSessionWithWorkspaceBinding(ctx,
		&models.TaskSession{ID: "session-after-ownerless", TaskID: environment.TaskID},
		&models.TaskEnvironment{TaskID: environment.TaskID, ExecutorType: string(models.ExecutorTypeLocal)})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind after ownerless claim error = %v, want reuse unsafe", err)
	}

	got, err := repo.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TaskEnvironmentStatusFailed || got.MaterializationSessionID != "" {
		t.Fatalf("ownerless environment = %+v, want failed without owner", got)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingFailsClosedForTerminalCreatingOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-terminal-owner")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-terminal-owner", WorkspaceID: "workspace-binding-terminal-owner", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	owner := &models.TaskSession{ID: "terminal-owner", TaskID: "task-binding-terminal-owner"}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
		TaskID:       owner.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal),
	}); err != nil {
		t.Fatalf("create materialization owner: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateFailed, "launch failed"); err != nil {
		t.Fatal(err)
	}

	err := repo.CreateTaskSessionWithWorkspaceBinding(ctx,
		&models.TaskSession{ID: "session-after-terminal-owner", TaskID: owner.TaskID},
		&models.TaskEnvironment{TaskID: owner.TaskID, ExecutorType: string(models.ExecutorTypeLocal)})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind after terminal owner error = %v, want reuse unsafe", err)
	}
	environment, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if environment.Status != models.TaskEnvironmentStatusFailed || environment.MaterializationSessionID != "" {
		t.Fatalf("terminal-owner environment = %+v, want failed without owner", environment)
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
