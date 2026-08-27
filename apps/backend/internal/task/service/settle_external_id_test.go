package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// TestSettleExternalIDNoOpWhenAbsent covers the handler's ability to call
// SettleExternalID unconditionally: a create with no external_id must settle
// as a trivial success so callers don't need a branch for the common case.
func TestSettleExternalIDNoOpWhenAbsent(t *testing.T) {
	svc, _, _ := createTestService(t)
	settled, survivor, err := svc.SettleExternalID(context.Background(), "task-does-not-exist", "")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !settled {
		t.Fatal("settled = false, want true (nothing to settle)")
	}
	if survivor != nil {
		t.Fatalf("survivor = %v, want nil", survivor)
	}
}

// TestSettleExternalIDSucceeds covers the normal path: exactly one row
// updated, dispatch may proceed.
func TestSettleExternalIDSucceeds(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	settled, survivor, err := svc.SettleExternalID(ctx, task.ID, "ext-1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !settled {
		t.Fatal("settled = false, want true")
	}
	if survivor != nil {
		t.Fatalf("survivor = %v, want nil on success", survivor)
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ExternalIDSettledAt == nil {
		t.Fatal("external_id_settled_at should be set")
	}
}

// TestSettleExternalIDIdentityLost covers the zero-row release race: the
// task survives but no longer holds the identity, so settlement affects zero
// rows. The caller (handler) must treat this as CreatedIdentityLost, not an
// error.
func TestSettleExternalIDIdentityLost(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.ReleaseTaskExternalID(ctx, "ws-1", "ext-1"); err != nil {
		t.Fatalf("release: %v", err)
	}

	settled, survivor, err := svc.SettleExternalID(ctx, task.ID, "ext-1")
	if err != nil {
		t.Fatalf("err = %v, want nil (task still exists)", err)
	}
	if settled {
		t.Fatal("settled = true, want false — the identity was released")
	}
	if survivor == nil || survivor.ID != task.ID {
		t.Fatalf("survivor = %v, want the surviving task", survivor)
	}
	if survivor.ExternalID != "" {
		t.Fatalf("survivor.ExternalID = %q, want empty", survivor.ExternalID)
	}
}

// TestSettleExternalIDTaskDeleted covers the zero-row delete race: the task
// no longer exists, so the caller must surface a not-found error rather than
// a fabricated success.
func TestSettleExternalIDTaskDeleted(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	settled, survivor, err := svc.SettleExternalID(ctx, task.ID, "ext-1")
	if !errors.Is(err, taskrepo.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
	if settled {
		t.Fatal("settled = true, want false")
	}
	if survivor != nil {
		t.Fatalf("survivor = %v, want nil", survivor)
	}
}
