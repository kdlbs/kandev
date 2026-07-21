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
