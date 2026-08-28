package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// step_entry_dispatch_test.go covers the defect this Build round fixes: the
// Office Default workflow's Review/Approval steps declare
// on_enter: [clear_decisions, queue_run_for_each_participant], but the
// legacy processOnEnter switch silently discarded both action types (no
// case, no default), so no reviewer/approver was ever woken. These tests
// drive the real E1 entry path (engine-driven on_turn_complete →
// applyEngineTransition → launchProcessOnEnter → processOnEnter) end to end
// against the real sqlite repo, so step-entry allocation and CAS-marker
// claiming are exercised for real, not stubbed.

// reviewLoopWorkflowJSON models a two-step Work/Review loop: Work's
// on_turn_complete advances into Review; Review declares the two
// engine-owned on_enter actions under test and its own on_turn_complete
// sends the task back to Work (modelling a rejection round for AC-D4).
const reviewLoopWorkflowJSON = `{
  "version": 1,
  "type": "kandev_workflow",
  "workflows": [
    {
      "name": "ReviewLoop",
      "steps": [
        {
          "name": "Work",
          "position": 0,
          "color": "bg-blue-500",
          "events": {
            "on_turn_complete": [{"type": "move_to_step", "config": {"step_position": 1}}]
          },
          "is_start_step": true,
          "allow_manual_move": true
        },
        {
          "name": "Review",
          "position": 1,
          "color": "bg-yellow-500",
          "events": {
            "on_enter": [
              {"type": "clear_decisions"},
              {"type": "queue_run_for_each_participant", "config": {"role": "reviewer"}}
            ],
            "on_turn_complete": [{"type": "move_to_step", "config": {"step_position": 0}}]
          },
          "allow_manual_move": true
        }
      ]
    }
  ]
}`

// stepEntryFakeRunQueue records every QueueRun call. Safe for concurrent use —
// processOnEnter's dispatch runs on a goroutine.
type stepEntryFakeRunQueue struct {
	mu    sync.Mutex
	calls []engine.QueueRunRequest
}

func (f *stepEntryFakeRunQueue) QueueRun(_ context.Context, req engine.QueueRunRequest) (engine.QueueOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	return "", nil
}

func (f *stepEntryFakeRunQueue) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// fakeParticipantStore returns a fixed participant list regardless of
// stepID/taskID — sufficient for a single-step-under-test scenario.
type fakeParticipantStore struct {
	participants []engine.ParticipantInfo
}

func (f *fakeParticipantStore) ListStepParticipants(_ context.Context, _, _ string) ([]engine.ParticipantInfo, error) {
	return f.participants, nil
}

func (f *fakeParticipantStore) ListTaskParticipants(_ context.Context, _ string) ([]engine.ParticipantInfo, error) {
	return f.participants, nil
}

// fakeDecisionStore is a minimal DecisionStore recording ClearStepDecisions
// calls; ListStepDecisions/RecordStepDecision are unused by this test.
type fakeDecisionStore struct {
	mu         sync.Mutex
	clearCalls int
}

func (f *fakeDecisionStore) ListStepDecisions(_ context.Context, _, _ string) ([]engine.DecisionInfo, error) {
	return nil, nil
}

func (f *fakeDecisionStore) RecordStepDecision(_ context.Context, _ engine.DecisionInfo) error {
	return nil
}

func (f *fakeDecisionStore) ClearStepDecisions(_ context.Context, _, _ string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clearCalls++
	return 0, nil
}

// reviewLoopFixture bundles the wiring shared by the AC-D3/AC-D4 tests.
type reviewLoopFixture struct {
	svc        *Service
	repo       sessionExecutorStore
	agentMgr   *mockAgentManager
	runQueue   *stepEntryFakeRunQueue
	decisions  *fakeDecisionStore
	nameToID   map[string]string
	taskRepoMk *mockTaskRepo
}

func newReviewLoopFixture(t *testing.T) *reviewLoopFixture {
	t.Helper()
	sg, nameToID := buildWorkflowFromJSON(t, reviewLoopWorkflowJSON)
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", nameToID["Work"])
	setSessionExecID(t, repo, "s1", "exec-1")

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createEngineService(t, repo, sg, agentMgr)

	runQueue := &stepEntryFakeRunQueue{}
	participants := &fakeParticipantStore{participants: []engine.ParticipantInfo{
		{ID: "participant-1", StepID: nameToID["Review"], Role: "reviewer", AgentProfileID: "agent-reviewer-1"},
	}}
	decisions := &fakeDecisionStore{}
	svc.SetEngineRunQueue(runQueue)
	svc.SetEngineParticipantStore(participants)
	svc.SetEngineDecisionStore(decisions)

	taskRepo := svc.taskRepo.(*mockTaskRepo)
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)

	return &reviewLoopFixture{
		svc: svc, repo: repo, agentMgr: agentMgr,
		runQueue: runQueue, decisions: decisions,
		nameToID: nameToID, taskRepoMk: taskRepo,
	}
}

// fireOnTurnComplete sets the session RUNNING, fires on_turn_complete, and
// blocks until processOnEnter's goroutine has finished — so assertions right
// after this call see the dispatch's side effects.
func (f *reviewLoopFixture) fireOnTurnComplete(t *testing.T, ctx context.Context) bool {
	t.Helper()
	setSessionState(t, ctx, f.repo, "s1", models.TaskSessionStateRunning)

	onEnterDone := make(chan struct{})
	f.svc.onProcessOnEnterComplete = func() { close(onEnterDone) }

	session, err := f.repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	transitioned := f.svc.processOnTurnCompleteViaEngine(ctx, "t1", session)

	select {
	case <-onEnterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for on_enter processing")
	}
	return transitioned
}

// TestProcessOnEnter_QueueRunForEachParticipant_SingleReviewerEntry_QueuesExactlyOneRun
// covers AC-D3: a single reviewer entering Review via an ordinary
// on_turn_complete transition produces exactly one queued run, and
// clear_decisions runs alongside it.
func TestProcessOnEnter_QueueRunForEachParticipant_SingleReviewerEntry_QueuesExactlyOneRun(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)

	transitioned := f.fireOnTurnComplete(t, ctx)
	if !transitioned {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	assertStepByName(t, ctx, f.repo, "s1", "Review", f.nameToID)

	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count = %d, want exactly 1 (AC-D3)", got)
	}
	if got := f.runQueue.calls[0].AgentProfileID; got != "agent-reviewer-1" {
		t.Errorf("queued run agent = %q, want %q", got, "agent-reviewer-1")
	}

	f.decisions.mu.Lock()
	clearCalls := f.decisions.clearCalls
	f.decisions.mu.Unlock()
	if clearCalls != 1 {
		t.Errorf("clear_decisions calls = %d, want 1", clearCalls)
	}
}

// TestProcessOnEnter_QueueRunForEachParticipant_SecondEntryAfterRejection_QueuesAnotherRun
// covers AC-D4: after a rejection round sends the task back to Work and it
// re-enters Review, the step-entry CAS marker for the *new* entry has never
// been claimed, so a further run is queued — the at-most-once guarantee is
// per step-entry, not a blanket suppression of repeat dispatch.
func TestProcessOnEnter_QueueRunForEachParticipant_SecondEntryAfterRejection_QueuesAnotherRun(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)

	// Work -> Review (first entry).
	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count after first entry = %d, want 1", got)
	}

	// Review -> Work (rejection round).
	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Review -> Work, got none")
	}
	assertStepByName(t, ctx, f.repo, "s1", "Work", f.nameToID)
	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count after rejection = %d, want still 1 (Work declares no queue_run_for_each_participant)", got)
	}

	// Work -> Review (second entry).
	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	assertStepByName(t, ctx, f.repo, "s1", "Review", f.nameToID)

	if got := f.runQueue.callCount(); got != 2 {
		t.Fatalf("queued run count after second Review entry = %d, want 2 (AC-D4: not suppressed)", got)
	}
}

// failingDecisionStore wraps fakeDecisionStore but always fails
// ClearStepDecisions, for AC-C2 below.
type failingDecisionStore struct {
	fakeDecisionStore
}

func (f *failingDecisionStore) ClearStepDecisions(_ context.Context, _, _ string) (int64, error) {
	f.mu.Lock()
	f.clearCalls++
	f.mu.Unlock()
	return 0, errors.New("boom: clear_decisions failure")
}

// TestProcessOnEnter_ClearDecisionsFailure_BlocksSubsequentOnEnterActions
// covers AC-C2: "IF clear_decisions fails, THEN the system shall not
// execute any remaining on_enter action for that step entry." Review's
// on_enter declares queue_run_for_each_participant immediately after
// clear_decisions (the exact order the Office Default workflow uses) — a
// stale decision from a previous round combined with a fresh participant
// fan-out could otherwise satisfy wait_for_quorum without every
// current-round reviewer actually voting.
func TestProcessOnEnter_ClearDecisionsFailure_BlocksSubsequentOnEnterActions(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)
	f.svc.SetEngineDecisionStore(&failingDecisionStore{})

	core, logs := observer.New(zapcore.ErrorLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	f.svc.logger = log

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}

	if got := f.runQueue.callCount(); got != 0 {
		t.Fatalf("queued run count after clear_decisions failure = %d, want 0 (AC-C2)", got)
	}

	var acC2Errors []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "processOnEnter: clear_decisions failed, aborting remaining on_enter actions for step entry" {
			acC2Errors = append(acC2Errors, e)
		}
	}
	if len(acC2Errors) != 1 {
		t.Fatalf("got %d AC-C2 ERROR log(s), want exactly 1 (all entries: %+v)", len(acC2Errors), logs.All())
	}
	fields := acC2Errors[0].ContextMap()
	if fields["workflow_id"] != "wf1" {
		t.Errorf("AC-C2 error workflow_id = %v, want %q", fields["workflow_id"], "wf1")
	}
	if fields["step_id"] != f.nameToID["Review"] {
		t.Errorf("AC-C2 error step_id = %v, want %q", fields["step_id"], f.nameToID["Review"])
	}
	if fields["task_id"] != "t1" {
		t.Errorf("AC-C2 error task_id = %v, want %q", fields["task_id"], "t1")
	}
	if cause, _ := fields["cause"].(string); cause == "" {
		t.Errorf("AC-C2 error missing cause field: %+v", fields)
	}
}

// TestProcessOnEnter_ClearDecisionsFailure_DoesNotLaunchEarlierAutoStart
// covers AC-C2 when auto_start_agent appears before clear_decisions. The
// auto-start action keeps its fixed final execution position, so a failed
// clear must abort the launch as well as later engine-owned actions.
func TestProcessOnEnter_ClearDecisionsFailure_DoesNotLaunchEarlierAutoStart(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)
	f.svc.SetEngineDecisionStore(&failingDecisionStore{})

	stepGetter, ok := f.svc.workflowStepGetter.(*mockStepGetter)
	if !ok {
		t.Fatalf("workflow step getter type = %T, want *mockStepGetter", f.svc.workflowStepGetter)
	}
	reviewStep := stepGetter.steps[f.nameToID["Review"]]
	reviewStep.Prompt = "review prompt"
	reviewStep.Events.OnEnter = []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterAutoStartAgent},
		{Type: wfmodels.OnEnterClearDecisions},
	}

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}

	f.agentMgr.mu.Lock()
	promptCount := len(f.agentMgr.capturedPrompts)
	f.agentMgr.mu.Unlock()
	if promptCount != 0 {
		t.Fatalf("auto-start prompt count after clear_decisions failure = %d, want 0", promptCount)
	}
}
