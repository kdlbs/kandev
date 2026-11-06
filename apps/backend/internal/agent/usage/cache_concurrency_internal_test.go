package usage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitCacheSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("cache operation did not reach its channel barrier")
	}
}

func TestUsageCacheConcurrentKeysAndFailures(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			cache := NewUsageCache()
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			var workers sync.WaitGroup
			t.Cleanup(func() { unblock(); workers.Wait() })
			var calls atomic.Int32
			cause := refusedUsage()
			reading := &ProviderUsage{FetchedAt: time.Now()}
			fetch := func(context.Context) (*ProviderUsage, error) {
				if calls.Add(1) == 1 {
					close(started)
				}
				<-release
				if fail {
					return nil, cause
				}
				return reading, nil
			}
			arriving := make(chan struct{})
			done := make(chan struct{}, 8)
			for range 8 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					arriving <- struct{}{}
					got, err := cache.GetOrFetch(t.Context(), "shared", fetch)
					if fail {
						if got != nil || !errors.Is(err, cause) {
							t.Errorf("lost shared failure: %v %v", got, err)
						}
					} else if got != reading || err != nil {
						t.Errorf("lost shared success: %v %v", got, err)
					}
					done <- struct{}{}
				}()
			}
			for range 8 {
				waitCacheSignal(t, arriving)
			}
			waitCacheSignal(t, started)
			// The other key must complete before the shared provider is released.
			otherDone := make(chan struct{})
			workers.Add(1)
			go func() {
				defer workers.Done()
				got, err := cache.GetOrFetch(t.Context(), "other", func(context.Context) (*ProviderUsage, error) { return reading, nil })
				if got != reading || err != nil {
					t.Errorf("independent key failed: %v %v", got, err)
				}
				close(otherDone)
			}()
			waitCacheSignal(t, otherDone)
			if calls.Load() != 1 {
				t.Fatalf("concurrent misses started %d provider calls before release", calls.Load())
			}
			unblock()
			for range 8 {
				waitCacheSignal(t, done)
			}
			if calls.Load() != 1 {
				t.Fatalf("same-key misses made %d provider calls", calls.Load())
			}
		})
	}
}

func TestUsageCacheCancellationDoesNotPoisonWaiter(t *testing.T) {
	cache := NewUsageCache()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		_, err := cache.GetOrFetch(ctx, "key", func(ctx context.Context) (*ProviderUsage, error) { close(started); <-ctx.Done(); return nil, ctx.Err() })
		cancelled <- err
	}()
	waitCacheSignal(t, started)
	waiting := make(chan error, 1)
	arriving := make(chan struct{})
	reading := &ProviderUsage{Provider: "healthy", FetchedAt: time.Now()}
	calls := 0
	go func() {
		close(arriving)
		got, err := cache.GetOrFetch(t.Context(), "key", func(context.Context) (*ProviderUsage, error) { calls++; return reading, nil })
		if got != reading && err == nil {
			err = errors.New("waiter lost reading")
		}
		waiting <- err
	}()
	waitCacheSignal(t, arriving)
	cancel()
	if err := <-cancelled; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel cause lost: %v", err)
	}
	if err := <-waiting; err != nil {
		t.Fatalf("cancel poisoned healthy waiter: %v", err)
	}
	if calls != 1 {
		t.Fatalf("healthy retry calls=%d", calls)
	}
}
