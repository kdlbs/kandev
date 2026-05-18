package updates

import (
	"sync"
	"time"
)

// Limiter is a single-slot rate limiter: at most one call is allowed per
// configured window. Used by Service.Check to bound how often the manual
// "check now" handler can hit GitHub.
//
// The limiter is safe for concurrent use.
type Limiter struct {
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	last time.Time
}

// LimiterOption customises a Limiter on construction. Used by tests to inject
// a deterministic clock instead of relying on wall time.
type LimiterOption func(*Limiter)

// WithClock overrides the time source used by the limiter. The provided
// function must return non-zero times.
func WithClock(now func() time.Time) LimiterOption {
	return func(l *Limiter) {
		if now != nil {
			l.now = now
		}
	}
}

// NewLimiter returns a Limiter that allows one call per window.
func NewLimiter(window time.Duration, opts ...LimiterOption) *Limiter {
	l := &Limiter{window: window, now: time.Now}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Allow records and authorises a call when the window has elapsed since the
// previous call. On allow it returns (true, 0). When denied it returns
// (false, retryAfter) where retryAfter is the positive duration until the next
// allowed call.
func (l *Limiter) Allow() (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.last.IsZero() || now.Sub(l.last) >= l.window {
		l.last = now
		return true, 0
	}
	retry := l.window - now.Sub(l.last)
	if retry <= 0 {
		retry = time.Nanosecond
	}
	return false, retry
}
