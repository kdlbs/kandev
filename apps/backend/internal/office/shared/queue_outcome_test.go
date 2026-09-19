package shared_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueOutcomeMatchesRunsService pins the "both MUST match" invariant
// shared.QueueOutcome's doc comment carries: office/shared and
// runs/service declare the same string type and constants independently
// (to avoid an office/shared <-> runs/service import cycle), and a change
// to one without the other would silently break callers comparing an
// outcome received from one package against a constant imported from the
// other.
func TestQueueOutcomeMatchesRunsService(t *testing.T) {
	cases := []struct {
		name   string
		shared shared.QueueOutcome
		runs   runsservice.QueueOutcome
	}{
		{"Queued", shared.QueueOutcomeQueued, runsservice.QueueOutcomeQueued},
		{"Deduped", shared.QueueOutcomeDeduped, runsservice.QueueOutcomeDeduped},
		{"Coalesced", shared.QueueOutcomeCoalesced, runsservice.QueueOutcomeCoalesced},
		{"None", shared.QueueOutcomeNone, runsservice.QueueOutcomeNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.shared) != string(tc.runs) {
				t.Fatalf("shared.QueueOutcome%s = %q, runsservice.QueueOutcome%s = %q; both MUST match",
					tc.name, tc.shared, tc.name, tc.runs)
			}
		})
	}
}
