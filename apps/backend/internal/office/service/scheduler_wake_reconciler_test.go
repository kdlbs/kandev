package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// waitForNextWholeSecond blocks until the wall clock crosses into a new
// second, so a CURRENT_TIMESTAMP write taken after it produces different
// text than one taken before it. Mirrors the sqlite package's helper of the
// same name (wake_receipts_test.go): that one proves ListStuckParents'
// generation comparison itself; this one is needed because recordReceipt's
// still-open-second guard runs against real wall-clock time, so tests that
// care whether a receipt was (or was not) persisted need to control which
// side of a second boundary they land on.
func waitForNextWholeSecond(t *testing.T) {
	t.Helper()
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(1100 * time.Millisecond)))
}

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

	// The seeded children's updated_at defaults to CURRENT_TIMESTAMP at
	// insert time, i.e. "now". recordReceipt refuses to persist a receipt
	// for a still-open generation second (see
	// TestParentWakeReconciler_RedeliversAfterSameSecondReopenAndRecomplete),
	// so this test — which asserts the receipt lands synchronously after a
	// single tick — needs the generation safely closed first.
	waitForNextWholeSecond(t)

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

// TestParentWakeReconciler_RedeliversAfterSameSecondReopenAndRecomplete is
// R2-F1's regression test. tasks.updated_at (CURRENT_TIMESTAMP) has only
// one-second resolution, so a reopen+recomplete landing in the same
// wall-clock second as a wake's delivery previously produced a
// child_generation byte-identical to the reopened generation, permanently
// hiding the parent from ListStuckParents (nothing else re-admits it: the
// edge-triggered path's idempotency key collides on the same child set
// too). recordReceipt now refuses to persist a receipt whose generation
// names a still-open second, so this drives the exact same-second
// collision through the full reconciler — not just ListStuckParents — via
// the same CURRENT_TIMESTAMP writer UpdateTaskState uses, and asserts the
// parent keeps getting redelivered instead of going silent.
func TestParentWakeReconciler_RedeliversAfterSameSecondReopenAndRecomplete(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)
	const childID = "parent-1-child-0"

	adoptOffice(t, svc, "ws-1")
	// Align to the start of a fresh second so seed + tick 1 + reopen +
	// recomplete + tick 2 below — all sub-millisecond work — have maximum
	// room to land inside one shared wall-clock second, the collision
	// window R2-F1 depends on.
	waitForNextWholeSecond(t)
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if got := len(dispatcher.Calls()); got != 1 {
		t.Fatalf("want exactly one engine dispatch after tick 1, got %d: %#v", got, dispatcher.Calls())
	}
	if receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1"); err != nil {
		t.Fatalf("get wake receipt after tick 1: %v", err)
	} else if receipt != nil {
		t.Fatalf("tick 1 persisted a receipt for a still-open generation second: %#v", receipt)
	}

	// The child is reopened and re-completed through the same
	// CURRENT_TIMESTAMP write UpdateTaskState performs, landing in the same
	// wall-clock second as tick 1 above.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, childID)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, childID)

	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if got := len(dispatcher.Calls()); got != 2 {
		t.Fatalf("LOST WAKE NOT RECOVERABLE: same-second reopen+recomplete must still be redelivered, got %d dispatches: %#v", got, dispatcher.Calls())
	}

	// Once real time moves past the generation's second, the deferred
	// receipt finally persists, reflecting the reopened generation.
	waitForNextWholeSecond(t)
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 3: %v", err)
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt after tick 3: %v", err)
	}
	if receipt == nil || receipt.DeliveryOperationID == "" {
		t.Fatalf("generation safely closed but no receipt was recorded: %#v", receipt)
	}
}

// raceDispatcher reproduces R4-F1: a reopen+recomplete landing in the scan's
// still-open second, but not finishing until after real time has crossed
// into the next second. Its first HandleTrigger call performs the reopen
// while still inside the scanned second and then blocks past the second
// boundary before returning, simulating the payload build, child-set
// revalidation, and engine dispatch that separate ListStuckParents' scan
// from recordReceipt's commit in production.
type raceDispatcher struct {
	t       *testing.T
	svc     *service.Service
	childID string
	calls   int
}

func (d *raceDispatcher) HandleTrigger(context.Context, string, engine.Trigger, any, string) error {
	d.calls++
	if d.calls == 1 {
		d.svc.ExecSQL(d.t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, d.childID)
		d.svc.ExecSQL(d.t, `UPDATE tasks SET state = 'COMPLETED', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, d.childID)
		waitForNextWholeSecond(d.t)
	}
	return nil
}

// TestParentWakeReconciler_ReopenDuringDispatchWindowIsRedelivered is R4-F1's
// regression test: the still-open-second guard must compare the scanned
// generation against an instant captured before the scan, not against real
// time at commit time. A guard comparing against commit-time "now" sees the
// reopen's second as closed (real time has moved on by the time the engine
// dispatch returns) and persists a receipt for it, permanently hiding the
// parent — the exact defect this reconciler exists to fix.
func TestParentWakeReconciler_ReopenDuringDispatchWindowIsRedelivered(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	const childID = "parent-1-child-0"

	adoptOffice(t, svc, "ws-1")
	waitForNextWholeSecond(t)
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	dispatcher := &raceDispatcher{t: t, svc: svc, childID: childID}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if dispatcher.calls != 1 {
		t.Fatalf("want exactly one engine dispatch after tick 1, got %d", dispatcher.calls)
	}

	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if dispatcher.calls != 2 {
		t.Fatalf("LOST WAKE: reopen landing in the scan's still-open second during dispatch produced no further dispatch, got %d calls", dispatcher.calls)
	}
}

// TestParentWakeReconciler_RedeliversAfterPersistedReceiptInvalidatedByReopen
// is R4-F2's regression test. Unlike the other reopen tests above, tick 1's
// receipt here is a genuinely persisted row for a closed generation (not
// deferred by the still-open-second guard), so a redelivery on tick 2 can
// only come from ListStuckParents' third OR arm
// (newest_child_updated_at != child_generation) — the other two arms both
// stay false because the reopen leaves child_set_key and the stored
// delivery_operation_id untouched.
func TestParentWakeReconciler_RedeliversAfterPersistedReceiptInvalidatedByReopen(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)
	const childID = "parent-1-child-0"

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")
	// The seeded children's updated_at defaults to CURRENT_TIMESTAMP at
	// insert time, i.e. "now". recordReceipt refuses to persist a receipt for
	// a still-open generation second, so this test — which needs tick 1's
	// receipt to actually land — waits for that generation to close first.
	waitForNextWholeSecond(t)

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt after tick 1: %v", err)
	}
	if receipt == nil || receipt.DeliveryOperationID == "" {
		t.Fatalf("tick 1 did not persist a receipt for a closed generation: %#v", receipt)
	}
	firstGeneration := receipt.ChildGeneration

	// Reopen and re-complete the child in a later, already-closed second, so
	// child_set_key stays byte-identical to what the persisted receipt
	// already matches.
	waitForNextWholeSecond(t)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, childID)
	svc.ExecSQL(t, `UPDATE tasks SET state = 'COMPLETED', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, childID)
	waitForNextWholeSecond(t)

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
