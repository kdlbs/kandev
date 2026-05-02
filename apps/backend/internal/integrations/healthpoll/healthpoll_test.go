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
	mu     sync.Mutex
	listFn func(ctx context.Context) ([]string, error)
	probed []string
}

func (f *fakeProber) ListConfiguredWorkspaces(ctx context.Context) ([]string, error) {
	if f.listFn != nil {
		return f.listFn(ctx)
	}
	return nil, nil
}

func (f *fakeProber) RecordAuthHealth(_ context.Context, workspaceID string) {
	f.mu.Lock()
	f.probed = append(f.probed, workspaceID)
	f.mu.Unlock()
}

func TestProbeAll_RecordsEachWorkspace(t *testing.T) {
	p := &fakeProber{
		listFn: func(_ context.Context) ([]string, error) {
			return []string{"ws-a", "ws-b"}, nil
		},
	}
	poller := New("test", p, logger.Default())

	poller.probeAll(context.Background())

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.probed) != 2 || p.probed[0] != "ws-a" || p.probed[1] != "ws-b" {
		t.Errorf("expected [ws-a ws-b], got %v", p.probed)
	}
}

func TestProbeAll_ListError_DoesNotPanic(t *testing.T) {
	p := &fakeProber{
		listFn: func(_ context.Context) ([]string, error) {
			return nil, errors.New("db down")
		},
	}
	poller := New("test", p, logger.Default())

	poller.probeAll(context.Background()) // expect: silent recovery + warning log

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.probed) != 0 {
		t.Errorf("no probes should run on list error, got %v", p.probed)
	}
}

func TestProbeAll_StopsWhenContextCancelled(t *testing.T) {
	p := &fakeProber{
		listFn: func(_ context.Context) ([]string, error) {
			return []string{"ws-a", "ws-b", "ws-c"}, nil
		},
	}
	poller := New("test", p, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the loop runs
	poller.probeAll(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.probed) != 0 {
		t.Errorf("expected no probes after cancel, got %v", p.probed)
	}
}

func TestStart_RunsImmediateProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &fakeProber{
			listFn: func(_ context.Context) ([]string, error) {
				return []string{"ws-1"}, nil
			},
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
		if len(p.probed) == 0 {
			t.Error("Start did not run an immediate probe")
		}
	})
}

func TestStart_IsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls int32
		p := &fakeProber{
			listFn: func(_ context.Context) ([]string, error) {
				atomic.AddInt32(&calls, 1)
				return []string{"ws-1"}, nil
			},
		}
		poller := New("test", p, logger.Default())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		poller.Start(ctx)
		poller.Start(ctx) // second call must be a no-op

		// Wait for the immediate-on-Start probe pass to finish. If the
		// second Start spawned a parallel loop we'd see two list calls.
		synctest.Wait()
		poller.Stop()

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("expected exactly 1 list call from initial probe, got %d", got)
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
