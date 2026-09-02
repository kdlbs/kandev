package bus

import (
	"context"
	"errors"
	"expvar"
	"testing"
)

type invokeHandlerContextKey struct{}

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

// TestInvokeHandler_PassesThroughHandlerError is reviewer-requested contract
// coverage. This behavior already exists on the current head: invokeHandler
// must forward the context and event unchanged, call the handler once, return
// its exact error, and leave the panic counter unchanged.
func TestInvokeHandler_PassesThroughHandlerError(t *testing.T) {
	log := newTestLogger(t)
	const pattern = "test.safe_handler.error"
	const subject = "test.safe_handler.error.session-abc123"
	patternKey := metricLabel("pattern", pattern, "mode", "regular")
	before := snapshotPanicCounter(t, patternKey)
	sentinel := errors.New("sentinel handler error")
	ctx := context.WithValue(context.Background(), invokeHandlerContextKey{}, "marker")
	event := NewEvent("test.type", "test-source", nil)
	var calls int
	var gotCtx context.Context
	var gotEvent *Event

	err := invokeHandler(ctx, log, "regular", subject, pattern, event,
		func(handlerCtx context.Context, handlerEvent *Event) error {
			calls++
			gotCtx = handlerCtx
			gotEvent = handlerEvent
			return sentinel
		})

	if !errors.Is(err, sentinel) {
		t.Fatalf("invokeHandler error = %v, want sentinel %v", err, sentinel)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if gotCtx != ctx {
		t.Fatal("invokeHandler did not forward the original context")
	}
	if gotEvent != event {
		t.Fatal("invokeHandler did not forward the original event")
	}
	if after := snapshotPanicCounter(t, patternKey); after != before {
		t.Fatalf("subscriberPanicTotal[%q] = %d, want unchanged value %d", patternKey, after, before)
	}
}
