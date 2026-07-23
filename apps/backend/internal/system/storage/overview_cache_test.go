package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestOverviewCacheReusesSuccessfulSummaryWithinTTL(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	provider := &countingOverview{}
	cache := newOverviewCacheForTest(provider, func() time.Time { return now })

	first, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	now = now.Add(14*time.Minute + 59*time.Second)
	second, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("Summary calls = %d, want 1", provider.calls)
	}
	if !second.AnalyzedAt.Equal(first.AnalyzedAt) {
		t.Fatalf("cached analyzed_at = %s, want %s", second.AnalyzedAt, first.AnalyzedAt)
	}
}

func TestOverviewCacheRefreshesAfterTTL(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	provider := &countingOverview{}
	cache := newOverviewCacheForTest(provider, func() time.Time { return now })
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	now = now.Add(15 * time.Minute)
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("expired Get: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("Summary calls = %d, want 2", provider.calls)
	}
}

func TestOverviewCacheForceRefreshBypassesTTL(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	provider := &countingOverview{}
	cache := newOverviewCacheForTest(provider, func() time.Time { return now })
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	now = now.Add(time.Minute)
	forced, err := cache.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("Summary calls = %d, want 2", provider.calls)
	}
	if !forced.AnalyzedAt.Equal(now) {
		t.Fatalf("forced analyzed_at = %s, want %s", forced.AnalyzedAt, now)
	}
}

func TestOverviewCacheKeepsLastSuccessfulSnapshotAfterFailedRefresh(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	provider := &countingOverview{}
	cache := newOverviewCacheForTest(provider, func() time.Time { return now })
	first, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	provider.err = errors.New("provider unavailable")
	now = now.Add(time.Minute)
	if _, err := cache.Refresh(context.Background()); !errors.Is(err, provider.err) {
		t.Fatalf("Refresh error = %v, want provider error", err)
	}
	cached, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after failed refresh: %v", err)
	}
	if cached.Summary != first.Summary || !cached.AnalyzedAt.Equal(first.AnalyzedAt) {
		t.Fatalf("cached snapshot = %#v, want %#v", cached, first)
	}
}

func TestOverviewCacheConcurrentRefreshWaitersReceiveFlightError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &countingOverview{}
		cache := NewOverviewCache(provider)
		previous, err := cache.Get(context.Background())
		if err != nil {
			t.Fatalf("initial Get: %v", err)
		}
		flightErr := errors.New("provider unavailable")
		provider.err = flightErr
		provider.started = make(chan struct{})
		provider.release = make(chan struct{})

		results := make(chan error, 2)
		go func() { _, err := cache.Refresh(context.Background()); results <- err }()
		<-provider.started
		go func() { _, err := cache.Refresh(context.Background()); results <- err }()
		synctest.Wait()
		close(provider.release)

		for range 2 {
			if err := <-results; !errors.Is(err, flightErr) {
				t.Fatalf("concurrent Refresh error = %v, want provider error", err)
			}
		}
		cached, err := cache.Get(context.Background())
		if err != nil {
			t.Fatalf("Get after failed refresh: %v", err)
		}
		if cached.Summary != previous.Summary || !cached.AnalyzedAt.Equal(previous.AnalyzedAt) {
			t.Fatalf("cached snapshot = %#v, want %#v", cached, previous)
		}
	})
}

func TestOverviewCacheCoalescesConcurrentMisses(t *testing.T) {
	provider := &countingOverview{started: make(chan struct{}), release: make(chan struct{})}
	cache := NewOverviewCache(provider)
	results := make(chan error, 2)
	go func() { _, err := cache.Get(context.Background()); results <- err }()
	<-provider.started
	go func() { _, err := cache.Get(context.Background()); results <- err }()
	close(provider.release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if provider.calls != 1 {
		t.Fatalf("Summary calls = %d, want 1", provider.calls)
	}
}

func TestOverviewCacheInvalidationDuringRefreshDiscardsStaleSnapshot(t *testing.T) {
	provider := &firstCallBlockingOverview{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	cache := NewOverviewCache(provider)
	firstResult := make(chan overviewResult, 1)
	go func() {
		snapshot, err := cache.Get(context.Background())
		firstResult <- overviewResult{snapshot: snapshot, err: err}
	}()
	<-provider.started

	cache.Invalidate()
	close(provider.release)
	if result := <-firstResult; result.err != nil {
		t.Fatalf("first Get: %v", result.err)
	}
	second, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after invalidation: %v", err)
	}
	if second.Summary.Workspaces != 2 {
		t.Fatalf("summary workspaces = %d, want refreshed value 2", second.Summary.Workspaces)
	}
}

func TestOverviewCacheGetInitiatorCancellationDoesNotCancelSharedFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &cancellableOverview{started: make(chan struct{}), release: make(chan struct{})}
		cache := NewOverviewCache(provider)
		initiatorCtx, cancelInitiator := context.WithCancel(context.Background())
		defer cancelInitiator()

		initiatorResult := make(chan overviewResult, 1)
		waiterResult := make(chan overviewResult, 1)
		go func() {
			snapshot, err := cache.Get(initiatorCtx)
			initiatorResult <- overviewResult{snapshot: snapshot, err: err}
		}()
		<-provider.started
		go func() {
			snapshot, err := cache.Get(context.Background())
			waiterResult <- overviewResult{snapshot: snapshot, err: err}
		}()
		synctest.Wait()

		cancelInitiator()
		if result := <-initiatorResult; !errors.Is(result.err, context.Canceled) {
			t.Fatalf("initiator Get error = %v, want context canceled", result.err)
		}
		synctest.Wait()
		select {
		case result := <-waiterResult:
			t.Fatalf("waiter Get completed before provider release: %+v", result)
		default:
		}

		close(provider.release)
		result := <-waiterResult
		if result.err != nil {
			t.Fatalf("waiter Get: %v", result.err)
		}
		if result.snapshot.Summary.Workspaces != 1 {
			t.Fatalf("waiter summary = %#v, want one provider result", result.snapshot.Summary)
		}
		if provider.calls != 1 {
			t.Fatalf("Summary calls = %d, want 1", provider.calls)
		}
		cached, err := cache.Get(context.Background())
		if err != nil {
			t.Fatalf("cached Get: %v", err)
		}
		if cached != result.snapshot {
			t.Fatalf("cached snapshot = %#v, want %#v", cached, result.snapshot)
		}
	})
}

func newOverviewCacheForTest(provider OverviewProvider, now func() time.Time) *OverviewCache {
	cache := NewOverviewCache(provider)
	cache.now = now
	return cache
}

type countingOverview struct {
	mu      sync.Mutex
	calls   int
	err     error
	started chan struct{}
	release chan struct{}
}

func (o *countingOverview) Summary(context.Context) (Summary, error) {
	o.mu.Lock()
	o.calls++
	call := o.calls
	err := o.err
	started, release := o.started, o.release
	o.mu.Unlock()
	if started != nil {
		close(started)
		<-release
	}
	if err != nil {
		return Summary{}, err
	}
	return Summary{Workspaces: call}, nil
}

func (o *countingOverview) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	return Capabilities{}
}

type overviewResult struct {
	snapshot OverviewSnapshot
	err      error
}

type cancellableOverview struct {
	calls   int
	started chan struct{}
	release chan struct{}
}

type firstCallBlockingOverview struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (o *firstCallBlockingOverview) Summary(context.Context) (Summary, error) {
	o.mu.Lock()
	o.calls++
	call := o.calls
	o.mu.Unlock()
	if call == 1 {
		close(o.started)
		<-o.release
	}
	return Summary{Workspaces: call}, nil
}

func (*firstCallBlockingOverview) Capabilities(
	context.Context,
	StorageMaintenanceSettings,
) Capabilities {
	return Capabilities{}
}

func (o *cancellableOverview) Summary(ctx context.Context) (Summary, error) {
	o.calls++
	close(o.started)
	select {
	case <-o.release:
		return Summary{Workspaces: o.calls}, nil
	case <-ctx.Done():
		return Summary{}, ctx.Err()
	}
}

func (o *cancellableOverview) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	return Capabilities{}
}
