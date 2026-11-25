package api

import (
	"sync"
	"time"
)

const (
	builtinMCPWarningBurst  = 5
	builtinMCPWarningWindow = time.Minute
)

type builtinMCPRejectionLogLimiter struct {
	mu          sync.Mutex
	windowStart time.Time
	emitted     int
}

func (l *builtinMCPRejectionLogLimiter) allow() bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windowStart.IsZero() || (!now.Before(l.windowStart) && now.Sub(l.windowStart) >= builtinMCPWarningWindow) {
		l.windowStart = now
		l.emitted = 0
	}
	if l.emitted >= builtinMCPWarningBurst {
		return false
	}
	l.emitted++
	return true
}
