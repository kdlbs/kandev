package shared_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueOutcomeMatchesRunsService verifies that the runs service exposes
// the shared queue contract, including every public outcome value.
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
		{"RateLimited", shared.QueueOutcomeRateLimited, runsservice.QueueOutcomeRateLimited},
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
