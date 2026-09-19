package usage

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"math"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// fakeUsageRepo is a Repository test double. If block is true,
// CreateTaskUsageEvent blocks until ctx is done and returns ctx.Err(),
// modeling a wedged database that only responds to context cancellation
// (R5-F2's Test B).
type fakeUsageRepo struct {
	mu      sync.Mutex
	created []*models.TaskUsageEvent
	err     error
	block   bool
	calls   int
}

func (f *fakeUsageRepo) CreateTaskUsageEvent(ctx context.Context, event *models.TaskUsageEvent) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	f.created = append(f.created, event)
	f.mu.Unlock()
	return nil
}

func (f *fakeUsageRepo) rowCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

func (f *fakeUsageRepo) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// expvarMapValue reads one key's counter out of an expvar.Map, returning 0
// for an absent key. Metrics vars are package-level and shared across every
// test in this binary, so assertions use before/after deltas rather than
// absolute values.
func expvarMapValue(m *expvar.Map, key string) int64 {
	var v int64
	m.Do(func(kv expvar.KeyValue) {
		if kv.Key == key {
			if iv, ok := kv.Value.(*expvar.Int); ok {
				v = iv.Value()
			}
		}
	})
	return v
}

func validPayload(usageEventID string) *usageEventPayload {
	return &usageEventPayload{
		UsageEventID: usageEventID,
		TaskID:       "task-1",
		Usage:        &promptUsagePayload{InputTokens: 1},
	}
}

// TestHandleBusEvent_UndecodableData_RecordsDecodeErrorAndReturnsNil pins
// AC-27's decode stage and AC-34's "callback always returns nil" rule.
func TestHandleBusEvent_UndecodableData_RecordsDecodeErrorAndReturnsNil(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonDecodeError))

	err := w.handleBusEvent(context.Background(), &bus.Event{Data: func() {}})
	if err != nil {
		t.Fatalf("handleBusEvent returned %v, want nil (must never fail the publish)", err)
	}

	after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonDecodeError))
	if after-before != 1 {
		t.Errorf("decode_error count delta = %d, want 1", after-before)
	}
}

// TestAdmit_ChannelFull_DropsOverflowWithoutBlocking pins AC-34: a send
// that would block is abandoned, not waited on, and counted
// dropped:overflow. The worker is never started, so nothing drains the
// channel and it fills deterministically.
func TestAdmit_ChannelFull_DropsOverflowWithoutBlocking(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	for i := 0; i < eventQueueCapacity; i++ {
		w.admit(validPayload(fmt.Sprintf("evt-%d", i)))
	}

	before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonOverflow))

	done := make(chan struct{})
	go func() {
		w.admit(validPayload("evt-overflow"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("admit blocked instead of dropping when the channel is full")
	}

	after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonOverflow))
	if after-before != 1 {
		t.Errorf("overflow count delta = %d, want 1", after-before)
	}
}

// TestAdmit_AfterShutdownBegins_DropsShutdownWithoutBlocking pins AC-34: an
// event offered after Stop has begun draining is refused, not queued, and
// counted dropped:shutdown (distinct from overflow).
func TestAdmit_AfterShutdownBegins_DropsShutdownWithoutBlocking(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	w.Start()
	w.Stop()

	before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonShutdown))
	w.admit(validPayload("evt-late"))
	after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonShutdown))
	if after-before != 1 {
		t.Errorf("shutdown count delta = %d, want 1", after-before)
	}
}

// TestProcessEvent_InvalidPayload_NeverReachesRepository pins that the
// validate stage's rejection is terminal - an invalid event never reaches
// the ownership/insert stages the repository owns.
func TestProcessEvent_InvalidPayload_NeverReachesRepository(t *testing.T) {
	repo := &fakeUsageRepo{}
	w := NewWriter(repo, nil, nil)
	w.processEvent(context.Background(), &usageEventPayload{UsageEventID: "", TaskID: "task-1"})
	if repo.callCount() != 0 {
		t.Errorf("repo.calls = %d, want 0", repo.callCount())
	}
}

func TestProcessEvent_TokenTotalOverflow_DropsOverflowWithoutRepositoryCall(t *testing.T) {
	repo := &fakeUsageRepo{}
	w := NewWriter(repo, nil, nil)
	before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", "overflow"))
	w.processEvent(context.Background(), &usageEventPayload{
		UsageEventID: "evt-overflow", TaskID: "task-1",
		Usage: &promptUsagePayload{InputTokens: math.MaxInt64, CachedReadTokens: 1},
	})
	after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", "overflow"))
	if after-before != 1 {
		t.Errorf("overflow count delta = %d, want 1", after-before)
	}
	if repo.callCount() != 0 {
		t.Errorf("repo.calls = %d, want 0 for an overflowing token total", repo.callCount())
	}
}

// TestProcessEvent_RepoSucceeds_RecordsWritten pins the nil-error
// classification branch.
func TestProcessEvent_RepoSucceeds_RecordsWritten(t *testing.T) {
	repo := &fakeUsageRepo{}
	w := NewWriter(repo, nil, nil)
	label := usageMetricLabel("source", CostSourceUnpriced, "provider", "")
	before := expvarMapValue(eventsWrittenTotal, label)

	w.processEvent(context.Background(), validPayload("evt-1"))

	after := expvarMapValue(eventsWrittenTotal, label)
	if after-before != 1 {
		t.Errorf("written count delta = %d, want 1", after-before)
	}
	if repo.rowCount() != 1 {
		t.Fatalf("repo rows = %d, want 1", repo.rowCount())
	}
}

// TestProcessEvent_RepoReturnsErrDuplicateUsageEvent_RecordsDropDuplicate
// pins that AC-33/AC-32's already-implemented redelivery outcome is
// classified as dropped:duplicate, not dropped:error.
func TestProcessEvent_RepoReturnsErrDuplicateUsageEvent_RecordsDropDuplicate(t *testing.T) {
	repo := &fakeUsageRepo{err: sqliterepo.ErrDuplicateUsageEvent}
	w := NewWriter(repo, nil, nil)
	before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonDuplicate))

	w.processEvent(context.Background(), validPayload("evt-1"))

	after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonDuplicate))
	if after-before != 1 {
		t.Errorf("duplicate count delta = %d, want 1", after-before)
	}
}

// TestProcessEvent_RepoReturnsOtherError_RecordsDropError pins that every
// other repository failure - including R5-F1's ownership-lookup hard error
// and transient-retry exhaustion, both already handled inside
// CreateTaskUsageEvent - falls through to dropped:error.
func TestProcessEvent_RepoReturnsOtherError_RecordsDropError(t *testing.T) {
	repo := &fakeUsageRepo{err: errors.New("boom")}
	w := NewWriter(repo, nil, nil)
	before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonError))

	w.processEvent(context.Background(), validPayload("evt-1"))

	after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonError))
	if after-before != 1 {
		t.Errorf("error count delta = %d, want 1", after-before)
	}
}

// TestStop_HealthyRepo_DrainsBufferedEventsAndReturnsPromptly is R5-F2's
// Test A: buffered events drain and are recorded, and Stop returns well
// under the 5-second deadline.
func TestStop_HealthyRepo_DrainsBufferedEventsAndReturnsPromptly(t *testing.T) {
	repo := &fakeUsageRepo{}
	w := NewWriter(repo, nil, nil)
	w.Start()

	const n = 5
	for i := 0; i < n; i++ {
		w.admit(validPayload(fmt.Sprintf("evt-%d", i)))
	}

	start := time.Now()
	w.Stop()
	elapsed := time.Since(start)

	if elapsed >= drainDeadline {
		t.Errorf("Stop took %v, want well under the %v drain deadline for a healthy repository", elapsed, drainDeadline)
	}
	if repo.rowCount() != n {
		t.Errorf("repo rows = %d, want %d (every buffered event must drain before Stop returns)", repo.rowCount(), n)
	}
}

// TestStop_WedgedRepo_HonorsDrainDeadlineAndDropsEverythingAsError is
// R5-F2's Test B: a repository that only responds to context cancellation
// forces Stop to wait out the full drain deadline, after which the
// in-flight event is counted dropped:error, buffered events are counted
// dropped:drain_timeout, and nothing is written. Once workCtx is cancelled, a buffered event still
// waiting in the select may itself reach the repository with an
// already-cancelled context (returning instantly, still an error) rather
// than being classified without a repository call at all - both outcomes
// are dropped:error and neither writes a row, so repo.calls is not pinned
// to an exact count here. Runs inside synctest so the 5-second deadline
// advances on the fake clock instead of costing real wall time.
func TestStop_WedgedRepo_HonorsDrainDeadlineAndDropsEverythingAsError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &fakeUsageRepo{block: true}
		w := NewWriter(repo, nil, nil)
		w.Start()

		// The first admitted event is dequeued by the worker and blocks
		// forever inside the repository call (in flight); the fake repo
		// blocks every call, so it can never dequeue a second one until
		// workCtx is cancelled - these three stay buffered in the channel.
		const buffered = 3
		w.admit(validPayload("evt-inflight"))
		for i := 0; i < buffered; i++ {
			w.admit(validPayload(fmt.Sprintf("evt-buffered-%d", i)))
		}

		beforeError := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonError))
		beforeDrain := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", "drain_timeout"))

		start := time.Now()
		w.Stop()
		elapsed := time.Since(start)

		if elapsed < drainDeadline {
			t.Errorf("Stop returned after %v, want at least the %v drain deadline", elapsed, drainDeadline)
		}
		if repo.rowCount() != 0 {
			t.Errorf("repo rows = %d, want 0 (nothing may be written once the drain deadline has expired)", repo.rowCount())
		}
		if calls := repo.callCount(); calls < 1 {
			t.Errorf("repo.calls = %d, want at least 1 (the in-flight event must reach the repository)", calls)
		}
		afterError := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonError))
		afterDrain := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", "drain_timeout"))
		if afterError-beforeError != 1 {
			t.Errorf("error-drop count delta = %d, want 1 for the in-flight event", afterError-beforeError)
		}
		if afterDrain-beforeDrain != buffered {
			t.Errorf("drain-timeout count delta = %d, want %d for buffered events", afterDrain-beforeDrain, buffered)
		}
	})
}

// TestSubscribe_PublishedEvent_IsRecordedWithoutBlockingThePublisher pins
// AC-24 (subscribes on the real session-prompt-usage wildcard subject) and
// AC-34 (Publish, which delivers synchronously on the publisher's own
// goroutine, must return promptly regardless of downstream processing).
// The repository blocks every call until its context is cancelled, so this
// is the assertion that actually distinguishes the two designs: a handler
// that wrote synchronously on the callback goroutine would make Publish
// itself hang for as long as the repository blocks, and a repo that
// returns instantly (the prior version of this test) can't tell that apart
// from the real async design. synctest.Wait pins that the worker has
// genuinely reached the blocking repository call - not merely that the
// payload is still sitting unconsumed in the channel - before asserting
// Publish already returned. Runs inside synctest so Stop's 5-second drain
// deadline advances on the fake clock instead of costing real wall time.
func TestSubscribe_PublishedEvent_IsRecordedWithoutBlockingThePublisher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &fakeUsageRepo{block: true}
		w := NewWriter(repo, nil, nil)
		w.Start()

		eb := bus.NewMemoryEventBus(logger.Default())
		if err := w.Subscribe(eb); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		subject := events.BuildSessionPromptUsageSubject("session-1")
		payload := map[string]any{
			"task_id":        "task-1",
			"session_id":     "session-1",
			"usage_event_id": "evt-published",
			"usage":          map[string]any{"input_tokens": int64(7)},
		}

		start := time.Now()
		if err := eb.Publish(context.Background(), subject, &bus.Event{Data: payload}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Errorf("Publish took %v, want near-instant (the callback must never block on the worker)", elapsed)
		}

		synctest.Wait()
		if calls := repo.callCount(); calls != 1 {
			t.Fatalf("repo.calls = %d, want 1 (the worker must have reached the repository despite Publish already returning)", calls)
		}
		if repo.rowCount() != 0 {
			t.Fatalf("repo rows = %d, want 0 (the fake never returns success while its call is still blocked)", repo.rowCount())
		}

		before := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonError))
		w.Stop()
		after := expvarMapValue(eventsDroppedTotal, usageMetricLabel("reason", dropReasonError))
		if after-before != 1 {
			t.Errorf("error-drop count delta = %d, want 1 (Stop's drain deadline cancels the in-flight call)", after-before)
		}
	})
}

// TestStartStop_Idempotent pins that calling Start or Stop more than once
// is a safe no-op (matches the internal/integrations/healthpoll shape).
func TestStartStop_Idempotent(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	w.Start()
	w.Start()
	w.Stop()
	w.Stop()
}
