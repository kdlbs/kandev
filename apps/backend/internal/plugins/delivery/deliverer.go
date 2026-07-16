// Package delivery implements event delivery for installed plugins, per
// docs/plans/plugins/GRPC-CONTRACT.md §5 ("Delivery / tools / webhooks
// semantics"): subscribing each plugin to its declared event subjects on
// the bus, delivering them over the plugin's live gRPC subprocess
// connection (via the injected Transport), retrying with backoff, and
// buffering events for plugins that are in the error state.
//
// This package deliberately does not import internal/plugins (the root
// package): internal/plugins.Service.SetDeliverer takes a small Deliverer
// interface (Refresh/Flush) defined on the consumer side specifically so
// this package can satisfy it structurally without creating an import
// cycle (internal/plugins already needs to reference the Deliverer type
// before this package exists, per its "Extension points" doc comment). All
// of this package's dependencies on plugin registration/transport data are
// expressed as small local interfaces (Transport, PluginLister) instead —
// internal/plugins.RuntimeTransport and backendapp's PluginLister adapter
// satisfy them structurally.
package delivery

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Default tuning values, per docs/plans/plugins/GRPC-CONTRACT.md §5: 10s
// per-attempt timeout, 3 retries backed off 5s/15s/45s, and a
// 100-event/5-minute-TTL ring buffer while a plugin is in the error state.
// Tests override these via Option so retry/backoff loops don't rely on real
// sleeps.
const (
	DefaultRequestTimeout = 10 * time.Second
	DefaultQueueSize      = 100
	DefaultRingBufferCap  = 100
	DefaultRingBufferTTL  = 5 * time.Minute
)

// DefaultRetryDelays is the backoff schedule between delivery attempts
// after the first: 5s, 15s, 45s (3 retries, per spec).
func DefaultRetryDelays() []time.Duration {
	return []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second}
}

// Transport delivers a single event to a plugin's live subprocess.
// Satisfied by *internal/plugins.RuntimeTransport in production (a thin
// proxy over runtime.Manager.Get + RemotePlugin.DeliverEvent); tests use a
// fake.
type Transport interface {
	DeliverEvent(ctx context.Context, pluginID string, e *pluginsdk.Event) error
}

// PluginRecord is the subset of a plugin installation Deliverer needs:
// identity, the event subjects it subscribes to (each a literal subject or
// a manifest.MatchSubject wildcard pattern such as "task.*"), and its
// current lifecycle status.
type PluginRecord struct {
	ID            string
	EventSubjects []string
	Status        string
}

// PluginLister returns the plugins Deliverer should track. Implementations
// should return every plugin whose status means it should keep receiving
// (store.StatusActive) or buffering (store.StatusError) deliveries;
// disabled/registered/uninstalled plugins should be omitted; Deliverer
// tears down their subscription and worker on the next Refresh.
type PluginLister interface {
	ActivePlugins() []PluginRecord
}

// Option configures a Deliverer at construction time.
type Option func(*Deliverer)

// WithRequestTimeout overrides the per-attempt delivery timeout (default
// DefaultRequestTimeout).
func WithRequestTimeout(t time.Duration) Option {
	return func(d *Deliverer) { d.requestTimeout = t }
}

// WithRetryDelays overrides the backoff schedule between attempts (default
// DefaultRetryDelays). len(delays) is the number of retries after the
// first attempt; pass an empty/nil slice for a single attempt with no
// retries, or all-zero durations to retry immediately (tests use this to
// avoid real sleeps).
func WithRetryDelays(delays []time.Duration) Option {
	return func(d *Deliverer) { d.retryDelays = delays }
}

// WithQueueSize overrides the bounded per-plugin delivery queue size
// (default DefaultQueueSize). Events are dropped (and logged) when a
// plugin's queue is full, so delivery never blocks the event bus.
func WithQueueSize(n int) Option {
	return func(d *Deliverer) { d.queueSize = n }
}

// WithRingBuffer overrides the per-plugin error-state buffer's capacity
// and TTL (default DefaultRingBufferCap / DefaultRingBufferTTL).
func WithRingBuffer(capacity int, ttl time.Duration) Option {
	return func(d *Deliverer) { d.ringBufferCap = capacity; d.ringBufferTTL = ttl }
}

// WithNow overrides the clock used for event occurred_at timestamps and
// ring buffer TTL bookkeeping (default time.Now). Tests inject a fake
// clock for deterministic assertions.
func WithNow(fn func() time.Time) Option {
	return func(d *Deliverer) { d.nowFn = fn }
}

// withOnProcessed is an unexported Option (only constructible from within
// this package) used by tests to observe exactly when a worker finishes
// handling each queued Delivery, so a test can wait for "all N events have
// been buffered/delivered" instead of racing a status flip against the
// worker goroutine with a sleep.
func withOnProcessed(fn func(pluginID string, d Delivery)) Option {
	return func(d *Deliverer) { d.onProcessed = fn }
}

// Deliverer subscribes each tracked plugin to the union of its declared
// event subjects on the bus and delivers events over Transport: one worker
// goroutine per plugin, sequential delivery, a bounded queue so event
// publishing is never blocked, retries with backoff, and an error-state
// ring buffer. It satisfies internal/plugins.Deliverer (Refresh/Flush)
// structurally — see the package doc comment.
type Deliverer struct {
	eventBus  bus.EventBus
	transport Transport
	lister    PluginLister
	log       *logger.Logger

	requestTimeout time.Duration
	retryDelays    []time.Duration
	queueSize      int
	ringBufferCap  int
	ringBufferTTL  time.Duration
	nowFn          func() time.Time

	mu      sync.Mutex
	workers map[string]*pluginWorker

	// onProcessed is an unexported test seam: when set, it is invoked once
	// a worker finishes handling a queued Delivery (buffered or delivered/
	// gave-up), letting tests wait deterministically for a worker to drain
	// its queue instead of racing Refresh/Flush against the worker
	// goroutine. Not part of any public Option — set via withOnProcessed
	// from within this package's own tests only.
	onProcessed func(pluginID string, d Delivery)
}

// New constructs a Deliverer. Call Refresh once after construction (and
// again after every install/status change) to establish bus subscriptions
// from the current plugin set.
func New(eventBus bus.EventBus, transport Transport, lister PluginLister, log *logger.Logger, opts ...Option) *Deliverer {
	d := &Deliverer{
		eventBus:       eventBus,
		transport:      transport,
		lister:         lister,
		log:            log,
		requestTimeout: DefaultRequestTimeout,
		retryDelays:    DefaultRetryDelays(),
		queueSize:      DefaultQueueSize,
		ringBufferCap:  DefaultRingBufferCap,
		ringBufferTTL:  DefaultRingBufferTTL,
		nowFn:          time.Now,
		workers:        make(map[string]*pluginWorker),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Refresh re-reads the active plugin set from PluginLister and reconciles
// bus subscriptions and workers: starts a worker + subscriptions for newly
// tracked plugins, updates cached status for existing ones (so an
// error<->active transition is picked up without recreating the worker or
// its buffer), and tears down workers/subscriptions for plugins no longer
// returned (disabled, uninstalled). Called by Service after Install,
// Enable, Disable, and any successful SetStatus transition — including from
// Service.handleStatusChange, invoked directly on runtime.Manager's
// supervision-loop goroutine, whose contract (see runtime.NewManager)
// requires the callback never block. Tearing down a removed worker calls
// w.stop() (which waits for its run loop to drain, including any in-flight
// delivery attempt — see pluginWorker's per-worker cancelable context) OUTSIDE
// d.mu, so a slow stop() never blocks a concurrent Refresh/Flush for a
// different plugin id that only needs the lock briefly.
func (d *Deliverer) Refresh() {
	records := d.lister.ActivePlugins()
	seen := make(map[string]bool, len(records))

	d.mu.Lock()
	for _, rec := range records {
		seen[rec.ID] = true
		w, ok := d.workers[rec.ID]
		if !ok {
			w = d.newWorker(rec.ID)
			d.workers[rec.ID] = w
			w.start()
		}
		w.updateRecord(rec)
		d.resubscribe(w, rec)
	}

	var toStop []*pluginWorker
	for id, w := range d.workers {
		if seen[id] {
			continue
		}
		w.unsubscribeAll()
		delete(d.workers, id)
		toStop = append(toStop, w)
	}
	d.mu.Unlock()

	for _, w := range toStop {
		w.stop()
	}
}

// Flush delivers every event buffered for pluginID (while it was in the
// error state) in order, then lets normal queue processing continue.
// Called by Service after an error -> active recovery transition. A no-op
// if pluginID has no tracked worker (e.g. it was uninstalled before
// recovery).
func (d *Deliverer) Flush(pluginID string) {
	d.mu.Lock()
	w, ok := d.workers[pluginID]
	d.mu.Unlock()
	if !ok {
		return
	}
	for _, item := range w.buffer.Drain() {
		w.enqueue(item)
	}
}

// Stop tears down every tracked worker and its bus subscriptions. Not part
// of the internal/plugins.Deliverer interface (Service never calls it);
// intended for graceful process shutdown by whatever wires this Deliverer.
func (d *Deliverer) Stop() {
	d.mu.Lock()
	workers := make([]*pluginWorker, 0, len(d.workers))
	for _, w := range d.workers {
		workers = append(workers, w)
	}
	d.workers = make(map[string]*pluginWorker)
	d.mu.Unlock()

	for _, w := range workers {
		w.unsubscribeAll()
		w.stop()
	}
}

// newWorker builds (but does not start) a worker for pluginID, wired with
// this Deliverer's current tuning options.
func (d *Deliverer) newWorker(id string) *pluginWorker {
	deliverCtx, cancelDeliver := context.WithCancel(context.Background())
	return &pluginWorker{
		id: id,
		deps: &workerDeps{
			transport:      d.transport,
			requestTimeout: d.requestTimeout,
			retryDelays:    d.retryDelays,
			log:            d.log,
			onProcessed:    d.onProcessed,
		},
		queue:         make(chan Delivery, d.queueSize),
		buffer:        newRingBuffer(d.ringBufferCap, d.ringBufferTTL, d.nowFn),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		deliverCtx:    deliverCtx,
		cancelDeliver: cancelDeliver,
	}
}

// resubscribe replaces w's bus subscriptions with one per unique event
// subject pattern in rec.EventSubjects.
func (d *Deliverer) resubscribe(w *pluginWorker, rec PluginRecord) {
	w.unsubscribeAll()

	seenPatterns := make(map[string]bool, len(rec.EventSubjects))
	for _, pattern := range rec.EventSubjects {
		if pattern == "" || seenPatterns[pattern] {
			continue
		}
		seenPatterns[pattern] = true

		sub, err := d.eventBus.Subscribe(pattern, d.makeHandler(w, pattern))
		if err != nil {
			d.log.Error("plugin delivery: failed to subscribe to event subject",
				zap.String("plugin_id", w.id), zap.String("subject", pattern), zap.Error(err))
			continue
		}
		w.addSub(sub)
	}
}

// makeHandler builds the bus.EventHandler that turns a matching event into
// a Delivery and enqueues it on w. pattern is re-checked against the
// concrete event type via manifest.MatchSubject — the same matcher plugin
// capabilities use elsewhere — so delivery is governed by one wildcard
// semantics regardless of the underlying EventBus's own subscription
// matching behavior.
func (d *Deliverer) makeHandler(w *pluginWorker, pattern string) bus.EventHandler {
	return func(_ context.Context, event *bus.Event) error {
		if !manifest.MatchSubject(pattern, event.Type) {
			return nil
		}

		payload, err := dataToMap(event.Data)
		if err != nil {
			d.log.Error("plugin delivery: failed to convert event payload",
				zap.String("plugin_id", w.id), zap.String("event_type", event.Type), zap.Error(err))
			return nil
		}

		w.enqueue(Delivery{
			PluginID: w.id,
			Event: &pluginsdk.Event{
				EventID:     uuid.New().String(),
				EventType:   event.Type,
				OccurredAt:  d.nowFn().UTC().Format(time.RFC3339),
				WorkspaceID: workspaceIDFromData(event.Data),
				Payload:     payload,
			},
		})
		return nil
	}
}

// workerDeps are the dependencies a pluginWorker needs to attempt
// delivery, factored out of Deliverer so pluginWorker doesn't hold a back
// reference to it.
type workerDeps struct {
	transport      Transport
	requestTimeout time.Duration
	retryDelays    []time.Duration
	log            *logger.Logger
	onProcessed    func(pluginID string, d Delivery)
}

// pluginWorker owns sequential delivery for one plugin: a bounded queue
// drained by a single goroutine (run), the bus subscriptions feeding that
// queue, and the error-state ring buffer. Lifecycle is owned by Deliverer
// (start/stop), per the goroutine-ownership convention in
// apps/backend/AGENTS.md.
type pluginWorker struct {
	id     string
	deps   *workerDeps
	queue  chan Delivery
	buffer *ringBuffer

	stopCh chan struct{}
	doneCh chan struct{}

	// deliverCtx is the parent context every delivery attempt's per-request
	// timeout (attemptDeliver) is derived from, canceled by stop() via
	// cancelDeliver. Without this, an in-flight attempt is bounded only by
	// its own requestTimeout (default 10s, operator-configurable higher),
	// so stop() — and therefore Refresh(), which calls stop() on a removed
	// worker — could block for that entire duration instead of returning as
	// soon as the worker is asked to stop.
	deliverCtx    context.Context
	cancelDeliver context.CancelFunc

	mu     sync.Mutex
	subs   []bus.Subscription
	status string
}

// start launches the worker's run loop. Must be called at most once.
func (w *pluginWorker) start() {
	go w.run()
}

// stop cancels any in-flight delivery attempt, signals the run loop to
// exit, and waits for it to drain. Idempotent only if called once
// (Deliverer never stops the same worker twice).
func (w *pluginWorker) stop() {
	w.cancelDeliver()
	close(w.stopCh)
	<-w.doneCh
}

// updateRecord applies the latest known status for the plugin this worker
// delivers to.
func (w *pluginWorker) updateRecord(rec PluginRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = rec.Status
}

// snapshotStatus returns the worker's current known status.
func (w *pluginWorker) snapshotStatus() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// addSub records a bus subscription as belonging to this worker, so
// unsubscribeAll can tear it down later.
func (w *pluginWorker) addSub(sub bus.Subscription) {
	w.mu.Lock()
	w.subs = append(w.subs, sub)
	w.mu.Unlock()
}

// unsubscribeAll cancels every bus subscription currently owned by this
// worker.
func (w *pluginWorker) unsubscribeAll() {
	w.mu.Lock()
	subs := w.subs
	w.subs = nil
	w.mu.Unlock()

	for _, sub := range subs {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
}

// enqueue pushes d onto the worker's queue without blocking: if the queue
// is full, the event is dropped and logged rather than backing up the bus
// publisher.
func (w *pluginWorker) enqueue(d Delivery) {
	select {
	case w.queue <- d:
	default:
		w.deps.log.Warn("plugin delivery queue full, dropping event",
			zap.String("plugin_id", w.id), zap.String("event_type", d.Event.EventType))
	}
}

// run drains the queue sequentially until stopCh fires.
func (w *pluginWorker) run() {
	defer close(w.doneCh)
	for {
		select {
		case <-w.stopCh:
			return
		case d, ok := <-w.queue:
			if !ok {
				return
			}
			w.process(d)
		}
	}
}

// process buffers d (if the plugin is currently known to be in the error
// state) or attempts delivery with retries, then notifies the onProcessed
// test seam (if any) that this Delivery has been fully handled.
func (w *pluginWorker) process(d Delivery) {
	if w.snapshotStatus() == store.StatusError {
		if dropped := w.buffer.Add(d); dropped != "" {
			w.deps.log.Warn("plugin event ring buffer overflow, dropped oldest buffered event",
				zap.String("plugin_id", w.id), zap.String("dropped_delivery_id", dropped))
		}
	} else {
		w.deliverWithRetry(w.deliverCtx, d)
	}
	if w.deps.onProcessed != nil {
		w.deps.onProcessed(w.id, d)
	}
}

// deliverWithRetry attempts d up to 1+len(retryDelays) times, sleeping
// (interruptibly) between attempts per the configured backoff schedule.
// Logs and gives up (event is lost from the live-delivery path; a
// subsequent health-poller error transition plus Flush is the recovery
// path for further events, per the at-least-once/error-buffer semantics in
// docs/specs/plugins/spec.md) if every attempt fails.
func (w *pluginWorker) deliverWithRetry(ctx context.Context, d Delivery) {
	var lastErr error
	attempts := 1 + len(w.deps.retryDelays)
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if !w.sleepOrStop(w.deps.retryDelays[attempt-1]) {
				return
			}
		}
		if err := w.attemptDeliver(ctx, d); err != nil {
			lastErr = err
			continue
		}
		return
	}
	w.deps.log.Warn("plugin event delivery failed after retries",
		zap.String("plugin_id", w.id),
		zap.String("delivery_id", d.Event.EventID),
		zap.String("event_type", d.Event.EventType),
		zap.Int("attempts", attempts),
		zap.Error(lastErr))
}

// sleepOrStop blocks for d or until the worker begins stopping. Returns
// false if stop won the race, per the select-based backoff convention in
// apps/backend/AGENTS.md ("never time.Sleep in a retry/backoff loop").
func (w *pluginWorker) sleepOrStop(d time.Duration) bool {
	if d <= 0 {
		select {
		case <-w.stopCh:
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-w.stopCh:
		return false
	}
}

// attemptDeliver makes one delivery attempt for d over Transport, bounded
// by requestTimeout.
func (w *pluginWorker) attemptDeliver(ctx context.Context, d Delivery) error {
	reqCtx, cancel := context.WithTimeout(ctx, w.deps.requestTimeout)
	defer cancel()
	return w.deps.transport.DeliverEvent(reqCtx, w.id, d.Event)
}
