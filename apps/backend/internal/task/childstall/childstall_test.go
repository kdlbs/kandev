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
		{
			name: "cancellation wins over a pending input",
			mutate: func(e *Evidence) {
				e.Settlement = models.ChildStallSettlementCancelled
				e.PendingInputs = []PendingInput{{MessageID: "m1"}, {MessageID: "q1", ParentQuestionID: "q1"}}
			},
			wantOutcome: OutcomeSuppressed,
			wantReason:  ReasonCancelled,
		},
		{
			name:        "a later settled turn supersedes the settlement",
			mutate:      func(e *Evidence) { e.Superseded = true },
			wantOutcome: OutcomeSuppressed,
			wantReason:  ReasonSuperseded,
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

func TestAdvanceNudgeRoutesConsecutiveMissingSignalStalls(t *testing.T) {
	policy := NudgePolicy{EscalateAfter: DefaultNudgeEscalateAfter}
	cause := CauseMissingCompletionSignal
	const entry = int64(5)

	cases := []struct {
		name          string
		prev          NudgeStreak
		cause         Cause
		transitionID  int64
		policy        NudgePolicy
		wantCount     int
		wantEscalated bool
		wantAction    NudgeAction
	}{
		{
			name:         "first stall nudges the child",
			cause:        cause,
			transitionID: entry,
			policy:       policy,
			wantCount:    1,
			wantAction:   NudgeActionNudgeChild,
		},
		{
			name:         "second stall on the same entry nudges again",
			prev:         NudgeStreak{Cause: cause, TransitionID: entry, Count: 1},
			cause:        cause,
			transitionID: entry,
			policy:       policy,
			wantCount:    2,
			wantAction:   NudgeActionNudgeChild,
		},
		{
			name:          "third stall on the same entry alerts the parent once",
			prev:          NudgeStreak{Cause: cause, TransitionID: entry, Count: 2},
			cause:         cause,
			transitionID:  entry,
			policy:        policy,
			wantCount:     3,
			wantEscalated: true,
			wantAction:    NudgeActionAlertParent,
		},
		{
			name:          "further stall on an escalated streak is suppressed",
			prev:          NudgeStreak{Cause: cause, TransitionID: entry, Count: 3, Escalated: true},
			cause:         cause,
			transitionID:  entry,
			policy:        policy,
			wantCount:     3,
			wantEscalated: true,
			wantAction:    NudgeActionSuppress,
		},
		{
			name:         "advanced workflow entry starts a new streak",
			prev:         NudgeStreak{Cause: cause, TransitionID: entry, Count: 2},
			cause:        cause,
			transitionID: entry + 1,
			policy:       policy,
			wantCount:    1,
			wantAction:   NudgeActionNudgeChild,
		},
		{
			name:         "changed cause starts a new streak",
			prev:         NudgeStreak{Cause: cause, TransitionID: entry, Count: 2},
			cause:        CauseInputRequired,
			transitionID: entry,
			policy:       policy,
			wantCount:    1,
			wantAction:   NudgeActionNudgeChild,
		},
		{
			name:         "a disabled threshold never escalates",
			prev:         NudgeStreak{Cause: cause, TransitionID: entry, Count: 5},
			cause:        cause,
			transitionID: entry,
			policy:       NudgePolicy{EscalateAfter: 0},
			wantCount:    6,
			wantAction:   NudgeActionNudgeChild,
		},
		{
			name:          "threshold of one escalates the first stall",
			cause:         cause,
			transitionID:  entry,
			policy:        NudgePolicy{EscalateAfter: 1},
			wantCount:     1,
			wantEscalated: true,
			wantAction:    NudgeActionAlertParent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, action := AdvanceNudge(tc.prev, tc.cause, tc.transitionID, tc.policy)
			if action != tc.wantAction {
				t.Fatalf("action = %q, want %q", action, tc.wantAction)
			}
			if got.Count != tc.wantCount {
				t.Fatalf("count = %d, want %d", got.Count, tc.wantCount)
			}
			if got.Escalated != tc.wantEscalated {
				t.Fatalf("escalated = %v, want %v", got.Escalated, tc.wantEscalated)
			}
			if got.Cause != tc.cause || got.TransitionID != tc.transitionID {
				t.Fatalf("streak key = (%q, %d), want (%q, %d)",
					got.Cause, got.TransitionID, tc.cause, tc.transitionID)
			}
		})
	}
}
