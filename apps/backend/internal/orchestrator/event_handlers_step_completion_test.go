//revive:disable:file-length-limit Existing completion-signal regression coverage is intentionally co-located.
package orchestrator

import (
	"context"
	"errors"
	"expvar"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type countingCompletionTurnService struct {
	TurnService
	completeCalls atomic.Int32
}

func (s *countingCompletionTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	s.completeCalls.Add(1)
	return s.TurnService.CompleteTurn(ctx, turnID)
}

// TestProcessOnTurnComplete_ExplicitSignalGating verifies the ADR 0015
// gating: when AutoAdvanceRequiresSignal=true, turn-end without a matching
// pending signal must NOT transition. With the signal present, the
// transition fires as normal.
func TestProcessOnTurnComplete_ExplicitSignalGating(t *testing.T) {
	ctx := context.Background()

	build := func(t *testing.T, withSignal bool, stepRequires bool) (svc *Service, taskID, sessionID string) {
		t.Helper()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: stepRequires,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc = createTestService(repo, stepGetter, newMockTaskRepo())

		if withSignal {
			signal := models.PendingStepCompletionSignal{
				StepID:     "step1",
				Source:     models.StepCompletionSourceAgent,
				Summary:    "all done",
				SignaledAt: time.Now().UTC(),
			}
			if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
				t.Fatalf("seed pending signal: %v", err)
			}
		}
		return svc, "t1", "s1"
	}

	t.Run("step requires, no signal → no transition", func(t *testing.T) {
		svc, taskID, sessionID := build(t, false, true)
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); got {
			t.Errorf("expected gating to BLOCK transition, got transition=true")
		}
		updated, _ := svc.repo.GetTask(ctx, taskID)
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected to stay on step1, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("step requires, signal present → transition fires", func(t *testing.T) {
		svc, taskID, sessionID := build(t, true, true)
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); !got {
			t.Errorf("expected transition with pending signal, got transition=false")
		}
		updated, _ := svc.repo.GetTask(ctx, taskID)
		if updated.WorkflowStepID != "step2" {
			t.Errorf("expected to move to step2, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("step does not require → legacy behaviour", func(t *testing.T) {
		svc, taskID, sessionID := build(t, false, false)
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); !got {
			t.Errorf("expected transition (step does not require signal), got transition=false")
		}
	})

	t.Run("step requires, signal for DIFFERENT step → still blocked", func(t *testing.T) {
		svc, taskID, sessionID := build(t, false, true)
		stale := models.PendingStepCompletionSignal{
			StepID:     "step_old", // stale entry — doesn't match current step
			Source:     models.StepCompletionSourceAgent,
			Summary:    "stale",
			SignaledAt: time.Now().UTC(),
		}
		if err := svc.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, stale); err != nil {
			t.Fatalf("seed stale signal: %v", err)
		}
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); got {
			t.Errorf("expected stale signal to be treated as absent, but got transition=true")
		}
	})
}

func TestReconcileDueCompletionIntentsSettlesCapturedTurn(t *testing.T) {
	ctx := context.Background()
	reconciled := expvar.Get("administrative_turn_completion_reconciled_total").(*expvar.Map)
	beforeSettled := completionMetricValue(reconciled, "outcome=settled;cause=quiet_grace")
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil || turn.CompletedAt == nil {
		t.Fatalf("GetTurn = (%+v, %v), want completed captured turn", turn, err)
	}
	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || intent.State != models.CompletionIntentStateSettled {
		t.Fatalf("GetCompletionIntent = (%+v, %v), want settled", intent, err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil || session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("GetTaskSession = (%+v, %v), want waiting session", session, err)
	}
	if got := completionMetricValue(reconciled, "outcome=settled;cause=quiet_grace"); got != beforeSettled+1 {
		t.Fatalf("completion reconciliation metric = %d, want %d", got, beforeSettled+1)
	}
}

func TestReconcileDueCompletionIntentsRearmsBehindPendingToolWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Add(-models.CompletionIntentQuietGrace)
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}))
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	requireNoError(t, err)
	requireNoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "tool-pending", TaskID: "t1", TaskSessionID: "s1", TurnID: "turn-1",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "call-1", "status": "running"}, CreatedAt: now, UpdatedAt: now,
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	turn, err := repo.GetTurn(ctx, "turn-1")
	requireNoError(t, err)
	if turn.CompletedAt != nil {
		t.Fatal("automatic reconciliation must not settle a turn with pending tool work")
	}
	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	requireNoError(t, err)
	if intent.State != models.CompletionIntentStatePending || !intent.EligibleAt.After(now) {
		t.Fatalf("intent after active-work gate = %+v, want pending and rearmed", intent)
	}
}

func TestReconcileDueCompletionIntentsPreservesRestartedBackgroundAttestation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Add(-models.CompletionIntentQuietGrace)
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}))
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	requireNoError(t, err)
	// This is the durable shape written from an adapter-attested background
	// frame before a backend restart. The fresh service deliberately has no
	// in-memory activity map, so this proves recovery remains fail-closed.
	requireNoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyBackgroundWorkAttested, true))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)
	turn, err := repo.GetTurn(ctx, "turn-1")
	requireNoError(t, err)
	if turn.CompletedAt != nil {
		t.Fatal("restart recovery must not settle an adapter-attested background turn")
	}
}

func TestReconcileCompletionIntentRechecksBarriersInsideSessionGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Add(-models.CompletionIntentQuietGrace)
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}))
	_, intent, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	requireNoError(t, err)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}

	lock, release := svc.acquireCancelInFlightGuard("s1")
	lock.Lock()
	done := make(chan struct{})
	go func() {
		svc.reconcileCompletionIntent(ctx, repo, intent)
		close(done)
	}()
	// Registered immediately after the goroutine starts, before any
	// t.Fatal path below: a fatal failure still needs the lock released and
	// the goroutine joined, or it leaks holding a lock into setupTestRepo's
	// later t.Cleanup closing the database out from under it.
	var releaseOnce sync.Once
	releaseGuard := func() {
		releaseOnce.Do(func() {
			lock.Unlock()
			release()
		})
	}
	t.Cleanup(func() {
		releaseGuard()
		<-done
	})
	select {
	case <-done:
		t.Fatal("reconciliation bypassed the session guard")
	default:
	}
	// This models cancellation beginning after the due scan found the intent
	// but before the reconciler can claim it. The guarded re-check must win.
	svc.cancellationOperations = map[string]*cancelOperation{"s1": {}}
	releaseGuard()
	<-done
	turn, err := repo.GetTurn(ctx, "turn-1")
	requireNoError(t, err)
	if turn.CompletedAt != nil {
		t.Fatal("reconciliation settled after a guarded cancellation barrier appeared")
	}
}

func completionMetricValue(metrics *expvar.Map, key string) int64 {
	value := metrics.Get(key)
	if value == nil {
		return 0
	}
	counter, ok := value.(*expvar.Int)
	if !ok {
		return 0
	}
	return counter.Value()
}

func TestReconcileDueCompletionIntentsRetriesTransientTurnLookupFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	turns := &failingActiveTurnLookup{
		TurnService: &repoTurnService{repo: repo},
		err:         errors.New("transient active turn lookup failure"),
	}
	svc.turnService = turns
	svc.reconcileDueCompletionIntents(ctx)

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent after transient failure: %v", err)
	}
	if intent.State != models.CompletionIntentStatePending {
		t.Fatalf("intent state after transient failure = %q, want pending for retry", intent.State)
	}

	turns.err = nil
	svc.reconcileDueCompletionIntents(ctx)
	intent, err = repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || intent.State != models.CompletionIntentStateSettled {
		t.Fatalf("GetCompletionIntent after retry = (%+v, %v), want settled", intent, err)
	}
}

// TestReconcileDueCompletionIntentsRecoversAbandonedSettlingClaim is the
// end-to-end crash-recovery regression: a process that dies right after
// claiming an intent for settlement (before doing any of the actual
// settlement work) leaves that row "settling" with no in-process handler
// left to release it. ListDueCompletionIntents only ever selects pending
// rows, so without lease-based reclaim this claim — and the coarse RUNNING
// turn behind it — would be stranded forever. A later reconcile pass, once
// the settling lease has expired, must reclaim and then fully settle the
// intent in the same call.
func TestReconcileDueCompletionIntentsRecoversAbandonedSettlingClaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	// Simulate a crash immediately after the claim: the lease is already in
	// the past, and nothing else ever touches this row.
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, "intent-1", now.Add(-5*time.Minute), now.Add(-3*time.Minute)); err != nil {
		t.Fatalf("simulate abandoned claim: %v", err)
	}
	stranded, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || stranded.State != models.CompletionIntentStateSettling {
		t.Fatalf("GetCompletionIntent after simulated crash = (%+v, %v), want settling", stranded, err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	recovered, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent after recovery pass: %v", err)
	}
	if recovered.State != models.CompletionIntentStateSettled {
		t.Fatalf("intent state after recovery pass = %q, want settled — a crashed settling claim must not strand the turn forever", recovered.State)
	}
	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatal("recovered turn was never completed")
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want WAITING_FOR_INPUT after recovered settlement", session.State)
	}
}

func TestReconcileDueCompletionIntentsSettlesOldTurnAfterTaskMoved(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step2"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil || turn.CompletedAt == nil {
		t.Fatalf("GetTurn = (%+v, %v), want completed old turn", turn, err)
	}
	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || intent.State != models.CompletionIntentStateSuperseded {
		t.Fatalf("GetCompletionIntent = (%+v, %v), want superseded", intent, err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil || session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("GetTaskSession = (%+v, %v), want waiting after old turn settlement", session, err)
	}
}

// TestReconcileCompletionIntentSettlesMovedIntentWithoutEvaluatingCurrentStep
// covers settleMovedCompletionIntent's negative half: the source turn's
// completion intent was requested against step1, but the task has since
// moved to step2 (task.moved owns the destination's own on-enter dispatch).
// Reconciling the stale intent must settle the exact old turn, but must NOT
// run processOnTurnCompleteViaEngine at all for this path — not for the old
// step (whose transition is irrelevant, since it was superseded) and not for
// the current step either. Giving step2 its own on_turn_complete auto-advance
// makes a wrongful evaluation observable: if reconciliation ran it, the task
// would move again to step3 on its own, re-evaluating a step transition no
// step_complete signal ever authorized for this settlement.
func TestReconcileCompletionIntentSettlesMovedIntentWithoutEvaluatingCurrentStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step2"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	stepGetter := newMockStepGetter()
	// step2's own on_turn_complete would move the task to step3 if evaluated.
	// The moved-intent path must never reach that evaluation: it is not this
	// settlement's job, and nothing here proves a real turn actually
	// completed against step2's requirements.
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["step3"] = &wfmodels.WorkflowStep{ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2}

	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || intent.State != models.CompletionIntentStateSuperseded {
		t.Fatalf("GetCompletionIntent = (%+v, %v), want superseded", intent, err)
	}
	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil || turn.CompletedAt == nil {
		t.Fatalf("GetTurn = (%+v, %v), want completed old turn", turn, err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil || session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("GetTaskSession = (%+v, %v), want waiting after old turn settlement", session, err)
	}
	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask after reconcile: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Fatalf("task moved to %q, want to stay on step2 — settleMovedCompletionIntent must not evaluate any step's on_turn_complete", updatedTask.WorkflowStepID)
	}
}

func TestReconcileDueCompletionIntentsDoesNotCloseSuccessorTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-old", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn old: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-old", TaskID: "t1", SessionID: "s1", TurnID: "turn-old", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if err := repo.CompleteTurn(ctx, "turn-old"); err != nil {
		t.Fatalf("CompleteTurn old: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-successor", TaskID: "t1", TaskSessionID: "s1", StartedAt: now.Add(time.Second)}); err != nil {
		t.Fatalf("CreateTurn successor: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	intent, err := repo.GetCompletionIntent(ctx, "intent-old")
	if err != nil || intent.State != models.CompletionIntentStateSuperseded {
		t.Fatalf("GetCompletionIntent = (%+v, %v), want superseded", intent, err)
	}
	successor, err := repo.GetTurn(ctx, "turn-successor")
	if err != nil || successor.CompletedAt != nil {
		t.Fatalf("GetTurn successor = (%+v, %v), want active successor", successor, err)
	}
}

func TestReconcileDueCompletionIntentsDrainsMovedStepHandoff(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step2"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "s1", "t1", "review the change", "", messagequeue.QueuedByWorkflow, false, nil, nil); err != nil {
		t.Fatalf("QueueMessageWithMetadata: %v", err)
	}

	svc.reconcileDueCompletionIntents(ctx)

	select {
	case <-agentMgr.promptDone:
	case <-time.After(time.Second):
		t.Fatal("settling old turn did not drain the current-step handoff")
	}
	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	if len(agentMgr.capturedPrompts) != 1 || agentMgr.capturedPrompts[0] != "review the change" {
		t.Fatalf("captured prompts = %#v, want one moved-step handoff", agentMgr.capturedPrompts)
	}
}

func TestSettleCompletionIntentForProviderTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.settleCompletionIntentForProviderTurn(ctx, "s1", "turn-1")

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || intent.State != models.CompletionIntentStateSettled {
		t.Fatalf("GetCompletionIntent = (%+v, %v), want settled", intent, err)
	}
}

func TestRearmCompletionIntentForActivity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.rearmCompletionIntentForActivity(ctx, "s1")

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if intent.LastPostSignalActivityAt.IsZero() {
		t.Fatal("activity did not persist a post-signal timestamp")
	}
	if !intent.EligibleAt.After(time.Now().UTC()) {
		t.Fatalf("rearmed eligible_at = %v, want future quiet deadline", intent.EligibleAt)
	}
}

// countingActiveTurnService tracks GetActiveTurn calls so a test can prove a
// per-session throttle actually short-circuits real DB work rather than just
// happening to produce the same end state.
type countingActiveTurnService struct {
	TurnService
	activeTurnCalls atomic.Int32
}

func (s *countingActiveTurnService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	s.activeTurnCalls.Add(1)
	return s.TurnService.GetActiveTurn(ctx, sessionID)
}

// TestRearmCompletionIntentForActivityThrottlesPerSession covers the
// message_streaming/thinking_streaming hot path: every token chunk in a
// stream calls rearmCompletionIntentForActivity while holding the session's
// cancelInFlight guard, so an unthrottled implementation would perform three
// DB round trips per chunk. A second call within the throttle window must
// not touch GetActiveTurn at all; a call after the window elapses must.
func TestRearmCompletionIntentForActivityThrottlesPerSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	turns := &countingActiveTurnService{TurnService: &repoTurnService{repo: repo}}
	svc.turnService = turns

	svc.rearmCompletionIntentForActivity(ctx, "s1")
	if got := turns.activeTurnCalls.Load(); got != 1 {
		t.Fatalf("first call GetActiveTurn calls = %d, want 1", got)
	}
	firstIntent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	firstEligibleAt := firstIntent.EligibleAt

	// A burst of activity within the throttle window (simulating a run of
	// streamed token chunks) must not perform any further DB reads.
	for i := 0; i < 5; i++ {
		svc.rearmCompletionIntentForActivity(ctx, "s1")
	}
	if got := turns.activeTurnCalls.Load(); got != 1 {
		t.Fatalf("GetActiveTurn calls after throttled burst = %d, want still 1", got)
	}
	afterBurst, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if !afterBurst.EligibleAt.Equal(firstEligibleAt) {
		t.Fatalf("eligible_at moved during throttled burst: %v -> %v", firstEligibleAt, afterBurst.EligibleAt)
	}

	// Manually age the throttle marker past the window to simulate real
	// elapsed time without a production time.Sleep.
	svc.completionIntentRearmThrottle.Store("s1", time.Now().UTC().Add(-completionIntentRearmThrottleInterval-time.Millisecond))
	svc.rearmCompletionIntentForActivity(ctx, "s1")
	if got := turns.activeTurnCalls.Load(); got != 2 {
		t.Fatalf("GetActiveTurn calls after throttle window elapsed = %d, want 2", got)
	}
}

func TestCompletionIntentReconcilerProcessesDueWorkAndStops(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	turns := &countingCompletionTurnService{TurnService: &repoTurnService{repo: repo}}
	svc.turnService = turns
	svc.completionIntentReconcileInterval = time.Millisecond
	svc.resetCompletionIntentReconciler()
	svc.startCompletionIntentReconciler()
	// Registered immediately, before any t.Fatalf path in the polling loop
	// below: a fatal there must not leave the reconciler's background
	// goroutine running against a database setupTestRepo's own t.Cleanup is
	// about to close. stopCompletionIntentReconciler is idempotent, so the
	// explicit call further down and this safety net cannot conflict.
	t.Cleanup(svc.stopCompletionIntentReconciler)

	deadline := time.Now().Add(time.Second)
	for {
		intent, err := repo.GetCompletionIntent(ctx, "intent-1")
		if err != nil {
			t.Fatalf("GetCompletionIntent: %v", err)
		}
		if intent.State == models.CompletionIntentStateSettled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("due completion intent was not settled by periodic reconciler: %+v", intent)
		}
		time.Sleep(time.Millisecond)
	}
	if got := turns.completeCalls.Load(); got != 1 {
		t.Fatalf("CompleteTurn calls = %d, want exactly one", got)
	}

	svc.stopCompletionIntentReconciler()
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-2", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn after worker shutdown: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-2", TaskID: "t1", SessionID: "s1", TurnID: "turn-2", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent after worker shutdown: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	intent, err := repo.GetCompletionIntent(ctx, "intent-2")
	if err != nil {
		t.Fatalf("GetCompletionIntent after worker shutdown: %v", err)
	}
	if intent.State != models.CompletionIntentStatePending {
		t.Fatalf("stopped reconciler changed a later intent to %q", intent.State)
	}
	if got := turns.completeCalls.Load(); got != 1 {
		t.Fatalf("CompleteTurn calls after shutdown = %d, want exactly one", got)
	}
}

func TestProcessOnTurnComplete_OfficeExplicitSignalGating(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-gate", "s-office-gate", "")

	stepID := "wfs-t-office-gate" // matches seedOfficeSession's stepID convention
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf-office", Name: "work", Position: 1,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), &mockAgentManager{})

	task, err := repo.GetTask(ctx, "t-office-gate")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s-office-gate")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if got := svc.processOnTurnComplete(ctx, task, session); got {
		t.Fatalf("expected Office gate to BLOCK transition, got transition=true")
	}

	updatedTask, err := repo.GetTask(ctx, "t-office-gate")
	if err != nil {
		t.Fatalf("re-read task: %v", err)
	}
	if updatedTask.WorkflowStepID != stepID {
		t.Errorf("expected task to stay on %q, got %q", stepID, updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s-office-gate")
	if err != nil {
		t.Fatalf("re-read session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected session WAITING_FOR_INPUT, got %q", updatedSession.State)
	}
}

// TestProcessOnTurnComplete_OfficeExplicitSignalGating_AllowsWithSignal proves
// the ADR 0015 gate's ALLOW half for Office: with a pending signal recorded
// for the current step, turn-end transitions as normal. Uses move_to_step
// (not move_to_next) to mirror office-default.yml's shipped Work step action.
func TestProcessOnTurnComplete_OfficeExplicitSignalGating_AllowsWithSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-allow", "s-office-allow", "")

	stepID := "wfs-t-office-allow" // matches seedOfficeSession's stepID convention
	reviewStepID := "wfs-office-review"
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf-office", Name: "work", Position: 1,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": reviewStepID}},
			},
		},
	}
	stepGetter.steps[reviewStepID] = &wfmodels.WorkflowStep{
		ID: reviewStepID, WorkflowID: "wf-office", Name: "review", Position: 2,
	}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), &mockAgentManager{})

	signal := models.PendingStepCompletionSignal{
		StepID:     stepID,
		Source:     models.StepCompletionSourceAgent,
		Summary:    "work complete",
		SignaledAt: time.Now().UTC(),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s-office-allow", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	task, err := repo.GetTask(ctx, "t-office-allow")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s-office-allow")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if got := svc.processOnTurnComplete(ctx, task, session); !got {
		t.Fatalf("expected Office gate to ALLOW transition with pending signal, got transition=false")
	}

	updatedTask, err := repo.GetTask(ctx, "t-office-allow")
	if err != nil {
		t.Fatalf("re-read task: %v", err)
	}
	if updatedTask.WorkflowStepID != reviewStepID {
		t.Errorf("expected task to move to %q, got %q", reviewStepID, updatedTask.WorkflowStepID)
	}
}
func TestProcessOnTurnComplete_BlocksWhileClarificationPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	task, _ := repo.GetTask(ctx, "t1")
	session, _ := repo.GetTaskSession(ctx, "s1")
	if got := svc.processOnTurnComplete(ctx, task, session); got {
		t.Fatal("pending clarification must block legacy on_turn_complete transition")
	}
	updated, _ := repo.GetTask(ctx, "t1")
	if updated.WorkflowStepID != "step1" {
		t.Fatalf("expected workflow step to remain step1, got %q", updated.WorkflowStepID)
	}
}

func TestProcessOnTurnCompleteViaEngine_BlocksWhileClarificationPendingEvenWithSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "done without answer",
		SignaledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})

	session, _ := repo.GetTaskSession(ctx, "s1")
	if got := svc.processOnTurnCompleteViaEngine(ctx, "t1", session); got {
		t.Fatal("pending clarification must block engine on_turn_complete transition even with completion signal")
	}
	session, _ = repo.GetTaskSession(ctx, "s1")
	if _, has := models.LoadPendingStepSignal(session.Metadata); has {
		t.Fatal("pending clarification must clear stale completion signal")
	}
	updated, _ := repo.GetTask(ctx, "t1")
	if updated.WorkflowStepID != "step1" {
		t.Fatalf("expected workflow step to remain step1, got %q", updated.WorkflowStepID)
	}
}

// TestProcessOnTurnCompleteViaEngine_OfficeExplicitSignalGating covers the
// production engine path for Office sessions. The legacy path has separate
// coverage, but production uses the workflow engine when it is configured.
func TestProcessOnTurnCompleteViaEngine_OfficeExplicitSignalGating(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-engine", "s-office-engine", "")

	stepID := "wfs-t-office-engine"
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf-office", Name: "Work", Position: 1,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["wfs-office-engine-review"] = &wfmodels.WorkflowStep{
		ID: "wfs-office-engine-review", WorkflowID: "wf-office", Name: "Review", Position: 2,
	}

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	session, err := repo.GetTaskSession(ctx, "s-office-engine")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if transitioned := svc.processOnTurnCompleteViaEngine(ctx, "t-office-engine", session); transitioned {
		t.Fatal("expected the engine path to block an Office turn without a signal")
	}

	task, err := repo.GetTask(ctx, "t-office-engine")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != stepID {
		t.Errorf("workflow step = %q, want %q", task.WorkflowStepID, stepID)
	}
}

// TestLoadPendingStepSignal_RoundTrip verifies the bag survives JSON
// rehydration — important for the backend-restart path where the bag is
// read from the DB as map[string]interface{} rather than the typed struct.
func TestLoadPendingStepSignal_RoundTrip(t *testing.T) {
	t.Run("typed struct", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Nanosecond)
		want := models.PendingStepCompletionSignal{
			StepID: "step-1", Source: "agent", Summary: "ok", SignaledAt: now,
		}
		meta := map[string]interface{}{
			models.SessionMetaKeyPendingStepCompletion: want,
		}
		got, ok := models.LoadPendingStepSignal(meta)
		if !ok || got.StepID != "step-1" || got.Source != "agent" {
			t.Errorf("typed struct round-trip failed: ok=%v got=%+v", ok, got)
		}
	})

	t.Run("json-rehydrated map", func(t *testing.T) {
		meta := map[string]interface{}{
			models.SessionMetaKeyPendingStepCompletion: map[string]interface{}{
				"step_id":     "step-2",
				"source":      "manual_fallback",
				"summary":     "user marked complete",
				"signaled_at": "2026-06-04T12:00:00Z",
			},
		}
		got, ok := models.LoadPendingStepSignal(meta)
		if !ok {
			t.Fatal("expected models.LoadPendingStepSignal to recognise map shape")
		}
		if got.StepID != "step-2" || got.Source != "manual_fallback" || got.Summary != "user marked complete" {
			t.Errorf("map round-trip mismatch: %+v", got)
		}
	})

	t.Run("absent key returns false", func(t *testing.T) {
		_, ok := models.LoadPendingStepSignal(map[string]interface{}{})
		if ok {
			t.Error("expected ok=false on empty metadata")
		}
	})

	t.Run("nil metadata returns false", func(t *testing.T) {
		_, ok := models.LoadPendingStepSignal(nil)
		if ok {
			t.Error("expected ok=false on nil metadata")
		}
	})
}

// TestOnStepCompletionSignaled covers the out-of-band subscriber that
// drives a step transition when a `step_complete_kandev` signal arrives
// AFTER the turn has already ended. The three branches:
//
//   - session still RUNNING (turn in flight): no-op, inline path will handle it.
//   - session WAITING + step matches + step gated: re-runs transition pipeline.
//   - signal stale (step has changed under us): clear the bag, no transition.
//   - step not signal-gated: do not advance (signal is not a manual-advance trigger).
func TestOnStepCompletionSignaled(t *testing.T) {
	ctx := context.Background()

	buildEvent := func(taskID, sessionID, stepID string) *bus.Event {
		return bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"step_id":    stepID,
		})
	}

	t.Run("session still RUNNING — subscriber is a no-op", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		// seedSession leaves the session in RUNNING; that's what we want.

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: true,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected to stay on step1 (turn in flight), got %q", updated.WorkflowStepID)
		}
	})

	t.Run("WAITING + matching step + gated → transition fires", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		// Flip session to WAITING_FOR_INPUT (the only state the subscriber acts on).
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		// Pre-write the signal in the bag — the subscriber re-runs the
		// inline turn-end path, which reads the bag for gating.
		signal := models.PendingStepCompletionSignal{
			StepID:     "step1",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "ok",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
			t.Fatalf("seed bag: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: true,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step2" {
			t.Errorf("expected transition to step2, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("WAITING + pending clarification → no transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedPendingClarificationMessage(t, repo, "t1", "s1")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		signal := models.PendingStepCompletionSignal{
			StepID:     "step1",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "ok",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
			t.Fatalf("seed bag: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: true,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected pending clarification to keep task on step1, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("stale step → bag cleared, no transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step_current")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		// Stale signal: written when step was "step_old", but the task has
		// already moved on to "step_current" via some other path.
		stale := models.PendingStepCompletionSignal{
			StepID:     "step_old",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "stale",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, stale); err != nil {
			t.Fatalf("seed stale signal: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step_current"] = &wfmodels.WorkflowStep{
			ID: "step_current", WorkflowID: "wf1", Name: "Current", Position: 5,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step_old"))

		updatedSession, _ := repo.GetTaskSession(ctx, "s1")
		if _, hasBag := models.LoadPendingStepSignal(updatedSession.Metadata); hasBag {
			t.Error("expected stale bag entry to be cleared")
		}
		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step_current" {
			t.Errorf("expected no transition (stale signal), got %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("stale event → valid bag for CURRENT step is preserved", func(t *testing.T) {
		// Pins the negative side of the StepID guard in the subscriber's
		// stale-step branch: a late step-A event must not erase a
		// freshly-written step-B bag (which can happen when the session
		// is reused across steps without auto_start_agent). A regression
		// here would silently leave signal-gated steps stuck waiting.
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step_current")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		// Bag holds a VALID signal for the current step (step_current).
		valid := models.PendingStepCompletionSignal{
			StepID:     "step_current",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "valid current-step signal",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, valid); err != nil {
			t.Fatalf("seed valid signal: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step_current"] = &wfmodels.WorkflowStep{
			ID: "step_current", WorkflowID: "wf1", Name: "Current", Position: 5,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		// Fire a STALE event (step_old != current step_current). The
		// guard must see that the bag's StepID is "step_current" (not
		// "step_old") and leave it alone.
		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step_old"))

		updatedSession, _ := repo.GetTaskSession(ctx, "s1")
		bag, hasBag := models.LoadPendingStepSignal(updatedSession.Metadata)
		if !hasBag {
			t.Fatal("expected valid bag to survive stale event")
		}
		if bag.StepID != "step_current" {
			t.Errorf("expected bag StepID=step_current, got %q", bag.StepID)
		}
	})

	t.Run("step not signal-gated → subscriber ignores", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}

		// Step explicitly NOT gated on the signal — even though one was
		// written and matches, the subscriber must not advance.
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: false,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected no transition for un-gated step, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("coordinator cancellation wins while signal subscriber waits", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		signal := models.PendingStepCompletionSignal{
			StepID: "step1", Source: models.StepCompletionSourceAgent,
			Summary: "done", SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
			t.Fatalf("seed signal: %v", err)
		}
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: true,
			Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type: wfmodels.OnTurnCompleteMoveToNext,
			}}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		guard, release := svc.acquireCancelInFlightGuard("s1")
		guard.Lock()
		done := make(chan struct{})
		go func() {
			svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))
			close(done)
		}()
		coordinatorStopWaitForGuardRefs(t, svc, "s1", 2)
		changed, _, err := repo.CancelActiveTaskSession(ctx, "s1", coordinatorMCPStopReason)
		if err != nil || !changed {
			t.Fatalf("cancel waiting session: changed=%v err=%v", changed, err)
		}
		guard.Unlock()
		release()
		coordinatorStopAwaitSignal(t, done, "guarded step-completion subscriber")

		updated, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if updated.WorkflowStepID != "step1" {
			t.Fatalf("stale signal advanced workflow after cancellation: %q", updated.WorkflowStepID)
		}
		updatedSession, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if updatedSession.State != models.TaskSessionStateCancelled {
			t.Fatalf("expected cancelled session, got %q", updatedSession.State)
		}
		if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); !hasSignal {
			t.Fatal("stop-winning subscriber consumed the queued completion signal")
		}
	})
}

// newGatedStepFailureService creates the workflow used by the turn-failure
// signal tests below. The first step advances only when a matching signal is
// present.
func newGatedStepFailureService(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seedSession leaves the session RUNNING.

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	return createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr), repo
}

func seedPendingStepCompletionSignal(t *testing.T, repo *sqliterepo.Repository, stepID, summary string) {
	t.Helper()
	signal := models.PendingStepCompletionSignal{
		StepID:     stepID,
		Source:     models.StepCompletionSourceAgent,
		Summary:    summary,
		SignaledAt: time.Now().UTC(),
	}
	if err := repo.SetSessionMetadataKey(context.Background(), "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
}

func stepCompletionSignalEvent(taskID, sessionID, stepID string) *bus.Event {
	return bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"step_id":    stepID,
	})
}

// TestStepCompletionSignalSurvivesTurnFailure is the regression test for the
// dropped-signal bug. A signal written during a running turn must be applied
// when that turn fails and settles the session into WAITING_FOR_INPUT.
func TestStepCompletionSignalSurvivesTurnFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGatedStepFailureService(t)
	seedPendingStepCompletionSignal(t, repo, "step1", "all done")

	// The subscriber sees a running session and correctly defers to the
	// turn-end path.
	svc.onStepCompletionSignaled(ctx, stepCompletionSignalEvent("t1", "s1", "step1"))
	if session, err := repo.GetTaskSession(ctx, "s1"); err != nil || session.State != models.TaskSessionStateRunning {
		t.Fatalf("expected session to remain RUNNING after subscriber no-op, got %+v (err=%v)", session, err)
	}
	if task, err := repo.GetTask(ctx, "t1"); err != nil || task.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition yet, got %+v (err=%v)", task, err)
	}

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session WAITING_FOR_INPUT after failure, got %q", session.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(session.Metadata); hasSignal {
		t.Error("expected the pending signal to be consumed by the transition")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("expected task to advance to step2 after the failed turn, got %q", task.WorkflowStepID)
	}
}

func TestStaleStepCompletionSignalDoesNotTransitionAfterTurnFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGatedStepFailureService(t)
	seedPendingStepCompletionSignal(t, repo, "step_old", "stale")

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session WAITING_FOR_INPUT after failure, got %q", session.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(session.Metadata); hasSignal {
		t.Error("expected the stale signal to be cleared by the reconciler")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("expected task to stay on step1 (stale signal must not transition), got %q", task.WorkflowStepID)
	}
}

// TestOfficeStepCompletionSignalDoesNotAdvanceAfterTurnFailure covers the
// Office-session exclusion. Office failures are terminal for the session and
// must not advance the workflow from a matching pending signal.
func TestOfficeStepCompletionSignalDoesNotAdvanceAfterTurnFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGatedStepFailureService(t)
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.ProjectID = "office-project"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("mark task as Office-owned: %v", err)
	}
	seedPendingStepCompletionSignal(t, repo, "step1", "all done")

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateFailed {
		t.Fatalf("expected Office session FAILED after failure, got %q", session.State)
	}

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("expected Office task to stay on step1, got %q", task.WorkflowStepID)
	}
}
