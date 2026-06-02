package github

import (
	"fmt"
	"strings"
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

// newPRFeedbackCache backs Service.GetPRFeedback (reviews + comments + checks,
// 4 sequential GitHub REST calls per miss). Its job is to collapse the
// render/mount bursts the task page produces — a single PR getting fetched
// dozens of times in a couple of seconds — without those duplicates each
// re-hammering GitHub and tripping its secondary rate limits.
//
// The TTL is deliberately shorter than the 30s status cache: feedback is the
// "fresh, on-demand" surface behind the PR popover/detail panel and the manual
// Refresh button, so staling it for 30s would make legitimate updates lag.
// 8s is long enough to cover a burst's tail (the singleflight in doOrFetch
// already coalesces the concurrent in-flight fetches) while keeping the worst-
// case staleness small.
func newPRFeedbackCache() *ttlCache {
	c := newTTLCache()
	c.ttl = 8 * time.Second
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

// del removes a single key from the cache. Used by Service.evictRepoNegative
// to drop a negative entry when a repository is (re)linked to a workspace
// or its watch is recreated, so a freshly-linked repo gets probed
// immediately instead of waiting out the 10-min TTL.
func (c *ttlCache) del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// cachedErr wraps an error stored in a ttlCache by doOrFetchClassified, so
// that a deterministic upstream failure (e.g. ErrRepoNotResolvable) can be
// negative-cached alongside the cache's normal success-value entries.
type cachedErr struct{ err error }

// doOrFetchClassified is doOrFetch with an optional error-classifier that
// lets callers negative-cache deterministic failures. When fetch() returns
// an error and shouldCache(err) is true, the error is stored under `key`
// (subsequent hits within the TTL return the same error). Transient errors
// (shouldCache=false) are NOT cached, matching doOrFetch's "errors are not
// cached" contract. Used by Service.repoErrorCache to collapse the
// SyncWatchesBatched storm against missing/unauthorized repos to one
// upstream gh call per 10 minutes per repo. Singleflight coalesces
// concurrent first-time misses on the same key into one fetch.
func (c *ttlCache) doOrFetchClassified(key string, fetch func() (any, error), shouldCache func(err error) bool) (any, error) {
	if v, ok := c.get(key); ok {
		if ce, ok := v.(cachedErr); ok {
			return nil, ce.err
		}
		return v, nil
	}
	gen := c.generation()
	sfKey := fmt.Sprintf("%d|%s", gen, key)
	v, err, _ := c.sf.Do(sfKey, func() (any, error) {
		if v, ok := c.get(key); ok {
			if ce, ok := v.(cachedErr); ok {
				return nil, ce.err
			}
			return v, nil
		}
		out, ferr := fetch()
		if ferr != nil {
			if shouldCache != nil && shouldCache(ferr) {
				c.setIfCurrentGeneration(key, cachedErr{err: ferr}, gen)
			}
			return nil, ferr
		}
		c.setIfCurrentGeneration(key, out, gen)
		return out, nil
	})
	return v, err
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
	// Key the singleflight on the generation as well as the cache key. Without
	// the generation prefix, a caller arriving after clear() would join an
	// already-in-flight fetch from the previous generation and receive its
	// stale result — singleflight's whole point is shared results. Bumping
	// gen on clear() guarantees the post-clear caller mints its own fetch
	// instead of inheriting the old one.
	sfKey := fmt.Sprintf("%d|%s", gen, key)
	v, err, _ := c.sf.Do(sfKey, func() (any, error) {
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

// newRepoErrorCache backs Service.repoErrorCache. It negative-caches
// deterministic "repository not resolvable" failures (see
// isRepoNotResolvableErr) so the SyncWatchesBatched storm against
// missing/unauthorized repos collapses to one upstream call per 10
// minutes per repo instead of one per 5s frontend poll. The TTL is
// deliberately longer than the status / feedback caches because the
// failure mode is "this repo doesn't exist or we can't see it" — a
// state that flips back only via an explicit re-link or token swap,
// both of which evict the entry directly.
func newRepoErrorCache() *ttlCache {
	c := newTTLCache()
	c.ttl = 10 * time.Minute
	return c
}

// repoErrorCacheKey is the case-insensitive (owner, repo) key for
// Service.repoErrorCache. GitHub's repository names are case-insensitive
// at the API surface, so "NBCUDTC/Bff" and "nbcudtc/bff" reach the same
// upstream resource; normalising the cache key prevents the negative
// entry from being bypassed by a caller that capitalises differently.
func repoErrorCacheKey(owner, repo string) string {
	return strings.ToLower(owner) + "/" + strings.ToLower(repo)
}
