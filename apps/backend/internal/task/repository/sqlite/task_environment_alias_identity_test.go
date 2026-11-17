package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.8
func TestTaskEnvironmentAliasIdentity(t *testing.T) {
	r := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, r, "alias-workspace")
	if err := r.CreateTask(ctx, &models.Task{ID: "alias-task", WorkspaceID: "alias-workspace", Title: "Alias"}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "alias-env", TaskID: "alias-task", ExecutorType: "worktree", WorkspacePath: "/tmp/alias", Status: models.TaskEnvironmentStatusReady}); err != nil {
		t.Fatal(err)
	}
	first := &models.TaskEnvironmentRepo{ID: "first", TaskEnvironmentID: "alias-env", RepositoryID: "repo", BranchSlug: "first", WorktreeID: "physical"}
	if err := r.CreateTaskEnvironmentRepo(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := *first
	second.ID = "second"
	second.BranchSlug = "second"
	err := r.CreateTaskEnvironmentRepo(ctx, &second)
	var ambiguous *models.WorktreeIdentityAmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("conflicting create: %v", err)
	}
	second.WorktreeID = "other"
	if err := r.CreateTaskEnvironmentRepo(ctx, &second); err != nil {
		t.Fatal(err)
	}
	second.WorktreeID = "physical"
	if err := r.UpdateTaskEnvironmentRepo(ctx, &second); !errors.As(err, &ambiguous) {
		t.Fatalf("conflicting update: %v", err)
	}
	rows, err := r.ListTaskEnvironmentRepos(ctx, "alias-env")
	if err != nil || len(rows) != 2 {
		t.Fatalf("inventory: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.ID == "second" && row.WorktreeID != "other" {
			t.Fatalf("failed update changed identity: %+v", row)
		}
	}
}

func TestTaskEnvironmentAliasIdentityTransition(t *testing.T) {
	r := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, r, "workspace")
	if err := r.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{ID: "env", TaskID: "task", ExecutorType: "worktree", WorkspacePath: "/tmp/owner", Status: models.TaskEnvironmentStatusReady}
	if err := r.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	slot := &models.TaskEnvironmentRepo{ID: "first", TaskEnvironmentID: "env", RepositoryID: "repo", BranchSlug: "first", WorktreeID: "physical", WorktreePath: "/tmp/owner/repo", WorktreeBranch: "main"}
	if err := r.CreateTaskEnvironmentRepo(ctx, slot); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO task_environment_repos (id,task_environment_id,repository_id,branch_slug,worktree_id,status,created_at,updated_at) VALUES ('alias','env','repo','alias','physical','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	var ambiguous *models.WorktreeIdentityAmbiguousError
	for _, replace := range []bool{false, true} {
		incoming := *slot
		incoming.WorktreeID = "replacement"
		if err := r.PersistTaskEnvironmentTransition(ctx, env, []*models.TaskEnvironmentRepo{&incoming}, replace); !errors.As(err, &ambiguous) {
			t.Errorf("transition replace=%v: %v", replace, err)
		}
	}
	for name, remove := range map[string]func() error{
		"slot":              func() error { return r.DeleteTaskEnvironmentRepo(ctx, "first") },
		"environment slots": func() error { return r.DeleteTaskEnvironmentReposByEnv(ctx, "env") },
		"environment":       func() error { return r.DeleteTaskEnvironment(ctx, "env") },
		"task environments": func() error { return r.DeleteTaskEnvironmentsByTask(ctx, "task") },
	} {
		if err := remove(); !errors.As(err, &ambiguous) {
			t.Errorf("%s: %v", name, err)
		}
	}

	var count int
	if err := r.db.Get(&count, `SELECT COUNT(*) FROM task_environment_repos WHERE worktree_id='physical'`); err != nil || count != 2 {
		t.Fatalf("inventory changed: count=%d %v", count, err)
	}
}
