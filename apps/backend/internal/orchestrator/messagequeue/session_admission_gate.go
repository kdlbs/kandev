package messagequeue

import (
	"context"
	"errors"
	"sync"
	"time"
)

// sessionAdmissionGate serves acquisition waiters in arrival order.
type sessionAdmissionGate struct {
	stateMu sync.Mutex
	locked  bool
	waiters []*sessionAdmissionWaiter
}

type sessionAdmissionWaiter struct {
	ready   chan struct{}
	granted bool
}

func newSessionAdmissionGate() *sessionAdmissionGate {
	return &sessionAdmissionGate{}
}

func (g *sessionAdmissionGate) lock(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	g.stateMu.Lock()
	if !g.locked && len(g.waiters) == 0 {
		g.locked = true
		g.stateMu.Unlock()
		return g.recheckAfterGrant(ctx)
	}
	waiter := &sessionAdmissionWaiter{ready: make(chan struct{})}
	g.waiters = append(g.waiters, waiter)
	g.stateMu.Unlock()

	select {
	case <-waiter.ready:
		return g.recheckAfterGrant(ctx)
	case <-ctx.Done():
		g.stateMu.Lock()
		if waiter.granted {
			g.handoffLocked()
		} else {
			g.removeWaiterLocked(waiter)
		}
		g.stateMu.Unlock()
		return ctx.Err()
	}
}

func (g *sessionAdmissionGate) recheckAfterGrant(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		g.unlock()
		return err
	}
	return nil
}

func (g *sessionAdmissionGate) unlock() {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()
	if g.locked {
		g.handoffLocked()
	}
}

func (g *sessionAdmissionGate) handoffLocked() {
	if len(g.waiters) == 0 {
		g.locked = false
		return
	}
	waiter := g.waiters[0]
	g.waiters[0] = nil
	g.waiters = g.waiters[1:]
	waiter.granted = true
	close(waiter.ready)
}

func (g *sessionAdmissionGate) removeWaiterLocked(target *sessionAdmissionWaiter) {
	for index, waiter := range g.waiters {
		if waiter == target {
			copy(g.waiters[index:], g.waiters[index+1:])
			g.waiters[len(g.waiters)-1] = nil
			g.waiters = g.waiters[:len(g.waiters)-1]
			return
		}
	}
}

// ErrSessionAdmissionTimeout reports an exhausted internal acquisition budget.
var ErrSessionAdmissionTimeout = errors.New("queue session admission timed out")

var admissionWaitTimeout = 30 * time.Second

func admissionWaitBudget(ctx context.Context) (context.Context, context.CancelFunc, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= admissionWaitTimeout {
		derived, cancel := context.WithCancel(ctx)
		return derived, cancel, false
	}
	waitCtx, cancel := context.WithTimeout(ctx, admissionWaitTimeout)
	return waitCtx, cancel, true
}

func admissionWaitError(err error, ownBudget bool) error {
	if ownBudget && errors.Is(err, context.DeadlineExceeded) {
		return ErrSessionAdmissionTimeout
	}
	return err
}

const lifecycleReadGateRetryInterval = 20 * time.Millisecond

func rlockWithContext(ctx context.Context, mu *sync.RWMutex) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if mu.TryRLock() {
			return nil
		}
		timer := time.NewTimer(lifecycleReadGateRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
