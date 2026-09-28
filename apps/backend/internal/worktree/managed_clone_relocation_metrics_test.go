package worktree

import (
	"expvar"
	"testing"
)

func TestManagedCloneRelocationOutcomeUsesClosedSet(t *testing.T) {
	var before int64
	if failedBefore := managedCloneRelocationTotal.Get(managedCloneRelocationOutcomeFailed); failedBefore != nil {
		before = failedBefore.(*expvar.Int).Value()
	}

	if got := incManagedCloneRelocationOutcome("task-controlled-path"); got != managedCloneRelocationOutcomeFailed {
		t.Fatalf("unknown outcome was recorded as %q, want fixed failure reason", got)
	}
	if got := managedCloneRelocationTotal.Get("task-controlled-path"); got != nil {
		t.Fatalf("unknown reason created an unbounded metric series: %v", got)
	}
	if got := managedCloneRelocationTotal.Get(managedCloneRelocationOutcomeFailed).(*expvar.Int).Value(); got != before+1 {
		t.Fatalf("failed outcome count = %d, want %d", got, before+1)
	}
}
