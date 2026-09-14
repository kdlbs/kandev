package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newAgentErrorTransientTestService is newAgentErrorTestService's sibling for
// the R4/R5 (transient-retry) and AC-E3 (auto-start) routes, which need a
// caller-supplied mockAgentManager (a custom promptErr, isAgentRunning, or
// promptDone channel) rather than the plain one newAgentErrorTestService
// builds internally.
func newAgentErrorTransientTestService(
	t *testing.T, repo *sqliterepo.Repository, stepGetter *mockStepGetter, agentMgr *mockAgentManager,
	configure func(*Service),
) (*Service, *observer.ObservedLogs) {
	t.Helper()
	svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), agentMgr)
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log
	if configure != nil {
		configure(svc)
	}
	svc.initWorkflowEngine()
	return svc, logs
}

// --- AC-A10: a user cancel racing a claimed retry timer must never let the
// cancel's own suppressed delivery mark the marker shared with the timer's
// delivery. Driven sequentially per two independent contexts, not via a real
// goroutine race — the marker's value is what's contractual, not the
// interleaving that produces it. ---

func TestDispatchKanbanAgentErrorTrigger_ConcurrentCancelDoesNotLeakMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	t.Cleanup(svc.cancelAllTransientRetries)

	// Represents a retry timer that has already been scheduled and is about
	// to reach retryTransientPrompt on its own.
	svc.transientRetries.Store("s1", &transientRetryEntry{attempt: 1, cancel: func() {}})

	if !svc.CancelTransientRetry(ctx, "t1", "s1") {
		t.Fatal("CancelTransientRetry = false, want true (a loop was active)")
	}
	if decisions.clearCalls != 0 {
		t.Fatalf("cancel's own delivery clearCalls = %d, want 0 (AC-A8 suppression)", decisions.clearCalls)
	}

	// The claimed timer still reaches R4 on its own, unaffected context. Its
	// event must not carry the cancel's UserInitiated marker.
	svc.retryTransientPrompt(ctx, "t1", "s1", "exec-1")

	if decisions.clearCalls != 1 {
		t.Fatalf("timer's own delivery clearCalls = %d, want 1 (AC-A8's marker must not leak into R4)", decisions.clearCalls)
	}
	// AC-A10 requires "at most one", never exactly one unconditionally: the
	// cancel outrunning the timer at one of retryTransientPrompt's ctx.Err()
	// checks is a legitimate zero-outcome variant of this same race (AC-B5).
	// This drive pins the timer arriving, so it also happens to observe 1
	// here, but the assertion must state the actual invariant.
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) > 1 {
		t.Fatalf("total dispatch INFO records across both deliveries = %d, want at most 1", len(got))
	}
}

// --- AC-A4: on_agent_error must evaluate against the step the task landed on
// after handleRecoverableFailureLocked's own on_turn_complete reconciliation
// (a pending step-completion signal moving step1 -> step2), not the stale
// step1 the handler observed on entry. Reads the task fresh, after the
// decision that used the earlier snapshot. ---

func TestDispatchKanbanAgentErrorTrigger_ReadsPostReconciliationStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seedSession leaves the session RUNNING.

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}

	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	seedPendingStepCompletionSignal(t, repo, "step1", "all done")

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "agent crashed",
	})

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("expected on_turn_complete reconciliation to move task to step2, got %q", task.WorkflowStepID)
	}

	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (on_agent_error must fire against step2's declared action, not step1's)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("dispatched records = %d, want 1", len(got))
	}
}

// --- AC-EO-11 (workflow-evaluate-only-operation-marking): a declined
// transition (target step fails to load) still reports a dispatch, and IS
// retried on redelivery — the engine defers the mark for a deferred
// transition (OperationMarkDeferred), and the caller only marks once
// applyEngineTransition actually commits. Superseded former behavior: AC-C7
// used to describe this operation as marked applied by the engine's
// unconditional mark and therefore never retried, which was the defect this
// spec's contract change fixes. ---

func TestDispatchKanbanAgentErrorTrigger_DeclinedTransitionIsRetried(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step-missing"}},
		}},
	}
	stepGetter.getStepFunc = func(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
		if id == "step-missing" {
			return nil, errors.New("step not found")
		}
		return stepGetter.steps[id], nil
	}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.handleRecoverableFailureLocked(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q, want step1 (the transition must have been declined)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1 (dispatch still reports success)", len(got))
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || applied {
		t.Fatalf("IsOperationApplied = %v, %v, want false, nil (a declined transition must leave the operation unmarked)", applied, err)
	}

	logs.TakeAll()
	svc.handleRecoverableFailureLocked(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step1 (still declined the same way)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Errorf("redelivery emitted %d dispatch record(s), want 1 (AC-EO-11: an unmarked declined transition retries instead of short-circuiting on Idempotent)", len(got))
	}
}

// --- AC-A6/E2: a step declaring no on_agent_error actions at all — the
// overwhelmingly common case — logs at DEBUG only, never INFO. ---

func TestDispatchKanbanAgentErrorTrigger_NoDeclaredActionsLogsDebugOnly(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)

	svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

	if got := filterLogs(logs, msgAgentErrorNoActions); len(got) != 1 {
		t.Fatalf("got %d %q DEBUG records, want 1", len(got), msgAgentErrorNoActions)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
		t.Errorf("got a dispatch INFO record, want none for a step with no declared actions")
	}
}

// --- AC-E3: taskDescription is pinned to the reloaded task's Description, so
// the prompt an auto_start_agent transition launches carries it. ---

func TestDispatchKanbanAgentErrorTrigger_TaskDescriptionReachesLaunchedPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.Description = "AC-E3 MARKER description"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task description: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}

	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
	svc, _ := newAgentErrorTransientTestService(t, repo, stepGetter, agentMgr, nil)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	select {
	case <-agentMgr.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the auto-start prompt")
	}

	agentMgr.mu.Lock()
	got := agentMgr.capturedPrompts[0]
	agentMgr.mu.Unlock()
	if !strings.Contains(got, "AC-E3 MARKER description") {
		t.Fatalf("launched prompt = %q, want it to contain the reloaded task's Description", got)
	}
}

// --- AC-E10: a move_to_step transition to a WIP-limited, full destination
// still applies (the task's WorkflowStepID moves) but on_enter dispatch is
// deferred — never fired — while the task sits queued. ---

func TestDispatchKanbanAgentErrorTrigger_WIPLimitedDestinationDefersOnEnter(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "occupant", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step2",
		Title: "Occupant", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create occupant: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1, WIPLimit: 1,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}

	svc, _ := newAgentErrorTestService(t, repo, stepGetter, nil)
	onEnterCalled := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterCalled <- struct{}{}:
		default:
		}
	}

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID = %q, want step2 (the transition itself must still apply)", task.WorkflowStepID)
	}
	if task.WIPAdmitted {
		t.Error("expected WIPAdmitted=false at a full-capacity destination")
	}
	if task.QueuedForStepID != "step2" {
		t.Errorf("QueuedForStepID = %q, want step2", task.QueuedForStepID)
	}
	select {
	case <-onEnterCalled:
		t.Error("expected on_enter dispatch to be deferred for a WIP-queued destination")
	case <-time.After(250 * time.Millisecond):
	}
}

// --- AC-E9: guard-gated and requires_approval transitions never apply
// automatically through on_agent_error. ---

func TestDispatchKanbanAgentErrorTrigger_GuardsAndApproval(t *testing.T) {
	ctx := context.Background()

	t.Run("unsatisfied wait_for_quorum guard blocks the transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{
					"step_id": "step2",
					"if": map[string]interface{}{
						"wait_for_quorum": map[string]interface{}{"role": "reviewer", "threshold": "all_approve"},
					},
				}},
			}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)
		// engineParticipants is deliberately left unwired, so the guard
		// evaluates unsatisfied rather than the transition applying.

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
		})

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		if task.WorkflowStepID != "step1" {
			t.Fatalf("WorkflowStepID = %q, want step1 (unsatisfied guard must block the transition)", task.WorkflowStepID)
		}
		if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
			t.Fatalf("got %d dispatch INFO records, want 1 (the action is still declared and counted)", len(got))
		}
	})

	t.Run("requires_approval transition never applies automatically", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{
					"step_id": "step2", "requires_approval": true,
				}},
			}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, nil)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
		})

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		if task.WorkflowStepID != "step1" {
			t.Fatalf("WorkflowStepID = %q, want step1 (requires_approval must never apply automatically)", task.WorkflowStepID)
		}
	})
}

// --- AC-E14: the two remaining branches of the walk-failure matrix. ---

func TestDispatchKanbanAgentErrorTrigger_WalkFailureAndEngineFailureLogsError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	stepGetter.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
		return nil, errors.New("step store unavailable")
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	if decisions.clearCalls != 0 {
		t.Errorf("clearCalls = %d, want 0 (the engine never reached the action list)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatchFailed); len(got) != 1 {
		t.Fatalf("got %d %q ERROR records, want 1", len(got), msgAgentErrorDispatchFailed)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
		t.Errorf("got a dispatch INFO record, want none")
	}
	if got := filterLogs(logs, msgAgentErrorNoActions); len(got) != 0 {
		t.Errorf("got %d %q DEBUG record(s), want none (AC-E5's ERROR and AC-E2's DEBUG are disjoint)", len(got), msgAgentErrorNoActions)
	}
}

func TestDispatchKanbanAgentErrorTrigger_RedeliveryIdempotentEngineNeverCallsLoadStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.handleRecoverableFailureLocked(ctx, data)
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("first delivery: got %d INFO records, want 1", len(got))
	}

	// Force a genuine cache miss on redelivery. Without this, both the walk's
	// own LoadStep and (if the engine wrongly called it too) the engine's
	// LoadStep would be served from workflowStore's step cache populated by
	// the first delivery, and a broken getter below would prove nothing about
	// which caller actually reached it.
	if err := svc.handleWorkflowStepCacheInvalidation(ctx, stepEvent(events.WorkflowStepUpdated, "step1", "wf1")); err != nil {
		t.Fatalf("invalidate step cache: %v", err)
	}
	stepGetter.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
		return nil, errors.New("step store unavailable")
	}
	callsBefore := stepGetter.GetStepCalls()
	logs.TakeAll()
	decisions.clearCalls = 0

	svc.handleRecoverableFailureLocked(ctx, data)

	if decisions.clearCalls != 0 {
		t.Errorf("clearCalls = %d, want 0 (idempotent redelivery)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
		t.Errorf("got a dispatch INFO record, want none")
	}
	if got := filterLogs(logs, msgAgentErrorNoActions); len(got) != 0 {
		t.Errorf("got a dispatch DEBUG record, want none (idempotent short-circuit, not an empty-actions dispatch)")
	}
	if got := filterLogs(logs, msgAgentErrorDispatchFailed); len(got) != 0 {
		t.Errorf("got a dispatch-failed ERROR record, want none (the engine must not have re-run LoadStep)")
	}
	// Exactly one real GetStep call is expected here: the pre-engine walk's
	// own LoadStep, which hits the now-broken getter on a genuine cache miss
	// and fails silently (AC-E14). A second call would mean the engine's own
	// loadExecutionContext/LoadStep ran despite the operation already being
	// applied — the isOperationAlreadyApplied short-circuit AC-E14's third
	// branch requires must run first.
	if got := stepGetter.GetStepCalls() - callsBefore; got != 1 {
		t.Errorf("GetStep calls on redelivery = %d, want exactly 1 (the walk's own call only; "+
			"the engine must not call LoadStep on an idempotent short-circuit)", got)
	}
}

// --- AC-F1: an absent or engine-nil snapshot (dispatch called before
// initWorkflowEngine ever populated agentErrorDeps) is a silent skip. ---

func TestDispatchKanbanAgentErrorTrigger_NilDepsIsANoOp(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), agentMgr)
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log
	// Deliberately never call svc.initWorkflowEngine(): agentErrorDeps stays nil.

	svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	if len(logs.All()) != 0 {
		t.Errorf("expected no log records before initWorkflowEngine, got %+v", logs.All())
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Errorf("WorkflowStepID = %q, want step1 (unchanged)", task.WorkflowStepID)
	}
}

// --- AC-F8: overlapping guard pairs — a silent guard earlier in the fixed
// evaluation order must preempt a later recording guard's own record, even
// when the later guard's own condition (a genuinely blocking sibling
// session) is also true. ---

func TestDispatchKanbanAgentErrorTrigger_OverlappingGuardPairs(t *testing.T) {
	ctx := context.Background()

	baseStep := func() *wfmodels.WorkflowStep {
		return &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
	}
	addBlockingSibling := func(t *testing.T, repo *sqliterepo.Repository, taskID string) {
		t.Helper()
		now := time.Now().UTC()
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "s2", TaskID: taskID, State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create sibling session: %v", err)
		}
	}

	t.Run("AC-F4 archived preempts AC-F5's sibling-working record", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		addBlockingSibling(t, repo, "t1")
		if err := repo.ArchiveTask(ctx, "t1"); err != nil {
			t.Fatalf("archive task: %v", err)
		}
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorAnotherSessionWorking); len(got) != 0 {
			t.Errorf("got %d %q records, want 0 (archived must preempt the F5 check entirely)",
				len(got), msgAgentErrorAnotherSessionWorking)
		}
	})

	t.Run("AC-F2 ephemeral preempts AC-F5's sibling-working record", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
			IsEphemeral: true, Title: "Ephemeral", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}))
		requireNoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
		}))
		addBlockingSibling(t, repo, "t1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorAnotherSessionWorking); len(got) != 0 {
			t.Errorf("got %d %q records, want 0 (ephemeral must preempt the F5 check entirely)",
				len(got), msgAgentErrorAnotherSessionWorking)
		}
	})

	t.Run("AC-F3 empty workflow step id preempts AC-F5's sibling-working record", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		addBlockingSibling(t, repo, "t1")
		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		task.WorkflowStepID = ""
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("update task: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorAnotherSessionWorking); len(got) != 0 {
			t.Errorf("got %d %q records, want 0 (empty step id must preempt the F5 check entirely)",
				len(got), msgAgentErrorAnotherSessionWorking)
		}
	})
}
