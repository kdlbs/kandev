package sqlite

// Coverage for the dispatchStepEntry seam every registered step-transition
// writer calls after its own commit: it fires with the ledger row's own
// entry identity, is a no-op with no dispatcher wired (the pre-boot and
// test-fixture default), and is never called by the detach writer
// (RemoveTaskFromWorkflow), which names no destination step. See
// docs/specs/office/system-design/step-entry-sequence-execution.md.

import (
	"context"
	"strconv"
	"sync"
	"testing"
)

// fakeStepEntryDispatcher records every DispatchStepEntry call for
// assertions. Safe for concurrent use even though these writers are not
// expected to race in tests, matching the interface's ctx-scoped call shape.
type fakeStepEntryDispatcher struct {
	mu    sync.Mutex
	calls []stepEntryDispatchCall
}

type stepEntryDispatchCall struct {
	taskID, workflowID, stepID, entryID string
	ctxErr                              error
}

func (f *fakeStepEntryDispatcher) DispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, stepEntryDispatchCall{
		taskID: taskID, workflowID: workflowID, stepID: stepID, entryID: entryID,
		ctxErr: ctx.Err(),
	})
}

func (f *fakeStepEntryDispatcher) callsForTask(taskID string) []stepEntryDispatchCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []stepEntryDispatchCall
	for _, c := range f.calls {
		if c.taskID == taskID {
			out = append(out, c)
		}
	}
	return out
}

// TestDispatchStepEntryNilDispatcherIsNoOp proves the pre-boot/test-fixture
// default (no SetStepEntryDispatcher call) never panics — every writer this
// file doesn't otherwise cover still exercises it implicitly, since none of
// them set a dispatcher either.
func TestDispatchStepEntryNilDispatcherIsNoOp(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "task-nil-dispatcher", "wf-1", "step-a")
}

func TestDispatchStepEntryUsesLiveContextAfterCommit(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	fake := &fakeStepEntryDispatcher{}
	repo.SetStepEntryDispatcher(fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo.dispatchStepEntry(ctx, "task-1", "wf-1", "step-1", "entry-1")

	calls := fake.callsForTask("task-1")
	if len(calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(calls))
	}
	if calls[0].ctxErr != nil {
		t.Fatalf("post-commit dispatch received canceled context: %v", calls[0].ctxErr)
	}
}

func TestCreateTaskDispatchesStepEntryWithLedgerRowID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	fake := &fakeStepEntryDispatcher{}
	repo.SetStepEntryDispatcher(fake)

	createStepTransitionsTestTask(t, repo, "task-dispatch-create", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-dispatch-create")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	calls := fake.callsForTask("task-dispatch-create")
	if len(calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.workflowID != "wf-1" || call.stepID != "step-a" {
		t.Fatalf("dispatch call = %+v, want workflow wf-1 step step-a", call)
	}
	wantEntryID := strconv.FormatInt(rows[0].id, 10)
	if call.entryID != wantEntryID {
		t.Fatalf("entryID = %q, want the ledger row's own id %q", call.entryID, wantEntryID)
	}
}

// TestDispatchStepEntryCarriesDistinctIDPerTransition proves entry identity
// is the ledger row's own primary key, not a shared/reused marker: each of a
// task's step changes dispatches with the id of the row that change wrote,
// in commit order, and no two calls share an id.
func TestDispatchStepEntryCarriesDistinctIDPerTransition(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	fake := &fakeStepEntryDispatcher{}
	repo.SetStepEntryDispatcher(fake)
	task := createStepTransitionsTestTask(t, repo, "task-multi-move", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask (first move): %v", err)
	}
	task.WorkflowStepID = "step-c"
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask (second move): %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-multi-move")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (genesis + 2 moves)", len(rows))
	}
	calls := fake.callsForTask("task-multi-move")
	if len(calls) != 3 {
		t.Fatalf("dispatch calls = %d, want 3", len(calls))
	}
	seen := map[string]bool{}
	for i, call := range calls {
		wantEntryID := strconv.FormatInt(rows[i].id, 10)
		if call.entryID != wantEntryID {
			t.Fatalf("call[%d].entryID = %q, want ledger row id %q", i, call.entryID, wantEntryID)
		}
		if seen[call.entryID] {
			t.Fatalf("entryID %q reused across transitions, want unique per row", call.entryID)
		}
		seen[call.entryID] = true
	}
}

// TestUpdateTaskNoOpMoveDoesNotDispatch proves a no-op move (position-only
// reorder) — which recordStepTransition already writes no ledger row for —
// also never reaches the dispatcher, since dispatchStepEntry is guarded on a
// non-empty entryID.
func TestUpdateTaskNoOpMoveDoesNotDispatch(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	fake := &fakeStepEntryDispatcher{}
	repo.SetStepEntryDispatcher(fake)
	task := createStepTransitionsTestTask(t, repo, "task-noop-dispatch", "wf-1", "step-a")

	task.Position = 5 // position-only reorder within the same step
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	calls := fake.callsForTask("task-noop-dispatch")
	if len(calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1 (genesis only; the no-op reorder must not dispatch)", len(calls))
	}
}

func TestAddTaskToWorkflowDispatchesStepEntry(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	fake := &fakeStepEntryDispatcher{}
	repo.SetStepEntryDispatcher(fake)
	createStepTransitionsTestTask(t, repo, "task-attach-dispatch", "", "")

	if err := repo.AddTaskToWorkflow(context.Background(), "task-attach-dispatch", "wf-2", "step-x", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-attach-dispatch")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	calls := fake.callsForTask("task-attach-dispatch")
	if len(calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(calls))
	}
	if calls[0].workflowID != "wf-2" || calls[0].stepID != "step-x" {
		t.Fatalf("dispatch call = %+v, want workflow wf-2 step step-x", calls[0])
	}
	wantEntryID := strconv.FormatInt(rows[0].id, 10)
	if calls[0].entryID != wantEntryID {
		t.Fatalf("entryID = %q, want ledger row id %q", calls[0].entryID, wantEntryID)
	}
}

// TestRemoveTaskFromWorkflowNeverDispatches pins the detach writer's
// exclusion from step entry: RemoveTaskFromWorkflow names no destination
// step, so dispatchStepEntry's stepID guard keeps it from ever firing —
// there is no entry sequence to run for a task leaving its workflow.
func TestRemoveTaskFromWorkflowNeverDispatches(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	fake := &fakeStepEntryDispatcher{}
	repo.SetStepEntryDispatcher(fake)
	createStepTransitionsTestTask(t, repo, "task-detach-dispatch", "wf-1", "step-a")

	if err := repo.RemoveTaskFromWorkflow(context.Background(), "task-detach-dispatch", "wf-1"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-detach-dispatch")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (genesis + detach)", len(rows))
	}
	if calls := fake.callsForTask("task-detach-dispatch"); len(calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1 (genesis only; detach must never dispatch)", len(calls))
	}
}
