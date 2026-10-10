package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestDeleteTask_TransfersBorrowedEnvironmentBeforeDeletingOwner(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedParentChildWorkspace(t, repo, "ws-transfer", "wf-transfer", "parent-task", "child-task")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:     "env-parent",
		TaskID: "parent-task",
		Status: models.TaskEnvironmentStatusReady,
		Repos:  []*models.TaskEnvironmentRepo{{RepositoryID: "repo-parent", WorktreeID: "wt-parent", WorktreePath: "/tmp/parent-worktree"}},
	}); err != nil {
		t.Fatalf("create parent environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-child",
		TaskID:            "child-task",
		State:             models.TaskSessionStateRunning,
		TaskEnvironmentID: "env-parent",
	}); err != nil {
		t.Fatalf("create child session: %v", err)
	}
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	beforeSession, err := repo.GetTaskSession(ctx, "session-child")
	if err != nil {
		t.Fatal(err)
	}
	beforeEnvironment, err := repo.GetTaskEnvironment(ctx, "env-parent")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ReparentDirectChildrenInWorkspace(ctx, "parent-task", "", "ws-transfer"); err != nil {
		t.Fatalf("promote surviving child: %v", err)
	}
	stillOwned, err := repo.GetTaskEnvironment(ctx, "env-parent")
	if err != nil || !reflect.DeepEqual(beforeEnvironment, stillOwned) {
		t.Fatalf("structural promotion changed the borrowed environment before service transfer: %v", err)
	}
	if err := svc.DeleteTask(ctx, "parent-task"); err != nil {
		t.Fatalf("delete parent task: %v", err)
	}

	env, err := repo.GetTaskEnvironment(ctx, "env-parent")
	if err != nil {
		t.Fatalf("borrowed environment should survive parent delete: %v", err)
	}
	if env.TaskID != "child-task" {
		t.Fatalf("borrowed environment owner = %q, want child-task", env.TaskID)
	}
	assertBorrowerSurvivesOwnerDelete(t, ctx, repo, beforeSession, beforeEnvironment, env)
}

func TestCleanupTaskResources_TransfersBorrowedEnvironmentBeforeCascadeDelete(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedParentChildWorkspace(t, repo, "ws-cascade-transfer", "wf-cascade-transfer", "parent-task", "child-task")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:     "env-parent",
		TaskID: "parent-task",
		Status: models.TaskEnvironmentStatusReady,
		Repos:  []*models.TaskEnvironmentRepo{{RepositoryID: "repo-parent", WorktreeID: "wt-parent", WorktreePath: "/tmp/parent-worktree"}},
	}); err != nil {
		t.Fatalf("create parent environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-child",
		TaskID:            "child-task",
		State:             models.TaskSessionStateRunning,
		TaskEnvironmentID: "env-parent",
	}); err != nil {
		t.Fatalf("create child session: %v", err)
	}
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	beforeSession, err := repo.GetTaskSession(ctx, "session-child")
	if err != nil {
		t.Fatal(err)
	}
	beforeEnvironment, err := repo.GetTaskEnvironment(ctx, "env-parent")
	if err != nil {
		t.Fatal(err)
	}
	svc.CleanupTaskResources(ctx, "parent-task", true)
	waitForCleanupDone(t, svc)
	if err := repo.ReparentDirectChildrenInWorkspace(ctx, "parent-task", "", "ws-cascade-transfer"); err != nil {
		t.Fatalf("promote surviving child before owner deletion: %v", err)
	}
	if err := repo.DeleteTask(ctx, "parent-task"); err != nil {
		t.Fatalf("delete parent task: %v", err)
	}

	env, err := repo.GetTaskEnvironment(ctx, "env-parent")
	if err != nil {
		t.Fatalf("borrowed environment should survive cascade owner delete: %v", err)
	}
	if env.TaskID != "child-task" {
		t.Fatalf("borrowed environment owner = %q, want child-task", env.TaskID)
	}
	assertBorrowerSurvivesOwnerDelete(t, ctx, repo, beforeSession, beforeEnvironment, env)
}

func assertBorrowerSurvivesOwnerDelete(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, beforeSession *models.TaskSession, beforeEnvironment, environment *models.TaskEnvironment) {
	t.Helper()
	if _, err := repo.GetTask(ctx, "parent-task"); !errors.Is(err, repository.ErrTaskNotFound) {
		t.Fatalf("owner task remains after deletion: %v", err)
	}
	child, err := repo.GetTask(ctx, "child-task")
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != "" || child.ArchivedAt != nil {
		t.Fatalf("surviving child is not a live root: %+v", child)
	}
	session, err := repo.GetTaskSession(ctx, "session-child")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeSession, session) || session.State != models.TaskSessionStateRunning || session.TaskEnvironmentID != environment.ID {
		t.Fatal("owner deletion changed the running borrower's session or environment link")
	}
	if environment.TaskID != child.ID || environment.OwnershipGeneration != beforeEnvironment.OwnershipGeneration+1 {
		t.Fatal("borrowed environment ownership was not transferred exactly once")
	}
	preserved := *environment
	preserved.TaskID = beforeEnvironment.TaskID
	preserved.OwnershipGeneration = beforeEnvironment.OwnershipGeneration
	preserved.UpdatedAt = beforeEnvironment.UpdatedAt
	if !reflect.DeepEqual(beforeEnvironment, &preserved) {
		t.Fatal("owner deletion changed borrowed environment identity or resources")
	}
}
