package subproc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// GitWorkClass identifies the kind of Git work waiting for a process slot.
type GitWorkClass string

const (
	GitInteractive GitWorkClass = "interactive"
	GitLifecycle   GitWorkClass = "lifecycle"
	GitBackground  GitWorkClass = "background"
)

var ErrInvalidGitWorkClass = errors.New("invalid git work class")

// ErrGitWorkClassRequired rejects the classless Acquire API on the process-
// wide class-aware Git pool. Git callers must identify their scheduling class
// so interactive work cannot be starved by lifecycle or background traffic.
var ErrGitWorkClassRequired = errors.New("git work class required")

// ErrAdmissionCanceled marks a command that never reached execution because
// its class-aware admission wait was canceled or timed out. Callers can use
// errors.Is with both this sentinel and the underlying context error.
var ErrAdmissionCanceled = errors.New("subprocess admission canceled")

func wrapAdmissionError(err error) error {
	return fmt.Errorf("%w: %w", ErrAdmissionCanceled, err)
}

var gitWorkClassOrder = [...]GitWorkClass{
	GitInteractive,
	GitLifecycle,
	GitBackground,
}

type classWaiter struct {
	class      int
	ctx        context.Context
	ready      chan struct{}
	enqueuedAt time.Time
	queued     bool
	canceled   bool
	granted    bool
	release    func()
}

type classAdmission struct {
	owner    *Throttle
	mu       sync.Mutex
	cap      int
	inflight int
	next     int
	queues   [len(gitWorkClassOrder)][]*classWaiter
}

// NewNamedClassThrottle creates a class-aware throttle. It is intended for
// process-wide pools whose aggregate and per-class metrics should be exposed.
func NewNamedClassThrottle(name string, cap int) *Throttle {
	t := &Throttle{name: name}
	t.admission = &classAdmission{owner: t, cap: cap}
	t.publishCap(cap)
	return t
}

func gitClassIndex(class GitWorkClass) (int, bool) {
	for i, candidate := range gitWorkClassOrder {
		if candidate == class {
			return i, true
		}
	}
	return 0, false
}

func (a *classAdmission) acquire(ctx context.Context, class GitWorkClass) (func(), error) {
	if err := ctx.Err(); err != nil {
		return noopRelease, err
	}
	idx, ok := gitClassIndex(class)
	if !ok {
		return noopRelease, fmt.Errorf("%w: %q", ErrInvalidGitWorkClass, class)
	}

	waiter := &classWaiter{
		class:      idx,
		ctx:        ctx,
		ready:      make(chan struct{}),
		enqueuedAt: time.Now(),
	}
	a.mu.Lock()
	if err := ctx.Err(); err != nil {
		a.mu.Unlock()
		return noopRelease, err
	}
	if a.cap <= 0 {
		a.recordAcquireLocked(class, 0)
		a.mu.Unlock()
		return noopRelease, nil
	}
	if a.inflight < a.cap && a.emptyLocked() {
		release := a.grantLocked(waiter)
		a.mu.Unlock()
		return release, nil
	}
	waiter.queued = true
	a.queues[idx] = append(a.queues[idx], waiter)
	a.owner.incWaiters(1)
	addClassWaiters(a.owner.name, class, 1)
	a.dispatchLocked()
	a.mu.Unlock()

	for {
		select {
		case <-waiter.ready:
			a.mu.Lock()
			if waiter.granted {
				release := waiter.release
				ctxErr := ctx.Err()
				a.mu.Unlock()
				if ctxErr != nil {
					// The dispatcher may have granted the waiter at the same
					// instant its context was canceled. Do not admit work after
					// cancellation; return the slot before reporting the error.
					release()
					return noopRelease, ctxErr
				}
				return release, nil
			}
			a.mu.Unlock()
		case <-ctx.Done():
			a.mu.Lock()
			if waiter.granted {
				release := waiter.release
				a.mu.Unlock()
				release()
				return noopRelease, ctx.Err()
			}
			if waiter.canceled {
				a.mu.Unlock()
				return noopRelease, ctx.Err()
			}
			if a.removeLocked(waiter) {
				a.dispatchLocked()
				a.mu.Unlock()
				return noopRelease, ctx.Err()
			}
			a.mu.Unlock()
		}
	}
}

func (a *classAdmission) emptyLocked() bool {
	for _, queue := range a.queues {
		if len(queue) != 0 {
			return false
		}
	}
	return true
}

func (a *classAdmission) dispatchLocked() {
	for a.cap > a.inflight {
		idx, ok := a.nextQueuedClassLocked()
		if !ok {
			return
		}
		waiter := a.queues[idx][0]
		a.queues[idx] = a.queues[idx][1:]
		a.next = (idx + 1) % len(gitWorkClassOrder)
		a.grantLocked(waiter)
	}
}

func (a *classAdmission) nextQueuedClassLocked() (int, bool) {
	for offset := 0; offset < len(gitWorkClassOrder); offset++ {
		idx := (a.next + offset) % len(gitWorkClassOrder)
		for len(a.queues[idx]) != 0 {
			waiter := a.queues[idx][0]
			if !waiter.canceled && waiter.ctx.Err() == nil {
				return idx, true
			}
			// Remove canceled waiters before advancing the round-robin
			// cursor. This makes a simultaneous release/cancel boundary
			// deterministic: cancellation wins even if its goroutine has
			// not reached the ctx.Done select branch yet.
			a.queues[idx] = a.queues[idx][1:]
			a.discardCanceledLocked(waiter)
		}
	}
	return 0, false
}

func (a *classAdmission) grantLocked(waiter *classWaiter) func() {
	waiter.granted = true
	a.inflight++
	wait := time.Duration(0)
	wasQueued := waiter.queued
	if wasQueued {
		wait = time.Since(waiter.enqueuedAt)
		a.owner.incWaiters(-1)
		addClassWaiters(a.owner.name, gitWorkClassOrder[waiter.class], -1)
		waiter.queued = false
	}
	a.owner.incInflight(1)
	class := gitWorkClassOrder[waiter.class]
	addClassInflight(a.owner.name, class, 1)
	a.owner.incAcquire(wait)
	addClassAcquire(a.owner.name, class, wait)
	var once sync.Once
	waiter.release = func() {
		once.Do(func() { a.release(class) })
	}
	if wasQueued {
		close(waiter.ready)
	}
	return waiter.release
}

func (a *classAdmission) release(class GitWorkClass) {
	a.mu.Lock()
	if a.inflight > 0 {
		a.inflight--
		a.owner.incInflight(-1)
		addClassInflight(a.owner.name, class, -1)
	}
	a.dispatchLocked()
	a.mu.Unlock()
}

func (a *classAdmission) removeLocked(waiter *classWaiter) bool {
	idx := waiter.class
	queue := a.queues[idx]
	for i, candidate := range queue {
		if candidate != waiter {
			continue
		}
		a.queues[idx] = append(queue[:i], queue[i+1:]...)
		a.discardCanceledLocked(waiter)
		return true
	}
	return false
}

func (a *classAdmission) discardCanceledLocked(waiter *classWaiter) {
	if !waiter.queued {
		return
	}
	waiter.queued = false
	waiter.canceled = true
	a.owner.incWaiters(-1)
	addClassWaiters(a.owner.name, gitWorkClassOrder[waiter.class], -1)
	a.recordAcquireLocked(gitWorkClassOrder[waiter.class], time.Since(waiter.enqueuedAt))
}

func (a *classAdmission) recordAcquireLocked(class GitWorkClass, wait time.Duration) {
	a.owner.incAcquire(wait)
	addClassAcquire(a.owner.name, class, wait)
}

func (a *classAdmission) setCapForTest(newCap int) func() {
	a.mu.Lock()
	previous := a.cap
	a.cap = newCap
	if newCap > 0 {
		a.dispatchLocked()
	}
	a.mu.Unlock()
	a.owner.publishCap(newCap)
	return func() {
		a.mu.Lock()
		a.cap = previous
		if previous > 0 {
			a.dispatchLocked()
		}
		a.mu.Unlock()
		a.owner.publishCap(previous)
	}
}

func (a *classAdmission) capacity() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cap
}

// Snapshot is the structured, process-local view of Git admission state.
type Snapshot struct {
	Pool     string                   `json:"pool"`
	Capacity int                      `json:"capacity"`
	Inflight int                      `json:"inflight"`
	Waiters  int                      `json:"waiters"`
	Classes  map[string]ClassSnapshot `json:"classes"`
}

type ClassSnapshot struct {
	Inflight          int64 `json:"inflight"`
	Waiters           int64 `json:"waiters"`
	AcquireTotal      int64 `json:"acquire_total"`
	AcquireWaitMillis int64 `json:"acquire_wait_millis_total"`
}

func (a *classAdmission) snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	classes := make(map[string]ClassSnapshot, len(gitWorkClassOrder))
	for _, class := range gitWorkClassOrder {
		classes[string(class)] = classSnapshot(a.owner.name, class)
	}
	return Snapshot{
		Pool:     a.owner.name,
		Capacity: a.cap,
		Inflight: a.inflight,
		Waiters:  a.waiterCountLocked(),
		Classes:  classes,
	}
}

func (a *classAdmission) waiterCountLocked() int {
	total := 0
	for _, queue := range a.queues {
		total += len(queue)
	}
	return total
}

// AcquireGit admits a process-wide Git operation in class.
func AcquireGit(ctx context.Context, class GitWorkClass) (func(), error) {
	return gitThrottle.AcquireClass(ctx, class)
}

// GitCapacity returns the configured process-wide Git admission capacity.
func GitCapacity() int { return gitThrottle.admission.capacity() }

// AdmissionSnapshot returns the current process-wide Git admission state.
func AdmissionSnapshot() Snapshot { return gitThrottle.admission.snapshot() }
