package database

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	defaultLogicalStatsTTL       = 15 * time.Minute
	defaultLogicalStatsRetry     = 30 * time.Second
	defaultLogicalStatsRetryMax  = 5 * time.Minute
	defaultLogicalStatsDeferWait = 5 * time.Second
	logicalStatsOutcomeCancelled = "cancelled"
	logicalStatsStatePending     = "pending"
	logicalStatsStateReady       = "ready"
	logicalStatsStateRefreshing  = "refreshing"
	logicalStatsStateStale       = "stale"
	logicalStatsStateUnavailable = "unavailable"
)

var errLogicalStatsDeferred = errors.New("logical statistics scan deferred")

type logicalStatsSnapshot struct {
	values     logicalStorageStats
	measuredAt time.Time
}

type logicalStatsRead struct {
	snapshot *logicalStatsSnapshot
	state    string
	error    string
}

type logicalStatsFlight struct {
	done       chan struct{}
	cancel     context.CancelFunc
	generation uint64
}

type logicalStatsCacheOptions struct {
	ttl          time.Duration
	retryMin     time.Duration
	retryMax     time.Duration
	deferWait    time.Duration
	now          func() time.Time
	onStart      func()
	onCompletion func(outcome string, duration time.Duration, snapshot *logicalStatsSnapshot, err error)
}

// logicalStatsCache keeps one complete process-local measurement and runs at
// most one refresh at a time. A caller only reads the current cache state.
type logicalStatsCache struct {
	scan    func(context.Context) (logicalStorageStats, error)
	ctx     context.Context
	cancel  context.CancelFunc
	options logicalStatsCacheOptions

	mu          sync.Mutex
	snapshot    *logicalStatsSnapshot
	flight      *logicalStatsFlight
	generation  uint64
	invalidated bool
	failures    int
	nextRetry   time.Time
	errorCode   string
	stopped     bool
	wg          sync.WaitGroup
}

func newLogicalStatsCache(
	parent context.Context,
	scan func(context.Context) (logicalStorageStats, error),
	options logicalStatsCacheOptions,
) *logicalStatsCache {
	if parent == nil {
		parent = context.Background()
	}
	if options.ttl <= 0 {
		options.ttl = defaultLogicalStatsTTL
	}
	if options.retryMin <= 0 {
		options.retryMin = defaultLogicalStatsRetry
	}
	if options.retryMax < options.retryMin {
		options.retryMax = defaultLogicalStatsRetryMax
	}
	if options.deferWait <= 0 {
		options.deferWait = defaultLogicalStatsDeferWait
	}
	if options.now == nil {
		options.now = time.Now
	}
	ctx, cancel := context.WithCancel(parent)
	return &logicalStatsCache{scan: scan, ctx: ctx, cancel: cancel, options: options}
}

// Read returns the current state and starts a missing or expired measurement
// in the background. It never waits for the scan.
func (c *logicalStatsCache) Read() logicalStatsRead {
	c.mu.Lock()
	now := c.options.now().UTC()
	if !c.stopped && c.flight == nil && c.needsRefreshLocked(now) && !now.Before(c.nextRetry) {
		c.startLocked()
	}
	read := c.readLocked(now)
	c.mu.Unlock()
	return read
}

// Retry bypasses the automatic backoff for one explicit user request.
func (c *logicalStatsCache) Retry() bool {
	c.mu.Lock()
	now := c.options.now().UTC()
	if c.stopped {
		c.mu.Unlock()
		return false
	}
	if c.flight == nil && c.needsRefreshLocked(now) {
		c.nextRetry = time.Time{}
		c.startLocked()
	}
	c.mu.Unlock()
	return true
}

// ReadStale exposes the last complete values without starting or presenting a
// refresh while persistence is known to be unhealthy.
func (c *logicalStatsCache) ReadStale() logicalStatsRead {
	c.mu.Lock()
	read := c.readLocked(c.options.now().UTC())
	if read.snapshot != nil {
		read.state = logicalStatsStateStale
	}
	c.mu.Unlock()
	return read
}

func (c *logicalStatsCache) needsRefreshLocked(now time.Time) bool {
	return c.snapshot == nil || c.invalidated || now.Sub(c.snapshot.measuredAt) >= c.options.ttl
}

func (c *logicalStatsCache) startLocked() {
	ctx, cancel := context.WithCancel(c.ctx)
	flight := &logicalStatsFlight{
		done: make(chan struct{}), cancel: cancel, generation: c.generation,
	}
	c.flight = flight
	c.wg.Add(1)
	go c.run(ctx, flight)
}

func (c *logicalStatsCache) readLocked(now time.Time) logicalStatsRead {
	var snapshot *logicalStatsSnapshot
	if c.snapshot != nil {
		copy := *c.snapshot
		snapshot = &copy
	}
	state := logicalStatsStatePending
	switch {
	case snapshot == nil && c.errorCode != "" && c.flight == nil:
		state = logicalStatsStateUnavailable
	case snapshot != nil && c.flight != nil:
		state = logicalStatsStateRefreshing
	case snapshot != nil && !c.invalidated && now.Sub(snapshot.measuredAt) < c.options.ttl:
		state = logicalStatsStateReady
	case snapshot != nil:
		state = logicalStatsStateStale
	}
	return logicalStatsRead{snapshot: snapshot, state: state, error: c.errorCode}
}

func (c *logicalStatsCache) run(ctx context.Context, flight *logicalStatsFlight) {
	defer c.wg.Done()
	started := c.options.now()
	if c.options.onStart != nil {
		c.options.onStart()
	}
	values, err := c.scan(ctx)
	completedAt := c.options.now().UTC()
	duration := completedAt.Sub(started)
	var published *logicalStatsSnapshot
	outcome := logicalStatsOutcomeCancelled

	c.mu.Lock()
	if c.flight == flight {
		if c.generation != flight.generation {
			c.flight = nil
		} else {
			c.flight = nil
			switch {
			case err == nil:
				snapshot := &logicalStatsSnapshot{values: values, measuredAt: completedAt}
				c.snapshot = snapshot
				c.invalidated = false
				c.failures = 0
				c.nextRetry = time.Time{}
				c.errorCode = ""
				published = snapshot
				outcome = "success"
			case errors.Is(err, errLogicalStatsDeferred):
				c.nextRetry = completedAt.Add(c.options.deferWait)
				outcome = "deferred"
			case errors.Is(err, context.Canceled) && c.ctx.Err() != nil:
				outcome = logicalStatsOutcomeCancelled
			default:
				c.failures++
				c.nextRetry = completedAt.Add(c.retryDelayLocked())
				c.errorCode = "scan_failed"
				outcome = "failure"
			}
		}
	}
	c.mu.Unlock()
	flight.cancel()
	if c.options.onCompletion != nil && outcome != logicalStatsOutcomeCancelled {
		c.options.onCompletion(outcome, duration, published, err)
	}
	close(flight.done)
}

func (c *logicalStatsCache) retryDelayLocked() time.Duration {
	delay := c.options.retryMin
	for attempt := 1; attempt < c.failures && delay < c.options.retryMax; attempt++ {
		delay *= 2
	}
	if delay > c.options.retryMax {
		return c.options.retryMax
	}
	return delay
}

// Invalidate marks the current snapshot stale. When clear is true, as after a
// database replacement, it removes the snapshot before another read can see it.
func (c *logicalStatsCache) Invalidate(clear bool) {
	c.mu.Lock()
	c.generation++
	if c.flight != nil {
		c.flight.cancel()
	}
	c.invalidated = true
	c.failures = 0
	c.nextRetry = time.Time{}
	c.errorCode = ""
	if clear {
		c.snapshot = nil
	}
	c.mu.Unlock()
}

func (c *logicalStatsCache) Close() {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
		c.cancel()
		if c.flight != nil {
			c.flight.cancel()
			c.flight = nil
		}
	}
	c.mu.Unlock()
	c.wg.Wait()
}
