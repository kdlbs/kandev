package github

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireGHSlot_BlocksWhenSaturated(t *testing.T) {
	restore := setGHSemaphoreForTest(2)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Hold both slots.
	releaseA, errA := acquireGHSlot(ctx)
	if errA != nil {
		t.Fatalf("acquire A: %v", errA)
	}
	releaseB, errB := acquireGHSlot(ctx)
	if errB != nil {
		t.Fatalf("acquire B: %v", errB)
	}

	// Third caller must block until a slot frees.
	acquired := make(chan struct{})
	var releaseC func()
	go func() {
		var err error
		releaseC, err = acquireGHSlot(ctx)
		if err != nil {
			t.Errorf("acquire C: unexpected err %v", err)
			return
		}
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatalf("third acquire returned without waiting; pool size 2 not enforced")
	case <-time.After(50 * time.Millisecond):
	}

	releaseA()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatalf("third acquire did not return after slot freed")
	}
	releaseB()
	releaseC()
}

func TestAcquireGHSlot_ContextCancelledWhileQueued(t *testing.T) {
	restore := setGHSemaphoreForTest(1)
	defer restore()

	// Saturate the single slot.
	holdCtx, holdCancel := context.WithCancel(context.Background())
	defer holdCancel()
	release, err := acquireGHSlot(holdCtx)
	if err != nil {
		t.Fatalf("acquire hold: %v", err)
	}
	defer release()

	// Queue a second caller with a short-lived context.
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	relQueued, err := acquireGHSlot(waitCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	// Release fn must be safe to call even on error so callers can `defer` it
	// unconditionally without leaking the held slot.
	relQueued()
}

func TestAcquireGHSlot_ConcurrentCapEnforced(t *testing.T) {
	const cap = 4
	const callers = 16
	restore := setGHSemaphoreForTest(cap)
	defer restore()

	var inFlight, peak int32
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := acquireGHSlot(context.Background())
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			defer release()
			cur := atomic.AddInt32(&inFlight, 1)
			// Track peak concurrency.
			for {
				old := atomic.LoadInt32(&peak)
				if cur <= old || atomic.CompareAndSwapInt32(&peak, old, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got > int32(cap) {
		t.Errorf("peak concurrency %d exceeded cap %d", got, cap)
	}
}

// TestAcquireGHSlot_DoubleReleaseIsNoOp regression-tests that a release
// closure called twice does not transfer a slot to another in-flight
// holder. Without sync.Once around the release, a double-defer (or a
// refactor that calls release in both an error path and via defer)
// would slowly drain the effective cap to zero.
func TestAcquireGHSlot_DoubleReleaseIsNoOp(t *testing.T) {
	restore := setGHSemaphoreForTest(2)
	defer restore()

	ctx := context.Background()
	releaseA, err := acquireGHSlot(ctx)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	releaseB, err := acquireGHSlot(ctx)
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}

	// Caller A releases twice — buggy code path. With sync.Once the second
	// call is a no-op; without it the second call would steal B's slot.
	releaseA()
	releaseA()

	// Pool should still have exactly one slot held (by B). A new acquire
	// must succeed (slot vacated by A) and a second concurrent acquire
	// must block (B still holds the other slot).
	releaseC, err := acquireGHSlot(ctx)
	if err != nil {
		t.Fatalf("acquire C after double release: %v", err)
	}
	defer releaseC()

	blocked := make(chan struct{})
	go func() {
		release, err := acquireGHSlot(ctx)
		if err == nil {
			release()
		}
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Errorf("third acquire returned without waiting — double release leaked a slot")
	case <-time.After(50 * time.Millisecond):
	}
	releaseB()
	<-blocked
}

// TestAcquireGHSlot_PreCancelledContextReturnsImmediately asserts the
// fast path: when ctx is already done at entry, we don't race the
// select against a free slot. Without the precheck the select would
// pick randomly between sem<- and ctx.Done(), letting a cancelled
// caller still acquire then fail downstream.
func TestAcquireGHSlot_PreCancelledContextReturnsImmediately(t *testing.T) {
	restore := setGHSemaphoreForTest(8)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for i := 0; i < 100; i++ {
		release, err := acquireGHSlot(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("iter %d: err = %v, want context.Canceled", i, err)
		}
		release() // must be safe even on the error path
	}
}

// Cap parsing tests live in internal/common/subproc/shared_test.go now
// that the env var, default, and resolver are owned by that package.
