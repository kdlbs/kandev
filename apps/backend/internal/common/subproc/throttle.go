// Package subproc provides a cross-package primitive for capping concurrent
// external-binary execs (gh, git, ...).
//
// Why this exists:
//
// On managed corporate macOS hosts (CrowdStrike Falcon + syspolicyd code
// signing), every fork/exec is intercepted, serialized, and validated.
// kandev workloads that fan out subprocesses (PR poller, worktree creation,
// agent lifecycle) can fire bursts of 40+ external execs in a few seconds,
// turning that EDR queue into a multi-second backlog. Other processes on the
// host queue behind us — including kandev's own retries — and the result is
// a system-wide UI freeze plus a wave of 30s-timeout kills inside kandev.
//
// A Throttle caps concurrent execs for one binary. Callers that arrive past
// the cap wait in line, context-aware: a cancelled/deadlined caller exits
// without spawning anything. Each binary owns its own Throttle so a busy
// gh stream cannot starve a worktree-creation git burst (or vice-versa).
package subproc

import (
	"context"
	"sync"
	"time"
)

// Throttle caps concurrent users of a resource via a fixed-size slot pool.
// Zero value is unusable — construct via NewThrottle or NewNamedThrottle.
//
// Throttle is safe for concurrent use. The slot pool can be hot-swapped via
// SetCapForTest so unit tests can drive the semaphore at small caps without
// affecting other tests in the same binary.
//
// When name != "" the Throttle publishes inflight / waiters / acquire / wait
// counters into the package-level expvar maps (see metrics.go) so /debug/vars
// reports queue depth and saturation per pool (gh, git, ...). Unnamed
// throttles stay silent — tests and ad-hoc pools don't pollute /debug/vars.
type Throttle struct {
	mu   sync.RWMutex
	sem  chan struct{}
	name string
}

// NewThrottle returns a Throttle with the given cap and no name. cap <= 0
// disables throttling (Acquire returns a no-op release immediately).
//
// Production code that wants the throttle's metrics published under
// /debug/vars should use NewNamedThrottle. NewThrottle stays in place for
// tests and for any caller that intentionally wants an unobservable pool.
func NewThrottle(cap int) *Throttle {
	return NewNamedThrottle("", cap)
}

// NewNamedThrottle returns a Throttle that publishes inflight / waiters /
// acquire-count / acquire-wait-millis under the package-level expvar maps
// using name as the label key. Use this for process-wide singletons (gh,
// git) so saturation is observable from /debug/vars without a separate
// metrics pipeline.
func NewNamedThrottle(name string, cap int) *Throttle {
	t := &Throttle{name: name}
	if cap > 0 {
		t.sem = make(chan struct{}, cap)
	}
	t.publishCap(cap)
	return t
}

// Acquire blocks until a slot is available or ctx is cancelled. The
// returned release function is safe to call any number of times — only
// the first invocation returns the slot to the pool — but every successful
// Acquire MUST call release at least once (typically via defer) to avoid
// permanently shrinking the pool. On context error it returns ctx.Err()
// and a no-op release so callers can defer unconditionally without
// leaking slots.
//
// Fast path: if ctx is already cancelled at entry, return immediately
// without racing the select against an available slot. Without this,
// Go's select picks randomly between sem<- and ctx.Done() when both are
// ready, so a cancelled caller might still acquire and then fail
// downstream — unobservable but non-deterministic.
func (t *Throttle) Acquire(ctx context.Context) (release func(), err error) {
	if err := ctx.Err(); err != nil {
		return noopRelease, err
	}
	sem := t.currentSem()
	if sem == nil {
		// Throttle disabled (cap<=0) or swapped out by a test teardown.
		// Still bump the acquire counter for visibility but don't record
		// a wait — there's nothing to queue behind.
		t.incAcquire(0)
		return noopRelease, nil
	}
	// Fast path: try to grab a slot without registering as a waiter.
	// Most acquires under normal load hit this branch and add zero metric
	// overhead. Only the contended path below has to bump the waiter gauge.
	select {
	case sem <- struct{}{}:
		t.incAcquire(0)
		t.incInflight(1)
		return t.releaseFunc(sem), nil
	default:
	}
	// Slow path: register as a waiter so /debug/vars reflects current
	// queue depth, then block on the select. The deferred dec ensures we
	// drop the waiter count whether we acquire, get cancelled, or panic.
	t.incWaiters(1)
	start := time.Now()
	defer t.incWaiters(-1)
	select {
	case sem <- struct{}{}:
		t.incAcquire(time.Since(start))
		t.incInflight(1)
		return t.releaseFunc(sem), nil
	case <-ctx.Done():
		t.incAcquire(time.Since(start))
		return noopRelease, ctx.Err()
	}
}

// releaseFunc returns an idempotent release closure that decrements the
// inflight gauge and frees a slot. sync.Once protects against a double
// release (deferred + early-return path in a future refactor) draining a
// slot belonging to another in-flight caller — without it the pool's
// effective cap would slowly leak to zero.
func (t *Throttle) releaseFunc(sem chan struct{}) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			select {
			case <-sem:
				t.incInflight(-1)
			default:
				// Pool was swapped out from under us (test teardown).
				// Releasing a slot we never held is a no-op.
			}
		})
	}
}

func noopRelease() {}

// currentSem returns the active semaphore under a read lock.
func (t *Throttle) currentSem() chan struct{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sem
}

// SetCapForTest replaces the slot pool with one of the given capacity and
// returns a restore function. Test-only — production code MUST NOT call
// this. Use cap=0 to disable throttling for a test.
//
// Restore is idempotent in the sense that the previous pool is captured
// at the call site; nested test setups stack via the returned closure.
func (t *Throttle) SetCapForTest(newCap int) func() {
	t.mu.Lock()
	prev := t.sem
	prevCap := 0
	if prev != nil {
		prevCap = cap(prev)
	}
	if newCap <= 0 {
		t.sem = nil
	} else {
		t.sem = make(chan struct{}, newCap)
	}
	t.mu.Unlock()
	t.publishCap(newCap)
	return func() {
		t.mu.Lock()
		t.sem = prev
		t.mu.Unlock()
		t.publishCap(prevCap)
	}
}
