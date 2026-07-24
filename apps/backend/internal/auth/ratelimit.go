package auth

import (
	"sync"
	"time"
)

// loginLimiter is a small fixed-window rate limiter keyed by an opaque string
// (IP + lowercase email). It exists to slow online password guessing; it is
// intentionally in-memory — a restart clearing the window is acceptable.
type loginLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	max     int
	entries map[string]*limiterEntry
	now     func() time.Time
}

type limiterEntry struct {
	windowStart time.Time
	count       int
}

const limiterPurgeThreshold = 1024

func newLoginLimiter(window time.Duration, maxAttempts int) *loginLimiter {
	return &loginLimiter{
		window:  window,
		max:     maxAttempts,
		entries: map[string]*limiterEntry{},
		now:     time.Now,
	}
}

// Allow records an attempt for key and reports whether it is within limits.
func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.purgeLocked(now)
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = &limiterEntry{windowStart: now, count: 1}
		return true
	}
	entry.count++
	return entry.count <= l.max
}

// Reset clears the window for key (called after a successful login).
func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// purgeLocked drops expired windows once the map grows past a threshold,
// keeping memory bounded without a background goroutine.
func (l *loginLimiter) purgeLocked(now time.Time) {
	if len(l.entries) < limiterPurgeThreshold {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= l.window {
			delete(l.entries, key)
		}
	}
}
