package github

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// fakeBPFetcher is a minimal BranchProtectionFetcher for testing the
// service-level cache wiring. It records call counts and returns canned
// values per (owner, repo, branch) key.
type fakeBPFetcher struct {
	calls atomic.Int64
	bp    BranchProtection
	err   error
}

func (f *fakeBPFetcher) FetchBranchProtection(_ context.Context, _, _, _ string) (BranchProtection, error) {
	f.calls.Add(1)
	return f.bp, f.err
}

// minimal Client surface — only the bits fetchRequiredReviews needs (it
// asserts the client implements BranchProtectionFetcher; everything else
// can be nil-valued).
type fakeBPClient struct {
	*fakeBPFetcher
	Client
}

func newServiceWithBPFetcher(f *fakeBPFetcher) *Service {
	return &Service{
		client:          fakeBPClient{fakeBPFetcher: f},
		protectionCache: newBranchProtectionCache(),
	}
}

func TestBranchProtectionCache_GetSet_TTL(t *testing.T) {
	// synctest.Test advances time in virtual ticks so a 60ms sleep is
	// instantaneous wall-clock — keeps the suite fast and deterministic
	// (no real sleeps that flake on heavily loaded CI runners).
	synctest.Test(t, func(t *testing.T) {
		c := newBranchProtectionCache()
		c.ttl = 50 * time.Millisecond
		key := "o/r@main"
		c.set(key, BranchProtection{HasRule: true, RequiredApprovingReviewCount: 2, FetchedAt: time.Now()})
		got, ok := c.get(key)
		if !ok || got.RequiredApprovingReviewCount != 2 {
			t.Fatalf("fresh entry: got=%+v ok=%v", got, ok)
		}
		time.Sleep(60 * time.Millisecond)
		if _, ok := c.get(key); ok {
			t.Fatalf("stale entry should not be returned")
		}
	})
}

func TestBranchProtectionCache_StaleEntryIsEvicted(t *testing.T) {
	c := newBranchProtectionCache()
	c.ttl = 1 * time.Millisecond
	c.set("o/r@main", BranchProtection{HasRule: true, FetchedAt: time.Now().Add(-time.Hour)})
	if _, ok := c.get("o/r@main"); ok {
		t.Fatalf("expected stale miss")
	}
	c.mu.RLock()
	_, present := c.m["o/r@main"]
	c.mu.RUnlock()
	if present {
		t.Fatalf("stale entry should have been deleted from map")
	}
}

func TestFetchRequiredReviews_CacheHit(t *testing.T) {
	f := &fakeBPFetcher{bp: BranchProtection{HasRule: true, RequiredApprovingReviewCount: 3}}
	s := newServiceWithBPFetcher(f)
	if got := s.fetchRequiredReviews(context.Background(), "o", "r", "main"); got == nil || *got != 3 {
		t.Fatalf("first call: %v", got)
	}
	if got := s.fetchRequiredReviews(context.Background(), "o", "r", "main"); got == nil || *got != 3 {
		t.Fatalf("cached call: %v", got)
	}
	if calls := f.calls.Load(); calls != 1 {
		t.Fatalf("fetcher called %d times, want 1 (cache miss + hit)", calls)
	}
}

func TestFetchRequiredReviews_NoRuleIsCached(t *testing.T) {
	f := &fakeBPFetcher{bp: BranchProtection{HasRule: false}}
	s := newServiceWithBPFetcher(f)
	if got := s.fetchRequiredReviews(context.Background(), "o", "r", "main"); got != nil {
		t.Fatalf("expected nil for no-rule, got %v", got)
	}
	// Second call must not hit the network — the negative result is cached
	// so we don't re-query upstream every poll.
	_ = s.fetchRequiredReviews(context.Background(), "o", "r", "main")
	if calls := f.calls.Load(); calls != 1 {
		t.Fatalf("fetcher called %d times, want 1", calls)
	}
}

func TestFetchRequiredReviews_ErrorIsNotCached(t *testing.T) {
	f := &fakeBPFetcher{err: errors.New("boom")}
	s := newServiceWithBPFetcher(f)
	if got := s.fetchRequiredReviews(context.Background(), "o", "r", "main"); got != nil {
		t.Fatalf("expected nil on error, got %v", got)
	}
	if got := s.fetchRequiredReviews(context.Background(), "o", "r", "main"); got != nil {
		t.Fatalf("expected nil on retry, got %v", got)
	}
	if calls := f.calls.Load(); calls != 2 {
		t.Fatalf("fetcher called %d times, want 2 (error not cached)", calls)
	}
}

func TestFetchRequiredReviews_ClientWithoutFetcherReturnsNil(t *testing.T) {
	// noop client doesn't satisfy BranchProtectionFetcher.
	s := &Service{client: nil, protectionCache: newBranchProtectionCache()}
	if got := s.fetchRequiredReviews(context.Background(), "o", "r", "main"); got != nil {
		t.Fatalf("expected nil when client lacks BranchProtectionFetcher, got %v", got)
	}
}

func TestFetchRequiredReviews_TTLRefetch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeBPFetcher{bp: BranchProtection{HasRule: true, RequiredApprovingReviewCount: 1}}
		s := newServiceWithBPFetcher(f)
		s.protectionCache.ttl = 50 * time.Millisecond
		_ = s.fetchRequiredReviews(context.Background(), "o", "r", "main")
		time.Sleep(60 * time.Millisecond)
		_ = s.fetchRequiredReviews(context.Background(), "o", "r", "main")
		if calls := f.calls.Load(); calls != 2 {
			t.Fatalf("fetcher called %d times, want 2 (refetch after TTL)", calls)
		}
	})
}
