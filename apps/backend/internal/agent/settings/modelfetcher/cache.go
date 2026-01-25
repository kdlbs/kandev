package modelfetcher

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
)

const (
	defaultTTL  = 10 * time.Second
	maxCacheAge = 1 * time.Minute
)

// CacheEntry holds cached model data for an agent
type CacheEntry struct {
	Models    []registry.ModelEntry
	CachedAt  time.Time
	ExpiresAt time.Time
	Error     error
}

// IsValid returns true if the cache entry is still valid
func (e *CacheEntry) IsValid() bool {
	return time.Now().Before(e.ExpiresAt) && e.Error == nil
}

// IsStale returns true if the cache entry is stale but still usable
func (e *CacheEntry) IsStale() bool {
	now := time.Now()
	return now.After(e.ExpiresAt) && now.Before(e.CachedAt.Add(maxCacheAge))
}

// Cache provides thread-safe caching for dynamic models
type Cache struct {
	entries map[string]*CacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

// NewCache creates a new model cache
func NewCache() *Cache {
	return &Cache{
		entries: make(map[string]*CacheEntry),
		ttl:     defaultTTL,
	}
}

// Get returns the cached models for an agent if available
func (c *Cache) Get(agentName string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[agentName]
	if !exists {
		return nil, false
	}

	return entry, true
}

// Set caches the models for an agent
func (c *Cache) Set(agentName string, models []registry.ModelEntry, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.entries[agentName] = &CacheEntry{
		Models:    models,
		CachedAt:  now,
		ExpiresAt: now.Add(c.ttl),
		Error:     err,
	}
}

// Invalidate removes the cache entry for an agent
func (c *Cache) Invalidate(agentName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, agentName)
}

// Clear removes all cache entries
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
}
