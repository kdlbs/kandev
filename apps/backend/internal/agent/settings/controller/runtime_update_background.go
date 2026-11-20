package controller

import (
	"context"
	"sync"
	"time"
)

const runtimeUpdateBackgroundInterval = 15 * time.Minute
const runtimeUpdateAvailabilityWindow = 30 * time.Second

type runtimeUpdateBackground struct {
	cancel     context.CancelFunc
	ctx        context.Context
	collector  *runtimeAvailabilityCollector
	deliveries *runtimeUpdateDeliveryQueue
	// replay coalesces subscription replay requests onto the worker.
	replay chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

// StartRuntimeUpdateBackground owns the single install-wide update sweep.
func (c *Controller) StartRuntimeUpdateBackground(parent context.Context, readiness ...<-chan struct{}) func() {
	c.runtimeBackgroundMu.Lock()
	defer c.runtimeBackgroundMu.Unlock()
	if c.runtimeBackground != nil {
		return c.runtimeBackgroundStop(c.runtimeBackground)
	}
	c.renewNativeCheckLifetimeLocked()
	ctx, cancel := context.WithCancel(parent)
	worker := &runtimeUpdateBackground{
		cancel:     cancel,
		ctx:        ctx,
		collector:  newRuntimeAvailabilityCollector(),
		deliveries: newRuntimeUpdateDeliveryQueue(),
		replay:     make(chan struct{}, 1),
	}
	c.runtimeBackground = worker
	lifetime := c.nativeCheckLifetime
	worker.wg.Add(5)
	go func() {
		defer worker.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-worker.replay:
				if err := c.ReplayRuntimeUpdateNotices(ctx); err != nil && ctx.Err() == nil {
					c.logger.Debug("runtime update replay unavailable")
				}
			}
		}
	}()
	go func() {
		defer worker.wg.Done()
		// Process shutdown cancels the parent before the HTTP drain; refuse
		// and cancel native checks then. The stop callback still joins them.
		<-ctx.Done()
		lifetime.shut()
	}()
	go func() {
		defer worker.wg.Done()
		c.runRuntimeAvailabilityCollector(ctx, worker.collector, worker.deliveries)
	}()
	go func() {
		defer worker.wg.Done()
		c.runRuntimeUpdateDeliveryWorker(ctx, worker.deliveries)
	}()
	go func() {
		defer worker.wg.Done()
		if len(readiness) > 0 && readiness[0] != nil {
			select {
			case <-readiness[0]:
			case <-ctx.Done():
				return
			}
		}
		ticker := time.NewTicker(runtimeUpdateBackgroundInterval)
		defer ticker.Stop()
		for ctx.Err() == nil {
			if err := c.RunRuntimeUpdatePass(ctx); err != nil && ctx.Err() == nil {
				c.logger.Debug("runtime release sources unavailable")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return c.runtimeBackgroundStop(worker)
}

// RequestRuntimeUpdateReplay schedules a replay of current update notices on
// the background worker and returns at once, so a subscriber's acknowledgement
// never waits for a status lookup such as a native check. Requests made while
// one is pending coalesce. It does nothing when the background is not running.
func (c *Controller) RequestRuntimeUpdateReplay() {
	c.runtimeBackgroundMu.Lock()
	worker := c.runtimeBackground
	c.runtimeBackgroundMu.Unlock()
	if worker == nil {
		return
	}
	select {
	case worker.replay <- struct{}{}:
	default:
	}
}

func (c *Controller) runtimeBackgroundStop(worker *runtimeUpdateBackground) func() {
	return func() {
		worker.once.Do(func() {
			worker.cancel()
			// Cancel and drain native checks before waiting for the pass.
			c.closeNativeCheckLifetime()
			worker.collector.close()
			worker.deliveries.close()
			worker.wg.Wait()
			c.runtimeUpdatePassMu.Lock()
			if c.updateJobStore != nil {
				c.updateJobStore.automaticWorkers.Wait()
			}
			c.runtimeUpdatePassMu.Unlock()
			c.runtimeBackgroundMu.Lock()
			if c.runtimeBackground == worker {
				c.runtimeBackground = nil
			}
			c.runtimeBackgroundMu.Unlock()
		})
	}
}
