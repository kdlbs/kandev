package orchestrator

import (
	"errors"
	"testing"
)

func TestQueuedDispatchAwaitingInitialAdmissionSurvivesWorkerCleanup(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	reservation := svc.markQueuedDispatchInFlight("session-admission", "queued-entry")
	claimed, err := svc.claimQueuedDispatchForExecution("session-admission", "queued-entry", reservation)
	if err != nil || !claimed {
		t.Fatalf("claim queued dispatch: claimed=%v err=%v", claimed, err)
	}

	guard := svc.lockCancelInFlightGuard("session-admission")
	defer guard.release()
	if retained := svc.retainQueuedDispatchForInitialAdmissionLocked("session-admission", "queued-entry"); retained != reservation {
		t.Fatalf("retained reservation = %p, want %p", retained, reservation)
	}

	// The async model-switch worker returns before lifecycle reaches its initial
	// prompt. Its deferred cleanup must leave the queue ownership in place.
	svc.clearQueuedDispatchInFlightIfCurrent("session-admission", reservation)
	if got := svc.acceptedQueuedDispatchForSession("session-admission"); got != reservation {
		t.Fatalf("accepted reservation after worker cleanup = %p, want %p", got, reservation)
	}
	if got := reservation.currentPhase(); got != queuedDispatchAwaitingAdmission {
		t.Fatalf("reservation phase after worker cleanup = %v, want awaiting admission", got)
	}
	if !svc.isQueuedDispatchAccepted("session-admission") {
		t.Fatal("awaiting initial admission did not block a competing Send Now")
	}
	if _, err := svc.pendingQueuedDispatchForSendNow("session-admission"); !errors.Is(err, ErrSendNowConflict) {
		t.Fatalf("Send Now result while initial prompt awaits admission = %v, want conflict", err)
	}

	svc.acceptRetainedQueuedDispatchLocked("session-admission", reservation)
	if got := reservation.currentPhase(); got != queuedDispatchLive {
		t.Fatalf("reservation phase after provider acceptance = %v, want live", got)
	}
	if svc.isQueuedDispatchAccepted("session-admission") {
		t.Fatal("provider-accepted queued dispatch still reported an admission conflict")
	}
	if _, err := svc.pendingQueuedDispatchForSendNow("session-admission"); err != nil {
		t.Fatalf("live reservation blocked Send Now: %v", err)
	}
}
