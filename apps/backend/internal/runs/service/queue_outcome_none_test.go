package service_test

import (
	"testing"

	"github.com/kandev/kandev/internal/workflow/engine"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueOutcomeNone_MatchesEngineDeclaration pins the "both MUST match"
// invariant both QueueOutcomeNone doc comments carry: runs/service and
// workflow/engine declare the same string type and zero-value constant
// independently, and a change to one without the other would silently break
// callers comparing an outcome received from one package against a constant
// imported from the other.
func TestQueueOutcomeNone_MatchesEngineDeclaration(t *testing.T) {
	if string(runsservice.QueueOutcomeNone) != string(engine.QueueOutcomeNone) {
		t.Fatalf("runsservice.QueueOutcomeNone = %q, engine.QueueOutcomeNone = %q; both MUST match",
			runsservice.QueueOutcomeNone, engine.QueueOutcomeNone)
	}
}
