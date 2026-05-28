package orchestrator

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeWatcherCounter implements WatcherTaskCounter for tests. Each test sets
// the value returned for a (integration, watchID) lookup.
type fakeWatcherCounter struct {
	mu       sync.Mutex
	counts   map[string]int
	err      error
	queryLog []string
}

func newFakeWatcherCounter() *fakeWatcherCounter {
	return &fakeWatcherCounter{counts: map[string]int{}}
}

func (f *fakeWatcherCounter) set(integration, watchID string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[integration+"|"+watchID] = n
}

func (f *fakeWatcherCounter) CountOpenWatcherCreatedTasks(_ context.Context, integration, watchID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queryLog = append(f.queryLog, integration+"|"+watchID)
	if f.err != nil {
		return 0, f.err
	}
	return f.counts[integration+"|"+watchID], nil
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

	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "watch-1", nil)
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
	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "", ptrInt(1))
	if !ok || release == nil {
		t.Fatal("expected bypass when watch id is empty")
	}
	release()
}

func TestAcquireWatcherSlot_UnderCapAcquires(t *testing.T) {
	counter := newFakeWatcherCounter()
	counter.set("linear", "w-1", 2)
	s := nopServiceWithCounter(t, counter)

	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(5))
	if !ok {
		t.Fatal("expected acquire to pass when count+pending < cap")
	}
	release()
}

func TestAcquireWatcherSlot_AtCapDefers(t *testing.T) {
	counter := newFakeWatcherCounter()
	counter.set("linear", "w-1", 5)
	s := nopServiceWithCounter(t, counter)

	_, ok := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(5))
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

	release1, ok1 := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
	if !ok1 {
		t.Fatal("expected first acquire to pass")
	}
	defer release1()

	_, ok2 := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
	if ok2 {
		t.Fatal("expected second acquire to defer (pending=1, cap=1)")
	}
}

func TestAcquireWatcherSlot_ReleaseRestoresSlot(t *testing.T) {
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)

	release1, ok1 := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
	if !ok1 {
		t.Fatal("expected first acquire to pass")
	}
	release1()

	// After release, the next event should pass (pending is back to 0).
	release2, ok2 := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
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
	_, ok1 := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
	if ok1 {
		t.Fatal("expected w-1 to be at cap")
	}
	release2, ok2 := s.acquireWatcherSlot(context.Background(), "linear", "w-2", ptrInt(1))
	if !ok2 {
		t.Fatal("expected w-2 to be acquirable (independent watch)")
	}
	release2()
}

func TestAcquireWatcherSlot_DifferentIntegrationsIsolated(t *testing.T) {
	counter := newFakeWatcherCounter()
	s := nopServiceWithCounter(t, counter)

	// Same watch id across two integrations must NOT collide.
	releaseLin, okLin := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
	if !okLin {
		t.Fatal("expected linear w-1 to acquire")
	}
	defer releaseLin()

	releaseJira, okJira := s.acquireWatcherSlot(context.Background(), "jira", "w-1", ptrInt(1))
	if !okJira {
		t.Fatal("expected jira w-1 to acquire independently")
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
		release, ok := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(c))
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

	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
	if !ok {
		t.Fatal("expected fail-open on count error")
	}
	release()
}

func TestAcquireWatcherSlot_NilCounterBypasses(t *testing.T) {
	// Service constructed before WatcherTaskCounter is wired must not panic.
	s := nopServiceWithCounter(t, nil)
	release, ok := s.acquireWatcherSlot(context.Background(), "linear", "w-1", ptrInt(1))
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
				if _, ok := s.acquireWatcherSlot(context.Background(), "linear", "w-burst", ptrInt(capValue)); ok {
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
		maxInflightTasks: ptrInt(1),
	}

	// Block CreateIssueTask so the first goroutine holds its slot for the
	// duration of the test. Without this, the first goroutine could
	// complete and release before the next handler enters, masking the bug.
	gate := make(chan struct{})
	creator := &blockingTaskCreator{calls: 0, block: gate}

	starter := &fakeTaskStarter{}
	s := &Service{
		logger:             nopLogger(t),
		watcherTaskCount:   counter,
		issueTaskCreator:   creator,
		watcherCoordinator: newTestCoordinator(t, nil, false, starter),
	}
	s.watcherCoordinator.SetTaskCreator(creator)

	// Fire 10 events back-to-back. Only one should pass the gate; the rest
	// must defer without spawning a goroutine.
	const burst = 10
	for i := 0; i < burst; i++ {
		s.dispatchWatcherEvent(context.Background(), "linear", src, "evt")
	}

	// Drain the in-flight goroutine.
	close(gate)
	// The coordinator runs synchronously inside the goroutine — we don't have
	// a clean wait primitive here, so wait until the in-flight count drops
	// back to 0 (release() has run).
	waitForPendingDrain(t, s, "linear|w-1")

	if creator.calls != 1 {
		t.Fatalf("expected exactly 1 CreateIssueTask call (gate enforced), got %d", creator.calls)
	}
}

// blockingTaskCreator counts calls and blocks each one on the provided
// channel until it's closed. Used to keep the first dispatch goroutine
// holding its pending slot while subsequent events race the gate.
type blockingTaskCreator struct {
	mu    sync.Mutex
	calls int
	block <-chan struct{}
}

func (b *blockingTaskCreator) CreateIssueTask(_ context.Context, _ *IssueTaskRequest) (*models.Task, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	<-b.block
	return &models.Task{ID: "t-blocked"}, nil
}

// waitForPendingDrain spins until the named watch's pending counter reaches 0
// or the test deadline expires. The pending map deletes empty entries, so we
// check membership.
func waitForPendingDrain(t *testing.T, s *Service, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.watcherMu.Lock()
		_, stillPending := s.pendingByWatch[key]
		s.watcherMu.Unlock()
		if !stillPending {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("timed out waiting for pending counter to drain for %s", key)
}
