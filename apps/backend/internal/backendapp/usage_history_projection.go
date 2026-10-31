package backendapp

import (
	"context"
	"errors"
	"sync"

	analyticsservice "github.com/kandev/kandev/internal/analytics/service"
	"github.com/kandev/kandev/internal/common/logger"
)

func startUsageHistoryProjection(ctx context.Context, store analyticsservice.UsageProjectionBatchStore, log *logger.Logger) func() error {
	return newUsageHistoryProjection(ctx, store, log).stop
}

// The controller owns the same worker across pauses. A restore permanently
// closes it, so a late E2E request cannot restart a writer after database close.
type usageHistoryProjectionControl struct {
	mu         sync.Mutex
	ctx        context.Context
	worker     *analyticsservice.UsageHistoryWorker
	closed     bool
	beforeStop func() error
}

func newUsageHistoryProjection(ctx context.Context, store analyticsservice.UsageProjectionBatchStore, log *logger.Logger) *usageHistoryProjectionControl {
	worker := analyticsservice.NewUsageHistoryWorker(store, log)
	worker.Start(ctx)
	return &usageHistoryProjectionControl{ctx: ctx, worker: worker}
}

func (c *usageHistoryProjectionControl) pause() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.worker.Stop()
}

func (c *usageHistoryProjectionControl) resume() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return context.Canceled
	}
	if err := c.ctx.Err(); err != nil {
		return err
	}
	c.worker.Start(c.ctx)
	return nil
}

func (c *usageHistoryProjectionControl) stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	var holdError error
	if c.beforeStop != nil {
		holdError = c.beforeStop()
	}
	return errors.Join(holdError, c.worker.Stop())
}
