package usage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"
)

const cacheTTL = 5 * time.Minute

// failureCacheTTL bounds base negative caching: after a fetch error, lookups for
// the same key return the cached error instead of re-querying the provider,
// so bursts of concurrent callers coalesce failures as well as successes.
const failureCacheTTL = 15 * time.Second

// maxBackoffTTL caps the exponential backoff applied to a repeatedly refused read.
const maxBackoffTTL = 15 * time.Minute

// maxBackoffShift bounds the exponent of the 429 backoff, so the shift below
// cannot overflow the duration it scales.
const maxBackoffShift = 10

// maxStaleUsageAge bounds how old a preserved success may be before a failing
// refresh reports the provider error instead of a stale reading.
const maxStaleUsageAge = 30 * time.Minute

// cachedEntry tracks the last success and the last failure independently, so
// a failed refresh never evicts a still-valid success: non-fresh callers keep
// being served the stale value while fresh callers share the recent error.
type cachedEntry struct {
	usage        *ProviderUsage
	successAt    time.Time
	err          error
	errAt        time.Time
	failures     int
	backoffUntil time.Time
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
// shared with concurrent and near-term callers through backoff negative
// caching, except cancellations, which only affect the cancelled caller.
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
			// A bounded stale reading beats no reading: serve the preserved
			// success instead of reporting a provider the caller cannot act on.
			// Only a real preserved success counts; a lookup that reports the
			// cached error must keep surfacing that error, not a nil reading.
			if stale, ok, lerr := c.lookup(key, maxAge); ok && stale != nil && lerr == nil {
				return stale, nil
			}
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
// maxAge, while a key inside its failure backoff serves the last success as
// explicitly stale until it passes maxStaleUsageAge, and reports the cached
// error beyond that.
func (c *UsageCache) lookup(key string, maxAge time.Duration) (*ProviderUsage, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, found := c.entries[key]
	if !found {
		return nil, false, nil
	}
	now := time.Now()
	if now.Before(e.backoffUntil) {
		// A negative-cache success stores a nil usage, so only clone a real
		// reading; otherwise fall through to the cached error.
		// A decoded pool with no usable accounts is authoritative capacity
		// evidence; an older reading cannot replace that refusal.
		if errors.Is(e.err, errNoUsablePoolCapacity) {
			return nil, true, e.err
		}
		if e.usage != nil && !e.successAt.IsZero() && now.Sub(e.successAt) < maxStaleUsageAge {
			stale := *e.usage
			stale.Stale = true
			return &stale, true, nil
		}
		if e.err != nil {
			return nil, true, e.err
		}
	}
	if !e.successAt.IsZero() && now.Sub(e.successAt) < maxAge {
		return e.usage, true, nil
	}
	return nil, false, nil
}

func (c *UsageCache) storeSuccess(key string, usage *ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cachedEntry{usage: usage, successAt: time.Now()}
}

// storeFailure records the error alongside — not instead of — any previous
// success, so callers keep the stale value, and holds the key in backoff so a
// refused read is not retried on every poll.
func (c *UsageCache) storeFailure(key string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		e = &cachedEntry{}
		c.entries[key] = e
	}
	e.failures++
	e.err = err
	e.errAt = time.Now()
	delay, hinted := backoffDelay(e.failures, err)
	if hinted {
		// The provider named its own retry point, so the next refusal starts a
		// fresh exponential window instead of extending a stale one.
		e.failures = 0
	}
	e.backoffUntil = e.errAt.Add(delay)
}

// backoffDelay returns how long a key stays in backoff after its nth failure.
// A provider retry hint wins and reports that it was applied; a refused read
// backs off exponentially from failureCacheTTL; any other failure keeps the
// base negative window.
func backoffDelay(failures int, err error) (time.Duration, bool) {
	var hint retryAfterHint
	switch {
	case errors.As(err, &hint) && hint.RetryAfter() > 0:
		return hint.RetryAfter(), true
	case isRateLimited(err):
		shift := min(max(failures-1, 0), maxBackoffShift)
		return min(failureCacheTTL<<shift, maxBackoffTTL), false
	default:
		return failureCacheTTL, false
	}
}

// Invalidate removes the cache entry for the given key.
func (c *UsageCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
