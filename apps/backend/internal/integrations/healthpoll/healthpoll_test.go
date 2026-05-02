package healthpoll

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/common/logger"
)

type fakeProber struct {
	mu          sync.Mutex
	hasFn       func(ctx context.Context) (bool, error)
	probedCount int
}

func (f *fakeProber) HasConfig(ctx context.Context) (bool, error) {
	if f.hasFn != nil {
		return f.hasFn(ctx)
	}
	return false, nil
}

func (f *fakeProber) RecordAuthHealth(_ context.Context) {
	f.mu.Lock()
	f.probedCount++
	f.mu.Unlock()
}

func TestProbeAll_RecordsWhenConfigured(t *testing.T) {
	p := &fakeProber{
		hasFn: func(_ context.Context) (bool, error) { return true, nil },
	}
	poller := New("test", p, logger.Default())

	poller.probeAll(context.Background())

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probedCount != 1 {
		t.Errorf("expected 1 probe, got %d", p.probedCount)
	}
}

func TestProbeAll_SkipsWhenNotConfigured(t *testing.T) {
	p := &fakeProber{
		hasFn: func(_ context.Context) (bool, error) { return false, nil },
	}
	poller := New("test", p, logger.Default())

	poller.probeAll(context.Background())

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probedCount != 0 {
		t.Errorf("expected no probe when not configured, got %d", p.probedCount)
	}
}

func TestProbeAll_HasConfigError_DoesNotPanic(t *testing.T) {
	p := &fakeProber{
		hasFn: func(_ context.Context) (bool, error) { return false, errors.New("db down") },
	}
	poller := New("test", p, logger.Default())

	poller.probeAll(context.Background()) // expect: silent recovery + warning log

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probedCount != 0 {
		t.Errorf("no probe should run on has-config error, got %d", p.probedCount)
	}
}

func TestProbeAll_StopsWhenContextCancelled(t *testing.T) {
	p := &fakeProber{
		hasFn: func(_ context.Context) (bool, error) { return true, nil },
	}
	poller := New("test", p, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the loop runs
	poller.probeAll(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probedCount != 0 {
		t.Errorf("expected no probe after cancel, got %d", p.probedCount)
	}
}

func TestStart_RunsImmediateProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &fakeProber{
			hasFn: func(_ context.Context) (bool, error) { return true, nil },
		}
		poller := New("test", p, logger.Default())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		poller.Start(ctx)
		defer poller.Stop()

		// synctest.Wait advances fake time until the spawned goroutine
		// finishes its immediate-on-Start probe and parks on the ticker.
		synctest.Wait()

		p.mu.Lock()
		defer p.mu.Unlock()
		if p.probedCount == 0 {
			t.Error("Start did not run an immediate probe")
		}
	})
}

func TestStart_IsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls int32
		p := &fakeProber{
			hasFn: func(_ context.Context) (bool, error) {
				atomic.AddInt32(&calls, 1)
				return true, nil
			},
		}
		poller := New("test", p, logger.Default())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		poller.Start(ctx)
		poller.Start(ctx) // second call must be a no-op

		// Wait for the immediate-on-Start probe pass to finish. If the
		// second Start spawned a parallel loop we'd see two has-config calls.
		synctest.Wait()
		poller.Stop()

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("expected exactly 1 has-config call from initial probe, got %d", got)
		}
	})
}

func TestStop_BeforeStart_IsNoOp(t *testing.T) {
	poller := New("test", &fakeProber{}, logger.Default())
	poller.Stop() // must not block or panic
}

func TestStop_NilProber_IsNoOp(t *testing.T) {
	poller := New("test", nil, logger.Default())
	poller.Start(context.Background()) // nil prober: no goroutine spawned
	poller.Stop()                      // must not block
}
