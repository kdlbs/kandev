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
