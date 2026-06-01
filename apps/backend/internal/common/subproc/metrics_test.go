package subproc

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestThrottle_MetricsPublishedForNamedPool covers the happy path where
// /debug/vars should now show inflight + acquire counts climbing as
// callers grab slots. Without these metrics the host-freeze
// investigation has no way to tell whether a stall is throttle queue
// time or something else.
func TestThrottle_MetricsPublishedForNamedPool(t *testing.T) {
	name := "test-pool-named-" + t.Name()
	tt := NewNamedThrottle(name, 2)
	if got := metricInt(subprocCap, name); got != 2 {
		t.Fatalf("cap gauge = %d, want 2", got)
	}

	r1, err := tt.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if got := metricInt(subprocInflight, name); got != 1 {
		t.Fatalf("inflight after 1 acquire = %d, want 1", got)
	}
	if got := metricInt(subprocAcquireTotal, name); got != 1 {
		t.Fatalf("acquire_total after 1 acquire = %d, want 1", got)
	}

	r2, err := tt.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire 2: %v", err)
	}
	if got := metricInt(subprocInflight, name); got != 2 {
		t.Fatalf("inflight after 2 acquires = %d, want 2", got)
	}
	r1()
	if got := metricInt(subprocInflight, name); got != 1 {
		t.Fatalf("inflight after 1 release = %d, want 1", got)
	}
	r2()
	if got := metricInt(subprocInflight, name); got != 0 {
		t.Fatalf("inflight after both released = %d, want 0", got)
	}
}

// TestThrottle_WaitersGaugeReflectsQueueDepth is the regression test for
// the failure mode that triggered this metric: under saturation we want
// to see waiters > 0 so an operator inspecting /debug/vars knows the
// queue, not the operation itself, is the bottleneck.
func TestThrottle_WaitersGaugeReflectsQueueDepth(t *testing.T) {
	name := "test-pool-waiters-" + t.Name()
	tt := NewNamedThrottle(name, 1)

	hold, err := tt.Acquire(context.Background())
	if err != nil {
		t.Fatalf("initial acquire: %v", err)
	}
	defer hold()

	var wg sync.WaitGroup
	wg.Add(3)
	releases := make(chan func(), 3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			r, _ := tt.Acquire(context.Background())
			releases <- r
		}()
	}
	// Give the goroutines time to register as waiters. We can't use
	// synctest here because the production code calls time.Now under
	// the hood; a short real wait is acceptable for this assertion.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if metricInt(subprocWaiters, name) == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := metricInt(subprocWaiters, name); got != 3 {
		t.Fatalf("waiters gauge = %d, want 3", got)
	}

	hold()
	r1 := <-releases
	r1()
	r2 := <-releases
	r2()
	r3 := <-releases
	r3()
	wg.Wait()
	if got := metricInt(subprocWaiters, name); got != 0 {
		t.Fatalf("waiters after drain = %d, want 0", got)
	}
}

// TestThrottle_UnnamedThrottleSkipsMetrics keeps the metrics surface
// clean for tests and ad-hoc pools — the global gh/git keys would
// otherwise see counter pollution from every test file that builds a
// NewThrottle(N) sample pool.
func TestThrottle_UnnamedThrottleSkipsMetrics(t *testing.T) {
	tt := NewThrottle(1)
	before := metricInt(subprocAcquireTotal, "")
	r, err := tt.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	r()
	after := metricInt(subprocAcquireTotal, "")
	if after != before {
		t.Fatalf("acquire_total bumped on unnamed throttle (before=%d after=%d)", before, after)
	}
}
