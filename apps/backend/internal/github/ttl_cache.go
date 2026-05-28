package github

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Short-TTL cache around GitHub responses. The gh CLI subprocess path plus
// multi-step status builds make round trips expensive; caching within a brief
// window keeps preset switching, pagination, and list re-renders snappy.
const (
	defaultCacheTTL     = 30 * time.Second
	defaultCacheMaxSize = 200
)

type ttlEntry struct {
	value     any
	expiresAt time.Time
}

// ttlCache is a tiny TTL map guarded by singleflight to coalesce concurrent
// misses for the same key. When size exceeds the cap, entries with the
// earliest expiry are dropped — good enough for a sub-minute window.
//
// `gen` is bumped on every `clear()`. A fetch in flight at the time of a
// clear (e.g. token swap) is allowed to finish, but its result is dropped on
// the floor by `setIfCurrentGeneration` instead of being written back into
// the cache — otherwise the new user could see stale repos from the prior
// user for up to one TTL.
type ttlCache struct {
	mu      sync.Mutex
	entries map[string]ttlEntry
	sf      singleflight.Group
	ttl     time.Duration
	maxSize int
	now     func() time.Time
	gen     uint64
}

func newTTLCache() *ttlCache {
	return &ttlCache{
		entries: make(map[string]ttlEntry),
		ttl:     defaultCacheTTL,
		maxSize: defaultCacheMaxSize,
		now:     time.Now,
	}
}

// newMergeMethodsCache uses a longer TTL than the default search/status
// caches: repo merge settings rarely change, so a 5-minute window cuts the
// per-PR-view API calls without making "I just toggled squash" feel stuck.
func newMergeMethodsCache() *ttlCache {
	c := newTTLCache()
	c.ttl = 5 * time.Minute
	return c
}

// newAccessibleReposCache backs the list-accessible-repos endpoint and the
// org-list lookup it composes from. 60s is enough to make repeated picker
// opens / typeahead bursts cheap without staling out "I just got added to a
// new org" too long.
func newAccessibleReposCache() *ttlCache {
	c := newTTLCache()
	c.ttl = 60 * time.Second
	return c
}

func (c *ttlCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.value, true
}

// clear drops every cached entry and bumps the generation counter so any
// fetch that was already in flight is prevented from writing its result back
// (its post-fetch setIfCurrentGeneration becomes a no-op). Used by the e2e
// mock controller to make a "GitHub repos unavailable" toggle visible
// immediately instead of waiting for the 60s TTL on a prior cached success
// to expire, and by the token-swap path so the new user never sees the old
// user's cached repos through a late-resolving singleflight.
func (c *ttlCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]ttlEntry)
	c.gen++
}

// generation returns the current generation counter. Callers snapshot this
// before launching a fetch and pass it back into setIfCurrentGeneration so a
// concurrent clear() can invalidate a still-pending write.
func (c *ttlCache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

func (c *ttlCache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		c.evictLocked()
	}
	c.entries[key] = ttlEntry{value: value, expiresAt: c.now().Add(c.ttl)}
}

// setIfCurrentGeneration writes `value` for `key` only if the cache's
// generation counter still matches `gen`. If a clear() ran since `gen` was
// snapshotted, the write is dropped on the floor — preventing a stale fetch
// from clobbering the post-clear empty cache.
func (c *ttlCache) setIfCurrentGeneration(key string, value any, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return
	}
	if len(c.entries) >= c.maxSize {
		c.evictLocked()
	}
	c.entries[key] = ttlEntry{value: value, expiresAt: c.now().Add(c.ttl)}
}

// evictLocked first drops expired entries; if still over the cap, drops the
// entries with the earliest expiry until back under the limit. Caller must
// hold c.mu.
func (c *ttlCache) evictLocked() {
	now := c.now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) < c.maxSize {
		return
	}
	var oldestKey string
	var oldestExp time.Time
	for len(c.entries) >= c.maxSize {
		oldestKey = ""
		for k, e := range c.entries {
			if oldestKey == "" || e.expiresAt.Before(oldestExp) {
				oldestKey = k
				oldestExp = e.expiresAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.entries, oldestKey)
	}
}

// doOrFetch returns a cached value when fresh; otherwise runs fetch under a
// singleflight guard, caches the result, and returns it. Errors are not
// cached. The returned value is shared — callers must not mutate it.
//
// The generation snapshot is taken BEFORE launching the singleflight: if a
// clear() races with this fetch, the post-fetch write becomes a no-op so the
// caller still receives the freshly-fetched value but the cache stays empty
// (the next ensure() will re-fetch under the new generation).
func (c *ttlCache) doOrFetch(key string, fetch func() (any, error)) (any, error) {
	if v, ok := c.get(key); ok {
		return v, nil
	}
	gen := c.generation()
	v, err, _ := c.sf.Do(key, func() (any, error) {
		if v, ok := c.get(key); ok {
			return v, nil
		}
		v, err := fetch()
		if err != nil {
			return nil, err
		}
		c.setIfCurrentGeneration(key, v, gen)
		return v, nil
	})
	return v, err
}

// searchCacheKey composes a cache key with length-prefixed string fields so
// that user-controllable inputs (e.g. customQuery) cannot collide with other
// keys by embedding the separator.
func searchCacheKey(kind, filter, customQuery string, page, perPage int) string {
	return fmt.Sprintf("%d:%s|%d:%s|%d:%s|%d|%d",
		len(kind), kind, len(filter), filter, len(customQuery), customQuery, page, perPage)
}

func prStatusCacheKey(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s#%d", owner, repo, number)
}
