package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeWatcherCounter implements WatcherTaskCounter for tests. Each test sets
// the value returned for a (metadataKey, watchID) lookup. Tests pass the
// integration name as the metadata key for brevity — the gate keys the
// counter purely by the metadata key, so the exact string only has to match
// between set() and the acquireWatcherSlot call.
type fakeWatcherCounter struct {
	mu       sync.Mutex
	counts   map[string]int
	err      error
	queryLog []string
}

func newFakeWatcherCounter() *fakeWatcherCounter {
	return &fakeWatcherCounter{counts: map[string]int{}}
}

func (f *fakeWatcherCounter) set(metadataKey, watchID string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[metadataKey+"|"+watchID] = n
}

func (f *fakeWatcherCounter) CountOpenWatcherCreatedTasks(_ context.Context, metadataKey, watchID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queryLog = append(f.queryLog, metadataKey+"|"+watchID)
	if f.err != nil {
		return 0, f.err
	}
	return f.counts[metadataKey+"|"+watchID], nil
}

// nopServiceWithCounter builds a Service with just enough wiring for the
// throttle gate. The watcher coordinator is intentionally nil — these tests
// exercise only acquireWatcherSlot.
func nopServiceWithCounter(t *testing.T, counter WatcherTaskCounter) *Service {
	t.Helper()
	return &Service{
		logger:           nopLogger(t),
		watcherTaskCount: counter,
	}
}

func ptrInt(v int) *int { return &v }

func TestAcquireWatcherSlot_NilCapBypasses(t *testing.T) {
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)

	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "watch-1", nil)
	if !ok {
		t.Fatal("expected acquire to pass when cap is nil (uncapped)")
	}
	if release == nil {
		t.Fatal("expected non-nil release func")
	}
	if len(counter.queryLog) != 0 {
		t.Fatalf("expected no DB query for uncapped watch, got %v", counter.queryLog)
	}
	release()
}

func TestAcquireWatcherSlot_EmptyWatchIDBypasses(t *testing.T) {
	s := nopServiceWithCounter(t, newFakeWatcherCounter())
	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "", ptrInt(1))
	if !ok || release == nil {
		t.Fatal("expected bypass when watch id is empty")
	}
	release()
}

func TestAcquireWatcherSlot_EmptyMetadataKeyBypasses(t *testing.T) {
	// A source that exposes no metadata key (WatchMetadataKey() == "") cannot
	// be counted, so the gate must treat the watch as uncapped rather than
	// querying with an empty key.
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)
	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "", "w-1", ptrInt(1))
	if !ok || release == nil {
		t.Fatal("expected bypass when metadata key is empty")
	}
	if len(counter.queryLog) != 0 {
		t.Fatalf("expected no DB query for empty metadata key, got %v", counter.queryLog)
	}
	release()
}

func TestAcquireWatcherSlot_UnderCapAcquires(t *testing.T) {
	counter := newFakeWatcherCounter()
	counter.set("linear", "w-1", 2)
	s := nopServiceWithCounter(t, counter)

	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(5))
	if !ok {
		t.Fatal("expected acquire to pass when count+pending < cap")
	}
	release()
}

func TestAcquireWatcherSlot_AtCapDefers(t *testing.T) {
	counter := newFakeWatcherCounter()
	counter.set("linear", "w-1", 5)
	s := nopServiceWithCounter(t, counter)

	_, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(5))
	if ok {
		t.Fatal("expected acquire to fail when count >= cap")
	}
}

func TestAcquireWatcherSlot_PendingCountedAgainstCap(t *testing.T) {
	// Carlos's burst race: two events arrive back-to-back. DB count is 0 for
	// both (no dedup row written yet). Cap is 1. Only the first should
	// acquire — the synchronous mutex + pending counter must reject the
	// second.
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)

	release1, ok1 := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if !ok1 {
		t.Fatal("expected first acquire to pass")
	}
	defer release1()

	_, ok2 := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if ok2 {
		t.Fatal("expected second acquire to defer (pending=1, cap=1)")
	}
}

func TestAcquireWatcherSlot_ReleaseRestoresSlot(t *testing.T) {
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)

	release1, ok1 := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if !ok1 {
		t.Fatal("expected first acquire to pass")
	}
	release1()

	// After release, the next event should pass (pending is back to 0).
	release2, ok2 := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if !ok2 {
		t.Fatal("expected second acquire to pass after first released")
	}
	release2()
}

func TestAcquireWatcherSlot_DifferentWatchesIsolated(t *testing.T) {
	counter := newFakeWatcherCounter()
	counter.set("linear", "w-1", 1)
	counter.set("linear", "w-2", 0)
	s := nopServiceWithCounter(t, counter)

	// w-1 is at cap, w-2 is empty — they must not share pending state.
	_, ok1 := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if ok1 {
		t.Fatal("expected w-1 to be at cap")
	}
	release2, ok2 := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-2", ptrInt(1))
	if !ok2 {
		t.Fatal("expected w-2 to be acquirable (independent watch)")
	}
	release2()
}

func TestAcquireWatcherSlot_DifferentIntegrationsIsolated(t *testing.T) {
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)

	// Acquire linear w-1 with cap=1 and HOLD it, saturating linear's pending
	// slot for that watch id.
	releaseLin, okLin := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if !okLin {
		t.Fatal("expected linear w-1 to acquire")
	}
	defer releaseLin()

	// A second linear w-1 must defer — the held slot saturates the cap.
	if _, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1)); ok {
		t.Fatal("expected second linear w-1 to defer (pending=1, cap=1)")
	}

	// jira w-1 shares the watch id but must NOT collide with linear's pending
	// state — the slot map is keyed by (integration, watchID). It must acquire.
	releaseJira, okJira := s.acquireWatcherSlot(context.Background(), "jira", "jira", "w-1", ptrInt(1))
	if !okJira {
		t.Fatal("expected jira w-1 to acquire independently of linear's held slot")
	}
	releaseJira()
}

func TestAcquireWatcherSlot_NonPositiveCapTreatedAsUncapped(t *testing.T) {
	// The API rejects <= 0 but a stale row could theoretically reach the gate
	// with 0 or negative. Treat as uncapped (fail-open) rather than block
	// everything.
	counter := newFakeWatcherCounter()
	counter.set("linear", "w-1", 100)
	s := nopServiceWithCounter(t, counter)

	for _, c := range []int{0, -1, -100} {
		release, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(c))
		if !ok {
			t.Fatalf("expected non-positive cap (%d) to bypass gate", c)
		}
		release()
	}
}

func TestAcquireWatcherSlot_CountErrorFailsOpen(t *testing.T) {
	counter := newFakeWatcherCounter()
	counter.err = errors.New("db down")
	s := nopServiceWithCounter(t, counter)

	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if !ok {
		t.Fatal("expected fail-open on count error")
	}
	release()
}

func TestAcquireWatcherSlot_NilCounterBypasses(t *testing.T) {
	// Service constructed before WatcherTaskCounter is wired must not panic.
	s := nopServiceWithCounter(t, nil)
	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-1", ptrInt(1))
	if !ok {
		t.Fatal("expected nil counter to bypass gate (fail-open)")
	}
	release()
}

// TestAcquireWatcherSlot_ConcurrentBurstRespectsCap fires N goroutines at the
// gate simultaneously with cap=1 and DB count=0. Exactly one must acquire; the
// rest must defer. Unlike the sequential dispatch test below, this exercises
// watcherMu under real contention — run with `-race` to catch a regression
// that drops the lock. Acquirers hold their slot (never release) so the cap
// stays saturated for the duration of the burst.
func TestAcquireWatcherSlot_ConcurrentBurstRespectsCap(t *testing.T) {
	const goroutines = 64
	for _, capValue := range []int{1, 5} {
		counter := newFakeWatcherCounter() // count=0 for the watch
		s := nopServiceWithCounter(t, counter)

		var acquired int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if _, ok := s.acquireWatcherSlot(context.Background(), "linear", "linear", "w-burst", ptrInt(capValue)); ok {
					atomic.AddInt64(&acquired, 1)
					// Hold the slot — do NOT release, so the cap stays full.
				}
			}()
		}
		close(start)
		wg.Wait()

		if int(acquired) != capValue {
			t.Fatalf("cap=%d: expected exactly %d acquisitions under %d concurrent callers, got %d",
				capValue, capValue, goroutines, acquired)
		}
	}
}

// TestDispatchWatcherEvent_GateBlocksBurstRace is the regression test for
// Carlos's burst race: 10 events arrive back-to-back with cap=1, the
// CreateIssueTask call blocks (simulating a slow DB write that hasn't
// committed the dedup row yet), and only ONE event must reach the
// coordinator. Without the pending counter, all 10 read count=0 and pass.
func TestDispatchWatcherEvent_GateBlocksBurstRace(t *testing.T) {
	counter := newFakeWatcherCounter() // count=0 for w-1
	src := &fakeWatcherSource{
		name:      "linear",
		reserveOK: true,
		buildReq: &IssueTaskRequest{
			WorkspaceID:    "ws-1",
			WorkflowStepID: "step-1",
		},
		watchID:          "w-1",
		metadataKey:      "linear_issue_watch_id",
		maxInflightTasks: ptrInt(1),
	}

	// Block CreateIssueTask so the admitted goroutine holds its slot for the
	// duration of the burst. `entered` / `done` are buffered to `burst` so a
	// (wrongly) over-admitted goroutine can never block on signaling — the
	// test must observe the over-admission, not deadlock on it.
	gate := make(chan struct{})
	entered := make(chan struct{}, burstSize)
	done := make(chan struct{}, burstSize)
	creator := &blockingTaskCreator{block: gate, entered: entered, done: done}

	starter := &fakeTaskStarter{}
	s := &Service{
		logger:             nopLogger(t),
		watcherTaskCount:   counter,
		issueTaskCreator:   creator,
		watcherCoordinator: newTestCoordinator(t, nil, false, starter),
	}
	s.watcherCoordinator.SetTaskCreator(creator)

	// Fire the burst back-to-back. Only one should pass the gate; the rest
	// must defer without spawning a goroutine.
	for i := 0; i < burstSize; i++ {
		s.dispatchWatcherEvent(context.Background(), src, "evt")
	}

	// Gate acquisition is synchronous (it happens before the goroutine spawns),
	// so once the burst loop returns the pending counter is final. Asserting on
	// it here catches over-admission deterministically, independent of how the
	// admitted goroutine(s) are scheduled — this is the real regression guard.
	s.watcherMu.Lock()
	pending := s.pendingByWatch["linear|w-1"]
	s.watcherMu.Unlock()
	if pending != 1 {
		t.Fatalf("expected exactly 1 slot acquired across the burst, got %d", pending)
	}

	// Gate acquisition (and thus the counter query) is synchronous within
	// dispatchWatcherEvent, so once the burst loop returns the queryLog is
	// final. Assert the counter was actually consulted with the source's
	// metadata key — this proves dispatch forwards WatchMetadataKey()
	// end-to-end rather than querying with an empty or hardcoded key.
	counter.mu.Lock()
	queryLog := append([]string(nil), counter.queryLog...)
	counter.mu.Unlock()
	if len(queryLog) == 0 {
		t.Fatal("expected the counter to be queried during the burst, got none")
	}
	for _, q := range queryLog {
		if q != "linear_issue_watch_id|w-1" {
			t.Fatalf("expected every counter query to use the source metadata key %q, got %q",
				"linear_issue_watch_id|w-1", q)
		}
	}

	// Confirm the admitted goroutine actually reached CreateIssueTask. Bounded
	// so a broken gate that admits nothing fails fast instead of hanging.
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no goroutine reached CreateIssueTask within 5s")
	}
	close(gate)

	// Wait for the admitted goroutine to finish before asserting the call
	// count, so a late over-admitted call can't slip past the assertion.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("admitted goroutine did not finish within 5s")
	}
	if got := creator.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 CreateIssueTask call (gate enforced), got %d", got)
	}
}

// burstSize is the number of events fired at the gate in the burst-race test.
const burstSize = 10

// blockingTaskCreator counts calls and blocks each one on `block` until it is
// closed. It signals `entered` when a call reaches CreateIssueTask and `done`
// when that call unblocks, so a test can wait deterministically (with a
// timeout) instead of polling. Both channels are buffered by the caller so an
// over-admitted goroutine never blocks on signaling.
type blockingTaskCreator struct {
	mu      sync.Mutex
	calls   int
	block   <-chan struct{}
	entered chan<- struct{}
	done    chan<- struct{}
}

func (b *blockingTaskCreator) CreateIssueTask(_ context.Context, _ *IssueTaskRequest) (*models.Task, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	b.entered <- struct{}{}
	<-b.block
	b.done <- struct{}{}
	return &models.Task{ID: "t-blocked"}, nil
}

func (b *blockingTaskCreator) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}
