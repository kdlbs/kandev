package usage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"
)

func refusedUsage() error {
	return fmt.Errorf("provider fetch: %w", newStatusError("test", http.StatusTooManyRequests, nil, nil))
}

// @covers AC-COSTS-SUBSCRIPTION-USAGE-001.10
func TestUsageCacheBackoffProgressionAndResets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewUsageCache()
		failure := refusedUsage()
		calls := 0
		fetch := func(context.Context) (*ProviderUsage, error) { calls++; return nil, failure }
		for i, delay := range []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
			if got, err := cache.GetOrFetchWithin(t.Context(), "key", 0, fetch); got != nil || !errors.Is(err, failure) {
				t.Fatalf("failure %d: got %v, %v", i, got, err)
			}
			wantCalls := calls
			time.Sleep(delay - time.Nanosecond)
			if _, err := cache.GetOrFetchWithin(t.Context(), "key", 0, fetch); !errors.Is(err, failure) || calls != wantCalls {
				t.Fatalf("failure %d retried before %v: calls=%d want=%d err=%v", i, delay, calls, wantCalls, err)
			}
			time.Sleep(time.Nanosecond)
		}
		// A provider hint replaces the accumulated exponent, including on a 503.
		failure = newStatusError("test", http.StatusServiceUnavailable, http.Header{"Retry-After": {"60"}}, nil)
		_, _ = cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
		time.Sleep(time.Minute)
		failure = refusedUsage()
		_, _ = cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
		before := calls
		time.Sleep(15 * time.Second)
		_, _ = cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
		if calls != before+1 {
			t.Fatal("provider hint did not reset the 429 progression")
		}
		time.Sleep(30 * time.Second)
		_, err := cache.GetOrFetchWithin(t.Context(), "key", 0, func(context.Context) (*ProviderUsage, error) { return &ProviderUsage{FetchedAt: time.Now()}, nil })
		if err != nil {
			t.Fatal(err)
		}
		_, _ = cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
		before = calls
		time.Sleep(15 * time.Second)
		_, _ = cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
		if calls != before+1 {
			t.Fatal("fresh success did not reset the 429 progression")
		}
	})
}

func TestUsageCacheNonRateFailureWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewUsageCache()
		cause := errors.New("transport unavailable")
		calls := 0
		fetch := func(context.Context) (*ProviderUsage, error) { calls++; return nil, cause }
		for i := 1; i <= 3; i++ {
			_, err := cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
			if !errors.Is(err, cause) || calls != i {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			time.Sleep(15*time.Second - time.Nanosecond)
			_, _ = cache.GetOrFetchWithin(t.Context(), "key", 0, fetch)
			if calls != i {
				t.Fatal("negative window was bypassed by caller freshness")
			}
			time.Sleep(time.Nanosecond)
		}
	})
}

// @covers AC-COSTS-SUBSCRIPTION-USAGE-001.11
func TestUsageCacheStaleBoundaryAndCopy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewUsageCache()
		reading := &ProviderUsage{Provider: "anthropic", FetchedAt: time.Now(), Windows: []UtilizationWindow{{Label: "5-hour", UtilizationPct: 95}}}
		_, err := cache.GetOrFetch(t.Context(), "key", func(context.Context) (*ProviderUsage, error) { return reading, nil })
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(29 * time.Minute)
		failure := newStatusError("test", http.StatusTooManyRequests, http.Header{"Retry-After": {"900"}}, nil)
		got, err := cache.GetOrFetch(t.Context(), "key", func(context.Context) (*ProviderUsage, error) { return nil, failure })
		if err != nil || got == nil || got == reading || !got.Stale || !got.FetchedAt.Equal(reading.FetchedAt) || reading.Stale {
			t.Fatalf("lost real stale copy: got=%+v original=%+v err=%v", got, reading, err)
		}
		forbidden := func(context.Context) (*ProviderUsage, error) {
			t.Error("fetched during active backoff")
			return nil, errors.New("unexpected fetch")
		}
		got, err = cache.GetOrFetchWithin(t.Context(), "key", 2*time.Hour, forbidden)
		if err != nil || got == nil || !got.Stale {
			t.Fatalf("caller freshness bypassed stale marker: %+v %v", got, err)
		}
		time.Sleep(time.Minute - time.Nanosecond)
		got, err = cache.GetOrFetchWithin(t.Context(), "key", 2*time.Hour, forbidden)
		if err != nil || got == nil || !got.Stale {
			t.Fatalf("reading younger than bound lost: %+v %v", got, err)
		}
		time.Sleep(time.Nanosecond)
		got, err = cache.GetOrFetchWithin(t.Context(), "key", 2*time.Hour, forbidden)
		if got != nil || !errors.Is(err, failure) {
			t.Fatalf("exact 30-minute bound returned %+v, %v", got, err)
		}
	})
}

// @covers AC-COSTS-SUBSCRIPTION-USAGE-001.12
func TestUsageCacheNilAndFailureCombinations(t *testing.T) {
	for _, seed := range []string{"absent", "nil", "over-age", "value-with-error"} {
		t.Run(seed, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cache := NewUsageCache()
				if seed == "nil" || seed == "over-age" {
					var reading *ProviderUsage
					if seed == "over-age" {
						reading = &ProviderUsage{FetchedAt: time.Now()}
					}
					_, _ = cache.GetOrFetch(t.Context(), "key", func(context.Context) (*ProviderUsage, error) { return reading, nil })
					time.Sleep(30 * time.Minute)
				}
				cause := refusedUsage()
				calls := 0
				fetch := func(context.Context) (*ProviderUsage, error) {
					calls++
					if seed == "value-with-error" {
						return &ProviderUsage{Provider: "must not be stored"}, cause
					}
					return nil, cause
				}
				for range 2 {
					got, err := cache.GetOrFetch(t.Context(), "key", fetch)
					if got != nil || !errors.Is(err, cause) {
						t.Fatalf("nil/error masked: got=%+v err=%v", got, err)
					}
				}
				if calls != 1 {
					t.Fatalf("failure not coalesced: %d calls", calls)
				}
			})
		})
	}
}

func TestUsageCacheOrdinaryFreshnessAndInvalidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewUsageCache()
		calls := 0
		fetch := func(context.Context) (*ProviderUsage, error) { calls++; return nil, nil }
		for range 2 {
			_, _ = cache.GetOrFetch(t.Context(), "nil", fetch)
		}
		if calls != 1 {
			t.Fatal("nil success was not cached")
		}
		time.Sleep(5 * time.Minute)
		_, _ = cache.GetOrFetch(t.Context(), "nil", fetch)
		if calls != 2 {
			t.Fatal("ordinary freshness boundary failed")
		}
		failure := refusedUsage()
		_, _ = cache.GetOrFetch(t.Context(), "refused", func(context.Context) (*ProviderUsage, error) { return nil, failure })
		cache.Invalidate("refused")
		_, err := cache.GetOrFetch(t.Context(), "refused", fetch)
		if err != nil || calls != 3 {
			t.Fatal("invalidation did not clear backoff")
		}
		if CacheKey("anthropic", "/one") == CacheKey("openai", "/one") || CacheKey("anthropic", "/one") == CacheKey("anthropic", "/two") {
			t.Fatal("credential/provider scopes collided")
		}
	})
}
