// Package ctxmutex provides a mutex with cancellable admission.
package ctxmutex

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Mutex preserves ordinary mutex operations while allowing a bounded wait.
// Its zero value is ready for use. A Mutex must not be copied after first use.
type Mutex struct {
	once sync.Once
	sem  *semaphore.Weighted
}

func (m *Mutex) initialize() {
	m.once.Do(func() { m.sem = semaphore.NewWeighted(1) })
}

// Lock waits until the mutex is available.
func (m *Mutex) Lock() {
	if err := m.LockContext(context.Background()); err != nil {
		panic(err)
	}
}

// LockContext acquires ownership or returns the context error without ownership.
func (m *Mutex) LockContext(ctx context.Context) error {
	m.initialize()
	return m.sem.Acquire(ctx, 1)
}

// TryLock acquires ownership only when the mutex is immediately available.
func (m *Mutex) TryLock() bool {
	m.initialize()
	return m.sem.TryAcquire(1)
}

// Unlock releases ownership.
func (m *Mutex) Unlock() {
	m.initialize()
	m.sem.Release(1)
}
