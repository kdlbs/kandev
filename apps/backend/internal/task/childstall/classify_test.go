package childstall

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const classifyTestTransitionID = int64(7)

func qualifiedEvidence() Evidence {
	return Evidence{
		Start:              models.ChildStallStart{ParentTaskID: "parent", WorkspaceID: "ws", TransitionID: classifyTestTransitionID},
		Settlement:         "",
		Child:              &ChildSnapshot{ParentID: "parent", WorkspaceID: "ws", State: v1.TaskStateInProgress, LatestTransitionID: classifyTestTransitionID},
		SessionState:       models.TaskSessionStateWaitingForInput,
		StepRequiresSignal: true,
		SettledFor:         time.Minute,
	}
}

func TestClassifyRoutesEachCause(t *testing.T) {
	policy := Policy{SettleGrace: time.Second, SignalWait: time.Minute}
	cases := []struct {
		name        string
		mutate      func(*Evidence)
		wantOutcome Outcome
		wantCause   Cause
		wantReason  string
	}{
		{
			name:        "missing completion signal",
			mutate:      func(*Evidence) {},
			wantOutcome: OutcomeQualified,
			wantCause:   CauseMissingCompletionSignal,
		},
		{
			name:        "input required",
			mutate:      func(e *Evidence) { e.PendingInputs = []PendingInput{{MessageID: "m1"}} },
			wantOutcome: OutcomeQualified,
			wantCause:   CauseInputRequired,
		},
		{
			name: "quota",
			mutate: func(e *Evidence) {
				e.SessionState = models.TaskSessionStateFailed
				e.SessionErrorClass = QuotaErrorClass
			},
			wantOutcome: OutcomeQualified,
			wantCause:   CauseQuota,
		},
		{
			name:        "execution error",
			mutate:      func(e *Evidence) { e.SessionState = models.TaskSessionStateFailed },
			wantOutcome: OutcomeQualified,
			wantCause:   CauseExecutionError,
		},
		{
			name:        "optional signal is ordinary",
			mutate:      func(e *Evidence) { e.StepRequiresSignal = false },
			wantOutcome: OutcomeSuppressed,
			wantReason:  ReasonOrdinary,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := qualifiedEvidence()
			tc.mutate(&evidence)
			got := Classify(evidence, policy)
			if got.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q", got.Outcome, tc.wantOutcome)
			}
			if tc.wantCause != "" && got.Cause != tc.wantCause {
				t.Fatalf("cause = %q, want %q", got.Cause, tc.wantCause)
			}
			if tc.wantReason != "" && got.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}

// TestClassifySuppressesAProcessedSignal pins the behaviour the child-stall
// reminder relies on: a step whose completion signal is already recorded must
// never classify as a missing signal, regardless of which session recorded it.
func TestClassifySuppressesAProcessedSignal(t *testing.T) {
	policy := Policy{SettleGrace: time.Second, SignalWait: time.Minute}

	settling := qualifiedEvidence()
	settling.PendingStepSignal = true
	settling.SettledFor = 30 * time.Second
	if got := Classify(settling, policy); got.Outcome != OutcomeSettling {
		t.Fatalf("a freshly processed signal must stay settling, got %+v", got)
	}

	settled := qualifiedEvidence()
	settled.PendingStepSignal = true
	settled.SettledFor = 2 * time.Minute
	got := Classify(settled, policy)
	if got.Outcome != OutcomeSuppressed || got.Reason != ReasonSignalUnprocessed {
		t.Fatalf("a processed signal must be suppressed, got %+v", got)
	}
}

// TestClassifyCarriesBlockedReason pins that a refusal reason gathered from the
// task's sessions reaches the missing-signal decision so the reminder can name
// the real block.
func TestClassifyCarriesBlockedReason(t *testing.T) {
	policy := Policy{SettleGrace: time.Second, SignalWait: time.Minute}
	evidence := qualifiedEvidence()
	evidence.BlockedReason = "This session runs agent profile a, but the current step uses b."

	got := Classify(evidence, policy)
	if got.Outcome != OutcomeQualified || got.Cause != CauseMissingCompletionSignal {
		t.Fatalf("decision = %+v, want a qualified missing signal", got)
	}
	if got.BlockedReason != evidence.BlockedReason {
		t.Fatalf("blocked reason = %q, want %q", got.BlockedReason, evidence.BlockedReason)
	}
}
