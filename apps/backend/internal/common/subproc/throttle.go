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
)

// Throttle caps concurrent users of a resource via a fixed-size slot pool.
// Zero value is unusable — construct via NewThrottle.
//
// Throttle is safe for concurrent use. The slot pool can be hot-swapped via
// SetCapForTest so unit tests can drive the semaphore at small caps without
// affecting other tests in the same binary.
type Throttle struct {
	mu  sync.RWMutex
	sem chan struct{}
}

// NewThrottle returns a Throttle with the given cap. cap <= 0 disables
// throttling (Acquire returns a no-op release immediately); use this when
// callers want to opt out under a test or in environments where the OS
// does not exhibit the fork/exec serialization problem.
func NewThrottle(cap int) *Throttle {
	if cap <= 0 {
		return &Throttle{}
	}
	return &Throttle{sem: make(chan struct{}, cap)}
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
		return noopRelease, nil
	}
	select {
	case sem <- struct{}{}:
		// sync.Once makes the release idempotent. Without it, a double
		// release (deferred + early-return path in a future refactor)
		// would drain a slot belonging to another in-flight caller,
		// slowly leaking the pool's effective cap to zero.
		var once sync.Once
		return func() {
			once.Do(func() {
				select {
				case <-sem:
				default:
					// Pool was swapped out from under us (test teardown).
					// Releasing a slot we never held is a no-op.
				}
			})
		}, nil
	case <-ctx.Done():
		return noopRelease, ctx.Err()
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
func (t *Throttle) SetCapForTest(cap int) func() {
	t.mu.Lock()
	prev := t.sem
	if cap <= 0 {
		t.sem = nil
	} else {
		t.sem = make(chan struct{}, cap)
	}
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		t.sem = prev
		t.mu.Unlock()
	}
}
