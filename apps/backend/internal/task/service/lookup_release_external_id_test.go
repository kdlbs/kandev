package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// TestGetTaskByExternalID covers the REST lookup route's service-level
// behavior: found (including unsettled), not found, and validation ordering
// (invalid external_id must not leak whether a workspace exists via a
// different status than "not found" -- validation happens after auth but
// this test focuses on the found/not-found/invalid trichotomy).
func TestGetTaskByExternalID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	found, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if found.ID != task.ID {
		t.Fatalf("found.ID = %s, want %s", found.ID, task.ID)
	}
	if found.ExternalIDSettledAt != nil {
		t.Fatal("unsettled task must still be returned by lookup")
	}

	if _, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-missing"); !errors.Is(err, taskrepo.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}

	if _, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-1\n"); err == nil {
		t.Fatal("expected a validation error for a control character in external_id")
	}
}

// TestGetTaskByExternalIDReturnsArchivedTasks pins "including archived" from
// the spec's lookup table.
func TestGetTaskByExternalIDReturnsArchivedTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	found, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if found.ArchivedAt == nil {
		t.Fatal("archived task should be returned with archived_at set, not auto-unarchived")
	}
}

// TestReleaseTaskExternalID covers the REST release route's service-level
// behavior: success frees the identity without deleting the task, and
// releasing an identity nothing holds reports false rather than erroring.
func TestReleaseTaskExternalID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	released, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released {
		t.Fatal("release should report the identity was held")
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("task must still exist after release: %v", err)
	}
	if reloaded.ExternalID != "" {
		t.Fatalf("reloaded.ExternalID = %q, want empty", reloaded.ExternalID)
	}

	again, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("second release: %v", err)
	}
	if again {
		t.Fatal("releasing an identity nothing holds should report false, not error")
	}

	if _, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1\n"); err == nil {
		t.Fatal("expected a validation error for a control character in external_id")
	}
}
