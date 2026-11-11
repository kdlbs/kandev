package orchestrator

import (
	"context"
	"time"
)

// RetryClock supplies time and cancellable waits for retained capacity episodes.
// Nil in ServiceConfig uses the system clock.
type RetryClock interface {
	Now() time.Time
	WaitUntil(context.Context, time.Time) bool
}

func (s *Service) transientRetryNow() time.Time {
	if s.config.RetryClock != nil {
		return s.config.RetryClock.Now()
	}
	return time.Now()
}

func (s *Service) waitTransientRetry(ctx context.Context, entry *transientRetryEntry, delay time.Duration) bool {
	if s.config.RetryClock != nil && !entry.policy.windowEndsAt.IsZero() {
		return s.config.RetryClock.WaitUntil(ctx, entry.retryAt)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
