package ledger

import "testing"

func TestVerdict(t *testing.T) {
	wakeQuestion := []string{"question"}
	cases := []struct {
		name string
		in   VerdictInput
		want string
	}{
		{"nothing", VerdictInput{}, VerdictNothingNeeded},
		{"no calls no proposal is not blocked", VerdictInput{Outcome: "completed", Calls: 0}, VerdictNothingNeeded},
		{"failed outcome blocks", VerdictInput{Outcome: "failed"}, VerdictBlocked},
		{"cancelled blocks", VerdictInput{Outcome: "cancelled"}, VerdictBlocked},
		{"interrupted blocks", VerdictInput{Outcome: "interrupted"}, VerdictBlocked},
		{"ceiling blocks", VerdictInput{Outcome: "stopped_at_ceiling"}, VerdictBlocked},
		{"pause blocks", VerdictInput{Outcome: "stopped_by_pause"}, VerdictBlocked},
		{"unknown outcome does not block", VerdictInput{Outcome: "unknown"}, VerdictNothingNeeded},
		{"all calls refused blocks", VerdictInput{Calls: 2, Refused: 2}, VerdictBlocked},
		{"some allowed does not block", VerdictInput{Calls: 2, Refused: 1}, VerdictNothingNeeded},
		{"truncated never blocks by refusals", VerdictInput{Calls: 2, Refused: 2, CallsTruncated: true}, VerdictNothingNeeded},
		{"retry list never blocks by refusals", VerdictInput{Calls: 2, Refused: 2, RetryPending: true}, VerdictNothingNeeded},
		{"refused with proposal is proposed", VerdictInput{Calls: 2, Refused: 2, Proposals: 1}, VerdictProposed},
		{"acted beats proposed", VerdictInput{Proposals: 1, Acted: true}, VerdictActed},
		{"blocked beats acted", VerdictInput{Outcome: "failed", Acted: true, Proposals: 1}, VerdictBlocked},
		{"blocked beats proposed", VerdictInput{Outcome: "cancelled", Proposals: 1}, VerdictBlocked},
		{"blocked beats needs_you", VerdictInput{Outcome: "failed", Trigger: "wake", WakeKinds: wakeQuestion, ActionPending: true}, VerdictBlocked},
		{"acted beats needs_you", VerdictInput{Acted: true, Trigger: "wake", WakeKinds: wakeQuestion, ActionPending: true}, VerdictActed},
		{"proposed beats needs_you", VerdictInput{Proposals: 1, Trigger: "wake", WakeKinds: wakeQuestion, ActionPending: true}, VerdictProposed},
		{"needs_you question", VerdictInput{Trigger: "wake", WakeKinds: wakeQuestion, ActionPending: true}, VerdictNeedsYou},
		{"needs_you permission", VerdictInput{Trigger: "wake", WakeKinds: []string{"stall", "permission"}, ActionPending: true}, VerdictNeedsYou},
		{"needs_you needs pending action", VerdictInput{Trigger: "wake", WakeKinds: wakeQuestion}, VerdictNothingNeeded},
		{"needs_you needs question or permission", VerdictInput{Trigger: "wake", WakeKinds: []string{"stall"}, ActionPending: true}, VerdictNothingNeeded},
		{"needs_you needs wake trigger", VerdictInput{Trigger: "message", WakeKinds: wakeQuestion, ActionPending: true}, VerdictNothingNeeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Verdict(tc.in); got != tc.want {
				t.Fatalf("Verdict(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
