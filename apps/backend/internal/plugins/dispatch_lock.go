package plugins

import (
	"context"
	"sync"
)

// contextRWMutex is a reader/writer lock whose reader admission honors a caller
// context. Writers keep exclusive access, a waiting writer blocks new readers,
// and a reader that abandons its context never holds or acquires the lease.
type contextRWMutex struct {
	mu             sync.Mutex
	readers        int
	writer         bool
	waitingWriters int
	// changed is closed and replaced on every state transition. Waiters capture
	// it under mu and then block on it, so no transition can be missed.
	changed chan struct{}
}

func newContextRWMutex() *contextRWMutex {
	return &contextRWMutex{changed: make(chan struct{})}
}

// RLockContext acquires a read lease, or returns the context error when ctx is
// done first. The returned function releases the lease.
func (m *contextRWMutex) RLockContext(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for {
		m.mu.Lock()
		if !m.writer && m.waitingWriters == 0 {
			m.readers++
			m.mu.Unlock()
			return m.RUnlock, nil
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// RLock acquires a read lease without a cancellation boundary. Callers that
// have a context should prefer RLockContext.
func (m *contextRWMutex) RLock() {
	m.mu.Lock()
	for m.writer || m.waitingWriters > 0 {
		changed := m.changed
		m.mu.Unlock()
		<-changed
		m.mu.Lock()
	}
	m.readers++
	m.mu.Unlock()
}

// TryRLock acquires a read lease only when no writer holds or awaits it.
func (m *contextRWMutex) TryRLock() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writer || m.waitingWriters > 0 {
		return false
	}
	m.readers++
	return true
}

func (m *contextRWMutex) RUnlock() {
	m.mu.Lock()
	m.readers--
	if m.readers == 0 {
		m.notifyLocked()
	}
	m.mu.Unlock()
}

// Lock acquires the write lease, waiting for every reader to release.
func (m *contextRWMutex) Lock() {
	m.mu.Lock()
	m.waitingWriters++
	for m.writer || m.readers > 0 {
		changed := m.changed
		m.mu.Unlock()
		<-changed
		m.mu.Lock()
	}
	m.waitingWriters--
	m.writer = true
	m.mu.Unlock()
}

func (m *contextRWMutex) Unlock() {
	m.mu.Lock()
	m.writer = false
	m.notifyLocked()
	m.mu.Unlock()
}

func (m *contextRWMutex) notifyLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

// keyedContextRWMutex hands out one contextRWMutex per key, creating it on
// first use and keeping it for the process lifetime (the plugin id keyspace is
// small and long-lived).
type keyedContextRWMutex struct {
	mu    sync.Mutex
	locks map[string]*contextRWMutex
}

func newKeyedContextRWMutex() *keyedContextRWMutex {
	return &keyedContextRWMutex{locks: make(map[string]*contextRWMutex)}
}

func (k *keyedContextRWMutex) lockFor(key string) *contextRWMutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	lock, ok := k.locks[key]
	if !ok {
		lock = newContextRWMutex()
		k.locks[key] = lock
	}
	return lock
}
