package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newAgentErrorTestService builds a Service wired against a real engine
// (svc.initWorkflowEngine) with an observed logger so tests can assert on
// this dispatch's own log records. configure runs before initWorkflowEngine
// so callback dependencies (engineDecisions, etc.) are picked up by
// buildWorkflowCallbacks.
func newAgentErrorTestService(
	t *testing.T, repo *sqliterepo.Repository, stepGetter *mockStepGetter, configure func(*Service),
) (*Service, *observer.ObservedLogs) {
	t.Helper()
	// handleRecoverableFailureLocked's last-but-one step (before this card's
	// dispatch) fires a background cleanupAgentExecution that dereferences
	// svc.executor — createTestServiceWithScheduler is the fixture that wires
	// one, unlike the bare createTestService used elsewhere in this package.
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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

func filterLogs(logs *observer.ObservedLogs, msg string) []observer.LoggedEntry {
	var out []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == msg {
			out = append(out, e)
		}
	}
	return out
}

func TestHandleRecoverableFailureDispatchesAfterSessionGuardRelease(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionAutoStartAgent}},
		},
	}
	svc, _ := newAgentErrorTestService(t, repo, stepGetter, nil)

	callback := &guardReacquiringAgentErrorCallback{svc: svc, done: make(chan struct{})}
	registry := engine.MapRegistry{engine.ActionAutoStartAgent: callback}
	workflowEngine := engine.New(svc.workflowStore, registry)
	svc.workflowEngine = workflowEngine
	svc.agentErrorDeps.Store(&agentErrorDispatchDeps{
		engine:   workflowEngine,
		registry: registry,
		store:    svc.workflowStore,
	})

	finished := make(chan struct{})
	go func() {
		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		})
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("recoverable failure did not finish while dispatching auto_start_agent")
	}
	select {
	case <-callback.done:
	case <-time.After(time.Second):
		t.Fatal("deferred on_agent_error callback did not complete")
	}
}

type guardReacquiringAgentErrorCallback struct {
	svc  *Service
	done chan struct{}
}

func (c *guardReacquiringAgentErrorCallback) Execute(
	_ context.Context, in engine.ActionInput,
) (engine.ActionResult, error) {
	lock, release := c.svc.acquireCancelInFlightGuard(in.State.SessionID)
	defer release()
	lock.Lock()
	close(c.done)
	lock.Unlock()
	return engine.ActionResult{}, nil
}

// --- AC-A1/A6/C1/C3: dispatch fires, records exactly one INFO, and a
// redelivery of the same failure is idempotent (no re-run, no record). ---

func TestDispatchKanbanAgentErrorTrigger_ClearDecisionsDispatchesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionClearDecisions},
			},
		},
	}

	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) {
		s.engineDecisions = decisions
	})

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1", decisions.clearCalls)
	}
	infos := filterLogs(logs, msgAgentErrorDispatched)
	if len(infos) != 1 {
		t.Fatalf("got %d %q records, want 1 (all: %+v)", len(infos), msgAgentErrorDispatched, logs.All())
	}
	fields := infos[0].ContextMap()
	if fields["task_id"] != "t1" || fields["session_id"] != "s1" || fields["step_id"] != "step1" {
		t.Errorf("unexpected identity fields: %+v", fields)
	}
	if fields["operation_id"] != "agent_error:session:s1:exec-1" {
		t.Errorf("operation_id = %v, want agent_error:session:s1:exec-1", fields["operation_id"])
	}
	if got := filterLogs(logs, msgAgentErrorNoActions); len(got) != 0 {
		t.Errorf("first delivery emitted %d %q record(s), want 0 (AC-A6's INFO and AC-E2's DEBUG are disjoint)", len(got), msgAgentErrorNoActions)
	}

	// AC-C3: a redelivery of the exact same failure must not re-run the
	// action list and must emit no dispatch record of its own.
	decisions.clearCalls = 0
	logs.TakeAll()
	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})
	if decisions.clearCalls != 0 {
		t.Errorf("redelivery clearCalls = %d, want 0 (idempotent)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
		t.Errorf("redelivery emitted %d %q record(s), want 0", len(got), msgAgentErrorDispatched)
	}
	if got := filterLogs(logs, msgAgentErrorNoActions); len(got) != 0 {
		t.Errorf("redelivery emitted %d %q record(s), want 0 (idempotent short-circuit, not an empty-actions dispatch)", len(got), msgAgentErrorNoActions)
	}
}

// --- AC-C2/C4/C8: idempotency key shape, distinct executions both fire,
// empty-AgentExecutionID collapses to one key. ---

func TestDispatchKanbanAgentErrorTrigger_IdempotencyKeyShape(t *testing.T) {
	ctx := context.Background()

	t.Run("two distinct executions on one session both dispatch", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})
		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-2"})

		if decisions.clearCalls != 2 {
			t.Fatalf("clearCalls = %d, want 2 (distinct executions must not collapse)", decisions.clearCalls)
		}
	})

	t.Run("two failures with empty AgentExecutionID collapse to one dispatch", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		if decisions.clearCalls != 1 {
			t.Fatalf("clearCalls = %d, want 1 (empty execution ids collapse to one key)", decisions.clearCalls)
		}
		infos := filterLogs(logs, msgAgentErrorDispatched)
		if len(infos) != 1 {
			t.Fatalf("got %d %q record(s), want 1", len(infos), msgAgentErrorDispatched)
		}
		if got := infos[0].ContextMap()["operation_id"]; got != "agent_error:session:s1" {
			t.Errorf("operation_id = %v, want agent_error:session:s1 (AC-C2's empty-execution-id shape)", got)
		}
	})
}

// --- AC-C6: an action that errors leaves the operation unmarked, so a
// redelivery re-runs the whole list from the start. ---

func TestDispatchKanbanAgentErrorTrigger_ActionErrorLeavesUnmarkedAndRetries(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &failingThenSucceedingDecisionStore{failCount: 1}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"}
	svc.handleRecoverableFailureLocked(ctx, data)
	if decisions.calls != 1 {
		t.Fatalf("first delivery calls = %d, want 1", decisions.calls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatchFailed); len(got) != 1 {
		t.Fatalf("got %d ERROR records for the failed first delivery, want 1", len(got))
	}

	logs.TakeAll()
	svc.handleRecoverableFailureLocked(ctx, data)
	if decisions.calls != 2 {
		t.Fatalf("redelivery calls = %d, want 2 (unmarked operation reruns the list)", decisions.calls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatched INFO records on the successful redelivery, want 1", len(got))
	}
}

type failingThenSucceedingDecisionStore struct {
	spyDecisionStore
	calls     int
	failCount int
}

func (d *failingThenSucceedingDecisionStore) ClearStepDecisions(ctx context.Context, taskID, stepID string) (int64, error) {
	d.calls++
	if d.calls <= d.failCount {
		return 0, errors.New("transient decision store failure")
	}
	return d.spyDecisionStore.ClearStepDecisions(ctx, taskID, stepID)
}

// --- AC-E3/E8/E10/E11: move_to_step applies the transition through
// applyEngineTransition — on_enter is dispatched (launchProcessOnEnter always
// fires onProcessOnEnterComplete, even with an empty on_enter list, which is
// what this test synchronizes on) and the session_step_history ledger row
// carries the new on_agent_error trigger label rather than auto_complete. ---

func TestDispatchKanbanAgentErrorTrigger_MoveToStepAppliesTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	recorder := &fakeStepHistoryRecorder{}
	svc.stepHistoryRecorder = recorder
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}
	svc.initWorkflowEngine()

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	select {
	case <-onEnterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for on_enter dispatch after the on_agent_error move")
	}

	updated, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if updated.WorkflowStepID != "step2" {
		t.Fatalf("task WorkflowStepID = %q, want step2", updated.WorkflowStepID)
	}

	calls := recorder.Calls()
	if len(calls) != 1 {
		t.Fatalf("got %d session_step_history calls, want 1: %+v", len(calls), calls)
	}
	if calls[0].trigger != wfmodels.StepTransitionTriggerAgentError {
		t.Errorf("session_step_history trigger = %q, want %q", calls[0].trigger, wfmodels.StepTransitionTriggerAgentError)
	}
	if calls[0].fromStepID != "step1" || calls[0].toStepID != "step2" {
		t.Errorf("session_step_history from/to = %q/%q, want step1/step2", calls[0].fromStepID, calls[0].toStepID)
	}
}

// --- AC-B1/B2: an Office-owned task never dispatches through this path,
// and a task-load failure produces zero dispatch (fail closed). ---

func TestDispatchKanbanAgentErrorTrigger_OfficeAndLoadFailureGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("office-owned task does not dispatch", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "t-office", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
			ProjectID: "proj1", Title: "Office Task", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}))
		requireNoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "s-office", TaskID: "t-office", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t-office", SessionID: "s-office", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0 (Office task must not dispatch through this path)", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
			t.Errorf("got %d dispatch INFO records for an Office task, want 0", len(got))
		}
	})

	t.Run("task load failure produces zero dispatch and one WARNING", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		svc.repo = &agentErrorTaskLoadErrorRepo{sessionExecutorStore: svc.repo, err: errors.New("db down")}

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorTaskLoadFailed); len(got) != 1 {
			t.Fatalf("got %d %q WARNINGs, want 1", len(got), msgAgentErrorTaskLoadFailed)
		}
	})
}

type agentErrorTaskLoadErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r *agentErrorTaskLoadErrorRepo) GetTask(_ context.Context, _ string) (*models.Task, error) {
	return nil, r.err
}

// --- AC-A5/A7/A8/F2/F3/F4/F5/F6/B5: the guard sequence. ---

func TestDispatchKanbanAgentErrorTrigger_Guards(t *testing.T) {
	ctx := context.Background()

	baseStep := func() *wfmodels.WorkflowStep {
		return &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
	}

	t.Run("AC-A5 empty session id never dispatches, no record", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: ""})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if len(logs.All()) != 0 {
			t.Errorf("expected no log records at all, got %+v", logs.All())
		}
	})

	t.Run("AC-A8 user-initiated cancel never dispatches, logs DEBUG", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.scheduleTransientRetry("t1", "s1", "exec-1", 1, 5*time.Second)
		t.Cleanup(svc.cancelAllTransientRetries)

		if !svc.CancelTransientRetry(ctx, "t1", "s1") {
			t.Fatal("CancelTransientRetry = false, want true (a loop was active)")
		}
		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0 (a user cancel must not dispatch on_agent_error)", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorUserInitiated); len(got) != 1 {
			t.Fatalf("got %d %q DEBUG records, want 1", len(got), msgAgentErrorUserInitiated)
		}
	})

	t.Run("AC-A9 a claimed retry timer that reaches R4/R5 still dispatches", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 1 {
			t.Errorf("clearCalls = %d, want 1 (a non-user-initiated recoverable failure must dispatch)", decisions.clearCalls)
		}
	})

	t.Run("AC-A7 vanished session is a DEBUG no-op", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "does-not-exist", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorSessionVanished); len(got) != 1 {
			t.Fatalf("got %d %q DEBUG records, want 1", len(got), msgAgentErrorSessionVanished)
		}
	})

	t.Run("AC-F6 non-not-found session reload error is a WARNING", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		svc.repo = &agentErrorSessionReloadErrorRepo{sessionExecutorStore: svc.repo, err: errors.New("db timeout")}

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorSessionReloadFailed); len(got) != 1 {
			t.Fatalf("got %d %q WARNINGs, want 1", len(got), msgAgentErrorSessionReloadFailed)
		}
	})

	t.Run("AC-F2/F3/F4 ephemeral, empty step id, and archived tasks skip silently", func(t *testing.T) {
		// IsEphemeral is create-only (UpdateTask's column list omits it) and
		// archived_at only moves through ArchiveTask, so each case seeds its
		// own repo state through the real write path rather than mutating a
		// loaded *models.Task and calling UpdateTask, which would silently
		// leave both columns unchanged and pass for the wrong reason.
		t.Run("ephemeral", func(t *testing.T) {
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

			stepGetter := newMockStepGetter()
			stepGetter.steps["step1"] = baseStep()
			decisions := &spyDecisionStore{}
			svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

			svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

			if decisions.clearCalls != 0 {
				t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
			}
			if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
				t.Errorf("got a dispatch record, want none")
			}
		})

		t.Run("empty workflow step id", func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
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
			if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
				t.Errorf("got a dispatch record, want none")
			}
		})

		t.Run("archived", func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
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
			if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
				t.Errorf("got a dispatch record, want none")
			}
		})
	})

	t.Run("AC-F5 another working session blocks dispatch, logs DEBUG", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		now := time.Now().UTC()
		requireNoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "s2", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0 (a sibling session is still working)", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorAnotherSessionWorking); len(got) != 1 {
			t.Fatalf("got %d %q DEBUG records, want 1", len(got), msgAgentErrorAnotherSessionWorking)
		}
	})

	t.Run("AC-F6 otherWorkingSessionID failure adds no record of its own", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = baseStep()
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		svc.repo = &agentErrorListSessionsErrorRepo{sessionExecutorStore: svc.repo, err: errors.New("list failure")}

		svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if decisions.clearCalls != 0 {
			t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorAnotherSessionWorking); len(got) != 0 {
			t.Errorf("dispatch emitted its own record for an otherWorkingSessionID failure, want none: %+v", got)
		}
		if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
			t.Errorf("expected no dispatch on an otherWorkingSessionID failure")
		}
	})
}

// --- AC-F7: a guard skip must not mark the operation applied, so a later
// delivery of the exact same failure can still dispatch once the guard
// condition that blocked it clears. ---

func TestDispatchKanbanAgentErrorTrigger_GuardSkipDoesNotSuppressLaterValidDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC()
	requireNoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s2", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"}

	// s2 is still working, so AC-F5 skips before the engine is ever called —
	// the operation must never be marked applied by this skip.
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)
	if decisions.clearCalls != 0 {
		t.Fatalf("first delivery clearCalls = %d, want 0 (blocked by sibling session)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorAnotherSessionWorking); len(got) != 1 {
		t.Fatalf("got %d %q DEBUG records, want 1", len(got), msgAgentErrorAnotherSessionWorking)
	}

	// s2 settles; the exact same failure (same session, same execution id,
	// same operation id) redelivers and must now dispatch.
	s2, err := repo.GetTaskSession(ctx, "s2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	s2.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, s2); err != nil {
		t.Fatalf("update session: %v", err)
	}
	logs.TakeAll()

	svc.dispatchKanbanAgentErrorTrigger(ctx, data)
	if decisions.clearCalls != 1 {
		t.Errorf("second delivery clearCalls = %d, want 1 (the earlier guard skip must not have marked the operation applied)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Errorf("got %d dispatch INFO records, want 1", len(got))
	}
}

type agentErrorSessionReloadErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r *agentErrorSessionReloadErrorRepo) GetTaskSession(_ context.Context, _ string) (*models.TaskSession, error) {
	return nil, r.err
}

type agentErrorListSessionsErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r *agentErrorListSessionsErrorRepo) ListTaskSessions(_ context.Context, _ string) ([]*models.TaskSession, error) {
	return nil, r.err
}

// --- AC-E4/E7/E12/E13/E14: the pre-engine walk. ---

func TestDispatchKanbanAgentErrorTrigger_ActionVocabularyWarn(t *testing.T) {
	ctx := context.Background()

	t.Run("unregistered action warns and leaves operation unmarked", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionQueueRun}}},
		}
		// No RunQueueAdapter wired, so queue_run has no registered callback.
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		warnings := filterLogs(logs, msgAgentErrorActionUnregistered)
		if len(warnings) != 1 {
			t.Fatalf("got %d %q WARNINGs, want 1 (all: %+v)", len(warnings), msgAgentErrorActionUnregistered, logs.All())
		}
		fields := warnings[0].ContextMap()
		wantFields := map[string]interface{}{
			"workflow_id": "wf1", "step_id": "step1", "step_name": "Step 1",
			"action_type": "queue_run", "task_id": "t1", "session_id": "s1",
		}
		for key, want := range wantFields {
			if fields[key] != want {
				t.Errorf("field %q = %v, want %v", key, fields[key], want)
			}
		}
		if got := filterLogs(logs, msgAgentErrorDispatchFailed); len(got) != 1 {
			t.Errorf("got %d %q ERROR record(s), want 1", len(got), msgAgentErrorDispatchFailed)
		}
		if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
			t.Errorf("got %d %q INFO record(s), want 0", len(got), msgAgentErrorDispatched)
		}
		applied, err := svc.workflowStore.IsOperationApplied(ctx, agentErrorOperationID("s1", "exec-1"))
		if err != nil {
			t.Fatalf("check operation ledger: %v", err)
		}
		if applied {
			t.Fatal("unregistered action was recorded as applied")
		}
	})

	t.Run("two move_to_step actions produce no warning", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step3"}},
			}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
		stepGetter.steps["step3"] = &wfmodels.WorkflowStep{ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if got := filterLogs(logs, msgAgentErrorActionUnregistered); len(got) != 0 {
			t.Errorf("got %d WARNING(s) for move_to_step actions, want 0: %+v", len(got), got)
		}
		updated, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		if updated.WorkflowStepID != "step2" {
			t.Errorf("WorkflowStepID = %q, want step2 (first eligible transition wins)", updated.WorkflowStepID)
		}
	})

	t.Run("unregistered action remains retryable on redelivery", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionClearDecisions},
				{Type: wfmodels.GenericActionQueueRun},
			}},
		}
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"}
		svc.handleRecoverableFailureLocked(ctx, data)
		if decisions.clearCalls != 0 {
			t.Fatalf("first delivery clearCalls = %d, want 0", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorDispatchFailed); len(got) != 1 {
			t.Fatalf("first delivery: got %d ERROR records, want 1", len(got))
		}

		logs.TakeAll()
		svc.handleRecoverableFailureLocked(ctx, data)
		if got := filterLogs(logs, msgAgentErrorActionUnregistered); len(got) != 1 {
			t.Errorf("redelivery: got %d WARNINGs, want 1 (walk runs before the idempotency short-circuit)", len(got))
		}
		if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
			t.Errorf("redelivery: got a dispatch record, want none while callback is unregistered")
		}
		if got := filterLogs(logs, msgAgentErrorDispatchFailed); len(got) != 1 {
			t.Errorf("redelivery: got %d ERROR records, want 1", len(got))
		}
	})

	t.Run("a Set* call between construction and dispatch is picked up (AC-E12/E13)", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil) // not wired yet

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})
		if got := filterLogs(logs, msgAgentErrorActionUnregistered); len(got) != 1 {
			t.Fatalf("before wiring: got %d WARNINGs, want 1", len(got))
		}
		if decisions.clearCalls != 0 {
			t.Fatalf("before wiring: clearCalls = %d, want 0", decisions.clearCalls)
		}
		operationID := agentErrorOperationID("s1", "exec-1")
		applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
		if err != nil {
			t.Fatalf("before wiring: check operation ledger: %v", err)
		}
		if applied {
			t.Fatal("before wiring: operation was marked applied")
		}

		logs.TakeAll()
		svc.SetEngineDecisionStore(decisions) // triggers reinitWorkflowEngine

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})
		if got := filterLogs(logs, msgAgentErrorActionUnregistered); len(got) != 0 {
			t.Errorf("after wiring: got %d WARNINGs, want 0 (clear_decisions is now registered)", len(got))
		}
		if decisions.clearCalls != 1 {
			t.Errorf("after wiring: clearCalls = %d, want 1", decisions.clearCalls)
		}
		applied, err = svc.workflowStore.IsOperationApplied(ctx, operationID)
		if err != nil {
			t.Fatalf("after wiring: check operation ledger: %v", err)
		}
		if !applied {
			t.Fatal("after wiring: successful operation was not marked applied")
		}
	})

	t.Run("AC-E14: a failed walk LoadStep does not block dispatch", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		realStep := &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		stepGetter.steps["step1"] = realStep
		calls := 0
		stepGetter.getStepFunc = func(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("transient step store failure")
			}
			return stepGetter.steps[id], nil
		}
		decisions := &spyDecisionStore{}
		svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

		if got := filterLogs(logs, msgAgentErrorActionUnregistered); len(got) != 0 {
			t.Errorf("got a walk WARNING despite the walk's own LoadStep failing, want none")
		}
		if decisions.clearCalls != 1 {
			t.Errorf("clearCalls = %d, want 1 (the engine's own LoadStep retry must still succeed)", decisions.clearCalls)
		}
		if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
			t.Errorf("got %d dispatch INFO records, want 1", len(got))
		}
	})
}
