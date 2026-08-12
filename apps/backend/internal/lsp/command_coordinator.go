package lsp

import (
	"context"
	"sync"
)

type commandResult struct {
	snapshot *LanguageSnapshot
	err      error
}

type commandBatch struct {
	action      Action
	coalesceKey string
	coalescible bool
	ctx         context.Context
	run         func(context.Context) (*LanguageSnapshot, error)
	done        chan struct{}
	result      commandResult
}

type commandLane struct {
	running *commandBatch
	queued  []*commandBatch
}

type commandCoordinator struct {
	mu    sync.Mutex
	lanes map[TaskLanguageKey]*commandLane
}

func (c *commandCoordinator) submit(
	ctx context.Context,
	key TaskLanguageKey,
	action Action,
	coalesceKey string,
	run func(context.Context) (*LanguageSnapshot, error),
) (*LanguageSnapshot, error) {
	return c.submitCommand(ctx, key, action, coalesceKey, true, run)
}

func (c *commandCoordinator) submitExclusive(
	ctx context.Context,
	key TaskLanguageKey,
	action Action,
	run func(context.Context) (*LanguageSnapshot, error),
) (*LanguageSnapshot, error) {
	return c.submitCommand(ctx, key, action, "", false, run)
}

func (c *commandCoordinator) submitCommand(
	ctx context.Context,
	key TaskLanguageKey,
	action Action,
	coalesceKey string,
	allowCoalesce bool,
	run func(context.Context) (*LanguageSnapshot, error),
) (*LanguageSnapshot, error) {
	c.mu.Lock()
	if c.lanes == nil {
		c.lanes = make(map[TaskLanguageKey]*commandLane)
	}
	lane := c.lanes[key]
	if lane == nil {
		lane = &commandLane{}
		c.lanes[key] = lane
	}
	var batch *commandBatch
	if allowCoalesce {
		batch = coalescibleBatch(lane, action, coalesceKey)
	}
	if batch == nil {
		batch = &commandBatch{
			action: action, coalesceKey: coalesceKey,
			coalescible: allowCoalesce,
			ctx:         context.WithoutCancel(ctx), run: run, done: make(chan struct{}),
		}
		if lane.running == nil {
			lane.running = batch
			go c.execute(key, lane, batch)
		} else {
			lane.queued = append(lane.queued, batch)
		}
	}
	c.mu.Unlock()

	select {
	case <-batch.done:
		return batch.result.snapshot, batch.result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func coalescibleBatch(lane *commandLane, action Action, coalesceKey string) *commandBatch {
	if len(lane.queued) > 0 {
		last := lane.queued[len(lane.queued)-1]
		if last.coalescible && last.action == action && last.coalesceKey == coalesceKey {
			return last
		}
		return nil
	}
	if lane.running != nil && lane.running.coalescible &&
		lane.running.action == action && lane.running.coalesceKey == coalesceKey {
		return lane.running
	}
	return nil
}

func (c *commandCoordinator) execute(key TaskLanguageKey, lane *commandLane, batch *commandBatch) {
	batch.result.snapshot, batch.result.err = batch.run(batch.ctx)

	c.mu.Lock()
	close(batch.done)
	if len(lane.queued) == 0 {
		delete(c.lanes, key)
		c.mu.Unlock()
		return
	}
	next := lane.queued[0]
	lane.queued = lane.queued[1:]
	lane.running = next
	go c.execute(key, lane, next)
	c.mu.Unlock()
}
