package github

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestTTLCache_HitAndMiss(t *testing.T) {
	cache := newTTLCache()
	var calls int
	fetch := func() (any, error) {
		calls++
		return "result", nil
	}
	for i := 0; i < 3; i++ {
		v, err := cache.doOrFetch("k", fetch)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if v != "result" {
			t.Fatalf("unexpected value: %v", v)
		}
	}
	if calls != 1 {
		t.Fatalf("expected fetch called once, got %d", calls)
	}
}

func TestTTLCache_Expiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newTTLCache()
		cache.ttl = 30 * time.Second
		var calls int
		fetch := func() (any, error) {
			calls++
			return calls, nil
		}
		if _, err := cache.doOrFetch("k", fetch); err != nil {
			t.Fatal(err)
		}
		time.Sleep(29 * time.Second)
		if _, err := cache.doOrFetch("k", fetch); err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatalf("expected cache hit before TTL; calls=%d", calls)
		}
		time.Sleep(2 * time.Second)
		v, err := cache.doOrFetch("k", fetch)
		if err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("expected refetch after TTL; calls=%d", calls)
		}
		if v != 2 {
			t.Fatalf("expected fresh value 2, got %v", v)
		}
	})
}

func TestTTLCache_ErrorNotCached(t *testing.T) {
	cache := newTTLCache()
	var calls int
	sentinel := errors.New("boom")
	fetch := func() (any, error) {
		calls++
		if calls == 1 {
			return nil, sentinel
		}
		return "ok", nil
	}
	if _, err := cache.doOrFetch("k", fetch); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	v, err := cache.doOrFetch("k", fetch)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v != "ok" {
		t.Fatalf("unexpected value: %v", v)
	}
	if calls != 2 {
		t.Fatalf("expected 2 fetch calls (error not cached), got %d", calls)
	}
}

func TestSearchCacheKey_KeyIsolation(t *testing.T) {
	if searchCacheKey("pr", "", "is:open", 1, 25) == searchCacheKey("issue", "", "is:open", 1, 25) {
		t.Fatal("kind should be part of key")
	}
	if searchCacheKey("pr", "", "is:open", 1, 25) == searchCacheKey("pr", "", "is:open", 2, 25) {
		t.Fatal("page should be part of key")
	}
	if searchCacheKey("pr", "", "is:open", 1, 25) == searchCacheKey("pr", "", "is:closed", 1, 25) {
		t.Fatal("customQuery should be part of key")
	}
	// A user-controllable customQuery must not be able to collide with
	// adjacent fields by embedding the separator.
	collisions := [][2][5]any{
		{{"pr", "a", "b", 1, 25}, {"pr", "a|b", "", 1, 25}},
		{{"pr", "", "a|b|1|25", 0, 0}, {"pr", "a", "b", 1, 25}},
	}
	for _, c := range collisions {
		a := searchCacheKey(c[0][0].(string), c[0][1].(string), c[0][2].(string), c[0][3].(int), c[0][4].(int))
		b := searchCacheKey(c[1][0].(string), c[1][1].(string), c[1][2].(string), c[1][3].(int), c[1][4].(int))
		if a == b {
			t.Fatalf("separator collision: %q == %q", a, b)
		}
	}
}

func TestTTLCache_Singleflight(t *testing.T) {
	cache := newTTLCache()
	var calls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	fetch := func() (any, error) {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return "v", nil
	}

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := cache.doOrFetch("k", fetch); err != nil {
				t.Errorf("fetch err: %v", err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected singleflight to coalesce; calls=%d", got)
	}
}

// Regression: a clear() that races with an in-flight singleflight fetch must
// invalidate the late write so the cache stays empty afterward. Otherwise a
// token swap (the user-facing trigger for clear()) could surface the
// previous user's repos for up to one TTL window — the cleared entries get
// re-populated by the still-pending fetch from the old generation.
func TestTTLCache_ClearInvalidatesInFlightFetch(t *testing.T) {
	cache := newTTLCache()
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	fetchDone := make(chan struct{})

	go func() {
		_, _ = cache.doOrFetch("k", func() (any, error) {
			close(fetchStarted)
			<-releaseFetch
			return "stale", nil
		})
		close(fetchDone)
	}()

	<-fetchStarted
	cache.clear()
	close(releaseFetch)
	<-fetchDone

	if v, ok := cache.get("k"); ok {
		t.Fatalf("expected cache to remain empty after clear(); got %v", v)
	}
}

// Regression: a caller arriving after clear() must not join a still-in-flight
// fetch from the previous generation via singleflight's shared result. Before
// keying singleflight by generation, B's doOrFetch joined A's in-flight call
// and received A's stale value — the post-clear cache stayed empty, but the
// caller still saw old data. After the fix, B mints its own fetch under the
// new generation.
func TestTTLCache_ClearMakesPostClearCallersMintNewFetch(t *testing.T) {
	cache := newTTLCache()
	var calls atomic.Int32
	fetchAStarted := make(chan struct{})
	releaseA := make(chan struct{})

	// Caller A: starts a fetch, blocks on releaseA, eventually returns "A".
	resultA := make(chan any, 1)
	go func() {
		v, _ := cache.doOrFetch("k", func() (any, error) {
			calls.Add(1)
			close(fetchAStarted)
			<-releaseA
			return "A", nil
		})
		resultA <- v
	}()

	<-fetchAStarted
	// Clear races between A's fetch starting and finishing. After this, any
	// new caller must NOT receive A's value via singleflight.
	cache.clear()

	// Caller B arrives after clear() while A is still in flight. B's fetch
	// returns "B" immediately (it does not block on releaseA), so under the
	// generation-keyed singleflight B must mint its own fetch and resolve right
	// away rather than joining A's still-blocked call.
	resultB := make(chan any, 1)
	go func() {
		v, _ := cache.doOrFetch("k", func() (any, error) {
			calls.Add(1)
			return "B", nil
		})
		resultB <- v
	}()

	// Block on B's result with a timeout guard instead of a sleep busy-loop. If
	// generation-keyed singleflight regressed (B joining A's blocked fetch), B
	// would never resolve and the timeout fires deterministically.
	select {
	case v := <-resultB:
		if v != "B" {
			t.Fatalf("caller B got %v, expected 'B' (joined A's stale fetch?)", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller B never completed — generation-keyed singleflight may still be blocking")
	}

	// Now let A finish; its write must be dropped via the generation guard.
	close(releaseA)
	<-resultA
	if v, ok := cache.get("k"); ok {
		// We expect either empty (A's setIfCurrentGeneration was a no-op AND B
		// fetched but landed under the new generation) or "B" — never "A".
		if v == "A" {
			t.Fatalf("stale A result leaked into cache: %v", v)
		}
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("expected at least 2 fetches (A then B); got %d", got)
	}
}

func TestTTLCache_MaxSizeEviction(t *testing.T) {
	cache := newTTLCache()
	cache.maxSize = 3
	for i := 0; i < 5; i++ {
		cache.set(fmt.Sprintf("k%d", i), i)
	}
	if len(cache.entries) > cache.maxSize {
		t.Fatalf("cache exceeded maxSize: %d", len(cache.entries))
	}
}
