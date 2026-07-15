package usage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

const cacheTTL = 5 * time.Minute

// failureCacheTTL bounds negative caching: after a fetch error, lookups for
// the same key return the cached error instead of re-querying the provider,
// so bursts of concurrent callers coalesce failures as well as successes.
const failureCacheTTL = 15 * time.Second

// cachedEntry tracks the last success and the last failure independently, so
// a failed refresh never evicts a still-valid success: non-fresh callers keep
// being served the stale value while fresh callers share the recent error.
type cachedEntry struct {
	usage     *ProviderUsage
	successAt time.Time
	err       error
	errAt     time.Time
}

// UsageCache is a thread-safe in-memory cache for provider usage responses.
type UsageCache struct {
	mu      sync.RWMutex
	entries map[string]*cachedEntry

	// fetchLocks serializes fetches per cache key so concurrent misses
	// coalesce into a single provider request instead of a burst.
	fetchMu    sync.Mutex
	fetchLocks map[string]*sync.Mutex
}

// NewUsageCache creates an empty UsageCache.
func NewUsageCache() *UsageCache {
	return &UsageCache{
		entries:    make(map[string]*cachedEntry),
		fetchLocks: make(map[string]*sync.Mutex),
	}
}

// CacheKey builds a deterministic cache key from provider name and credential path.
func CacheKey(provider, credentialPath string) string {
	h := sha256.Sum256([]byte(provider + ":" + credentialPath))
	return fmt.Sprintf("%x", h[:8])
}

// GetOrFetch returns a cached entry if fresh; otherwise calls fetchFn, stores the
// result, and returns it. A nil result from fetchFn is stored as a negative cache
// entry so callers avoid hammering a provider that returned nothing.
func (c *UsageCache) GetOrFetch(
	ctx context.Context,
	key string,
	fetchFn func(ctx context.Context) (*ProviderUsage, error),
) (*ProviderUsage, error) {
	return c.GetOrFetchWithin(ctx, key, cacheTTL, fetchFn)
}

// GetOrFetchWithin is GetOrFetch with a caller-chosen staleness bound: the
// cached entry is served only while younger than maxAge. Fetch failures are
// shared with concurrent and near-term callers via a short-lived negative
// entry, except cancellations, which only affect the cancelled caller.
func (c *UsageCache) GetOrFetchWithin(
	ctx context.Context,
	key string,
	maxAge time.Duration,
	fetchFn func(ctx context.Context) (*ProviderUsage, error),
) (*ProviderUsage, error) {
	if usage, ok, err := c.lookup(key, maxAge); ok {
		return usage, err
	}
	lock := c.keyLock(key)
	lock.Lock()
	defer lock.Unlock()
	// Re-check after acquiring the per-key lock: a concurrent caller may have
	// completed the fetch while this one was waiting.
	if usage, ok, err := c.lookup(key, maxAge); ok {
		return usage, err
	}
	usage, err := fetchFn(ctx)
	if err != nil {
		// Do not poison the cache when this caller's context was cancelled —
		// the provider was not necessarily at fault.
		if ctx.Err() == nil {
			c.storeFailure(key, err)
		}
		return nil, err
	}
	c.storeSuccess(key, usage)
	return usage, nil
}

func (c *UsageCache) keyLock(key string) *sync.Mutex {
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()
	lock, ok := c.fetchLocks[key]
	if !ok {
		lock = &sync.Mutex{}
		c.fetchLocks[key] = lock
	}
	return lock
}

// lookup returns the cached value when still valid: successes live for
// maxAge, failures for at most failureCacheTTL (never longer than maxAge).
// A preserved stale success does not mask a newer failure for callers whose
// maxAge excludes it.
func (c *UsageCache) lookup(key string, maxAge time.Duration) (*ProviderUsage, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, found := c.entries[key]
	if !found {
		return nil, false, nil
	}
	if !e.successAt.IsZero() && time.Since(e.successAt) < maxAge {
		return e.usage, true, nil
	}
	if e.err != nil && time.Since(e.errAt) < min(failureCacheTTL, maxAge) {
		return nil, true, e.err
	}
	return nil, false, nil
}

func (c *UsageCache) storeSuccess(key string, usage *ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cachedEntry{usage: usage, successAt: time.Now()}
}

// storeFailure records the error alongside — not instead of — any previous
// success, so callers with a wider staleness bound keep the stale value.
func (c *UsageCache) storeFailure(key string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		e = &cachedEntry{}
		c.entries[key] = e
	}
	e.err = err
	e.errAt = time.Now()
}

// Invalidate removes the cache entry for the given key.
func (c *UsageCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
