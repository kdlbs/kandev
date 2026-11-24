package changes

import (
	"context"
	"sync"
)

type startCaptureLimiter struct {
	mu      sync.Mutex
	entries map[string]*startCaptureLimiterEntry
}

type startCaptureLimiterEntry struct {
	slots chan struct{}
	users int
}

func newStartCaptureLimiter() *startCaptureLimiter {
	return &startCaptureLimiter{entries: make(map[string]*startCaptureLimiterEntry)}
}

func (l *startCaptureLimiter) acquire(ctx context.Context, executionID string) (func(), bool) {
	l.mu.Lock()
	entry := l.entries[executionID]
	if entry == nil {
		entry = &startCaptureLimiterEntry{slots: make(chan struct{}, turnChangeStartCaptureConcurrency)}
		l.entries[executionID] = entry
	}
	entry.users++
	l.mu.Unlock()

	select {
	case entry.slots <- struct{}{}:
		return func() {
			<-entry.slots
			l.releaseReference(executionID, entry)
		}, true
	case <-ctx.Done():
		l.releaseReference(executionID, entry)
		return nil, false
	}
}

func (l *startCaptureLimiter) releaseReference(executionID string, entry *startCaptureLimiterEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.users--
	if entry.users == 0 && l.entries[executionID] == entry {
		delete(l.entries, executionID)
	}
}
