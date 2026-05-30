package shared

import (
	"context"
	"sync"
	"time"
)

// Janitor periodic-flushes the managed ACP debug writers and prunes old debug
// files so an always-on debug session can't grow disk without bound. It owns a
// single background goroutine with explicit Start/Stop lifecycle per the
// backend goroutine-ownership conventions (ctx cancel + wg.Wait, no
// time.Sleep).
type Janitor struct {
	mgr         *acpLogManager
	flushEvery  time.Duration
	retainEvery time.Duration
	idleTimeout time.Duration

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

const (
	defaultACPFlushEvery  = 2 * time.Second
	defaultACPRetainEvery = 5 * time.Minute
	defaultACPIdleTimeout = 10 * time.Minute
)

// NewACPJanitor returns a Janitor over the process-wide debug-log registry
// with default intervals. It is a no-op to Start when debug mode is off.
func NewACPJanitor() *Janitor {
	return newJanitor(acpLog, defaultACPFlushEvery, defaultACPRetainEvery, defaultACPIdleTimeout)
}

func newJanitor(mgr *acpLogManager, flushEvery, retainEvery, idleTimeout time.Duration) *Janitor {
	return &Janitor{
		mgr:         mgr,
		flushEvery:  flushEvery,
		retainEvery: retainEvery,
		idleTimeout: idleTimeout,
	}
}

// Start runs an immediate retention pass and then spawns the background loop.
// Idempotent; calling Start again while running is a no-op. When debug mode is
// off it does nothing — there is no work to do.
func (j *Janitor) Start(ctx context.Context) {
	if !debugMode {
		return
	}
	j.mu.Lock()
	if j.started {
		j.mu.Unlock()
		return
	}
	j.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	// Add to the WaitGroup while still holding j.mu so a concurrent Stop()
	// (which must take j.mu before it can Wait) can't reach wg.Wait() before
	// this Add — that would be a WaitGroup misuse on a zero counter.
	j.wg.Add(1)
	j.mu.Unlock()

	// Initial pass on startup so a previous run's files are pruned promptly.
	j.mgr.prune(time.Now())

	go j.loop(loopCtx)
}

func (j *Janitor) loop(ctx context.Context) {
	defer j.wg.Done()
	flush := time.NewTicker(j.flushEvery)
	defer flush.Stop()
	retain := time.NewTicker(j.retainEvery)
	defer retain.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-flush.C:
			j.mgr.flushAll()
		case <-retain.C:
			// Flush first so on-disk mtimes reflect recent buffered writes
			// before prune evaluates file age, then close idle writers/rings
			// and prune.
			j.mgr.flushAll()
			j.mgr.closeIdle(j.idleTimeout)
			j.mgr.prune(time.Now())
		}
	}
}

// Stop cancels the loop, waits for it to drain, then flushes and closes all
// open writers. Idempotent.
func (j *Janitor) Stop() {
	j.mu.Lock()
	if !j.started {
		j.mu.Unlock()
		return
	}
	j.started = false
	cancel := j.cancel
	j.cancel = nil
	j.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	j.wg.Wait()
	j.mgr.closeAll()
}
