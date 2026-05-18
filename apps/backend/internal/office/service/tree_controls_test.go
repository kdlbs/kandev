package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

type mockTaskCanceller struct {
	mu      sync.Mutex
	taskIDs []string
}

func (m *mockTaskCanceller) CancelTaskExecution(_ context.Context, taskID string, _ string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskIDs = append(m.taskIDs, taskID)
	return nil
}

func (m *mockTaskCanceller) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.taskIDs)
}

func insertTreeControlTask(t *testing.T, svc *service.Service, id, parentID, state string) {
	t.Helper()
	svc.ExecSQL(t, `
		INSERT INTO tasks (id, workspace_id, title, state, parent_id, created_at, updated_at)
		VALUES (?, 'ws-1', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, id, id, state, parentID)
}

func TestPauseTaskTree(t *testing.T) {
	canceller := &mockTaskCanceller{}
	svc := newTestService(t, service.ServiceOptions{TaskCanceller: canceller})
	ctx := context.Background()
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")

	hold, err := svc.PauseTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PauseTaskTree: %v", err)
	}
	if hold.Mode != models.TreeHoldModePause {
		t.Fatalf("hold mode = %q, want pause", hold.Mode)
	}
	preview, err := svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree: %v", err)
	}
	if preview.ActiveHold == nil || preview.ActiveHold.ID != hold.ID {
		t.Fatalf("active hold = %+v, want %s", preview.ActiveHold, hold.ID)
	}
	if canceller.count() != 2 {
		t.Fatalf("cancel count = %d, want 2", canceller.count())
	}
}

func TestCancelAndRestoreTaskTree(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")
	insertTreeControlTask(t, svc, "child", "root", "TODO")
	insertTreeControlTask(t, svc, "already-cancelled", "root", "CANCELLED")

	hold, err := svc.CancelTaskTree(ctx, "root", "user:test")
	if err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}
	for _, taskID := range []string{"root", "child", "already-cancelled"} {
		fields, err := svc.GetTaskExecutionFieldsForTest(ctx, taskID)
		if err != nil {
			t.Fatalf("fields %s: %v", taskID, err)
		}
		if fields.State != "CANCELLED" {
			t.Fatalf("state %s = %s, want CANCELLED", taskID, fields.State)
		}
	}
	if _, err := svc.RestoreTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("RestoreTaskTree: %v", err)
	}
	wantStates := map[string]string{
		"root":              "IN_PROGRESS",
		"child":             "TODO",
		"already-cancelled": "CANCELLED",
	}
	for taskID, want := range wantStates {
		fields, err := svc.GetTaskExecutionFieldsForTest(ctx, taskID)
		if err != nil {
			t.Fatalf("fields %s after restore: %v", taskID, err)
		}
		if fields.State != want {
			t.Fatalf("state %s = %s, want %s", taskID, fields.State, want)
		}
	}
	preview, err := svc.PreviewTaskTree(ctx, "root")
	if err != nil {
		t.Fatalf("PreviewTaskTree: %v", err)
	}
	if preview.ActiveHold != nil {
		t.Fatalf("active hold after restore = %+v, want nil; cancel hold was %s", preview.ActiveHold, hold.ID)
	}
}
