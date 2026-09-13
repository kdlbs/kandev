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

// TestPauseTaskTree_RecordsTerminalShapeForCancelledRuns is the Testing
// round 3 regression test. PauseTaskTree cancels every member task's runs
// via repo.CancelRunsForTasks, which bypassed office_loop_terminal_total
// entirely — the same bypass class Review round 1 fixed for the
// single-run CancelRun call sites. A queued run never launched, so
// pausing its tree must classify the cancellation as unlaunched_failed.
func TestPauseTaskTree_RecordsTerminalShapeForCancelledRuns(t *testing.T) {
	canceller := &mockTaskCanceller{}
	svc := newTestService(t, service.ServiceOptions{TaskCanceller: canceller})
	ctx := context.Background()
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")

	agent := makeAgent("tree-pause-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"root"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	key := service.LoopMetricLabel("workspace", agent.WorkspaceID, "shape", string(service.ShapeUnlaunchedFailed))
	before := terminalShapeExpvarInt(t, key)

	if _, err := svc.PauseTaskTree(ctx, "root"); err != nil {
		t.Fatalf("PauseTaskTree: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("unlaunched_failed delta = %d, want 1", after-before)
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

// TestCancelTaskTree_RecordsTerminalShapeForCancelledRuns mirrors
// TestPauseTaskTree_RecordsTerminalShapeForCancelledRuns for
// CancelTaskTree's own repo.CancelRunsForTasks call site.
func TestCancelTaskTree_RecordsTerminalShapeForCancelledRuns(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	insertTreeControlTask(t, svc, "root", "", "IN_PROGRESS")

	agent := makeAgent("tree-cancel-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"root"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	key := service.LoopMetricLabel("workspace", agent.WorkspaceID, "shape", string(service.ShapeUnlaunchedFailed))
	before := terminalShapeExpvarInt(t, key)

	if _, err := svc.CancelTaskTree(ctx, "root", "user:test"); err != nil {
		t.Fatalf("CancelTaskTree: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("unlaunched_failed delta = %d, want 1", after-before)
	}
}
