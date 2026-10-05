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
	wg         sync.WaitGroup
	once       sync.Once
}

// StartRuntimeUpdateBackground owns the single install-wide update sweep.
func (c *Controller) StartRuntimeUpdateBackground(parent context.Context, readiness ...<-chan struct{}) func() {
	c.runtimeBackgroundMu.Lock()
	defer c.runtimeBackgroundMu.Unlock()
	if c.runtimeBackground != nil {
		return c.runtimeBackgroundStop(c.runtimeBackground)
	}
	ctx, cancel := context.WithCancel(parent)
	worker := &runtimeUpdateBackground{
		cancel:     cancel,
		ctx:        ctx,
		collector:  newRuntimeAvailabilityCollector(),
		deliveries: newRuntimeUpdateDeliveryQueue(),
	}
	c.runtimeBackground = worker
	worker.wg.Add(3)
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

func (c *Controller) runtimeBackgroundStop(worker *runtimeUpdateBackground) func() {
	return func() {
		worker.once.Do(func() {
			worker.cancel()
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
