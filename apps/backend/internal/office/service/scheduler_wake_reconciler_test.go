package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
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

// oneShotFailureDispatcher wraps a queueRunDispatcher and fails the first
// call for a chosen trigger, then delegates every other call (including
// later calls for that same trigger) to the wrapped dispatcher. It records
// every call it sees so a test can assert dispatch counts and operation ids
// on both the failed attempt and the later recovery.
type oneShotFailureDispatcher struct {
	inner       *queueRunDispatcher
	failTrigger engine.Trigger
	failArmed   bool
	failErr     error

	mu    sync.Mutex
	calls []dispatcherCall
}

func (d *oneShotFailureDispatcher) HandleTrigger(
	ctx context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	d.mu.Lock()
	d.calls = append(d.calls, dispatcherCall{taskID, trigger, payload, opID})
	d.mu.Unlock()
	if d.failArmed && trigger == d.failTrigger {
		d.failArmed = false
		return d.failErr
	}
	return d.inner.HandleTrigger(ctx, taskID, trigger, payload, opID)
}

func (d *oneShotFailureDispatcher) Calls() []dispatcherCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]dispatcherCall, len(d.calls))
	copy(out, d.calls)
	return out
}

// countingDispatcher wraps a queueRunDispatcher and counts every call it
// sees, so a test can assert the reconciler never even attempted a
// dispatch instead of only checking the resulting run count.
type countingDispatcher struct {
	inner *queueRunDispatcher

	mu sync.Mutex
	n  int
}

func (d *countingDispatcher) HandleTrigger(
	ctx context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	d.mu.Lock()
	d.n++
	d.mu.Unlock()
	return d.inner.HandleTrigger(ctx, taskID, trigger, payload, opID)
}

func (d *countingDispatcher) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.n
}

// publishChildDone publishes the task.moved event that fires
// finalizeDone -> queueChildrenCompletedRun for parentID, the same event
// shape TestWakeOperationID_UnifiedAcrossEdgeAndReconcilerPaths uses.
func publishChildDone(t *testing.T, eb bus.EventBus, parentID, workerID string) {
	t.Helper()
	ctx := context.Background()
	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   parentID + "-child-0",
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-done",
		"to_step_name":              "Done",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": workerID,
		"parent_id":                 parentID,
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}
}

// TestParentWakeReconciler_RecoversFailedEdgeDispatch is the regression test
// requested on PR #3271's review (discussion r3909576685): queueChildrenCompletedRun
// can fail after AreAllChildrenTerminal succeeds (for example when
// GetChildSetKey errors), and finalizeDone only logs that failure at Warn
// because ParentWakeReconciler is the documented recovery path. This proves
// the recovery actually happens: the edge-triggered dispatch is made to fail
// once, then a reconciler Tick must re-deliver the same wake (same operation
// id) as a real queued run.
func TestParentWakeReconciler_RecoversFailedEdgeDispatch(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	dispatcher := &oneShotFailureDispatcher{
		inner:       &queueRunDispatcher{svc: svc},
		failTrigger: engine.TriggerOnChildrenCompleted,
		failArmed:   true,
		failErr:     fmt.Errorf("injected on_children_completed dispatch failure"),
	}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	// Edge-triggered path: the child lands in Done, firing
	// finalizeDone -> queueChildrenCompletedRun for parent-1, which fails
	// on the injected dispatcher error and is swallowed at Warn.
	publishChildDone(t, eb, "parent-1", "worker-1")

	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("want exactly one dispatch attempt after the failed edge publish, got %d: %#v",
			len(calls), calls)
	}
	edgeOpID := calls[0].opID
	if edgeOpID == "" {
		t.Fatal("failed edge dispatch attempt carried an empty operation id")
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("failed edge dispatch left a queued run: %#v", runs)
	}
	receipt, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt: %v", err)
	}
	if receipt != nil {
		t.Fatalf("failed edge dispatch left a receipt: %#v", receipt)
	}

	// Level-triggered path: ListStuckParents still finds parent-1 stuck
	// because nothing was persisted for it, so Tick must re-deliver.
	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	calls = dispatcher.Calls()
	if len(calls) != 2 {
		t.Fatalf("want a second dispatch attempt from the reconciler, got %d: %#v",
			len(calls), calls)
	}
	reconcilerOpID := calls[1].opID
	if reconcilerOpID != edgeOpID {
		t.Fatalf("reconciler recovery used a different operation id than the failed edge attempt: edge=%q reconciler=%q",
			edgeOpID, reconcilerOpID)
	}

	runsAfter, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs after tick: %v", err)
	}
	if len(runsAfter) != 1 {
		t.Fatalf("want exactly one queued run after recovery, got %d: %#v", len(runsAfter), runsAfter)
	}
	if runsAfter[0].Reason != service.RunReasonTaskChildrenCompleted {
		t.Fatalf("recovered run reason = %q, want %q", runsAfter[0].Reason, service.RunReasonTaskChildrenCompleted)
	}

	receiptAfter, err := svc.GetWakeReceiptForTest(ctx, "parent-1")
	if err != nil {
		t.Fatalf("get wake receipt after tick: %v", err)
	}
	if receiptAfter == nil || receiptAfter.DeliveryOperationID != edgeOpID {
		t.Fatalf("recovery receipt = %#v, want an operation id matching the failed edge attempt %q", receiptAfter, edgeOpID)
	}
}

// TestParentWakeReconciler_DoesNotDoubleQueueASuccessfulEdgeDispatch is the
// non-vacuity control for TestParentWakeReconciler_RecoversFailedEdgeDispatch:
// identical setup, but the edge dispatch is left to succeed. It must be the
// one that queues the run, and the following Tick must add nothing, proving
// the recovery assertions above discriminate on the edge path's outcome
// rather than passing regardless of it.
func TestParentWakeReconciler_DoesNotDoubleQueueASuccessfulEdgeDispatch(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	dispatcher := &countingDispatcher{inner: &queueRunDispatcher{svc: svc}}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	publishChildDone(t, eb, "parent-1", "worker-1")

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("successful edge dispatch did not queue a run: %#v", runs)
	}
	if dispatcher.Count() != 1 {
		t.Fatalf("want exactly one dispatch attempt from the successful edge publish, got %d", dispatcher.Count())
	}

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if dispatcher.Count() != 1 {
		t.Fatalf("want the reconciler to skip dispatch entirely behind a successful edge delivery, got %d attempts", dispatcher.Count())
	}

	runsAfter, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs after tick: %v", err)
	}
	if len(runsAfter) != 1 {
		t.Fatalf("tick added a run behind a successful edge dispatch: %#v", runsAfter)
	}
}

// TestWakeOperationID_UnifiedAcrossEdgeAndReconcilerPaths is the regression
// test for the duplicate on_children_completed wake race: the edge-triggered
// path (queueChildrenCompletedRun, fired off task.moved) and the
// level-triggered ParentWakeReconciler can both dispatch a wake for the same
// parent within one window. Before the fix they derived different operation
// ids ("children_completed:<parent>" vs the sha256-based
// "task_children_completed:<parent>:<hash>"), so idx_run_idempotency could
// never dedupe them and both runs executed. Both producers must now derive
// the identical operation id for the same parent + child set so the shared
// idempotency key (and therefore the DB's unique index) actually collapses
// the second dispatch.
func TestWakeOperationID_UnifiedAcrossEdgeAndReconcilerPaths(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	// Edge-triggered path: a child lands in Done, firing
	// finalizeDone -> queueChildrenCompletedRun for parent-1.
	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   "parent-1-child-0",
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-done",
		"to_step_name":              "Done",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": "worker-1",
		"parent_id":                 "parent-1",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	// Level-triggered path: one reconciler tick sweeping the same
	// still-stuck parent (the edge dispatch above never writes a receipt
	// or a run row — fakeDispatcher only records the call — so
	// ListStuckParents still finds parent-1).
	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	calls := dispatcher.Calls()
	if len(calls) != 2 {
		t.Fatalf("want 2 dispatcher calls (edge + reconciler), got %d: %#v", len(calls), calls)
	}
	edgeOpID, reconcilerOpID := calls[0].opID, calls[1].opID
	if edgeOpID == "" || reconcilerOpID == "" {
		t.Fatalf("expected non-empty operation ids, got edge=%q reconciler=%q", edgeOpID, reconcilerOpID)
	}
	if edgeOpID != reconcilerOpID {
		t.Fatalf("edge and reconciler paths derived different operation ids for the same parent+child-set: edge=%q reconciler=%q",
			edgeOpID, reconcilerOpID)
	}
}
