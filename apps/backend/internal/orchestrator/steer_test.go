package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type blockingQueueRepository struct {
	messagequeue.Repository
	insertStarted chan struct{}
	releaseInsert chan struct{}
	insertOnce    sync.Once
}

func (r *blockingQueueRepository) Insert(ctx context.Context, msg *messagequeue.QueuedMessage, maxPerSession int) error {
	r.insertOnce.Do(func() {
		close(r.insertStarted)
		<-r.releaseInsert
	})
	return r.Repository.Insert(ctx, msg, maxPerSession)
}

// TestSteerEligible_GatedByFlagCapabilityAndState covers every condition the
// steer gate depends on. The matrix is the observable contract behind the
// composer's steer-vs-queue affordance.
func TestSteerEligible_GatedByFlagCapabilityAndState(t *testing.T) {
	const sessionID = "session-steer-eligible"

	setGenerating := func(svc *Service) {
		// A claimed foreground turn with no background work reads as generating.
		svc.registerBackgroundTask(sessionID, "bg-1")
		svc.markForegroundIdle(sessionID)
		svc.completeBackgroundTask(sessionID, "bg-1")
	}

	t.Run("flag off is never eligible", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		advertisePromptQueueingForTest(t, svc, sessionID)
		if svc.SteerEligible(sessionID, models.TaskSessionStateRunning) {
			t.Fatal("steer eligible with the flag off")
		}
	})

	t.Run("no advertisement is never eligible", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		svc.config.ClaudeMidTurnSteering = true
		if svc.SteerEligible(sessionID, models.TaskSessionStateRunning) {
			t.Fatal("steer eligible without the negotiated advertisement")
		}
	})

	t.Run("non-running state is never eligible", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		svc.config.ClaudeMidTurnSteering = true
		advertisePromptQueueingForTest(t, svc, sessionID)
		for _, state := range []models.TaskSessionState{
			models.TaskSessionStateWaitingForInput,
			models.TaskSessionStateCompleted,
			models.TaskSessionStateIdle,
			models.TaskSessionStateCreated,
		} {
			if svc.SteerEligible(sessionID, state) {
				t.Fatalf("steer eligible in %s state", state)
			}
		}
	})

	t.Run("background-idle is not steered (handoff path owns it)", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		svc.config.ClaudeMidTurnSteering = true
		svc.config.ClaudeBackgroundPromptHandoff = true
		advertisePromptQueueingForTest(t, svc, sessionID)
		svc.registerBackgroundTask(sessionID, "bg-1")
		svc.markForegroundIdle(sessionID)
		if svc.ForegroundActivity(sessionID) != v1.ForegroundActivityBackground {
			t.Fatal("precondition: expected background activity")
		}
		if svc.SteerEligible(sessionID, models.TaskSessionStateRunning) {
			t.Fatal("background-idle session should be admitted via handoff, not steer")
		}
	})

	t.Run("background-idle is not steered even with handoff off (flags independent)", func(t *testing.T) {
		// Regression: SteerEligible must read the raw foreground activity, not the
		// public ForegroundActivity that collapses to Generating whenever
		// ClaudeBackgroundPromptHandoff is off. With steering on but handoff off, a
		// background-idle session would otherwise look generating and be wrongly
		// steer-eligible — the two flags are independent.
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		svc.config.ClaudeMidTurnSteering = true
		svc.config.ClaudeBackgroundPromptHandoff = false
		advertisePromptQueueingForTest(t, svc, sessionID)
		svc.registerBackgroundTask(sessionID, "bg-1")
		svc.markForegroundIdle(sessionID)
		if svc.foregroundActivityValue(sessionID) != v1.ForegroundActivityBackground {
			t.Fatal("precondition: raw activity should be background")
		}
		if svc.SteerEligible(sessionID, models.TaskSessionStateRunning) {
			t.Fatal("background-idle session is steer-eligible with handoff off")
		}
	})

	t.Run("generating running with capability and flag is eligible", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		svc.config.ClaudeMidTurnSteering = true
		advertisePromptQueueingForTest(t, svc, sessionID)
		setGenerating(svc)
		if !svc.SteerEligible(sessionID, models.TaskSessionStateRunning) {
			t.Fatal("generating, advertised, flag-on session should be steer-eligible")
		}
	})
}

// TestSteerTask_OrderRuleQueuesBehindPending pins that a steer never jumps ahead
// of an already-queued message: it enqueues behind the pending one (preserving
// order) rather than dispatching a steer or erroring.
func TestSteerTask_OrderRuleQueuesBehindPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.config.ClaudeMidTurnSteering = true
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus

	const taskID = "task-order"
	const sessionID = "session-order"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	advertisePromptQueueingForTest(t, svc, sessionID)
	svc.registerBackgroundTask(sessionID, "bg-1")
	svc.markForegroundIdle(sessionID)
	svc.completeBackgroundTask(sessionID, "bg-1")

	if _, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, "queued first", "", "", false, nil); err != nil {
		t.Fatalf("seed queued message: %v", err)
	}

	result, err := svc.SteerTask(ctx, taskID, sessionID, "steer second", "", false, nil)
	if err != nil {
		t.Fatalf("SteerTask with a queued message errored: %v", err)
	}
	if result == nil || result.StopReason != steerQueuedStopReason {
		t.Fatalf("SteerTask result = %+v, want a queued steer (%q)", result, steerQueuedStopReason)
	}
	// The steer must have joined the queue behind the pending message, in order.
	status := svc.messageQueue.GetStatus(ctx, sessionID)
	if status == nil || status.Count != 2 {
		t.Fatalf("queue count = %v, want 2 (pending + enqueued steer)", status)
	}
	if status.Entries[0].Content != "queued first" || status.Entries[1].Content != "steer second" {
		t.Fatalf("queue order = [%q, %q], want [queued first, steer second]",
			status.Entries[0].Content, status.Entries[1].Content)
	}
	if len(eventBus.events) != 1 {
		t.Fatalf("queue status event count = %d, want 1", len(eventBus.events))
	}
	if eventBus.events[0].subject != events.MessageQueueStatusChanged {
		t.Fatalf("queue status event subject = %q, want %q", eventBus.events[0].subject, events.MessageQueueStatusChanged)
	}
	eventData, ok := eventBus.events[0].event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("queue status event data = %T, want map[string]interface{}", eventBus.events[0].event.Data)
	}
	if eventData["count"] != 2 {
		t.Fatalf("queue status event count = %v, want 2", eventData["count"])
	}
	if _, inFlight := svc.steerInFlight.Load(sessionID); inFlight {
		t.Fatal("declined steer left an in-flight slot claimed")
	}
}

// TestSteerTask_DoesNotOvertakeQueueWriter pins the cross-entry-point ordering
// boundary: an ordinary queue writer that starts first must complete before a
// steer can decide that the queue is empty and dispatch.
func TestSteerTask_DoesNotOvertakeQueueWriter(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-order-race"
	const sessionID = "session-order-race"

	svc, agentMgr := steerDispatchService(t, taskID, sessionID)
	queueRepo := &blockingQueueRepository{
		Repository:    messagequeue.NewMemoryRepository(),
		insertStarted: make(chan struct{}),
		releaseInsert: make(chan struct{}),
	}
	svc.messageQueue = messagequeue.NewService(queueRepo, 10, testLogger())

	ordinaryDone := make(chan error, 1)
	go func() {
		_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, "ordinary first", "", "user", false, nil)
		ordinaryDone <- err
	}()
	<-queueRepo.insertStarted

	steerDone := make(chan struct {
		result *PromptResult
		err    error
	}, 1)
	go func() {
		result, err := svc.SteerTask(ctx, taskID, sessionID, "steer second", "", false, nil)
		steerDone <- struct {
			result *PromptResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case result := <-steerDone:
		close(queueRepo.releaseInsert)
		t.Fatalf("steer completed before the earlier queue write: result=%+v err=%v", result.result, result.err)
	case <-time.After(100 * time.Millisecond):
	}

	close(queueRepo.releaseInsert)
	if err := <-ordinaryDone; err != nil {
		t.Fatalf("ordinary queue write: %v", err)
	}
	result := <-steerDone
	if result.err != nil {
		t.Fatalf("steer: %v", result.err)
	}
	if result.result == nil || result.result.StopReason != steerQueuedStopReason {
		t.Fatalf("steer result = %+v, want queued behind ordinary message", result.result)
	}
	if got := len(agentMgr.getCapturedSteerCalls()); got != 0 {
		t.Fatalf("steer dispatch count = %d, want 0", got)
	}
	status := svc.messageQueue.GetStatus(ctx, sessionID)
	if status == nil || status.Count != 2 || status.Entries[0].Content != "ordinary first" || status.Entries[1].Content != "steer second" {
		t.Fatalf("queue after race = %+v, want ordinary first then steer second", status)
	}
}

// steerDispatchService builds a scheduler-backed service (real executor over a
// fake agent manager) with a generating, steer-eligible session so SteerTask
// reaches actual dispatch.
func steerDispatchService(t *testing.T, taskID, sessionID string) (*Service, *mockAgentManager) {
	t.Helper()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	agentMgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		return "exec-steer", nil
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.config.ClaudeMidTurnSteering = true
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	advertisePromptQueueingForTest(t, svc, sessionID)
	svc.registerBackgroundTask(sessionID, "bg-1")
	svc.markForegroundIdle(sessionID)
	svc.completeBackgroundTask(sessionID, "bg-1") // generating
	return svc, agentMgr
}

// TestSteerTask_DispatchesAndReleasesInFlightSlot drives SteerTask all the way to
// dispatch (the branch no other test reaches) and pins the contract that keeps
// the feature working turn-after-turn: the steer dispatches with dispatchOnly,
// carrying the operator's prompt, and — critically — releases the single
// in-flight slot afterwards so the next steer dispatches rather than being
// permanently declined-and-enqueued. Deleting `defer s.steerInFlight.Delete` in
// SteerTask must fail this test.
func TestSteerTask_DispatchesAndReleasesInFlightSlot(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dispatch"
	const sessionID = "session-dispatch"

	t.Run("dispatches then releases the slot for the next steer", func(t *testing.T) {
		svc, agentMgr := steerDispatchService(t, taskID, sessionID)

		res, err := svc.SteerTask(ctx, taskID, sessionID, "steer one", "", false, nil)
		if err != nil {
			t.Fatalf("first steer dispatch errored: %v", err)
		}
		if res == nil || res.StopReason == steerQueuedStopReason {
			t.Fatalf("first steer result = %+v, want a real dispatch (not enqueued)", res)
		}
		calls := agentMgr.getCapturedSteerCalls()
		if len(calls) != 1 {
			t.Fatalf("steer dispatch count = %d, want 1", len(calls))
		}
		if calls[0].ExecutionID != "exec-steer" {
			t.Fatalf("steer execution = %q, want exec-steer", calls[0].ExecutionID)
		}
		if !calls[0].DispatchOnly {
			t.Fatal("steer must dispatch with dispatchOnly=true (dispatch-and-continue)")
		}
		if !strings.Contains(calls[0].Prompt, "steer one") {
			t.Fatalf("steer prompt %q does not carry the operator's message", calls[0].Prompt)
		}
		if _, inFlight := svc.steerInFlight.Load(sessionID); inFlight {
			t.Fatal("in-flight slot was not released after dispatch")
		}

		// A second steer must dispatch too — proving the slot really released.
		res2, err := svc.SteerTask(ctx, taskID, sessionID, "steer two", "", false, nil)
		if err != nil {
			t.Fatalf("second steer errored: %v", err)
		}
		if res2 == nil || res2.StopReason == steerQueuedStopReason {
			t.Fatalf("second steer result = %+v, want dispatch — slot not released", res2)
		}
		if got := len(agentMgr.getCapturedSteerCalls()); got != 2 {
			t.Fatalf("steer dispatch count = %d, want 2", got)
		}
	})

	t.Run("releases the slot when dispatch fails", func(t *testing.T) {
		svc, agentMgr := steerDispatchService(t, taskID, sessionID)
		agentMgr.steerErr = errors.New("agent dispatch boom")

		if _, err := svc.SteerTask(ctx, taskID, sessionID, "steer", "", false, nil); err == nil {
			t.Fatal("expected the dispatch error to propagate")
		}
		if _, inFlight := svc.steerInFlight.Load(sessionID); inFlight {
			t.Fatal("in-flight slot was not released after a dispatch error")
		}
	})
}

// TestSteerTask_NotEligibleReturnsSentinel pins that an ineligible session yields
// the fall-back sentinel rather than dispatching.
func TestSteerTask_NotEligibleReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	// Flag deliberately off.
	const taskID = "task-noteligible"
	const sessionID = "session-noteligible"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	advertisePromptQueueingForTest(t, svc, sessionID)

	_, err := svc.SteerTask(ctx, taskID, sessionID, "steer", "", false, nil)
	if !errors.Is(err, ErrSteerNotEligible) {
		t.Fatalf("SteerTask with flag off = %v, want ErrSteerNotEligible", err)
	}
}

// TestSteerTask_SingleInFlight pins the one-steer-per-session rule: while a steer
// holds the in-flight slot, a second attempt is enqueued (not dispatched) and
// does not disturb the prior steer's in-flight claim.
func TestSteerTask_SingleInFlight(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.config.ClaudeMidTurnSteering = true

	const taskID = "task-inflight"
	const sessionID = "session-inflight"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	advertisePromptQueueingForTest(t, svc, sessionID)
	svc.registerBackgroundTask(sessionID, "bg-1")
	svc.markForegroundIdle(sessionID)
	svc.completeBackgroundTask(sessionID, "bg-1")

	// Occupy the in-flight slot as a prior steer dispatch would.
	svc.steerInFlight.Store(sessionID, struct{}{})

	result, err := svc.SteerTask(ctx, taskID, sessionID, "steer", "", false, nil)
	if err != nil {
		t.Fatalf("second concurrent steer errored: %v", err)
	}
	if result == nil || result.StopReason != steerQueuedStopReason {
		t.Fatalf("second steer result = %+v, want a queued steer (%q)", result, steerQueuedStopReason)
	}
	// The prior steer's in-flight claim must survive the decline.
	if _, inFlight := svc.steerInFlight.Load(sessionID); !inFlight {
		t.Fatal("declining a second steer cleared the prior steer's in-flight slot")
	}
	if status := svc.messageQueue.GetStatus(ctx, sessionID); status == nil || status.Count != 1 {
		t.Fatalf("queue count = %v, want 1 (enqueued second steer)", status)
	}
}

// TestDrainPaths_DeferToInFlightSteer pins the fix for the ordering gap
// SteerTask's own comment documents: WithSessionAdmission's per-session lock
// is released before the blocking dispatch, so a message can be queued — and
// the underlying turn can complete — in the window between a steer's
// admission and its actual dispatch. Without this guard, an ordinary drain
// reacting to that completion would dispatch the later-queued message ahead
// of the steer that was admitted first, reopening the exact ordering gap the
// shared lock was meant to close. Both non-cancelling drain paths —
// drainQueuedMessageForPromptableSessionLocked (and, transitively,
// DrainQueuedMessage, which delegates to it) and takeIfPromptableLocked —
// must defer to an in-flight steer the same way they already defer to
// isQueuedDispatchInFlight.
func TestDrainPaths_DeferToInFlightSteer(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const taskID = "task-drain-race"
	const sessionID = "session-drain-race"
	// Idle/promptable: the underlying turn has already completed, exactly the
	// state that would let an ordinary drain fire — while the steer admitted
	// against that same turn is still working through its own dispatch.
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateIdle)

	if _, err := svc.messageQueue.QueueMessage(
		ctx, sessionID, taskID, "queued after admission", "", "user", false, nil,
	); err != nil {
		t.Fatalf("seed queued message: %v", err)
	}
	entryID := svc.messageQueue.GetStatus(ctx, sessionID).Entries[0].ID

	// Simulate the race window directly, the same way TestSteerTask_SingleInFlight
	// does: a steer has claimed the in-flight slot but not yet dispatched.
	svc.steerInFlight.Store(sessionID, struct{}{})

	t.Run("drainQueuedMessageForPromptableSessionLocked backs off", func(t *testing.T) {
		if dispatched := svc.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID); dispatched {
			t.Fatal("drain dispatched a queued message while a steer was in flight")
		}
		if status := svc.messageQueue.GetStatus(ctx, sessionID); status == nil || status.Count != 1 {
			t.Fatalf("queue count = %v, want 1 (message must remain queued)", status)
		}
	})

	t.Run("takeIfPromptableLocked backs off", func(t *testing.T) {
		dispatched, err := svc.takeIfPromptableLocked(ctx, taskID, sessionID, entryID)
		if err != nil {
			t.Fatalf("takeIfPromptableLocked: %v", err)
		}
		if dispatched {
			t.Fatal("takeIfPromptableLocked dispatched while a steer was in flight")
		}
		if status := svc.messageQueue.GetStatus(ctx, sessionID); status == nil || status.Count != 1 {
			t.Fatalf("queue count = %v, want 1 (message must remain queued)", status)
		}
	})
}
