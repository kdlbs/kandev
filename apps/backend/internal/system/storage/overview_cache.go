package storage

import (
	"context"
	"sync"
	"time"
)

const defaultOverviewCacheTTL = 15 * time.Minute

// OverviewSnapshot is a successful storage overview and the time it was measured.
type OverviewSnapshot struct {
	Summary    Summary
	AnalyzedAt time.Time
}

type OverviewInvalidator interface {
	Invalidate()
}

// OverviewCache keeps one process-local successful overview snapshot.
type OverviewCache struct {
	provider OverviewProvider
	ttl      time.Duration
	now      func() time.Time

	mu         sync.Mutex
	snapshot   *OverviewSnapshot
	flight     *overviewFlight
	generation uint64
}

type overviewFlight struct {
	done       chan struct{}
	snapshot   OverviewSnapshot
	err        error
	generation uint64
}

func NewOverviewCache(provider OverviewProvider) *OverviewCache {
	return &OverviewCache{provider: provider, ttl: defaultOverviewCacheTTL, now: time.Now}
}

func (c *OverviewCache) Get(ctx context.Context) (OverviewSnapshot, error) {
	c.mu.Lock()
	if c.snapshot != nil && c.now().Sub(c.snapshot.AnalyzedAt) < c.ttl {
		snapshot := *c.snapshot
		c.mu.Unlock()
		return snapshot, nil
	}
	if c.flight != nil {
		flight := c.flight
		c.mu.Unlock()
		return waitForOverviewRefresh(ctx, flight)
	}
	flight := c.startRefreshLocked()
	c.mu.Unlock()
	go c.refresh(context.WithoutCancel(ctx), flight)
	return waitForOverviewRefresh(ctx, flight)
}

// Refresh bypasses the freshness window and replaces the shared snapshot on success.
func (c *OverviewCache) Refresh(ctx context.Context) (OverviewSnapshot, error) {
	c.mu.Lock()
	if c.flight != nil {
		flight := c.flight
		c.mu.Unlock()
		return waitForOverviewRefresh(ctx, flight)
	}
	flight := c.startRefreshLocked()
	c.mu.Unlock()
	c.refresh(ctx, flight)
	return flight.snapshot, flight.err
}

func (c *OverviewCache) Capabilities(ctx context.Context, settings StorageMaintenanceSettings) Capabilities {
	return c.provider.Capabilities(ctx, settings)
}

func (c *OverviewCache) Invalidate() {
	c.mu.Lock()
	c.generation++
	c.snapshot = nil
	c.flight = nil
	c.mu.Unlock()
}

func (c *OverviewCache) startRefreshLocked() *overviewFlight {
	flight := &overviewFlight{done: make(chan struct{}), generation: c.generation}
	c.flight = flight
	return flight
}

func (c *OverviewCache) refresh(ctx context.Context, flight *overviewFlight) {
	summary, err := c.provider.Summary(ctx)
	c.mu.Lock()
	if err == nil {
		snapshot := OverviewSnapshot{Summary: summary, AnalyzedAt: c.now().UTC()}
		flight.snapshot = snapshot
		if flight.generation == c.generation {
			c.snapshot = &snapshot
		}
	}
	flight.err = err
	if c.flight == flight {
		c.flight = nil
	}
	close(flight.done)
	c.mu.Unlock()
}

func waitForOverviewRefresh(ctx context.Context, flight *overviewFlight) (OverviewSnapshot, error) {
	select {
	case <-flight.done:
		return flight.snapshot, flight.err
	case <-ctx.Done():
		return OverviewSnapshot{}, ctx.Err()
	}
}
