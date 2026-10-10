package orchestrator

import (
	"context"
	"sync"
)

type cancelInFlightMutex struct {
	stateMu sync.Mutex
	locked  bool
	waiters []*cancelInFlightMutexWaiter
}

type cancelInFlightMutexWaiter struct {
	ready   chan struct{}
	granted bool
}

func newCancelInFlightMutex() *cancelInFlightMutex {
	return &cancelInFlightMutex{}
}

func (m *cancelInFlightMutex) Lock() {
	if err := m.LockContext(context.Background()); err != nil {
		panic("background cancel guard acquisition failed: " + err.Error())
	}
}

func (m *cancelInFlightMutex) TryLock() bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if m.locked || len(m.waiters) != 0 {
		return false
	}
	m.locked = true
	return true
}

func (m *cancelInFlightMutex) LockContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	m.stateMu.Lock()
	if err := ctx.Err(); err != nil {
		m.stateMu.Unlock()
		return err
	}
	if !m.locked && len(m.waiters) == 0 {
		m.locked = true
		m.stateMu.Unlock()
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	}

	waiter := &cancelInFlightMutexWaiter{ready: make(chan struct{})}
	m.waiters = append(m.waiters, waiter)
	m.stateMu.Unlock()

	select {
	case <-waiter.ready:
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		m.stateMu.Lock()
		if waiter.granted {
			m.handoffLocked()
		} else {
			m.removeWaiterLocked(waiter)
		}
		m.stateMu.Unlock()
		return ctx.Err()
	}
}

func (m *cancelInFlightMutex) Unlock() {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if !m.locked {
		panic("unlock of unlocked cancelInFlightMutex")
	}
	m.handoffLocked()
}

func (m *cancelInFlightMutex) handoffLocked() {
	if len(m.waiters) == 0 {
		m.locked = false
		return
	}
	waiter := m.waiters[0]
	m.waiters[0] = nil
	m.waiters = m.waiters[1:]
	waiter.granted = true
	close(waiter.ready)
}

func (m *cancelInFlightMutex) removeWaiterLocked(target *cancelInFlightMutexWaiter) {
	for index, waiter := range m.waiters {
		if waiter != target {
			continue
		}
		copy(m.waiters[index:], m.waiters[index+1:])
		m.waiters[len(m.waiters)-1] = nil
		m.waiters = m.waiters[:len(m.waiters)-1]
		return
	}
}
