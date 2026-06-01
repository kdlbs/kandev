package subproc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThrottle_BlocksPastCap(t *testing.T) {
	tt := NewThrottle(2)
	ctx := context.Background()

	rA, err := tt.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	rB, err := tt.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}

	blocked := make(chan struct{})
	go func() {
		r, _ := tt.Acquire(ctx)
		defer r()
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Errorf("third acquire returned without waiting — throttle did not block")
	case <-time.After(50 * time.Millisecond):
	}
	rA()
	<-blocked
	rB()
}

func TestThrottle_ContextCancelWhileQueued(t *testing.T) {
	tt := NewThrottle(1)
	ctx := context.Background()
	r, err := tt.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer r()

	cctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := tt.Acquire(cctx)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("acquire never returned after cancel")
	}
}

func TestThrottle_DoubleReleaseIsNoOp(t *testing.T) {
	tt := NewThrottle(2)
	ctx := context.Background()
	rA, _ := tt.Acquire(ctx)
	rB, _ := tt.Acquire(ctx)

	rA()
	rA() // double release must be a no-op

	rC, err := tt.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire after double release: %v", err)
	}
	defer rC()

	blocked := make(chan struct{})
	go func() {
		r, _ := tt.Acquire(ctx)
		defer r()
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Errorf("4th acquire ran — double release leaked a slot")
	case <-time.After(50 * time.Millisecond):
	}
	rB()
	<-blocked
}

func TestThrottle_PeakConcurrencyHonoured(t *testing.T) {
	tt := NewThrottle(4)
	var active, peak int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := tt.Acquire(context.Background())
			if err != nil {
				return
			}
			defer r()
			cur := atomic.AddInt64(&active, 1)
			for {
				p := atomic.LoadInt64(&peak)
				if cur <= p || atomic.CompareAndSwapInt64(&peak, p, cur) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt64(&active, -1)
		}()
	}
	wg.Wait()
	if peak > 4 {
		t.Errorf("peak concurrency = %d, want <= 4", peak)
	}
}

func TestThrottle_DisabledWhenCapZero(t *testing.T) {
	tt := NewThrottle(0)
	r, err := tt.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire on disabled throttle: %v", err)
	}
	r()
}

func TestThrottle_PreCancelledContextReturnsImmediately(t *testing.T) {
	tt := NewThrottle(8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 50; i++ {
		release, err := tt.Acquire(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("iter %d: err = %v, want context.Canceled", i, err)
		}
		release()
	}
}
