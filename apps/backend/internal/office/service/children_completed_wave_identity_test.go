package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/waveidentity"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// childMoveEvent builds the task.moved event queueChildrenCompletedRun's
// caller (finalizeDone) reacts to for a child of parentID landing in Done.
func childMoveEvent(childID, parentID string) *bus.Event {
	return bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   childID,
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-done",
		"to_step_name":              "Done",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": "worker-1",
		"parent_id":                 parentID,
	})
}

// TestQueueChildrenCompletedRun_DispatchesWaveIdentity is
// AC-OFFICE-WAKE-WAVE-IDENTITY-002.16's engine-seam half for P2: the edge-
// triggered path must attach the wave identity derived from its own
// terminality-confirming ListWaveMembers read.
func TestQueueChildrenCompletedRun_DispatchesWaveIdentity(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	if err := eb.Publish(ctx, "task.moved", childMoveEvent("parent-1-child-0", "parent-1")); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 dispatch, got %d: %#v", len(calls), calls)
	}
	payload, ok := calls[0].payload.(engine.OnChildrenCompletedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want engine.OnChildrenCompletedPayload", calls[0].payload)
	}
	wantIDs := []string{"parent-1-child-0", "parent-1-child-1", "parent-1-child-2"}
	if want := waveidentity.WaveKey("parent-1", wantIDs); payload.WaveKey != want {
		t.Fatalf("WaveKey = %q, want %q", payload.WaveKey, want)
	}
	if want := waveidentity.WaveString("parent-1", wantIDs); payload.WaveString != want {
		t.Fatalf("WaveString = %q, want %q", payload.WaveString, want)
	}
}

// TestQueueChildrenCompletedRun_NoWaveMembers_NoDispatch is AC-...-001.7
// exercised through the edge path: every child is ephemeral (so
// AreAllChildrenTerminal — which does not filter on is_ephemeral — still
// reports the parent ready) but none is a wave member, so no trigger is
// dispatched.
func TestQueueChildrenCompletedRun_NoWaveMembers_NoDispatch(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	svc.ExecSQL(t, `UPDATE tasks SET is_ephemeral = 1 WHERE parent_id = 'parent-1'`)

	if err := eb.Publish(ctx, "task.moved", childMoveEvent("parent-1-child-0", "parent-1")); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	if calls := dispatcher.Calls(); len(calls) != 0 {
		t.Fatalf("dispatched %d calls, want 0 (parent has no wave members)", len(calls))
	}
}

// TestParentWakeReconciler_DispatchesWaveIdentity is the P3 twin of
// TestQueueChildrenCompletedRun_DispatchesWaveIdentity.
func TestParentWakeReconciler_DispatchesWaveIdentity(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 dispatch, got %d: %#v", len(calls), calls)
	}
	payload, ok := calls[0].payload.(engine.OnChildrenCompletedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want engine.OnChildrenCompletedPayload", calls[0].payload)
	}
	wantIDs := []string{"parent-1-child-0", "parent-1-child-1", "parent-1-child-2"}
	if want := waveidentity.WaveKey("parent-1", wantIDs); payload.WaveKey != want {
		t.Fatalf("WaveKey = %q, want %q", payload.WaveKey, want)
	}
}

// TestChildrenCompletedWaveIdentity_IdenticalAcrossEdgeAndReconcilerPaths
// is this task's acceptance bullet on derivation parity: P2 and P3 must
// derive byte-identical wave keys for the same parent and wave-member set,
// so idx_run_wake_wave actually collapses a race between them (proven
// fully in Task 07; here only derivation parity is checked).
func TestChildrenCompletedWaveIdentity_IdenticalAcrossEdgeAndReconcilerPaths(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	if err := eb.Publish(ctx, "task.moved", childMoveEvent("parent-1-child-0", "parent-1")); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	calls := dispatcher.Calls()
	if len(calls) != 2 {
		t.Fatalf("want 2 dispatcher calls (edge + reconciler), got %d: %#v", len(calls), calls)
	}
	edge, ok := calls[0].payload.(engine.OnChildrenCompletedPayload)
	if !ok {
		t.Fatalf("edge payload type = %T, want engine.OnChildrenCompletedPayload", calls[0].payload)
	}
	reconciler, ok := calls[1].payload.(engine.OnChildrenCompletedPayload)
	if !ok {
		t.Fatalf("reconciler payload type = %T, want engine.OnChildrenCompletedPayload", calls[1].payload)
	}
	if edge.WaveKey == "" || reconciler.WaveKey == "" {
		t.Fatalf("expected non-empty wave keys, got edge=%q reconciler=%q", edge.WaveKey, reconciler.WaveKey)
	}
	if edge.WaveKey != reconciler.WaveKey {
		t.Fatalf("edge and reconciler derived different wave keys: edge=%q reconciler=%q",
			edge.WaveKey, reconciler.WaveKey)
	}
}
