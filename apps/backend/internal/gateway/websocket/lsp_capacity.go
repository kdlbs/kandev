package websocket

import (
	"os"
	"strconv"
	"sync"
)

const defaultLSPMaxConnections = 8

type lspCapacityLimiter struct {
	mu     sync.Mutex
	max    int
	active int
}

func newLSPCapacityLimiter(max int) *lspCapacityLimiter {
	if max <= 0 {
		max = defaultLSPMaxConnections
	}
	return &lspCapacityLimiter{max: max}
}

func newLSPCapacityLimiterFromEnv() *lspCapacityLimiter {
	raw := os.Getenv("KANDEV_LSP_MAX_CONNECTIONS")
	if raw == "" {
		return newLSPCapacityLimiter(defaultLSPMaxConnections)
	}
	max, err := strconv.Atoi(raw)
	if err != nil {
		return newLSPCapacityLimiter(defaultLSPMaxConnections)
	}
	return newLSPCapacityLimiter(max)
}

func (l *lspCapacityLimiter) TryAcquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active >= l.max {
		return false
	}
	l.active++
	return true
}

func (l *lspCapacityLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active > 0 {
		l.active--
	}
}
