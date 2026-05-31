package health

import (
	"context"
	"sync"
	"time"
)

// CachedChecker wraps a Checker and caches its result for the given TTL.
// Both successful and failed results are cached for the full TTL.
type CachedChecker struct {
	inner    Checker
	ttl      time.Duration
	mu       sync.Mutex
	cached   []Issue
	cachedAt time.Time
}

// NewCachedChecker returns a CachedChecker wrapping inner with the given TTL.
func NewCachedChecker(inner Checker, ttl time.Duration) *CachedChecker {
	return &CachedChecker{inner: inner, ttl: ttl}
}

// Name delegates to the wrapped checker.
func (c *CachedChecker) Name() string { return c.inner.Name() }

// Category delegates to the wrapped checker.
func (c *CachedChecker) Category() string { return c.inner.Category() }

// Check returns cached issues if within TTL; otherwise calls inner and caches.
func (c *CachedChecker) Check(ctx context.Context) []Issue {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cachedAt.IsZero() && time.Since(c.cachedAt) < c.ttl {
		return c.cached
	}
	issues := c.inner.Check(ctx)
	c.cached = issues
	c.cachedAt = time.Now()
	return issues
}
