package usage

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// ContractVersion is AC-5's per-row contract version: it advances only when
// a column's meaning changes, never when a column is merely added.
const ContractVersion = 1

// eventQueueCapacity is AC-34's fixed bounded-channel size between the bus
// callback and the single worker goroutine.
const eventQueueCapacity = 1024

// drainDeadline is AC-34's bound on how long Stop waits for buffered events
// to finish draining before abandoning whatever remains.
const drainDeadline = 5 * time.Second

// Repository is the narrow persistence capability the writer needs.
// Implemented by *sqlite.Repository in production; AC-33's ownership check
// and AC-32's retry/backoff classification already live inside it (Task 7),
// so the writer only builds a complete row and classifies the error it
// gets back.
type Repository interface {
	CreateTaskUsageEvent(ctx context.Context, event *models.TaskUsageEvent) error
}

// Writer subscribes to the session-prompt-usage bus subject and records one
// task_usage_events row per observed event. The bus callback only decodes
// and hands off (AC-34); a single long-lived worker goroutine does the
// database work serially, off the publisher's own goroutine, so
// occurred_at acquisition stays monotonic with arrival order.
type Writer struct {
	repo    Repository
	pricing PricingLookup
	log     *logger.Logger

	events chan *usageEventPayload

	mu       sync.Mutex
	started  bool
	draining bool

	workCtx    context.Context
	cancelWork context.CancelFunc
	wg         sync.WaitGroup
}

// NewWriter constructs a Writer. pricing may be nil, in which case every
// event resolves cost_source=unpriced (AC-8's no-lookup-wired degradation).
// log may be nil; metrics are still recorded, just not logged.
func NewWriter(repo Repository, pricing PricingLookup, log *logger.Logger) *Writer {
	workCtx, cancel := context.WithCancel(context.Background())
	return &Writer{
		repo:       repo,
		pricing:    pricing,
		log:        log,
		events:     make(chan *usageEventPayload, eventQueueCapacity),
		workCtx:    workCtx,
		cancelWork: cancel,
	}
}

// Start launches the single worker goroutine. Idempotent.
func (w *Writer) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true
	w.wg.Add(1)
	go w.run()
}

// Stop stops accepting new events, drains whatever is already buffered, and
// returns once the buffer is empty or drainDeadline has elapsed, whichever
// comes first (AC-34). Anything still buffered at the deadline - including
// the event in flight, whose context is cancelled at that instant - is
// counted dropped:drain_timeout and never written. Idempotent.
func (w *Writer) Stop() {
	w.mu.Lock()
	if !w.started || w.draining {
		w.mu.Unlock()
		return
	}
	w.draining = true
	close(w.events)
	w.mu.Unlock()

	timer := time.AfterFunc(drainDeadline, w.cancelWork)
	defer timer.Stop()
	w.wg.Wait()
}

// Subscribe registers the writer's bus callback on the session-prompt-usage
// subject (AC-24). Call after Start.
func (w *Writer) Subscribe(eb bus.EventBus) error {
	_, err := eb.Subscribe(events.BuildSessionPromptUsageWildcardSubject(), w.handleBusEvent)
	return err
}

// run is the single serial worker (AC-34): it drains w.events until the
// channel is closed and empty, or until workCtx is cancelled (only ever by
// Stop's drain-deadline timer), whichever comes first.
func (w *Writer) run() {
	defer w.wg.Done()
	for {
		select {
		case payload, ok := <-w.events:
			if !ok {
				return
			}
			if w.workCtx.Err() != nil {
				w.recordDropped(dropReasonDrainTimeout, payload.TaskID)
				continue
			}
			w.processEvent(w.workCtx, payload)
		case <-w.workCtx.Done():
			for payload := range w.events {
				w.recordDropped(dropReasonDrainTimeout, payload.TaskID)
			}
			return
		}
	}
}

// handleBusEvent is the bus callback (AC-34): decode only, then hand off to
// the worker over the bounded channel. It always returns nil immediately -
// the bus delivers synchronously on the publisher's own goroutine, and this
// callback must never block an agent turn or fail the publish.
func (w *Writer) handleBusEvent(_ context.Context, event *bus.Event) error {
	payload, err := decodePayload(event)
	if err != nil {
		w.recordDropped(dropReasonDecodeError, "")
		return nil
	}
	w.admit(payload)
	return nil
}

// admit hands payload to the worker over the bounded channel without
// blocking (AC-34): a full channel is abandoned as dropped:overflow; an
// event offered after Stop has begun draining is refused as
// dropped:shutdown. The send and the shutdown check share w.mu with Stop's
// close(w.events), so a send can never race a close.
func (w *Writer) admit(p *usageEventPayload) {
	w.mu.Lock()
	if w.draining {
		w.mu.Unlock()
		w.recordDropped(dropReasonShutdown, p.TaskID)
		return
	}
	select {
	case w.events <- p:
		w.mu.Unlock()
	default:
		w.mu.Unlock()
		w.recordDropped(dropReasonOverflow, p.TaskID)
	}
}

// processEvent implements AC-27's validate/ownership/insert stages for one
// dequeued event. occurred_at is acquired exactly once here, on the single
// serial worker, before the first (and only) repository call - the
// repository's own AC-32 retries never re-acquire it.
func (w *Writer) processEvent(ctx context.Context, p *usageEventPayload) {
	if reason := validateStage(p); reason != "" {
		w.recordDropped(reason, p.TaskID)
		return
	}

	occurredAt := time.Now().UTC()
	event := w.buildRow(ctx, p, occurredAt)
	if event == nil {
		w.recordDropped(dropReasonOverflow, p.TaskID)
		return
	}

	err := w.repo.CreateTaskUsageEvent(ctx, event)
	switch {
	case err == nil:
		w.recordWritten(event.CostSource, event.Provider)
	case errors.Is(err, sqliterepo.ErrDuplicateUsageEvent):
		w.recordDropped(dropReasonDuplicate, p.TaskID)
	default:
		w.recordDropped(dropReasonError, p.TaskID)
	}
}
