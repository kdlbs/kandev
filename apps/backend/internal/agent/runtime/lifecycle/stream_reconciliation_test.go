package lifecycle

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestDisconnectReconcilesBeforeTerminalOutcome(t *testing.T) {
	now := time.Now().UTC()
	decision := DecideSubmissionReconciliation(ReconciliationEvidence{
		State:            SubmissionReconciling,
		OwnerReachable:   true,
		JournalAvailable: true,
		Now:              now,
		Deadline:         now.Add(time.Minute),
	})
	if decision.State != SubmissionReconciling || !decision.QueueBlocked || !decision.CanRetryStateQuery {
		t.Fatalf("reconciliation decision = %#v", decision)
	}
	decision = DecideSubmissionReconciliation(ReconciliationEvidence{
		TerminalObserved: true, TerminalOutcome: "completed", JournalAvailable: true,
	})
	if decision.State != SubmissionRecovered || decision.QueueBlocked || decision.Outcome != "completed" {
		t.Fatalf("terminal decision = %#v", decision)
	}
}

func TestUncertainSubmissionBlocksQueueAndKeepsStop(t *testing.T) {
	now := time.Now().UTC()
	decision := DecideSubmissionReconciliation(ReconciliationEvidence{
		JournalAvailable: true,
		Now:              now,
		Deadline:         now.Add(-time.Second),
	})
	if decision.State != SubmissionUncertain || !decision.QueueBlocked || !decision.StopAvailable || decision.CanRetryStateQuery {
		t.Fatalf("uncertain decision = %#v", decision)
	}
	decision = DecideSubmissionReconciliation(ReconciliationEvidence{StopRequested: true})
	if decision.State != SubmissionStopped || !decision.StopAvailable {
		t.Fatalf("stop decision = %#v", decision)
	}
}

func TestDisconnectMarksAcceptedPromptOutcomeUncertain(t *testing.T) {
	execution := &AgentExecution{
		SessionID:                "session-1",
		promptDoneCh:             make(chan PromptCompletionSignal, 1),
		promptGeneration:         4,
		startupAttemptGeneration: 2,
		deliverySubmissionID:     "prompt:1",
	}
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	streamManager.handleUpdatesDisconnectWithGeneration(execution, errors.New("connection reset"), 2)

	select {
	case signal := <-execution.promptDoneCh:
		if !signal.Uncertain || !signal.IsError {
			t.Fatalf("disconnect signal = %#v, want uncertain error", signal)
		}
		wrapped := fmt.Errorf("%w: %s", ErrUncertainPromptDelivery, signal.Error)
		if !errors.Is(wrapped, ErrUncertainPromptDelivery) {
			t.Fatal("uncertain delivery sentinel was not preserved")
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect did not signal prompt completion")
	}
}
