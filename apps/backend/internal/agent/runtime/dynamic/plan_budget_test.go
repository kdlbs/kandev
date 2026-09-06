package dynamic

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/planinjection"
)

// TestDynamicPlanBudgetDoesNotExceedContinuationFieldLimit is the AC-002.4
// assertion: continuationFieldLimit is unexported, so this relationship can
// only be checked from inside this package. A later increase to
// planinjection.DynamicBudget above continuationFieldLimit would silently
// restore bounded()'s head-only truncation and cut the reducer's own notice
// off the end.
func TestDynamicPlanBudgetDoesNotExceedContinuationFieldLimit(t *testing.T) {
	if planinjection.DynamicBudget > continuationFieldLimit {
		t.Fatalf("planinjection.DynamicBudget = %d, want <= continuationFieldLimit = %d",
			planinjection.DynamicBudget, continuationFieldLimit)
	}
}

// TestBoundedIsNoOpOnReducedPlanOutput asserts the second condition AC-002.4
// requires: because bounded() unconditionally TrimSpaces before its length
// check, the reducer's output must carry no leading or trailing whitespace
// whenever its input carries none, or the call would silently re-trim
// already-bounded plan text.
func TestBoundedIsNoOpOnReducedPlanOutput(t *testing.T) {
	var doc strings.Builder
	for i := 1; i <= 300; i++ {
		doc.WriteString("## Section ")
		doc.WriteString(strings.Repeat("x", 20))
		doc.WriteString("\n")
		doc.WriteString(strings.Repeat("y", 60))
		doc.WriteString("\n")
	}

	out, reduced, _ := planinjection.Reduce(doc.String(), planinjection.DynamicBudget)
	if !reduced {
		t.Fatal("fixture did not actually reduce the plan; strengthen it")
	}

	if got := bounded(out); got != out {
		t.Fatalf("bounded(reducerOutput) = %q, want it unchanged (%q)", got, out)
	}
}
