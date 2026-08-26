package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// adoptOffice satisfies HasOfficeAdoption via an office_projects row, the
// same shape scheduler_recovery_test.go uses.
func adoptOffice(t *testing.T, svc *service.Service, wsID string) {
	t.Helper()
	svc.ExecSQL(t, `
		INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, 'Office Project', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "office-project-"+wsID, wsID)
}

// seedStuckParent creates a parent task with three terminal (non-archived)
// children and an assignee, satisfying ListStuckParents' predicate.
func seedStuckParent(t *testing.T, svc *service.Service, wsID, parentID, agentID string) {
	t.Helper()
	createTestAgent(t, svc, wsID, agentID)
	insertTestTask(t, svc, parentID, wsID)
	setTestTaskAssignee(t, svc, parentID, agentID)

	states := []string{"COMPLETED", "COMPLETED", "CANCELLED"}
	for i, state := range states {
		childID := fmt.Sprintf("%s-child-%d", parentID, i)
		insertTestTask(t, svc, childID, wsID)
		svc.ExecSQL(t, `UPDATE tasks SET parent_id = ?, state = ? WHERE id = ?`,
			parentID, state, childID)
	}
}

func TestParentWakeReconciler_SkipsWithoutOfficeAdoption(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("reconciler ran without Office adoption: %#v", runs)
	}
}

func TestParentWakeReconciler_EmitsRunForStuckParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("want exactly one run after tick 1, got %d: %#v", len(runs), runs)
	}
	run := runs[0]
	if run.Reason != service.RunReasonTaskChildrenCompleted {
		t.Fatalf("run reason = %q, want %q", run.Reason, service.RunReasonTaskChildrenCompleted)
	}
	if run.IdempotencyKey != nil {
		t.Fatalf("run idempotency key = %q, want nil (NULL)", *run.IdempotencyKey)
	}

	// Tick 2: the tick-1 run is still queued, so ListStuckParents' NOT
	// EXISTS-against-runs filter excludes parent-1 from the candidate set
	// entirely (see wake_receipts_test.go for a dedicated test of that
	// filter) — the sweep must be a no-op, with no second run.
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	runsAfter, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs after tick 2: %v", err)
	}
	if len(runsAfter) != 1 {
		t.Fatalf("tick 2 created a new run: %#v", runsAfter)
	}
}

func TestParentWakeReconciler_SkipsCompletedParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED' WHERE id = ?`, "parent-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("reconciler swept an already-terminal parent: %#v", runs)
	}
}

func TestParentWakeReconciler_SkipsEphemeralParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	svc.ExecSQL(t, `UPDATE tasks SET is_ephemeral = 1 WHERE id = ?`, "parent-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("reconciler swept an ephemeral parent: %#v", runs)
	}
}

func TestParentWakeReconciler_SkipsPausedAssigneeWithoutPartialReceipt(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	if err := svc.UpdateAgentStatusFields(ctx, "worker-1", string(models.AgentStatusPaused), "test"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("reconciler emitted a run for a paused assignee: %#v", runs)
	}

	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt: %v", err)
	}
	if receipt != nil {
		t.Fatalf("paused-assignee skip left a partial receipt: %#v", receipt)
	}
}
