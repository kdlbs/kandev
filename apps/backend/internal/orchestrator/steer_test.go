package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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
// of an already-queued message.
func TestSteerTask_OrderRuleQueuesBehindPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.config.ClaudeMidTurnSteering = true

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

	_, err := svc.SteerTask(ctx, taskID, sessionID, "steer", "", false, nil)
	if !errors.Is(err, ErrSteerWouldReorder) {
		t.Fatalf("SteerTask with a queued message = %v, want ErrSteerWouldReorder", err)
	}
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
// holds the in-flight slot, a second attempt is rejected with ErrSteerInFlight.
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

	_, err := svc.SteerTask(ctx, taskID, sessionID, "steer", "", false, nil)
	if !errors.Is(err, ErrSteerInFlight) {
		t.Fatalf("second concurrent steer = %v, want ErrSteerInFlight", err)
	}
}
