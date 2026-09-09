package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
)

// fakeRoutineRunSyncer captures SyncRunStatus calls so tests can assert
// finalizeDone calls it with the right (taskID, terminalStatus) pair
// without pulling in the routines package.
type fakeRoutineRunSyncer struct {
	calls []struct{ taskID, terminalStatus string }
}

func (f *fakeRoutineRunSyncer) SyncRunStatus(_ context.Context, taskID, terminalStatus string) error {
	f.calls = append(f.calls, struct{ taskID, terminalStatus string }{taskID, terminalStatus})
	return nil
}

// TestFinalizeDone_SyncsRoutineRunOnDone verifies that a task moving
// into the Done step closes out its linked routine run via the wired
// RoutineRunSyncer (office-routine-runs D2) — the seam that lets a
// heavy routine's next fire through instead of gating on task_created
// forever.
func TestFinalizeDone_SyncsRoutineRunOnDone(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	syncer := &fakeRoutineRunSyncer{}
	svc.SetRoutineRunSyncer(syncer)

	createTestAgent(t, svc, "ws-1", "worker-done")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-done")

	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   taskID,
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-done",
		"to_step_name":              "Done",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": "worker-done",
		"parent_id":                 "",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	if len(syncer.calls) != 1 {
		t.Fatalf("expected 1 SyncRunStatus call, got %d", len(syncer.calls))
	}
	if syncer.calls[0].taskID != taskID {
		t.Errorf("taskID = %q, want %q", syncer.calls[0].taskID, taskID)
	}
	if syncer.calls[0].terminalStatus != "done" {
		t.Errorf("terminalStatus = %q, want done", syncer.calls[0].terminalStatus)
	}
}

// TestFinalizeDone_SyncsRoutineRunOnCancelled mirrors the Done case for
// a task moved to Cancelled — categorizeStep groups both under
// stepCategoryDone, so finalizeDone must distinguish them itself when
// telling the routine run which terminal status to write.
func TestFinalizeDone_SyncsRoutineRunOnCancelled(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	syncer := &fakeRoutineRunSyncer{}
	svc.SetRoutineRunSyncer(syncer)

	createTestAgent(t, svc, "ws-1", "worker-cancel")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-cancel")

	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   taskID,
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-cancelled",
		"to_step_name":              "Cancelled",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": "worker-cancel",
		"parent_id":                 "",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	if len(syncer.calls) != 1 {
		t.Fatalf("expected 1 SyncRunStatus call, got %d", len(syncer.calls))
	}
	if syncer.calls[0].terminalStatus != "cancelled" {
		t.Errorf("terminalStatus = %q, want cancelled", syncer.calls[0].terminalStatus)
	}
}

// TestFinalizeDone_NoSyncerIsANoop guards the optional-wiring contract:
// a service without a RoutineRunSyncer wired (most test services, and
// any real deploy before this seam existed) must not panic or error on
// a task reaching Done.
func TestFinalizeDone_NoSyncerIsANoop(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-nosync")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-nosync")

	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   taskID,
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-done",
		"to_step_name":              "Done",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": "worker-nosync",
		"parent_id":                 "",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}
}
