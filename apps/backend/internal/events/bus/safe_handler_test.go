package bus

import (
	"context"
	"expvar"
	"testing"
)

// snapshotPanicCounter returns the current value of subscriberPanicTotal for
// key, or 0 if the key has never been incremented.
func snapshotPanicCounter(t *testing.T, key string) int64 {
	t.Helper()
	v := subscriberPanicTotal.Get(key)
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		t.Fatalf("subscriberPanicTotal[%q] is not an *expvar.Int: %T", key, v)
	}
	return iv.Value()
}

// TestInvokeHandler_PanicIncrementsPatternLabelledCounter pins the fix for
// the unbounded-cardinality finding: subscriberPanicTotal must be keyed by
// the bounded subscription pattern, never the concrete delivery subject
// (which embeds per-session/per-run identifiers in production, e.g.
// events.BuildGitWSEventSubject). A recovered panic increments exactly one
// key, and it is the pattern-based one — a refactor that swaps the label
// back to subject, or drops the Add entirely, fails this test.
func TestInvokeHandler_PanicIncrementsPatternLabelledCounter(t *testing.T) {
	log := newTestLogger(t)
	const pattern = "test.safe_handler.pattern"
	const subject = "test.safe_handler.pattern.session-abc123"
	patternKey := metricLabel("pattern", pattern, "mode", "regular")
	subjectKey := metricLabel("subject", subject, "mode", "regular")

	before := snapshotPanicCounter(t, patternKey)

	err := invokeHandler(context.Background(), log, "regular", subject, pattern,
		NewEvent("test.type", "test-source", nil),
		func(_ context.Context, _ *Event) error {
			panic("handler blew up")
		})
	if err != nil {
		t.Fatalf("invokeHandler returned non-nil for a recovered panic: %v", err)
	}

	after := snapshotPanicCounter(t, patternKey)
	if after != before+1 {
		t.Fatalf("subscriberPanicTotal[%q] = %d, want %d (before %d + 1)", patternKey, after, before+1, before)
	}

	if got := snapshotPanicCounter(t, subjectKey); got != 0 {
		t.Fatalf("subscriberPanicTotal incremented a subject-labelled key %q (value %d); "+
			"the label must be pattern-based only, never the concrete per-session subject", subjectKey, got)
	}
}
