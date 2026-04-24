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
type ttlCache struct {
	mu      sync.Mutex
	entries map[string]ttlEntry
	sf      singleflight.Group
	ttl     time.Duration
	maxSize int
	now     func() time.Time
}

func newTTLCache() *ttlCache {
	return &ttlCache{
		entries: make(map[string]ttlEntry),
		ttl:     defaultCacheTTL,
		maxSize: defaultCacheMaxSize,
		now:     time.Now,
	}
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

func (c *ttlCache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
func (c *ttlCache) doOrFetch(key string, fetch func() (any, error)) (any, error) {
	if v, ok := c.get(key); ok {
		return v, nil
	}
	v, err, _ := c.sf.Do(key, func() (any, error) {
		if v, ok := c.get(key); ok {
			return v, nil
		}
		v, err := fetch()
		if err != nil {
			return nil, err
		}
		c.set(key, v)
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
