package reachability

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeRepository is an in-memory Repository double for poller tests: it
// isolates poller mechanics (scheduling, concurrency, lifecycle) from the
// SQL-backed hysteresis contract, which store_test.go already covers
// end-to-end against a real database.
type fakeRepository struct {
	mu        sync.Mutex
	executors []*models.Executor
	records   map[string]*models.ExecutorReachability
	listErr   error
	upsertErr error

	upsertCalls int32
}

// errRepoRefused simulates a repository-level write failure — a test double
// for the last-write-wins WHERE clause silently refusing a write, without
// needing a real SQLite race to reproduce it.
var errRepoRefused = errors.New("write refused")

func newFakeRepository(executors ...*models.Executor) *fakeRepository {
	return &fakeRepository{executors: executors, records: map[string]*models.ExecutorReachability{}}
}

func (f *fakeRepository) ListSSHExecutorsForReachability(context.Context) ([]*models.Executor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := append([]*models.Executor(nil), f.executors...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepository) GetExecutorReachability(_ context.Context, executorID string) (*models.ExecutorReachability, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[executorID]
	if !ok {
		return nil, models.ErrExecutorReachabilityNotFound
	}
	cp := *rec
	return &cp, nil
}

func (f *fakeRepository) UpsertExecutorReachability(_ context.Context, obs models.ExecutorReachabilityObservation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	atomic.AddInt32(&f.upsertCalls, 1)
	if f.upsertErr != nil {
		return f.upsertErr
	}
	state := obs.InitialState
	failures := obs.InitialFailures
	if existing, ok := f.records[obs.ExecutorID]; ok {
		if obs.Reason == "" {
			failures = 0
			state = models.ExecutorReachabilityStateReachable
		} else {
			failures = existing.ConsecutiveFailures + 1
			state = existing.State
			if isStickyReason(obs.Reason) || failures >= obs.FailureThreshold {
				state = models.ExecutorReachabilityStateUnreachable
			}
		}
	}
	f.records[obs.ExecutorID] = &models.ExecutorReachability{
		ExecutorID: obs.ExecutorID, State: state, Reason: obs.Reason, Message: obs.Message,
		ConsecutiveFailures: failures, Host: obs.Host, CheckedAt: timePtr(obs.CheckedAt), UpdatedAt: time.Now().UTC(),
	}
	return nil
}

func (f *fakeRepository) ResetExecutorReachability(_ context.Context, executorID, host string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[executorID] = &models.ExecutorReachability{ExecutorID: executorID, State: models.ExecutorReachabilityStateUnknown, Host: host, UpdatedAt: time.Now().UTC()}
	return nil
}

func (f *fakeRepository) upsertCount() int32 { return atomic.LoadInt32(&f.upsertCalls) }

func timePtr(t time.Time) *time.Time { return &t }

func sshExecutor(id string) *models.Executor {
	return &models.Executor{ID: id, Name: id, Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive, Config: map[string]string{"ssh_host": id}}
}

// concurrencyProbe blocks on a gate until released, tracking the maximum
// number of simultaneous in-flight calls it observed.
type concurrencyProbe struct {
	mu       sync.Mutex
	inFlight int
	maxSeen  int
	release  chan struct{}
}

func newConcurrencyProbe() *concurrencyProbe {
	return &concurrencyProbe{release: make(chan struct{})}
}

func (c *concurrencyProbe) probe(ctx context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.maxSeen {
		c.maxSeen = c.inFlight
	}
	c.mu.Unlock()

	select {
	case <-c.release:
	case <-ctx.Done():
		c.mu.Lock()
		c.inFlight--
		c.mu.Unlock()
		return lifecycle.SSHProbeOutcome{Host: executor.ID, Cancelled: true}
	}

	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
	return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
}

func TestPollerStart_RunsImmediatePass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"), sshExecutor("b"))
		p := New(repo, 60, logger.Default())
		p.probe = func(_ context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
			return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)
		defer p.Stop()

		synctest.Wait()

		if got := repo.upsertCount(); got != 2 {
			t.Fatalf("upsert calls = %d, want 2 (one immediate pass over two executors)", got)
		}
	})
}

func TestPollerStart_IsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"))
		p := New(repo, 60, logger.Default())
		p.probe = func(_ context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
			return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)
		p.Start(ctx) // second call must be a no-op

		synctest.Wait()
		p.Stop()

		if got := repo.upsertCount(); got != 1 {
			t.Fatalf("upsert calls = %d, want exactly 1 (a second Start must not spawn a parallel loop)", got)
		}
	})
}

func TestPollerStop_BeforeStart_IsNoOp(t *testing.T) {
	p := New(newFakeRepository(), 60, logger.Default())
	p.Stop() // must not block or panic
}

func TestPollerZeroInterval_DisablesScheduledPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"))
		p := New(repo, 0, logger.Default())
		p.probe = func(_ context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
			return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)
		defer p.Stop()

		synctest.Wait()
		// Advance well past what would have been several ticks at the
		// default interval, to prove no pass ever runs.
		time.Sleep(10 * time.Minute)
		synctest.Wait()

		if got := repo.upsertCount(); got != 0 {
			t.Fatalf("upsert calls = %d, want 0 (interval=0 must run no scheduled pass)", got)
		}
	})
}

func TestPollerZeroInterval_ProbeNowStillWorks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"))
		p := New(repo, 0, logger.Default())
		p.probe = func(_ context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
			return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)
		defer p.Stop()
		synctest.Wait()

		if ok := p.ProbeNow(sshExecutor("a")); !ok {
			t.Fatal("ProbeNow returned false while the poller is started")
		}
		synctest.Wait()

		if got := repo.upsertCount(); got != 1 {
			t.Fatalf("upsert calls = %d, want 1 from the off-cycle probe", got)
		}
	})
}

func TestPollerPass_NeverExceedsPassConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var executors []*models.Executor
		for i := 0; i < passConcurrency+3; i++ {
			executors = append(executors, sshExecutor(string(rune('a'+i))))
		}
		repo := newFakeRepository(executors...)
		p := New(repo, 60, logger.Default())
		cp := newConcurrencyProbe()
		p.probe = cp.probe

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)

		// Let every worker reach the gate.
		synctest.Wait()
		close(cp.release)
		p.Stop()

		cp.mu.Lock()
		defer cp.mu.Unlock()
		if cp.maxSeen > passConcurrency {
			t.Fatalf("max simultaneous probes = %d, want at most passConcurrency (%d)", cp.maxSeen, passConcurrency)
		}
		if cp.maxSeen != passConcurrency {
			t.Fatalf("max simultaneous probes = %d, want exactly passConcurrency (%d) since more executors than slots were queued", cp.maxSeen, passConcurrency)
		}
	})
}

func TestPollerPass_OverlappingTickIsSkippedNotQueued(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"))
		p := New(repo, 60, logger.Default())
		cp := newConcurrencyProbe()
		p.probe = cp.probe

		before := passSkippedTotal.Value()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)
		defer func() {
			close(cp.release)
			p.Stop()
		}()

		// The immediate pass is now blocked on the probe gate. Advance past
		// a full tick while it is still running.
		synctest.Wait()
		time.Sleep(61 * time.Second)
		synctest.Wait()

		if got := passSkippedTotal.Value() - before; got != 1 {
			t.Fatalf("pass_skipped_total delta = %d, want 1 (the overlapping tick must be dropped, not queued)", got)
		}
	})
}

func TestPollerStop_DiscardsCancelledProbeResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"))
		p := New(repo, 60, logger.Default())
		cp := newConcurrencyProbe()
		p.probe = cp.probe

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)

		// The immediate pass is now blocked on the probe gate (never
		// released), so Stop must cancel it in flight.
		synctest.Wait()
		p.Stop()

		if got := repo.upsertCount(); got != 0 {
			t.Fatalf("upsert calls = %d, want 0 — a probe cancelled by Stop must never be written", got)
		}
	})
}

func TestPollerProbeNow_RefusedAfterStop(t *testing.T) {
	repo := newFakeRepository(sshExecutor("a"))
	p := New(repo, 60, logger.Default())
	p.probe = func(_ context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
		return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	p.Stop()

	if ok := p.ProbeNow(sshExecutor("a")); ok {
		t.Fatal("ProbeNow returned true after Stop; a stopped poller must refuse new work")
	}
}

func TestPollerListFailure_AbandonsPassWithoutWriting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newFakeRepository(sshExecutor("a"))
		repo.listErr = context.DeadlineExceeded
		p := New(repo, 60, logger.Default())
		p.probe = func(_ context.Context, executor *models.Executor) lifecycle.SSHProbeOutcome {
			return lifecycle.SSHProbeOutcome{Host: executor.ID, Success: true}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.Start(ctx)
		defer p.Stop()
		synctest.Wait()

		if got := repo.upsertCount(); got != 0 {
			t.Fatalf("upsert calls = %d, want 0 when listing executors fails", got)
		}
	})
}
