package delivery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// fakeTransport is a controllable Transport for tests: DeliverEvent calls
// through to an injectable handler (defaulting to a no-op success), so
// tests can assert on what was delivered and simulate failures/blocking
// without a real network round trip.
type fakeTransport struct {
	mu      sync.Mutex
	handler func(pluginID string, e *pluginsdk.Event) error
	// ctxHandler, when set, takes priority over handler and additionally
	// receives DeliverEvent's ctx — for tests that need to observe/react to
	// ctx cancellation (e.g. proving an in-flight attempt is interrupted by
	// worker stop()).
	ctxHandler func(ctx context.Context, pluginID string, e *pluginsdk.Event) error
}

func (f *fakeTransport) DeliverEvent(ctx context.Context, pluginID string, e *pluginsdk.Event) error {
	f.mu.Lock()
	ch := f.ctxHandler
	h := f.handler
	f.mu.Unlock()
	if ch != nil {
		return ch(ctx, pluginID, e)
	}
	if h == nil {
		return nil
	}
	return h(pluginID, e)
}

func (f *fakeTransport) setHandler(h func(pluginID string, e *pluginsdk.Event) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = h
}

func (f *fakeTransport) setCtxHandler(h func(ctx context.Context, pluginID string, e *pluginsdk.Event) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxHandler = h
}

// fakeLister is a mutable, concurrency-safe PluginLister for tests.
type fakeLister struct {
	mu      sync.Mutex
	records []PluginRecord
}

func (f *fakeLister) ActivePlugins() []PluginRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PluginRecord, len(f.records))
	copy(out, f.records)
	return out
}

func (f *fakeLister) set(records ...PluginRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = records
}

// newTestDeliverer wires a Deliverer with zero retry backoff by default so
// tests never rely on real sleeps; individual tests override retry delays
// when they specifically exercise backoff/retry behavior.
func newTestDeliverer(t *testing.T, eventBus bus.EventBus, transport Transport, lister PluginLister, opts ...Option) *Deliverer {
	t.Helper()
	base := []Option{WithRetryDelays(nil)}
	d := New(eventBus, transport, lister, logger.Default(), append(base, opts...)...)
	t.Cleanup(d.Stop)
	return d
}

func requireNoTimeout[T any](t *testing.T, ch <-chan T, timeout time.Duration, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func requireTimeout[T any](t *testing.T, ch <-chan T, window time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected %s", what)
	case <-time.After(window):
	}
}

func TestDeliverer_DeliversOnMatchingEvent(t *testing.T) {
	receivedCh := make(chan *pluginsdk.Event, 1)
	transport := &fakeTransport{}
	transport.setHandler(func(pluginID string, e *pluginsdk.Event) error {
		if pluginID != "plug1" {
			t.Errorf("DeliverEvent pluginID = %q, want plug1", pluginID)
		}
		receivedCh <- e
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister)
	d.Refresh()

	ev := bus.NewEvent("task.state_changed", "test", map[string]interface{}{"task_id": "abc"})
	if err := eventBus.Publish(context.Background(), "task.state_changed", ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got := requireNoTimeout(t, receivedCh, 2*time.Second, "event delivery")
	if got.EventType != "task.state_changed" {
		t.Errorf("EventType = %q, want task.state_changed", got.EventType)
	}
	if got.EventID == "" {
		t.Error("EventID must not be empty")
	}
	if got.Payload["task_id"] != "abc" {
		t.Errorf("Payload[task_id] = %v, want abc", got.Payload["task_id"])
	}
}

func TestDeliverer_NonMatchingEventIsNotDelivered(t *testing.T) {
	receivedCh := make(chan struct{}, 1)
	transport := &fakeTransport{}
	transport.setHandler(func(string, *pluginsdk.Event) error {
		receivedCh <- struct{}{}
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister)
	d.Refresh()

	ev := bus.NewEvent("office.comment.created", "test", map[string]interface{}{})
	if err := eventBus.Publish(context.Background(), "office.comment.created", ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	requireTimeout(t, receivedCh, 150*time.Millisecond, "delivery for non-subscribed subject")
}

func TestDeliverer_SequentialPerPlugin(t *testing.T) {
	var inFlight int32
	release := make(chan struct{})
	arrivedCh := make(chan string, 10)

	transport := &fakeTransport{}
	transport.setHandler(func(_ string, e *pluginsdk.Event) error {
		if atomic.AddInt32(&inFlight, 1) > 1 {
			t.Errorf("more than one concurrent delivery in flight for the same plugin")
		}
		arrivedCh <- e.EventID
		<-release
		atomic.AddInt32(&inFlight, -1)
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister)
	d.Refresh()

	ctx := context.Background()
	_ = eventBus.Publish(ctx, "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))
	_ = eventBus.Publish(ctx, "task.updated", bus.NewEvent("task.updated", "test", map[string]interface{}{}))

	first := requireNoTimeout(t, arrivedCh, 2*time.Second, "first delivery")

	// Second delivery must not start while the first is still in flight.
	requireTimeout(t, arrivedCh, 150*time.Millisecond, "second delivery starting before first completed")

	release <- struct{}{}
	second := requireNoTimeout(t, arrivedCh, 2*time.Second, "second delivery")
	release <- struct{}{}

	if first == second {
		t.Errorf("expected two distinct delivery ids, got the same twice: %q", first)
	}
}

func TestDeliverer_RetriesOnErrorThenSucceeds(t *testing.T) {
	var attempts int32
	arrivedCh := make(chan struct{}, 5)
	transport := &fakeTransport{}
	transport.setHandler(func(string, *pluginsdk.Event) error {
		n := atomic.AddInt32(&attempts, 1)
		arrivedCh <- struct{}{}
		if n < 3 {
			return fmt.Errorf("simulated failure %d", n)
		}
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister,
		WithRetryDelays([]time.Duration{0, 0, 0}))
	d.Refresh()

	_ = eventBus.Publish(context.Background(), "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))

	for i := 0; i < 3; i++ {
		requireNoTimeout(t, arrivedCh, 2*time.Second, fmt.Sprintf("attempt %d", i+1))
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts = %d, want 3 (2 failures + 1 success)", got)
	}
}

func TestDeliverer_GivesUpAfterAllRetries(t *testing.T) {
	var attempts int32
	arrivedCh := make(chan struct{}, 10)
	transport := &fakeTransport{}
	transport.setHandler(func(string, *pluginsdk.Event) error {
		atomic.AddInt32(&attempts, 1)
		arrivedCh <- struct{}{}
		return fmt.Errorf("simulated permanent failure")
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister,
		WithRetryDelays([]time.Duration{0, 0, 0}))
	d.Refresh()

	_ = eventBus.Publish(context.Background(), "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))

	for i := 0; i < 4; i++ {
		requireNoTimeout(t, arrivedCh, 2*time.Second, fmt.Sprintf("attempt %d", i+1))
	}
	requireTimeout(t, arrivedCh, 150*time.Millisecond, "a 5th attempt (should give up after 1+3 retries)")
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Errorf("attempts = %d, want 4 (1 initial + 3 retries)", got)
	}
}

func TestDeliverer_BuffersWhileErrorAndFlushReplaysInOrder(t *testing.T) {
	arrivedCh := make(chan string, 10)
	transport := &fakeTransport{}
	transport.setHandler(func(_ string, e *pluginsdk.Event) error {
		arrivedCh <- e.EventType
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusError})

	processedCh := make(chan struct{}, 10)
	d := newTestDeliverer(t, eventBus, transport, lister,
		withOnProcessed(func(string, Delivery) { processedCh <- struct{}{} }))
	d.Refresh()

	ctx := context.Background()
	_ = eventBus.Publish(ctx, "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))
	_ = eventBus.Publish(ctx, "task.updated", bus.NewEvent("task.updated", "test", map[string]interface{}{}))

	// Wait until the worker has actually buffered both events before
	// flipping status and flushing, so this test doesn't race the worker
	// goroutine's queue-drain against the recovery transition below.
	requireNoTimeout(t, processedCh, 2*time.Second, "first event buffered")
	requireNoTimeout(t, processedCh, 2*time.Second, "second event buffered")

	requireTimeout(t, arrivedCh, 150*time.Millisecond, "delivery while plugin is in error state")

	// Recovery: status flips to active (mirrors Service.handleStatusChange
	// after the runtime manager reports the plugin healthy again) and
	// Refresh/Flush are called, mirroring the
	// Service.SetStatus -> Refresh, Service.handleStatusChange -> Flush
	// contract.
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})
	d.Refresh()
	d.Flush("plug1")

	first := requireNoTimeout(t, arrivedCh, 2*time.Second, "replayed first buffered event")
	second := requireNoTimeout(t, arrivedCh, 2*time.Second, "replayed second buffered event")

	if first != "task.created" || second != "task.updated" {
		t.Errorf("replay order = [%s %s], want [task.created task.updated]", first, second)
	}
}

func TestDeliverer_RingBufferOverflowDropsOldest(t *testing.T) {
	arrivedCh := make(chan string, 10)
	transport := &fakeTransport{}
	transport.setHandler(func(_ string, e *pluginsdk.Event) error {
		arrivedCh <- e.EventType
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusError})

	processedCh := make(chan struct{}, 10)
	d := newTestDeliverer(t, eventBus, transport, lister, WithRingBuffer(2, 5*time.Minute),
		withOnProcessed(func(string, Delivery) { processedCh <- struct{}{} }))
	d.Refresh()

	ctx := context.Background()
	_ = eventBus.Publish(ctx, "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))
	_ = eventBus.Publish(ctx, "task.updated", bus.NewEvent("task.updated", "test", map[string]interface{}{}))
	_ = eventBus.Publish(ctx, "task.deleted", bus.NewEvent("task.deleted", "test", map[string]interface{}{}))

	// Wait until the worker has buffered all three events (and evicted the
	// oldest) before flipping status and flushing.
	for i := 0; i < 3; i++ {
		requireNoTimeout(t, processedCh, 2*time.Second, fmt.Sprintf("event %d buffered", i+1))
	}

	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})
	d.Refresh()
	d.Flush("plug1")

	first := requireNoTimeout(t, arrivedCh, 2*time.Second, "replayed first surviving buffered event")
	second := requireNoTimeout(t, arrivedCh, 2*time.Second, "replayed second surviving buffered event")
	if first != "task.updated" || second != "task.deleted" {
		t.Errorf("replay order = [%s %s], want [task.updated task.deleted] (task.created should have been dropped as oldest on overflow)", first, second)
	}
	requireTimeout(t, arrivedCh, 150*time.Millisecond, "a 3rd replayed event")
}

func TestDeliverer_RingBufferOverflowLogsAggregatedDrops(t *testing.T) {
	clock := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	now := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return clock
	}
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap(): %v", err)
	}

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusError})
	processedCh := make(chan struct{}, 10)
	d := New(eventBus, &fakeTransport{}, lister, log,
		WithRetryDelays(nil),
		WithRingBuffer(1, 5*time.Minute),
		WithNow(now),
		WithOverflowLogInterval(time.Minute),
		withOnProcessed(func(string, Delivery) { processedCh <- struct{}{} }))
	t.Cleanup(d.Stop)
	d.Refresh()

	ctx := context.Background()
	for _, subject := range []string{"task.created", "task.updated", "task.deleted"} {
		if err := eventBus.Publish(ctx, subject, bus.NewEvent(subject, "test", map[string]interface{}{})); err != nil {
			t.Fatalf("Publish(%s): %v", subject, err)
		}
	}
	for i := 0; i < 3; i++ {
		requireNoTimeout(t, processedCh, 2*time.Second, fmt.Sprintf("event %d buffered", i+1))
	}

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("overflow warnings after two drops = %d, want 1", len(entries))
	}
	if got := entries[0].ContextMap()["dropped_count"]; got != int64(1) {
		t.Fatalf("first dropped_count = %v, want 1", got)
	}
	if got := entries[0].ContextMap()["latest_dropped_delivery_id"]; got == nil || got == "" {
		t.Fatalf("first latest_dropped_delivery_id = %v, want a non-empty id", got)
	}
	if _, ok := entries[0].ContextMap()["dropped_delivery_id"]; ok {
		t.Fatal("first overflow warning still uses the ambiguous dropped_delivery_id field")
	}

	clockMu.Lock()
	clock = clock.Add(time.Minute)
	clockMu.Unlock()
	if err := eventBus.Publish(ctx, "task.archived", bus.NewEvent("task.archived", "test", map[string]interface{}{})); err != nil {
		t.Fatalf("Publish(task.archived): %v", err)
	}
	requireNoTimeout(t, processedCh, 2*time.Second, "post-interval event buffered")

	entries = observed.All()
	if len(entries) != 2 {
		t.Fatalf("overflow warnings after interval = %d, want 2", len(entries))
	}
	if got := entries[1].ContextMap()["dropped_count"]; got != int64(2) {
		t.Fatalf("aggregated dropped_count = %v, want 2", got)
	}
	if got := entries[1].ContextMap()["latest_dropped_delivery_id"]; got == nil || got == "" {
		t.Fatalf("aggregated latest_dropped_delivery_id = %v, want a non-empty id", got)
	}
	if _, ok := entries[1].ContextMap()["dropped_delivery_id"]; ok {
		t.Fatal("aggregated overflow warning still uses the ambiguous dropped_delivery_id field")
	}
}

func TestDeliverer_RingBufferOverflowLimiterIsPerPlugin(t *testing.T) {
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap(): %v", err)
	}

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(
		PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusError},
		PluginRecord{ID: "plug2", EventSubjects: []string{"task.*"}, Status: store.StatusError},
	)
	processedCh := make(chan struct{}, 10)
	d := New(eventBus, &fakeTransport{}, lister, log,
		WithRetryDelays(nil),
		WithRingBuffer(1, 5*time.Minute),
		WithOverflowLogInterval(time.Minute),
		withOnProcessed(func(string, Delivery) { processedCh <- struct{}{} }))
	t.Cleanup(d.Stop)
	d.Refresh()

	for _, subject := range []string{"task.created", "task.updated"} {
		if err := eventBus.Publish(context.Background(), subject, bus.NewEvent(subject, "test", map[string]interface{}{})); err != nil {
			t.Fatalf("Publish(%s): %v", subject, err)
		}
	}
	for i := 0; i < 4; i++ {
		requireNoTimeout(t, processedCh, 2*time.Second, fmt.Sprintf("plugin event %d buffered", i+1))
	}

	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("overflow warnings for two plugins = %d, want 2", len(entries))
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.ContextMap()["plugin_id"].(string)] = true
	}
	if !seen["plug1"] || !seen["plug2"] {
		t.Fatalf("overflow warning plugin ids = %v, want both plugins", seen)
	}
}

func TestDeliverer_RefreshStopsDeliveryWhenPluginRemoved(t *testing.T) {
	arrivedCh := make(chan struct{}, 5)
	transport := &fakeTransport{}
	transport.setHandler(func(string, *pluginsdk.Event) error {
		arrivedCh <- struct{}{}
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister)
	d.Refresh()

	_ = eventBus.Publish(context.Background(), "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))
	requireNoTimeout(t, arrivedCh, 2*time.Second, "delivery while plugin active")

	// Disabled: PluginLister stops returning it (mirrors Service filtering
	// out StatusDisabled).
	lister.set()
	d.Refresh()

	_ = eventBus.Publish(context.Background(), "task.updated", bus.NewEvent("task.updated", "test", map[string]interface{}{}))
	requireTimeout(t, arrivedCh, 150*time.Millisecond, "delivery after plugin removed from active set")
}

// TestDeliverer_RefreshReturnsPromptlyDuringInFlightDelivery pins the fix
// for Refresh blocking on a worker's in-flight delivery attempt: before the
// fix, attemptDeliver's ctx was context.Background() (only bounded by
// requestTimeout, here deliberately set far longer than the test's
// timeout), and stop() waited on that same attempt to finish before
// returning — so Refresh (which calls stop() for a removed worker) could
// block for the full requestTimeout. Service.handleStatusChange calls
// Refresh from the runtime supervision goroutine, which must never block
// (runtime.NewManager's contract). The fix derives attemptDeliver's ctx
// from a per-worker context canceled by stop(), so the in-flight attempt
// is interrupted immediately instead.
func TestDeliverer_RefreshReturnsPromptlyDuringInFlightDelivery(t *testing.T) {
	entered := make(chan struct{})
	released := make(chan struct{})
	var sawCancel int32

	transport := &fakeTransport{}
	transport.setCtxHandler(func(ctx context.Context, _ string, _ *pluginsdk.Event) error {
		close(entered)
		select {
		case <-ctx.Done():
			atomic.AddInt32(&sawCancel, 1)
			return ctx.Err()
		case <-released:
			return nil
		}
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	d := newTestDeliverer(t, eventBus, transport, lister, WithRequestTimeout(30*time.Second))
	d.Refresh()

	_ = eventBus.Publish(context.Background(), "task.created", bus.NewEvent("task.created", "test", map[string]interface{}{}))
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the delivery attempt to start")
	}

	// Remove the plugin so Refresh must tear its worker down while the
	// delivery attempt above is still blocked inside the transport call.
	lister.set()

	done := make(chan struct{})
	go func() {
		d.Refresh()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Refresh() did not return promptly while a delivery was in flight (requestTimeout was 30s)")
	}

	if atomic.LoadInt32(&sawCancel) != 1 {
		t.Fatal("in-flight delivery attempt was never canceled by worker stop()")
	}
	close(released)
}

// barrierSubject is the control subject the negative-delivery assertions
// publish on after the event under test — see assertNotDelivered.
const barrierSubject = "delivery_test.barrier"

// newSubjectFixture wires a Deliverer with a single plugin subscribed to
// pattern (plus the barrier subject used by assertNotDelivered) and returns
// the bus plus the channel every delivered event lands on.
func newSubjectFixture(t *testing.T, pattern string) (bus.EventBus, <-chan *pluginsdk.Event) {
	t.Helper()

	receivedCh := make(chan *pluginsdk.Event, 4)
	transport := &fakeTransport{}
	transport.setHandler(func(_ string, e *pluginsdk.Event) error {
		receivedCh <- e
		return nil
	})

	eventBus := bus.NewMemoryEventBus(logger.Default())
	lister := &fakeLister{}
	lister.set(PluginRecord{
		ID:            "plug1",
		EventSubjects: []string{pattern, barrierSubject},
		Status:        store.StatusActive,
	})

	d := newTestDeliverer(t, eventBus, transport, lister)
	d.Refresh()

	return eventBus, receivedCh
}

// publishSubject publishes one event on subject stamped with eventType,
// mirroring how kandev publishes per-session events: a suffixed subject
// carrying the bare type constant.
func publishSubject(t *testing.T, eventBus bus.EventBus, subject, eventType string) {
	t.Helper()
	ev := bus.NewEvent(eventType, "test", map[string]interface{}{"session_id": "sess-1"})
	if err := eventBus.Publish(context.Background(), subject, ev); err != nil {
		t.Fatalf("Publish(%s): %v", subject, err)
	}
}

// publishForPattern subscribes a single plugin with pattern, publishes one
// event on subject, and returns the delivered pluginsdk.Event (failing the
// test if nothing arrives). Only used for the positive cases, so it never
// waits out its timeout on a passing run.
func publishForPattern(t *testing.T, pattern, subject, eventType string) *pluginsdk.Event {
	t.Helper()

	eventBus, receivedCh := newSubjectFixture(t, pattern)
	publishSubject(t, eventBus, subject, eventType)

	return requireNoTimeout(t, receivedCh, 2*time.Second,
		fmt.Sprintf("delivery of %s to pattern %s", subject, pattern))
}

// assertNotDelivered asserts pattern does not receive an event published on
// subject, without waiting out a wall clock window: after the event under
// test it publishes a control event on barrierSubject, which the same plugin
// also subscribes to. The memory bus enqueues synchronously inside Publish
// and the worker drains its single queue in FIFO order, so the barrier
// arriving first is proof the event under test was never enqueued.
func assertNotDelivered(t *testing.T, pattern, subject, eventType string) {
	t.Helper()

	eventBus, receivedCh := newSubjectFixture(t, pattern)
	publishSubject(t, eventBus, subject, eventType)
	publishSubject(t, eventBus, barrierSubject, barrierSubject)

	got := requireNoTimeout(t, receivedCh, 2*time.Second, "barrier delivery")
	if got.EventType != barrierSubject {
		t.Fatalf("pattern %q must not receive subject %q, got EventType %q", pattern, subject, got.EventType)
	}
}

// TestDeliverer_DeliversPerSessionSuffixedSubject is the regression test for
// the catch-22 that made every per-session-suffixed subject undeliverable:
// the event is published on a 3-segment subject but stamped with the bare
// 2-segment type constant, so re-checking the manifest pattern against
// event.Type dropped it after the bus subscription had already matched.
func TestDeliverer_DeliversPerSessionSuffixedSubject(t *testing.T) {
	got := publishForPattern(t,
		"session_prompt_usage.updated.*",
		"session_prompt_usage.updated.sess-1",
		"session_prompt_usage.updated")
	if got.EventType != "session_prompt_usage.updated.sess-1" {
		t.Errorf("EventType = %q, want the concrete published subject session_prompt_usage.updated.sess-1", got.EventType)
	}
	if got.Payload["session_id"] != "sess-1" {
		t.Errorf("Payload[session_id] = %v, want sess-1", got.Payload["session_id"])
	}
}

// TestDeliverer_PerSessionSubjectFamilies covers the rest of the suffixed
// subject families the docs advertise as subscribable, including the
// 3-segment base type file.change.notified (whose per-session subject has
// four segments) and the per-run office subject.
func TestDeliverer_PerSessionSubjectFamilies(t *testing.T) {
	cases := []struct {
		pattern   string
		subject   string
		eventType string
	}{
		{"shell.output.*", "shell.output.sess-1", "shell.output"},
		{"shell.exit.*", "shell.exit.sess-1", "shell.exit"},
		{"process.output.*", "process.output.sess-1", "process.output"},
		{"process.status.*", "process.status.sess-1", "process.status"},
		{"agent.stream.*", "agent.stream.sess-1", "agent.stream"},
		{"git.event.*", "git.event.sess-1", "git.event"},
		{"git.ws.*", "git.ws.sess-1", "git.ws"},
		{"file.change.notified.*", "file.change.notified.sess-1", "file.change.notified"},
		{"permission_request.received.*", "permission_request.received.sess-1", "permission_request.received"},
		{"context_window.updated.*", "context_window.updated.sess-1", "context_window.updated"},
		{"available_commands.updated.*", "available_commands.updated.sess-1", "available_commands.updated"},
		{"session_mode.changed.*", "session_mode.changed.sess-1", "session_mode.changed"},
		{"agent_capabilities.updated.*", "agent_capabilities.updated.sess-1", "agent_capabilities.updated"},
		{"session_models.updated.*", "session_models.updated.sess-1", "session_models.updated"},
		{"session_info.updated.*", "session_info.updated.sess-1", "session_info.updated"},
		{"session_todos.updated.*", "session_todos.updated.sess-1", "session_todos.updated"},
		// office.run.event_appended already publishes with Type == subject;
		// it must keep working now that the subject is what gets matched.
		{"office.run.event_appended.*", "office.run.event_appended.run-1", "office.run.event_appended.run-1"},
	}

	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			got := publishForPattern(t, tc.pattern, tc.subject, tc.eventType)
			if got.EventType != tc.subject {
				t.Errorf("EventType = %q, want %q", got.EventType, tc.subject)
			}
		})
	}
}

// TestDeliverer_SubjectUnrelatedToTypeIsDelivered covers the one shape where
// the published subject is not even a suffixed form of the event's own type:
// handleGitCommitCreated / handleGitCommitsReset / handleBranchSwitched in the
// orchestrator publish on git.ws.<sessionId> while stamping events.GitEvent
// ("git.event"). Matching on the subject makes these deliverable under
// "git.ws.*"; matching on the type could never have delivered them at all.
func TestDeliverer_SubjectUnrelatedToTypeIsDelivered(t *testing.T) {
	got := publishForPattern(t, "git.ws.*", "git.ws.sess-1", "git.event")
	if got.EventType != "git.ws.sess-1" {
		t.Errorf("EventType = %q, want the concrete published subject git.ws.sess-1", got.EventType)
	}

	// ...and the type it carries must not make it match that type's pattern.
	assertNotDelivered(t, "git.event.*", "git.ws.sess-1", "git.event")
}

// TestDeliverer_UnsuffixedSubscriptionsUnchanged pins the backward-compatible
// half: subjects published unsuffixed have subject == type, so both exact
// and wildcard 2-segment subscriptions still match and still see the same
// EventType they saw before.
func TestDeliverer_UnsuffixedSubscriptionsUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		subject string
	}{
		{"exact", "task.created", "task.created"},
		{"wildcard", "task.*", "task.created"},
		{"three segment exact", "message.queue.status_changed", "message.queue.status_changed"},
		{"three segment wildcard", "message.queue.*", "message.queue.status_changed"},
		{"cross plugin wildcard", "plugin.other-plugin.*", "plugin.other-plugin.ping"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := publishForPattern(t, tc.pattern, tc.subject, tc.subject)
			if got.EventType != tc.subject {
				t.Errorf("EventType = %q, want %q", got.EventType, tc.subject)
			}
		})
	}
}

// TestDeliverer_SegmentCountStrictnessPreserved proves the manifest re-check
// still governs delivery after switching it to the concrete subject: NATS'
// multi-segment ">" wildcard (which the underlying bus honors, and which
// manifest.MatchSubject deliberately does not) must not deliver, and neither
// must a pattern whose segment count differs from the subject's.
func TestDeliverer_SegmentCountStrictnessPreserved(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		subject string
	}{
		// The bus subscription matches ">" but manifest.MatchSubject does not.
		{"multi segment nats wildcard", "shell.>", "shell.output.sess-1"},
		{"multi segment nats wildcard on unsuffixed", "task.>", "task.created"},
		// Too few pattern segments for the subject.
		{"two segment pattern vs three segment subject", "shell.*", "shell.output.sess-1"},
		// Too many pattern segments for the subject.
		{"three segment pattern vs two segment subject", "task.created.*", "task.created"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNotDelivered(t, tc.pattern, tc.subject, tc.subject)
		})
	}
}

// TestDeliverer_WrongSessionSuffixNotDelivered checks a literal session id in
// the pattern only receives that session's events.
func TestDeliverer_WrongSessionSuffixNotDelivered(t *testing.T) {
	assertNotDelivered(t, "shell.output.sess-other", "shell.output.sess-1", "shell.output")
}

// TestDeliverer_UnstampedEventFallsBackToType covers an Event handed to the
// handler without going through Publish (no Subject stamped): matching falls
// back to event.Type so nothing regresses for a bus implementation or test
// that constructs deliveries by hand.
func TestDeliverer_UnstampedEventFallsBackToType(t *testing.T) {
	receivedCh := make(chan *pluginsdk.Event, 1)
	transport := &fakeTransport{}
	transport.setHandler(func(_ string, e *pluginsdk.Event) error {
		receivedCh <- e
		return nil
	})

	lister := &fakeLister{}
	lister.set(PluginRecord{ID: "plug1", EventSubjects: []string{"task.*"}, Status: store.StatusActive})

	captureBus := &handlerCapturingBus{MemoryEventBus: bus.NewMemoryEventBus(logger.Default())}
	d := newTestDeliverer(t, captureBus, transport, lister)
	d.Refresh()

	handler := captureBus.handlerFor("task.*")
	if handler == nil {
		t.Fatal("deliverer did not subscribe to task.*")
	}
	// No Subject field: straight to the handler, as an unstamped event.
	if err := handler(context.Background(), &bus.Event{ID: "e1", Type: "task.created"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got := requireNoTimeout(t, receivedCh, 2*time.Second, "event delivery")
	if got.EventType != "task.created" {
		t.Errorf("EventType = %q, want task.created", got.EventType)
	}
}

// handlerCapturingBus records the handler registered for each subscription
// pattern so a test can invoke it directly with a hand-built Event.
type handlerCapturingBus struct {
	*bus.MemoryEventBus

	mu       sync.Mutex
	handlers map[string]bus.EventHandler
}

func (b *handlerCapturingBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	b.mu.Lock()
	if b.handlers == nil {
		b.handlers = make(map[string]bus.EventHandler)
	}
	b.handlers[subject] = handler
	b.mu.Unlock()
	return b.MemoryEventBus.Subscribe(subject, handler)
}

func (b *handlerCapturingBus) handlerFor(subject string) bus.EventHandler {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.handlers[subject]
}
