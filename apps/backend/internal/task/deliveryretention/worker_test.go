package deliveryretention

import (
	"context"
	"errors"
	"expvar"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/maintenance"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestDeliveryRetentionWorker(t *testing.T) {
	t.Run("starts immediately and can restart", func(t *testing.T) {
		clock := newFakeClock()
		repo := &workerRepository{calls: make(chan string, 8)}
		pool := db.NewPool(nil, nil)
		worker := New(repo, pool, Options{After: clock.After, OnSnapshot: func(Snapshot) {}})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if name := receiveCall(t, repo.calls); name != "retention" {
			t.Fatalf("first repository call = %q", name)
		}
		if delay := receiveDelay(t, clock); delay != defaultInterval {
			t.Fatalf("idle delay = %s, want %s", delay, defaultInterval)
		}
		worker.Stop()
		if got := repo.activeCalls(); got != 0 {
			t.Fatalf("active calls after Stop = %d", got)
		}

		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if delay := receiveDelay(t, clock); delay != defaultInterval {
			t.Fatalf("restart idle delay = %s, want %s", delay, defaultInterval)
		}
		worker.Stop()
	})

	t.Run("yields when maintenance is busy", func(t *testing.T) {
		clock := newFakeClock()
		pool := db.NewPool(nil, nil)
		repo := &workerRepository{calls: make(chan string, 8)}
		snapshots := make(chan Snapshot, 1)
		worker := New(repo, pool, Options{After: clock.After, OnSnapshot: func(s Snapshot) { snapshots <- s }})
		release, ok := maintenance.ForPool(pool).TryAcquire()
		if !ok {
			t.Fatal("failed to acquire maintenance guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		snapshot := receiveSnapshot(t, snapshots)
		if snapshot.BusyBatches == 0 {
			t.Fatalf("busy batches = %d, want at least one", snapshot.BusyBatches)
		}
		select {
		case call := <-repo.calls:
			t.Fatalf("repository called while maintenance held: %q", call)
		default:
		}
		release()
		worker.Stop()
	})

	t.Run("stop cancels and joins an admitted batch", func(t *testing.T) {
		clock := newFakeClock()
		repo := &workerRepository{calls: make(chan string, 8), blockUntilCanceled: true}
		worker := New(repo, db.NewPool(nil, nil), Options{After: clock.After})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		_ = receiveCall(t, repo.calls)
		worker.Stop()
		if got := repo.activeCalls(); got != 0 {
			t.Fatalf("active calls after Stop = %d", got)
		}
		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		worker.Stop()
	})

	t.Run("structural errors suspend destructive phases until readiness recovers", func(t *testing.T) {
		clock := newFakeClock()
		repo := &workerRepository{
			calls:           make(chan string, 8),
			retentionErrors: []error{errors.New("unsupported retention format"), nil},
		}
		var readinessCalls int
		snapshots := make(chan Snapshot, 4)
		worker := New(repo, db.NewPool(nil, nil), Options{
			After:      clock.After,
			RetryDelay: func(int) time.Duration { return 3 * time.Second },
			Readiness: func(context.Context) error {
				readinessCalls++
				return nil
			},
			OnSnapshot: func(snapshot Snapshot) { snapshots <- snapshot },
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if got := receiveCall(t, repo.calls); got != "retention" {
			t.Fatalf("first batch = %q", got)
		}
		failed := receiveSnapshot(t, snapshots)
		if !failed.StructuralFailure || failed.FailureReason != "format" {
			t.Fatalf("structural failure snapshot = %+v", failed)
		}
		retryTimer := receiveTimer(t, clock)
		if retryTimer.delay != 3*time.Second {
			t.Fatalf("readiness retry delay = %s", retryTimer.delay)
		}
		worker.Stop()
		if err := worker.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if got := receiveCall(t, repo.calls); got != "retention" {
			t.Fatalf("first post-readiness batch = %q", got)
		}
		for _, phase := range []string{"orphans", "retirements"} {
			if got := receiveCall(t, repo.calls); got != phase {
				t.Fatalf("post-readiness phase = %q, want %q", got, phase)
			}
		}
		if recovered := receiveSnapshot(t, snapshots); !recovered.ReadinessRecovered {
			t.Fatalf("readiness snapshot = %+v", recovered)
		}
		if readinessCalls != 1 {
			t.Fatalf("readiness checks = %d, want 1", readinessCalls)
		}
		select {
		case got := <-repo.calls:
			t.Fatalf("structural failure did not stop the pass before later phases: %q", got)
		default:
		}
		worker.Stop()
	})
}

func TestDeliveryRetentionRetryBackoff(t *testing.T) {
	for attempt := 1; attempt <= 12; attempt++ {
		delay := jitteredRetryDelay(attempt)
		bound := time.Second << (attempt - 1)
		if bound > maxRetryBackoff {
			bound = maxRetryBackoff
		}
		if delay < bound/2 || delay > bound {
			t.Fatalf("attempt %d delay = %s, want between %s and %s", attempt, delay, bound/2, bound)
		}
	}
}

type workerRepository struct {
	calls              chan string
	blockUntilCanceled bool
	active             int
	activeMu           sync.Mutex
	retentionResult    models.AgentDeliveryRetentionBatchResult
	retentionErrors    []error
}

func (r *workerRepository) run(ctx context.Context, name string) error {
	r.activeMu.Lock()
	r.active++
	r.activeMu.Unlock()
	defer func() {
		r.activeMu.Lock()
		r.active--
		r.activeMu.Unlock()
	}()
	r.calls <- name
	if r.blockUntilCanceled {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (r *workerRepository) activeCalls() int {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	return r.active
}

func (r *workerRepository) RunAgentDeliveryRetentionBatch(ctx context.Context, _ models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	err := r.run(ctx, "retention")
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	if err == nil && len(r.retentionErrors) > 0 {
		err = r.retentionErrors[0]
		r.retentionErrors = r.retentionErrors[1:]
	}
	return r.retentionResult, err
}

func (r *workerRepository) RunAgentDeliveryOrphanBatch(ctx context.Context, _ models.AgentDeliveryOrphanBatchOptions) (models.AgentDeliveryOrphanBatchResult, error) {
	return models.AgentDeliveryOrphanBatchResult{}, r.run(ctx, "orphans")
}

func (r *workerRepository) RunAgentDeliveryRetirementBatch(ctx context.Context, _ models.AgentDeliveryRetirementBatchOptions) (models.AgentDeliveryRetirementBatchResult, error) {
	return models.AgentDeliveryRetirementBatchResult{}, r.run(ctx, "retirements")
}

type fakeClock struct {
	requests chan fakeTimer
}

type fakeTimer struct {
	delay time.Duration
	fire  chan time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{requests: make(chan fakeTimer, 32)} }

func (c *fakeClock) After(delay time.Duration) <-chan time.Time {
	request := fakeTimer{delay: delay, fire: make(chan time.Time, 1)}
	c.requests <- request
	return request.fire
}

func fireAfter(t *testing.T, clock *fakeClock, want time.Duration) {
	t.Helper()
	request := receiveTimer(t, clock)
	if request.delay != want {
		t.Fatalf("timer delay = %s, want %s", request.delay, want)
	}
	request.fire <- time.Now()
}

func receiveDelay(t *testing.T, clock *fakeClock) time.Duration {
	t.Helper()
	return receiveTimer(t, clock).delay
}

func receiveTimer(t *testing.T, clock *fakeClock) fakeTimer {
	t.Helper()
	select {
	case request := <-clock.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("worker did not request a timer")
		return fakeTimer{}
	}
}

func receiveCall(t *testing.T, calls <-chan string) string {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("worker did not call repository")
		return ""
	}
}

func receiveSnapshot(t *testing.T, snapshots <-chan Snapshot) Snapshot {
	t.Helper()
	select {
	case snapshot := <-snapshots:
		return snapshot
	case <-time.After(time.Second):
		t.Fatal("worker did not report pass snapshot")
		return Snapshot{}
	}
}

func TestDeliveryRetentionMetrics(t *testing.T) {
	beforeRows := metricValue(rowsTotal, "inbox_payload")
	beforeBytes := metricValue(bytesTotal, "inbox")
	var snapshot Snapshot
	productive := &workerRepository{calls: make(chan string, 1), retentionResult: models.AgentDeliveryRetentionBatchResult{
		Candidates: 1, EventPayloadsPruned: 1, PayloadBytesPruned: 256,
	}}
	worker := New(productive, db.NewPool(nil, nil), Options{})
	if _, err := worker.runBatch(t.Context(), "retention", &snapshot); err != nil {
		t.Fatal(err)
	}
	if got := metricValue(rowsTotal, "inbox_payload"); got != beforeRows+1 {
		t.Fatalf("committed row counter = %d, want %d", got, beforeRows+1)
	}
	if got := metricValue(bytesTotal, "inbox"); got != beforeBytes+256 {
		t.Fatalf("committed byte counter = %d, want %d", got, beforeBytes+256)
	}
	if lastSuccess.Value() == 0 {
		t.Fatal("successful batch did not update last-success time")
	}
	worker.Stop()
}

func metricValue(metrics *expvar.Map, key string) int64 {
	value, ok := metrics.Get(key).(*expvar.Int)
	if !ok {
		return 0
	}
	return value.Value()
}

func TestDeliveryRetentionQuiescence(t *testing.T) {
	clock := newFakeClock()
	repo := &workerRepository{calls: make(chan string, 1), blockUntilCanceled: true}
	worker := New(repo, db.NewPool(nil, nil), Options{After: clock.After})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_ = receiveCall(t, repo.calls)
	done := make(chan struct{})
	go func() { worker.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not join the canceled repository batch")
	}
	if got := repo.activeCalls(); got != 0 {
		t.Fatalf("active repository calls after quiescence = %d", got)
	}
}
