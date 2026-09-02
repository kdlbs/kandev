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
// children and an assignee, satisfying ListStuckParents' predicate. The
// project_id marker (matching scheduler_recovery_test.go's own
// 'office-project' marker) satisfies the authoritative-Office-task
// predicate ListStuckParents applies alongside adoptOffice's workspace-level
// HasOfficeAdoption signal — the two are independent checks.
func seedStuckParent(t *testing.T, svc *service.Service, wsID, parentID, agentID string) {
	t.Helper()
	createTestAgent(t, svc, wsID, agentID)
	insertTestTask(t, svc, parentID, wsID)
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID)
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
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

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
	if len(runs) != 0 {
		t.Fatalf("reconciler inserted a run instead of dispatching through the workflow engine: %#v", runs)
	}
	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("want exactly one engine dispatch after tick 1, got %d: %#v", len(calls), calls)
	}
	if calls[0].taskID != "parent-1" {
		t.Fatalf("engine dispatch task = %q, want parent-1", calls[0].taskID)
	}
	if calls[0].opID == "" {
		t.Fatal("engine dispatch operation id is empty")
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt: %v", err)
	}
	if receipt == nil || receipt.DeliveryOperationID == "" {
		t.Fatalf("engine dispatch did not persist an operation-backed receipt: %#v", receipt)
	}

	// Tick 2: the operation-backed receipt excludes parent-1 for the same
	// child set, so the sweep must be a no-op.
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	runsAfter, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs after tick 2: %v", err)
	}
	if len(runsAfter) != 0 {
		t.Fatalf("tick 2 created an unexpected run: %#v", runsAfter)
	}
	if got := len(dispatcher.Calls()); got != 1 {
		t.Fatalf("tick 2 dispatched a duplicate engine operation: got %d calls", got)
	}
}

// TestParentWakeReconciler_RedeliversAfterChildReopenedAndRecompleted is the
// reopened-child regression test end-to-end: a child that completes, is
// reopened, and completes again with the same terminal state produces a
// byte-identical child_set_key, but the mutation happened after the first
// wake's receipt was recorded. The reconciler must sweep it again on the
// next tick, and the second dispatch's operation id must differ from the
// first so the engine's permanent operation ledger does not swallow it as
// already-applied.
func TestParentWakeReconciler_RedeliversAfterChildReopenedAndRecompleted(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("want exactly one engine dispatch after tick 1, got %d: %#v", len(calls), calls)
	}
	firstOpID := calls[0].opID

	// One of the terminal children is reopened and re-completed after the
	// receipt from tick 1 was recorded — a supported board action.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = '2099-01-01 00:00:00' WHERE id = ?`, "parent-1-child-0")
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = '2099-01-01 00:00:01' WHERE id = ?`, "parent-1-child-0")

	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	callsAfter := dispatcher.Calls()
	if len(callsAfter) != 2 {
		t.Fatalf("LOST WAKE NOT RECOVERABLE: want 2 engine dispatches after re-completion, got %d: %#v", len(callsAfter), callsAfter)
	}
	secondOpID := callsAfter[1].opID
	if secondOpID == firstOpID {
		t.Fatalf("second delivery reused the first operation id %q; the engine's permanent operation ledger would swallow it as already-applied", secondOpID)
	}
}

// TestParentWakeReconciler_RedeliversAfterPersistedReceiptInvalidatedByReopen
// is R4-F2's regression test. Unlike
// TestParentWakeReconciler_RedeliversAfterChildReopenedAndRecompleted, the
// reopen here lands in a later generation whose text still differs from the
// first receipt's, so the redelivery on tick 2 is driven purely by
// ListStuckParents' third OR arm (newest_child_updated_at != child_generation)
// — the other two arms both stay false because the reopen leaves
// child_set_key and the stored delivery_operation_id untouched.
func TestParentWakeReconciler_RedeliversAfterPersistedReceiptInvalidatedByReopen(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)
	const childID = "parent-1-child-0"

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt after tick 1: %v", err)
	}
	if receipt == nil || receipt.DeliveryOperationID == "" {
		t.Fatalf("tick 1 did not persist a receipt: %#v", receipt)
	}
	firstGeneration := receipt.ChildGeneration

	// Reopen and re-complete the child in a later, distinct generation, so
	// child_set_key stays byte-identical to what the persisted receipt
	// already matches but child_generation changes.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = '2099-01-01 00:00:00' WHERE id = ?`, childID)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = '2099-01-01 00:00:01' WHERE id = ?`, childID)

	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if got := len(dispatcher.Calls()); got != 2 {
		t.Fatalf("LOST WAKE: reopen after a persisted receipt for the prior generation was not redelivered, got %d dispatches: %#v", got, dispatcher.Calls())
	}
	receiptAfter, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt after tick 2: %v", err)
	}
	if receiptAfter == nil || receiptAfter.ChildGeneration == firstGeneration {
		t.Fatalf("receipt still reflects the stale generation after redelivery: %#v", receiptAfter)
	}
}

// seedTerminalChildrenCompletedRun inserts a finished
// task_children_completed run for parentTaskID at requestedAt.
func seedTerminalChildrenCompletedRun(t *testing.T, svc *service.Service, runID, agentID, parentTaskID, requestedAt string) {
	t.Helper()
	svc.ExecSQL(t, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES (?, ?, 'task_children_completed', ?, 'finished', ?)
	`, runID, agentID, `{"task_id":"`+parentTaskID+`"}`, requestedAt)
}

// TestParentWakeReconciler_SameSecondReopenSuppressedByEdgePathRun and
// TestParentWakeReconciler_LaterSecondReopenRedeliveredAfterEdgePathRun pin
// the accepted residual: ListStuckParents' NOT EXISTS runs arm
// (wake_receipts.go) drops a candidate whose newest child generation is not
// strictly newer than a terminal task_children_completed run's
// requested_at, in the same wall-clock second as that run. In production the
// edge path (cascadeChildrenCompleted) always writes exactly this kind of
// run for the parent's first completion, so a reopen+recomplete landing in
// that same second is not recovered by this reconciler — only a later
// second is. Both tests below seed a real runs row rather than using
// fakeDispatcher's no-run defaults, closing the coverage gap that hid this
// behind R6-F1/R6-F2: every other reconciler test leaves the runs table
// empty, so the NOT EXISTS arm was never exercised.
func TestParentWakeReconciler_SameSecondReopenSuppressedByEdgePathRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)
	const childID = "parent-1-child-0"

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	// The edge path's run for the parent's first completion, requested in
	// the same second the reopen+recomplete below will also land in.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = '2050-06-01 12:00:00' WHERE id = ?`, childID)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = '2050-06-01 12:00:00' WHERE id = ?`, childID)
	seedTerminalChildrenCompletedRun(t, svc, "edge-run-1", "worker-1", "parent-1", "2050-06-01 12:00:00")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if got := len(dispatcher.Calls()); got != 0 {
		t.Fatalf("ACCEPTED RESIDUAL regressed: a same-second reopen must stay suppressed by the edge path's run, got %d dispatches: %#v", got, dispatcher.Calls())
	}
}

func TestParentWakeReconciler_LaterSecondReopenRedeliveredAfterEdgePathRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)
	const childID = "parent-1-child-0"

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	// The edge path's run for the parent's first completion.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = '2050-06-01 12:00:00' WHERE id = ?`, childID)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = '2050-06-01 12:00:00' WHERE id = ?`, childID)
	seedTerminalChildrenCompletedRun(t, svc, "edge-run-1", "worker-1", "parent-1", "2050-06-01 12:00:00")

	// Reopen+recomplete in a strictly later second than the edge run.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = '2050-06-01 12:00:01' WHERE id = ?`, childID)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = '2050-06-01 12:00:01' WHERE id = ?`, childID)

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if got := len(dispatcher.Calls()); got != 1 {
		t.Fatalf("want exactly one dispatch for a later-second reopen, got %d: %#v", got, dispatcher.Calls())
	}
}

func TestParentWakeReconciler_SkipsWithoutEngineDispatcher(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
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
		t.Fatalf("reconciler inserted a run without an engine dispatcher: %#v", runs)
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt: %v", err)
	}
	if receipt != nil {
		t.Fatalf("missing dispatcher left a delivery receipt: %#v", receipt)
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

// seedStuckParentNoAssignee creates a parent task with terminal children
// but no runner participant row, so RunnerProjection returns an empty
// string and ListStuckParents' assignee_agent_profile_id != ” guard
// filters it out. Used to verify that unresolvable candidates cannot
// starve the LIMIT ahead of a healthy parent.
func seedStuckParentNoAssignee(t *testing.T, svc *service.Service, wsID, parentID string) {
	t.Helper()
	insertTestTask(t, svc, parentID, wsID)
	states := []string{"COMPLETED", "COMPLETED", "CANCELLED"}
	for i, state := range states {
		childID := fmt.Sprintf("%s-child-%d", parentID, i)
		insertTestTask(t, svc, childID, wsID)
		svc.ExecSQL(t, `UPDATE tasks SET parent_id = ?, state = ? WHERE id = ?`,
			parentID, state, childID)
	}
}

// seedStuckParentDanglingAssignee is seedStuckParent but points the runner
// participant row at an agent_profile_id with no matching agent_profiles
// row — R3-B's sticky case: a dangling reference that a LEFT JOIN against
// agent_profiles let through as COALESCE'd-to-idle, occupying a LIMIT slot
// forever since nothing about a dangling reference ever resolves itself.
func seedStuckParentDanglingAssignee(t *testing.T, svc *service.Service, wsID, parentID string) {
	t.Helper()
	insertTestTask(t, svc, parentID, wsID)
	setTestTaskAssignee(t, svc, parentID, parentID+"-dangling-agent")
	states := []string{"COMPLETED", "COMPLETED", "CANCELLED"}
	for i, state := range states {
		childID := fmt.Sprintf("%s-child-%d", parentID, i)
		insertTestTask(t, svc, childID, wsID)
		svc.ExecSQL(t, `UPDATE tasks SET parent_id = ?, state = ? WHERE id = ?`,
			parentID, state, childID)
	}
}

// TestParentWakeReconciler_UnresolvedAndPausedDoNotStarveHealthyParent is
// R2-A's regression test, tightened by R3-C after review proved it vacuous:
// deleting both SQL assignee predicates and re-running left it unchanged
// green, because ORDER BY parent_task_id already put "healthy-parent" ahead
// of every "stuck-*"/"zz-*" candidate (h < s, h < z) regardless of whether
// the fix was present. The healthy parent is now named to sort strictly
// after every sticky candidate ("zz-healthy-parent" > "stuck-*"), so this
// test goes red against a reintroduced ordering-only guard instead of
// passing by accident.
//
// The unresolved-runner and paused-agent candidates are excluded by
// ListStuckParents' WHERE clause before its LIMIT is applied, so — now
// that both live in SQL — they cost no LIMIT slot at all and cannot by
// themselves starve anything; they stay here purely as regression coverage
// that this fix does not regress. The 5 dangling-agent-profile candidates
// are what actually drives this test red against R3-B's unfixed LEFT JOIN:
// a LEFT JOIN against agent_profiles COALESCEs a dangling reference's NULL
// status to 'idle', which passes the assignee filter and — unlike the
// unresolved/paused cases — genuinely consumes a LIMIT slot, is exactly as
// sticky (nothing about a dangling reference ever starts resolving), and
// five of them alone are enough to fill maxWakeReconcilePerTick (5) ahead
// of "zz-healthy-parent" on every tick, forever.
func TestParentWakeReconciler_UnresolvedAndPausedDoNotStarveHealthyParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")

	// 3 candidates with no resolvable runner at all.
	for i := 0; i < 3; i++ {
		seedStuckParentNoAssignee(t, svc, "ws-1", fmt.Sprintf("stuck-unresolved-%d", i))
	}
	// 2 candidates with a paused assignee.
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("stuck-paused-%d", i)
		agentID := fmt.Sprintf("paused-agent-%d", i)
		seedStuckParent(t, svc, "ws-1", id, agentID)
		if err := svc.UpdateAgentStatusFields(ctx, agentID, string(models.AgentStatusPaused), "test"); err != nil {
			t.Fatalf("pause agent %s: %v", agentID, err)
		}
	}
	// 5 candidates with a dangling agent_profile_id (R3-B) — enough on
	// their own to fill maxWakeReconcilePerTick ahead of the healthy parent.
	for i := 0; i < 5; i++ {
		seedStuckParentDanglingAssignee(t, svc, "ws-1", fmt.Sprintf("stuck-dangling-%d", i))
	}
	// The one genuinely healthy stuck parent, named to sort strictly after
	// every sticky candidate above so an ordered scan reaches it last.
	seedStuckParent(t, svc, "ws-1", "zz-healthy-parent", "zz-healthy-worker")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	for tick := 0; tick < 3; tick++ {
		if err := handler.Tick(ctx); err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
	}

	for _, call := range dispatcher.Calls() {
		if call.taskID == "zz-healthy-parent" {
			return
		}
	}
	t.Fatalf("zz-healthy-parent was never dispatched across 3 ticks behind 10 sticky candidates: %#v", dispatcher.Calls())
}
