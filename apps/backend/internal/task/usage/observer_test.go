package usage

import (
	"context"
	"expvar"
	"fmt"
	"sync"
	"testing"
	"time"
)

type observed struct{ taskID, sessionID string }

type observerRecorder struct {
	mu   sync.Mutex
	seen []observed
	fn   func(ctx context.Context, taskID, sessionID string)
}

func (r *observerRecorder) observe(ctx context.Context, taskID, sessionID string) {
	r.mu.Lock()
	r.seen = append(r.seen, observed{taskID, sessionID})
	fn := r.fn
	r.mu.Unlock()
	if fn != nil {
		fn(ctx, taskID, sessionID)
	}
}

func (r *observerRecorder) snapshot() []observed {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]observed(nil), r.seen...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func sessionPayload(id, taskID, sessionID string) *usageEventPayload {
	p := validPayload(id)
	p.TaskID, p.SessionID = taskID, sessionID
	return p
}

func observerDropped() int64 {
	if v, ok := expvar.Get("coordinator_usage_observer_dropped_total").(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

func TestObserver_NotifiedAfterEachSuccessfulInsertInArrivalOrder(t *testing.T) {
	repo := &fakeUsageRepo{}
	w := NewWriter(repo, nil, nil)
	rec := &observerRecorder{}
	w.SetRecordedObserver(rec.observe)
	w.Start()
	defer w.Stop()

	w.admit(sessionPayload("e1", "task-a", "sess-a"))
	w.admit(sessionPayload("e2", "task-b", "sess-b"))
	waitFor(t, "two notices", func() bool { return len(rec.snapshot()) == 2 })

	got := rec.snapshot()
	if got[0] != (observed{"task-a", "sess-a"}) || got[1] != (observed{"task-b", "sess-b"}) {
		t.Fatalf("notices = %v", got)
	}
}

func TestObserver_NoNoticeForADroppedEvent(t *testing.T) {
	repo := &fakeUsageRepo{err: context.DeadlineExceeded}
	w := NewWriter(repo, nil, nil)
	rec := &observerRecorder{}
	w.SetRecordedObserver(rec.observe)
	w.Start()
	w.admit(sessionPayload("e1", "task-a", "sess-a"))
	w.Stop()
	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("notified for a row that was never stored: %v", got)
	}
}

func TestObserver_UnsetChangesNothing(t *testing.T) {
	repo := &fakeUsageRepo{}
	w := NewWriter(repo, nil, nil)
	w.Start()
	w.admit(sessionPayload("e1", "task-a", "sess-a"))
	w.Stop()
	if repo.rowCount() != 1 {
		t.Fatalf("rows = %d", repo.rowCount())
	}
	if len(w.notices) != 0 {
		t.Fatal("a notice was queued with no observer set")
	}
}

func TestObserver_FullQueueDropsAndCountsWithoutBlockingTheWriter(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	w.SetRecordedObserver(func(context.Context, string, string) {})
	// The consumer is not started, so nothing drains the notice channel.
	for i := 0; i < observerQueueCapacity; i++ {
		w.processEvent(context.Background(), sessionPayload(fmt.Sprintf("fill-%d", i), "task-a", "sess-a"))
	}
	before := observerDropped()
	done := make(chan struct{})
	go func() {
		w.processEvent(context.Background(), sessionPayload("overflow", "task-a", "sess-a"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the writer blocked on a full observer queue")
	}
	if d := observerDropped() - before; d != 1 {
		t.Fatalf("dropped delta = %d, want 1", d)
	}
}

func TestObserver_ReplacedObserverRunsQueuedNoticesAndClearedOneDiscardsThem(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	first, second := &observerRecorder{}, &observerRecorder{}
	w.SetRecordedObserver(first.observe)
	w.processEvent(context.Background(), sessionPayload("q1", "task-a", "sess-a"))
	w.processEvent(context.Background(), sessionPayload("q2", "task-b", "sess-b"))
	w.SetRecordedObserver(second.observe)
	w.Start()
	waitFor(t, "replacement to receive queued notices", func() bool { return len(second.snapshot()) == 2 })
	if len(first.snapshot()) != 0 {
		t.Fatal("the replaced observer ran")
	}
	w.Stop()

	w2 := NewWriter(&fakeUsageRepo{}, nil, nil)
	cleared := &observerRecorder{}
	w2.SetRecordedObserver(cleared.observe)
	before := observerDropped()
	w2.processEvent(context.Background(), sessionPayload("c1", "task-a", "sess-a"))
	w2.SetRecordedObserver(nil)
	w2.Start()
	w2.Stop()
	if len(cleared.snapshot()) != 0 {
		t.Fatal("a cleared observer ran")
	}
	if observerDropped() != before {
		t.Fatal("a discard after clear was counted as a drop")
	}
}

func TestObserver_PanicIsRecoveredAndLaterNoticesStillRun(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	rec := &observerRecorder{}
	rec.fn = func(_ context.Context, taskID, _ string) {
		if taskID == "boom" {
			panic("observer exploded")
		}
	}
	w.SetRecordedObserver(rec.observe)
	w.Start()
	defer w.Stop()
	w.admit(sessionPayload("p1", "boom", "sess-a"))
	w.admit(sessionPayload("p2", "task-b", "sess-b"))
	waitFor(t, "notice after a panic", func() bool { return len(rec.snapshot()) == 2 })
}

func TestObserver_EachCallGetsA45SecondChildContext(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	got := make(chan time.Duration, 1)
	w.SetRecordedObserver(func(ctx context.Context, _, _ string) {
		deadline, ok := ctx.Deadline()
		if !ok {
			got <- -1
			return
		}
		got <- time.Until(deadline)
	})
	w.Start()
	defer w.Stop()
	w.admit(sessionPayload("d1", "task-a", "sess-a"))
	select {
	case d := <-got:
		if d < 40*time.Second || d > observerCallTimeout {
			t.Fatalf("deadline in %v, want about 45s", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("observer never ran")
	}
}

func TestObserver_StopCancelsARunningCallAndJoins(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	entered := make(chan struct{})
	returned := make(chan error, 1)
	w.SetRecordedObserver(func(ctx context.Context, _, _ string) {
		close(entered)
		<-ctx.Done()
		returned <- ctx.Err()
	})
	w.Start()
	w.admit(sessionPayload("s1", "task-a", "sess-a"))
	<-entered

	stopped := make(chan struct{})
	go func() { w.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not cancel the running observer and join")
	}
	if err := <-returned; err == nil {
		t.Fatal("the observer context was not cancelled")
	}
}

func TestObserver_StopDiscardsQueuedNoticesWithoutCountingThem(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	entered := make(chan struct{})
	var once sync.Once
	rec := &observerRecorder{}
	rec.fn = func(ctx context.Context, _, _ string) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
	}
	w.SetRecordedObserver(rec.observe)
	w.Start()
	w.admit(sessionPayload("r1", "task-a", "sess-a"))
	<-entered
	w.admit(sessionPayload("r2", "task-b", "sess-b"))
	w.admit(sessionPayload("r3", "task-c", "sess-c"))
	waitFor(t, "buffered events to be stored", func() bool { return len(w.events) == 0 })
	before := observerDropped()
	w.Stop()
	if got := rec.snapshot(); len(got) != 1 {
		t.Fatalf("observer ran %d times, queued notices must be discarded: %v", len(got), got)
	}
	if observerDropped() != before {
		t.Fatal("discarded notices were counted")
	}
}

func TestObserver_NoticesFromTheDrainStillReachTheQueueBeforeItCloses(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	rec := &observerRecorder{}
	w.SetRecordedObserver(rec.observe)
	w.Start()
	for i := 0; i < 20; i++ {
		w.admit(sessionPayload(fmt.Sprintf("dr-%d", i), "task-a", "sess-a"))
	}
	w.Stop()
	if w.notices == nil {
		t.Fatal("notice channel missing")
	}
}

func TestObserver_RegisteringAfterStopIsANoOp(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	w.Start()
	w.Stop()
	rec := &observerRecorder{}
	w.SetRecordedObserver(rec.observe)
	w.processEvent(context.Background(), sessionPayload("late", "task-a", "sess-a"))
	if len(w.notices) != 0 || len(rec.snapshot()) != 0 {
		t.Fatal("an observer registered after Stop took effect")
	}
}

func TestObserver_RepeatedStartStartsOneConsumer(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	rec := &observerRecorder{}
	w.SetRecordedObserver(rec.observe)
	w.Start()
	w.Start()
	w.admit(sessionPayload("o1", "task-a", "sess-a"))
	waitFor(t, "notice", func() bool { return len(rec.snapshot()) == 1 })
	w.Stop()
}
