package usage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeHostClient struct {
	hasCreds bool
	usage    *ProviderUsage
	err      error
	calls    int
}

func (f *fakeHostClient) FetchUsage(_ context.Context) (*ProviderUsage, error) {
	f.calls++
	return f.usage, f.err
}

func (f *fakeHostClient) HasSubscriptionCredentials() bool { return f.hasCreds }

func TestHostServiceList(t *testing.T) {
	now := time.Now()
	okUsage := &ProviderUsage{
		Provider:  "anthropic",
		Plan:      "max",
		Windows:   []UtilizationWindow{{Label: "5-hour", UtilizationPct: 42, ResetAt: now}},
		FetchedAt: now,
	}
	noCreds := &fakeHostClient{hasCreds: false}
	ok := &fakeHostClient{hasCreds: true, usage: okUsage}
	failing := &fakeHostClient{hasCreds: true, err: errors.New("boom")}

	svc := &HostService{
		cache: NewUsageCache(),
		entries: []hostEntry{
			{agentID: "claude-acp", cacheKey: "k1", client: noCreds},
			{agentID: "codex-acp", cacheKey: "k2", client: ok},
			{agentID: "other-acp", cacheKey: "k3", client: failing},
		},
	}

	got := svc.List(context.Background(), false)
	if len(got) != 2 {
		t.Fatalf("List = %+v, want 2 entries", got)
	}
	if got[0].AgentID != "codex-acp" || got[0].Usage != okUsage || got[0].Error != "" {
		t.Errorf("entry[0] = %+v", got[0])
	}
	if got[1].AgentID != "other-acp" || got[1].Usage != nil || got[1].Error != hostUsageFetchError {
		t.Errorf("entry[1] = %+v", got[1])
	}
	if noCreds.calls != 0 {
		t.Errorf("client without creds was fetched %d times", noCreds.calls)
	}

	// Second List hits the cache for the successful entry.
	_ = svc.List(context.Background(), false)
	if ok.calls != 1 {
		t.Errorf("expected cached fetch, got %d calls", ok.calls)
	}
}

func TestHostServiceList_FreshBypassesStaleCache(t *testing.T) {
	now := time.Now()
	okUsage := &ProviderUsage{Provider: "anthropic", FetchedAt: now}
	ok := &fakeHostClient{hasCreds: true, usage: okUsage}
	svc := &HostService{
		cache:   NewUsageCache(),
		entries: []hostEntry{{agentID: "claude-acp", cacheKey: "k1", client: ok}},
	}

	_ = svc.List(context.Background(), false)
	if ok.calls != 1 {
		t.Fatalf("initial fetch calls = %d", ok.calls)
	}

	// Entry is younger than freshMaxAge: fresh serves the cached value.
	_ = svc.List(context.Background(), true)
	if ok.calls != 1 {
		t.Errorf("fresh within clamp should hit cache, calls = %d", ok.calls)
	}

	// Age the entry past freshMaxAge but below the 5-minute TTL: a normal List
	// still serves the cache, a fresh List re-queries the provider.
	svc.cache.mu.Lock()
	svc.cache.entries["k1"].successAt = now.Add(-freshMaxAge - time.Second)
	svc.cache.mu.Unlock()

	_ = svc.List(context.Background(), false)
	if ok.calls != 1 {
		t.Errorf("stale-but-within-TTL cached List should not refetch, calls = %d", ok.calls)
	}
	_ = svc.List(context.Background(), true)
	if ok.calls != 2 {
		t.Errorf("fresh List should refetch, calls = %d", ok.calls)
	}
}

func TestNewHostServiceRegistersHostAgents(t *testing.T) {
	svc := NewHostService(nil)
	if len(svc.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(svc.entries))
	}
	if svc.entries[0].agentID != "claude-acp" || svc.entries[1].agentID != "codex-acp" {
		t.Errorf("agent IDs = %q, %q", svc.entries[0].agentID, svc.entries[1].agentID)
	}
}

func TestUsageCacheCoalescesConcurrentFetches(t *testing.T) {
	cache := NewUsageCache()
	var calls atomic.Int32
	fetch := func(_ context.Context) (*ProviderUsage, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &ProviderUsage{Provider: "anthropic", FetchedAt: time.Now()}, nil
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.GetOrFetchWithin(context.Background(), "k", freshMaxAge, fetch); err != nil {
				t.Errorf("GetOrFetchWithin: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Errorf("concurrent misses fetched %d times, want 1", got)
	}
}

func TestUsageCacheCoalescesFailures(t *testing.T) {
	cache := NewUsageCache()
	var calls atomic.Int32
	fetch := func(_ context.Context) (*ProviderUsage, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil, errors.New("boom")
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.GetOrFetchWithin(context.Background(), "k", freshMaxAge, fetch); err == nil {
				t.Error("expected error from coalesced failing fetch")
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Errorf("concurrent failing fetches called provider %d times, want 1", got)
	}

	// Once the negative entry ages past failureCacheTTL, the provider is retried.
	cache.mu.Lock()
	cache.entries["k"].errAt = time.Now().Add(-failureCacheTTL - time.Second)
	cache.mu.Unlock()
	if _, err := cache.GetOrFetchWithin(context.Background(), "k", freshMaxAge, fetch); err == nil {
		t.Error("expected error on retry after TTL")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expired failure entry should refetch, calls = %d, want 2", got)
	}
}

func TestUsageCacheFailureKeepsStaleSuccess(t *testing.T) {
	cache := NewUsageCache()
	okUsage := &ProviderUsage{Provider: "anthropic", FetchedAt: time.Now()}
	okFetch := func(_ context.Context) (*ProviderUsage, error) { return okUsage, nil }
	failFetch := func(_ context.Context) (*ProviderUsage, error) { return nil, errors.New("boom") }

	if _, err := cache.GetOrFetchWithin(context.Background(), "k", freshMaxAge, okFetch); err != nil {
		t.Fatalf("seed fetch: %v", err)
	}
	// Age the success past the fresh clamp but within the regular TTL.
	cache.mu.Lock()
	cache.entries["k"].successAt = time.Now().Add(-freshMaxAge - time.Second)
	cache.mu.Unlock()

	// A failed fresh refresh returns the error to the fresh caller...
	if _, err := cache.GetOrFetchWithin(context.Background(), "k", freshMaxAge, failFetch); err == nil {
		t.Fatal("expected error from failed fresh fetch")
	}
	// ...but must not evict the stale-but-valid success for non-fresh callers.
	got, err := cache.GetOrFetchWithin(context.Background(), "k", cacheTTL, failFetch)
	if err != nil || got != okUsage {
		t.Errorf("stale success was evicted: usage=%v err=%v", got, err)
	}
}

func TestUsageCacheDoesNotCacheCancellation(t *testing.T) {
	cache := NewUsageCache()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (*ProviderUsage, error) {
		calls.Add(1)
		return nil, ctx.Err()
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.GetOrFetchWithin(cancelled, "k", freshMaxAge, fetch); err == nil {
		t.Fatal("expected cancellation error")
	}

	// The cancellation must not have been recorded as a provider failure.
	okUsage := &ProviderUsage{Provider: "anthropic", FetchedAt: time.Now()}
	got, err := cache.GetOrFetchWithin(context.Background(), "k", freshMaxAge,
		func(_ context.Context) (*ProviderUsage, error) { return okUsage, nil })
	if err != nil || got != okUsage {
		t.Errorf("cancelled fetch poisoned the cache: usage=%v err=%v", got, err)
	}
	if calls.Load() != 1 {
		t.Errorf("fetch calls = %d, want 1", calls.Load())
	}
}
