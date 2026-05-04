package github

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func newTestTrackerLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// captureBus collects events published to a single subject so tests can
// assert on transitions without racing.
type captureBus struct {
	mu     sync.Mutex
	events []*bus.Event
	inner  bus.EventBus
}

func newCaptureBus(t *testing.T, subject string) *captureBus {
	t.Helper()
	log := newTestTrackerLogger(t)
	cb := &captureBus{inner: bus.NewMemoryEventBus(log)}
	if _, err := cb.inner.Subscribe(subject, func(_ context.Context, e *bus.Event) error {
		cb.mu.Lock()
		cb.events = append(cb.events, e)
		cb.mu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return cb
}

func (c *captureBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	return c.inner.Publish(ctx, subject, event)
}

func (c *captureBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return c.inner.Subscribe(subject, handler)
}

func (c *captureBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return c.inner.QueueSubscribe(subject, queue, handler)
}

func (c *captureBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return c.inner.Request(ctx, subject, event, timeout)
}

func (c *captureBus) Close()            { c.inner.Close() }
func (c *captureBus) IsConnected() bool { return c.inner.IsConnected() }

// captured returns a snapshot of events captured so far. MemoryEventBus
// dispatches to subscribers synchronously, so callers can read this directly
// after Record() / Publish() returns without polling.
func (c *captureBus) captured() []*bus.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*bus.Event(nil), c.events...)
}

func (c *captureBus) requireCount(t *testing.T, count int, msg string) []*bus.Event {
	t.Helper()
	evs := c.captured()
	if len(evs) != count {
		t.Fatalf("%s: expected %d events, got %d", msg, count, len(evs))
	}
	return evs
}

func TestParseRateHeaders_DecodesAllFields(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Limit", "5000")
	resp.Header.Set("X-RateLimit-Remaining", "4242")
	resp.Header.Set("X-RateLimit-Reset", "2000000000")
	resp.Header.Set("X-RateLimit-Resource", "graphql")

	snap, ok := parseRateHeaders(resp, ResourceCore)
	if !ok {
		t.Fatalf("expected snapshot to be parsed")
	}
	if snap.Resource != ResourceGraphQL {
		t.Errorf("resource: got %q want graphql", snap.Resource)
	}
	if snap.Limit != 5000 || snap.Remaining != 4242 {
		t.Errorf("limit/remaining mismatch: %d / %d", snap.Limit, snap.Remaining)
	}
	if snap.ResetAt.Unix() != 2000000000 {
		t.Errorf("reset_at: got %d want 2000000000", snap.ResetAt.Unix())
	}
}

func TestParseRateHeaders_FallsBackToDefaultResource(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Limit", "30")
	resp.Header.Set("X-RateLimit-Remaining", "29")
	resp.Header.Set("X-RateLimit-Reset", "2000000000")

	snap, ok := parseRateHeaders(resp, ResourceSearch)
	if !ok || snap.Resource != ResourceSearch {
		t.Fatalf("expected default resource search, got %q (ok=%v)", snap.Resource, ok)
	}
}

func TestParseRateHeaders_NoHeadersReturnsFalse(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	if _, ok := parseRateHeaders(resp, ResourceCore); ok {
		t.Fatalf("expected no snapshot when headers absent")
	}
}

func TestRateTracker_RecordEmitsAndTracksExhaustion(t *testing.T) {
	log := newTestTrackerLogger(t)
	cb := newCaptureBus(t, events.GitHubRateLimitUpdated)
	tr := NewRateTracker(cb, log)

	future := time.Now().Add(20 * time.Minute)

	// First snapshot: healthy. Should publish once (initial update).
	tr.Record(RateSnapshot{
		Resource:  ResourceGraphQL,
		Limit:     5000,
		Remaining: 4000,
		ResetAt:   future,
		UpdatedAt: time.Now(),
	})
	if tr.IsExhausted(ResourceGraphQL) {
		t.Fatalf("not exhausted yet")
	}

	// Second snapshot: exhausted -> must publish (transition).
	tr.Record(RateSnapshot{
		Resource:  ResourceGraphQL,
		Limit:     5000,
		Remaining: 0,
		ResetAt:   future,
		UpdatedAt: time.Now(),
	})
	if !tr.IsExhausted(ResourceGraphQL) {
		t.Fatalf("expected exhausted after Remaining=0 + future reset")
	}
	if d := tr.WaitDuration(ResourceGraphQL); d <= 0 || d > 21*time.Minute {
		t.Fatalf("WaitDuration out of range: %v", d)
	}

	// Third snapshot: recovered -> publishes recovery transition.
	tr.Record(RateSnapshot{
		Resource:  ResourceGraphQL,
		Limit:     5000,
		Remaining: 1500,
		ResetAt:   future,
		UpdatedAt: time.Now(),
	})

	evts := cb.requireCount(t, 3, "expected 3 events (initial, exhausted, recovered)")
	// Last two events should carry transition strings.
	gotTransitions := []string{}
	for _, e := range evts {
		payload, ok := e.Data.(*RateLimitUpdatedEvent)
		if !ok {
			t.Fatalf("unexpected event payload type %T", e.Data)
		}
		gotTransitions = append(gotTransitions, payload.ExhaustionTransition)
	}
	if gotTransitions[1] != "exhausted" || gotTransitions[2] != "recovered" {
		t.Errorf("transitions = %v, want [* exhausted recovered]", gotTransitions)
	}
}

func TestRateTracker_DebouncesNonTransitionUpdates(t *testing.T) {
	log := newTestTrackerLogger(t)
	cb := newCaptureBus(t, events.GitHubRateLimitUpdated)
	tr := NewRateTracker(cb, log)
	now := time.Now()

	// Two healthy snapshots within the debounce window -> only first emits.
	// MemoryEventBus.Publish dispatches synchronously, so reads after Record
	// see the final state without sleeping.
	tr.Record(RateSnapshot{Resource: ResourceCore, Limit: 5000, Remaining: 4999, ResetAt: now.Add(time.Hour), UpdatedAt: now})
	tr.Record(RateSnapshot{Resource: ResourceCore, Limit: 5000, Remaining: 4998, ResetAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Second)})
	cb.requireCount(t, 1, "expected 1 debounced event")

	// Outside the window -> emits again.
	tr.Record(RateSnapshot{Resource: ResourceCore, Limit: 5000, Remaining: 4997, ResetAt: now.Add(time.Hour), UpdatedAt: now.Add(2 * rateUpdateDebounce)})
	cb.requireCount(t, 2, "expected second event after debounce window")
}

func TestRateTracker_MarkRateExhaustedSeedsUnknownLimit(t *testing.T) {
	log := newTestTrackerLogger(t)
	cb := newCaptureBus(t, events.GitHubRateLimitUpdated)
	tr := NewRateTracker(cb, log)

	tr.markRateExhausted(ResourceGraphQL, time.Time{})

	if !tr.IsExhausted(ResourceGraphQL) {
		t.Fatalf("expected exhausted after markRateExhausted")
	}
	snap, ok := tr.Snapshot(ResourceGraphQL)
	if !ok {
		t.Fatalf("snapshot not stored")
	}
	if snap.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", snap.Remaining)
	}
	// Default reset (~1h) means WaitDuration > 0.
	if d := tr.WaitDuration(ResourceGraphQL); d <= 0 {
		t.Errorf("WaitDuration should be positive, got %v", d)
	}
}

func TestRateTracker_NilBusIsNoop(t *testing.T) {
	tr := NewRateTracker(nil, nil)
	tr.Record(RateSnapshot{Resource: ResourceCore, Limit: 5000, Remaining: 0, ResetAt: time.Now().Add(time.Minute)})
	if !tr.IsExhausted(ResourceCore) {
		t.Fatalf("tracker should still record when bus is nil")
	}
}
