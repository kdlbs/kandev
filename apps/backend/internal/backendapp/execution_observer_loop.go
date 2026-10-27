package backendapp

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func (o *executionObserver) wake() {
	select {
	case o.wakeups <- struct{}{}:
	default:
	}
}

func (o *executionObserver) start(parent context.Context, log *logger.Logger) func() error {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runExecutionObserver(ctx, func(ctx context.Context) {
			if err := o.refresh(ctx); err != nil && ctx.Err() == nil {
				log.Warn("Execution observation refresh failed")
			}
		}, o.wakeups)
	}()
	return func() error { cancel(); <-done; return nil }
}

func runExecutionObserver(ctx context.Context, refresh func(context.Context), wakes <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-wakes:
		case <-ticker.C:
		}
	}
}
