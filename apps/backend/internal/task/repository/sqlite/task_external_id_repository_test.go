package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedWorkspaceForExternalID(t *testing.T, repo *Repository, id string) {
	t.Helper()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: id, Name: id}); err != nil {
		t.Fatalf("seed workspace %q: %v", id, err)
	}
}

// TestCreateTaskPersistsExternalID covers the golden path: a task created
// with an external_id stores it (unsettled — settlement is the caller's
// job per the create-sequence contract) and GetTask/GetTaskByExternalID both
// surface it.
func TestCreateTaskPersistsExternalID(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-persist")

	task := &models.Task{WorkspaceID: "ws-persist", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task with external_id: %v", err)
	}
	if task.ID == "" {
		t.Fatal("expected an assigned task ID")
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if reloaded.ExternalID != "ext-1" {
		t.Fatalf("reloaded.ExternalID = %q, want ext-1", reloaded.ExternalID)
	}
	if reloaded.ExternalIDSettledAt != nil {
		t.Fatalf("reloaded.ExternalIDSettledAt = %v, want nil (settlement is a separate step)", reloaded.ExternalIDSettledAt)
	}

	byExternal, err := repo.GetTaskByExternalID(ctx, "ws-persist", "ext-1")
	if err != nil {
		t.Fatalf("get task by external id: %v", err)
	}
	if byExternal.ID != task.ID {
		t.Fatalf("GetTaskByExternalID returned %s, want %s", byExternal.ID, task.ID)
	}
}

// TestGetTaskByExternalIDNotFound covers the lookup-miss path.
func TestGetTaskByExternalIDNotFound(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-missing")

	if _, err := repo.GetTaskByExternalID(ctx, "ws-missing", "ext-nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

// TestCreateTaskDuplicateExternalIDConflict is the TOCTOU-backstop path: a
// second insert for an already-held (workspace_id, external_id) must
// classify specifically as ErrExternalIDConflict, attributable to
// uniq_tasks_external_id — never a generic/opaque failure.
func TestCreateTaskDuplicateExternalIDConflict(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-conflict")

	first := &models.Task{WorkspaceID: "ws-conflict", Title: "First", ExternalID: "ext-race"}
	if err := repo.CreateTask(ctx, first); err != nil {
		t.Fatalf("create first task: %v", err)
	}

	second := &models.Task{WorkspaceID: "ws-conflict", Title: "Second", ExternalID: "ext-race"}
	err := repo.CreateTask(ctx, second)
	if !errors.Is(err, ErrExternalIDConflict) {
		t.Fatalf("err = %v, want ErrExternalIDConflict", err)
	}
}

// TestCreateTaskPrimaryKeyCollisionIsNotClassifiedAsExternalIDConflict covers
// the spec's uniqueness/concurrency edge case: a task-row insert that
// collides on the tasks primary key (an explicit duplicate ID), rather than
// on uniq_tasks_external_id, must surface as a plain error — never
// misclassified as ErrExternalIDConflict, which would make CreateTask's
// re-read backstop wrongly treat an unrelated ID collision as "someone else
// already holds this identity" and fabricate a Found outcome.
func TestCreateTaskPrimaryKeyCollisionIsNotClassifiedAsExternalIDConflict(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-pk-collision")

	first := &models.Task{ID: "dup-id", WorkspaceID: "ws-pk-collision", Title: "First", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, first); err != nil {
		t.Fatalf("create first task: %v", err)
	}

	second := &models.Task{ID: "dup-id", WorkspaceID: "ws-pk-collision", Title: "Second", ExternalID: "ext-2"}
	err := repo.CreateTask(ctx, second)
	if err == nil {
		t.Fatal("expected an error for a duplicate primary key")
	}
	if errors.Is(err, ErrExternalIDConflict) {
		t.Fatalf("err = %v, want a plain error — a primary-key collision is unrelated to uniq_tasks_external_id", err)
	}

	// The second task's own identity (ext-2) must not have been claimed by
	// anything else — it's simply absent, since the insert never committed.
	if _, lookupErr := repo.GetTaskByExternalID(ctx, "ws-pk-collision", "ext-2"); !errors.Is(lookupErr, ErrTaskNotFound) {
		t.Fatalf("GetTaskByExternalID(ext-2) err = %v, want ErrTaskNotFound", lookupErr)
	}
}

// TestCreateTaskSameExternalIDDifferentWorkspaces confirms uniqueness is
// scoped per workspace, not global.
func TestCreateTaskSameExternalIDDifferentWorkspaces(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-a")
	seedWorkspaceForExternalID(t, repo, "ws-b")

	a := &models.Task{WorkspaceID: "ws-a", Title: "A", ExternalID: "ext-shared"}
	if err := repo.CreateTask(ctx, a); err != nil {
		t.Fatalf("create task in ws-a: %v", err)
	}
	b := &models.Task{WorkspaceID: "ws-b", Title: "B", ExternalID: "ext-shared"}
	if err := repo.CreateTask(ctx, b); err != nil {
		t.Fatalf("create task in ws-b with the same external_id: %v", err)
	}
}

// TestSettleTaskExternalID exercises the conditional-UPDATE contract: one
// row affected on a valid unsettled row, zero rows once already settled or
// once the identity has moved on (released), per the spec's settlement
// predicate requirement (external_id must be part of the WHERE clause).
func TestSettleTaskExternalID(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-settle")

	task := &models.Task{WorkspaceID: "ws-settle", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	ok, err := repo.SettleTaskExternalID(ctx, task.ID, "ext-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !ok {
		t.Fatal("first settlement should affect exactly one row")
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload after settle: %v", err)
	}
	if reloaded.ExternalIDSettledAt == nil {
		t.Fatal("external_id_settled_at should be set after settlement")
	}

	ok, err = repo.SettleTaskExternalID(ctx, task.ID, "ext-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("second settle: %v", err)
	}
	if ok {
		t.Fatal("settling an already-settled row must affect zero rows")
	}
}

// TestSettleTaskExternalIDRequiresExternalIDPredicate is the spec's explicit
// load-bearing case: release nulls both columns, so a predicate guarding
// only on id + settled_at IS NULL would wrongly stamp a released row.
func TestSettleTaskExternalIDRequiresExternalIDPredicate(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-settle-released")

	task := &models.Task{WorkspaceID: "ws-settle-released", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	released, err := repo.ReleaseTaskExternalID(ctx, "ws-settle-released", "ext-1")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released == nil {
		t.Fatal("release should report the identity was held")
	}

	ok, err := repo.SettleTaskExternalID(ctx, task.ID, "ext-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("settle after release: %v", err)
	}
	if ok {
		t.Fatal("settlement after release must affect zero rows — the external_id predicate no longer matches")
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload after settle-after-release: %v", err)
	}
	if reloaded.ExternalID != "" || reloaded.ExternalIDSettledAt != nil {
		t.Fatalf("released task must stay released: external_id=%q settled_at=%v", reloaded.ExternalID, reloaded.ExternalIDSettledAt)
	}
}

// TestReleaseTaskExternalID covers release freeing the identity without
// deleting the task, and reports false when nothing holds the identity.
func TestReleaseTaskExternalID(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-release")

	task := &models.Task{WorkspaceID: "ws-release", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.SettleTaskExternalID(ctx, task.ID, "ext-1", time.Now().UTC()); err != nil {
		t.Fatalf("settle: %v", err)
	}

	released, err := repo.ReleaseTaskExternalID(ctx, "ws-release", "ext-1")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released == nil {
		t.Fatal("release should report the identity was held")
	}
	if released.ID != task.ID {
		t.Fatalf("released.ID = %s, want %s", released.ID, task.ID)
	}
	if !released.UpdatedAt.After(task.UpdatedAt) {
		t.Fatalf("released.UpdatedAt = %v, want after original create time %v — release must bump updated_at like any other task mutation", released.UpdatedAt, task.UpdatedAt)
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("task must still exist after release: %v", err)
	}
	if reloaded.ExternalID != "" {
		t.Fatalf("reloaded.ExternalID = %q, want empty after release", reloaded.ExternalID)
	}
	if reloaded.ExternalIDSettledAt != nil {
		t.Fatal("reloaded.ExternalIDSettledAt should be nil after release")
	}

	if _, err := repo.GetTaskByExternalID(ctx, "ws-release", "ext-1"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound after release", err)
	}

	again, err := repo.ReleaseTaskExternalID(ctx, "ws-release", "ext-1")
	if err != nil {
		t.Fatalf("second release: %v", err)
	}
	if again != nil {
		t.Fatal("releasing an identity nothing holds should report nil, not a task")
	}
}

// TestReleaseTaskExternalIDAllowsReuse confirms a released identity can be
// claimed by a brand-new task — idempotency is scoped to the task's
// lifetime, not the identity's.
func TestReleaseTaskExternalIDAllowsReuse(t *testing.T) {
	repo, _ := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	seedWorkspaceForExternalID(t, repo, "ws-reuse")

	first := &models.Task{WorkspaceID: "ws-reuse", Title: "First", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, first); err != nil {
		t.Fatalf("create first task: %v", err)
	}
	if _, err := repo.ReleaseTaskExternalID(ctx, "ws-reuse", "ext-1"); err != nil {
		t.Fatalf("release: %v", err)
	}

	second := &models.Task{WorkspaceID: "ws-reuse", Title: "Second", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, second); err != nil {
		t.Fatalf("recreate after release: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("recreate after release must produce a new task")
	}
}
