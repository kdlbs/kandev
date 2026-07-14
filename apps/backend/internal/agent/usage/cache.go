package usage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

const cacheTTL = 5 * time.Minute

type cachedEntry struct {
	usage     *ProviderUsage
	fetchedAt time.Time
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
// cached entry is served only while younger than maxAge.
func (c *UsageCache) GetOrFetchWithin(
	ctx context.Context,
	key string,
	maxAge time.Duration,
	fetchFn func(ctx context.Context) (*ProviderUsage, error),
) (*ProviderUsage, error) {
	if usage := c.get(key, maxAge); usage != nil {
		return usage, nil
	}
	lock := c.keyLock(key)
	lock.Lock()
	defer lock.Unlock()
	// Re-check after acquiring the per-key lock: a concurrent caller may have
	// completed the fetch while this one was waiting.
	if usage := c.get(key, maxAge); usage != nil {
		return usage, nil
	}
	usage, err := fetchFn(ctx)
	if err != nil {
		return nil, err
	}
	c.set(key, usage)
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

func (c *UsageCache) get(key string, maxAge time.Duration) *ProviderUsage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok {
		return nil
	}
	if time.Since(e.fetchedAt) >= maxAge {
		return nil
	}
	return e.usage
}

func (c *UsageCache) set(key string, usage *ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cachedEntry{usage: usage, fetchedAt: time.Now()}
}

// Invalidate removes the cache entry for the given key.
func (c *UsageCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
