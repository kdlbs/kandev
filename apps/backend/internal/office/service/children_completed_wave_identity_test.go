package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/waveidentity"
	"github.com/kandev/kandev/internal/workflow/engine"
)

func TestParentWakeReconciler_DispatchesWaveIdentity(t *testing.T) {
	svc := newTestService(t)
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(context.Background()); err != nil {
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
	if want := waveidentity.WaveString("parent-1", wantIDs); payload.WaveString != want {
		t.Fatalf("WaveString = %q, want %q", payload.WaveString, want)
	}
}

func TestParentWakeReconciler_DoesNotRedispatchStatePathCompletion(t *testing.T) {
	svc, repo := newTestServiceWithRunsService(t)
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `CREATE TABLE workspaces (
		id TEXT PRIMARY KEY,
		office_workflow_id TEXT DEFAULT ''
	)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		project_id TEXT DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TODO',
		workflow_step_id TEXT DEFAULT '',
		assignee_user_id TEXT NOT NULL DEFAULT '',
		assignment_generation INTEGER NOT NULL DEFAULT 0,
		is_ephemeral INTEGER DEFAULT 0,
		origin TEXT DEFAULT 'manual',
		parent_id TEXT DEFAULT '',
		archived_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `CREATE TABLE workflow_step_participants (
		id TEXT PRIMARY KEY,
		step_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		decision_required INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("create workflow step participants table: %v", err)
	}
	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-state-path", "worker-state-path")
	childSetKey, generation, err := repo.GetChildSetKeyAndGeneration(ctx, "parent-state-path")
	if err != nil {
		t.Fatalf("get state-path child identity: %v", err)
	}
	if err := repo.RecordWorkflowChildrenCompletedWake(
		ctx, "parent-state-path", childSetKey, generation, "on_children_completed:state-path",
	); err != nil {
		t.Fatalf("record state-path completion: %v", err)
	}

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if calls := dispatcher.Calls(); len(calls) != 0 {
		t.Fatalf("state-path completion was dispatched again by reconciler: %#v", calls)
	}
}

func TestParentWakeReconciler_NoWaveMembersDoesNotDispatch(t *testing.T) {
	svc := newTestService(t)
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	svc.ExecSQL(t, `UPDATE tasks SET is_ephemeral = 1 WHERE parent_id = 'parent-1'`)

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if calls := dispatcher.Calls(); len(calls) != 0 {
		t.Fatalf("dispatched %d calls, want none (parent has no wave members)", len(calls))
	}
}
func TestParentWakeReconciler_DoesNotDispatchOrRecordReceiptWithCancelledWaveMember(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	svc.ExecSQL(t, `UPDATE tasks SET state = 'CANCELLED' WHERE id = ?`, "parent-1-child-2")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if calls := dispatcher.Calls(); len(calls) != 0 {
		t.Fatalf("dispatched %d workflow calls with a cancelled wave member, want 0", len(calls))
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt: %v", err)
	}
	if receipt != nil {
		t.Fatalf("cancelled wave member left a recovery receipt: %#v", receipt)
	}
}
