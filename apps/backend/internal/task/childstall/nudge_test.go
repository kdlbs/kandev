package childstall

import "testing"

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
			name:         "an escalated streak resets after the entry advances",
			prev:         NudgeStreak{Cause: cause, TransitionID: entry, Count: 3, Escalated: true},
			cause:        cause,
			transitionID: entry + 1,
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
