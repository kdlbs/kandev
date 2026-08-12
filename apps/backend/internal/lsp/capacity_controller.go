package lsp

import (
	"context"
	"errors"
	"fmt"
)

// releaseCapacity frees one proven server generation and asynchronously starts
// the oldest accepted queued generation in the slot reserved by Capacity.Release.
// A Stop never waits for another task's installer or task-host launch.
func (c *Controller) releaseCapacity(
	ctx context.Context,
	key TaskLanguageKey,
	generation uint64,
) {
	next := c.capacity.Release(key, generation)
	if next == nil {
		return
	}
	c.dispatchCapacityPromotion(ctx, *next)
}

func (c *Controller) dispatchCapacityPromotion(ctx context.Context, entry QueueEntry) {
	c.lifecycleMu.Lock()
	workCtx := context.WithoutCancel(ctx)
	tracked := false
	if c.lifecycleCtx != nil {
		if c.lifecycleCtx.Err() != nil || c.lifecycleCancel == nil {
			c.lifecycleMu.Unlock()
			c.drainCapacityReservations(entry)
			return
		}
		workCtx = c.lifecycleCtx
		c.lifecycleWG.Add(1)
		tracked = true
	}
	c.lifecycleMu.Unlock()

	go func() {
		if tracked {
			defer c.lifecycleWG.Done()
		}
		started := make(chan struct{})
		_, _ = c.commands.submitOwnedExclusive(workCtx, entry.Key, ActionReconcile,
			func(workCtx context.Context) (*LanguageSnapshot, error) {
				close(started)
				return c.promoteCapacityEntry(workCtx, entry)
			})
		select {
		case <-started:
		default:
			// A terminal command can cancel a queued promotion before its
			// callback runs. Return that already-reserved slot to the queue.
			c.releaseCapacity(workCtx, entry.Key, entry.Generation)
		}
	}()
}

func (c *Controller) drainCapacityReservations(entry QueueEntry) {
	next := &entry
	for next != nil {
		next = c.capacity.Release(next.Key, next.Generation)
	}
}

func (c *Controller) promoteCapacityEntry(
	ctx context.Context,
	entry QueueEntry,
) (*LanguageSnapshot, error) {
	state, _, err := c.store.GetTaskLSPLanguage(ctx, entry.Key.TaskID, entry.Key.Language)
	if err != nil {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		return nil, err
	}
	settings, err := c.loadSettings(ctx, entry.Key.TaskID)
	if err != nil {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		return nil, err
	}
	if state.Generation != entry.Generation || state.Phase != PhaseQueued ||
		effectivePolicy(*state, settings) != PolicyKeepWarm {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		snapshot := c.languageSnapshot(*state, settings, nil)
		return &snapshot, nil
	}
	releaseAdmission, err := c.tasks.AcquireTaskLSPAdmission(ctx, entry.Key.TaskID)
	if err != nil {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		snapshot, transitionErr := c.transition(
			ctx, *state, settings, PhaseWaitingForTask, "", "",
		)
		// The queue entry has already been removed and this state was verified
		// keep-warm above. Schedule from that durable intent even when persisting
		// waiting_for_task fails and therefore produces no snapshot; otherwise no
		// capacity or lifecycle event remains to retry the stranded generation.
		c.scheduleRecovery(entry.Key)
		return snapshot, errors.Join(fmt.Errorf("%w: %v", ErrTaskNotReady, err), transitionErr)
	}
	defer releaseAdmission()

	task, err := c.tasks.GetTask(ctx, entry.Key.TaskID)
	if err != nil {
		return c.failCapacityPromotion(ctx, *state, settings, "task_lookup_failed", err)
	}
	if task.ArchivedAt != nil {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		snapshot, transitionErr := c.transition(ctx, *state, settings, PhaseWaitingForTask, "", "")
		return snapshot, transitionErr
	}
	environment, err := c.tasks.GetTaskEnvironmentForTaskLSP(ctx, entry.Key.TaskID)
	if err != nil {
		return c.failCapacityPromotion(ctx, *state, settings, "task_environment_unavailable", err)
	}
	if !readyTaskEnvironment(environment) {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		snapshot, transitionErr := c.transition(ctx, *state, settings, PhaseWaitingForTask, "", "")
		return snapshot, transitionErr
	}
	if !ExecutorSupportsLSP(environment.ExecutorType) {
		c.releaseCapacity(ctx, entry.Key, entry.Generation)
		snapshot, transitionErr := c.transition(
			ctx, *state, settings, PhaseUnsupported, "unsupported_executor", "",
		)
		return snapshot, transitionErr
	}
	return c.launchReserved(ctx, *state, settings, environment, state.LastAction)
}

func (c *Controller) failCapacityPromotion(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	errorCode string,
	cause error,
) (*LanguageSnapshot, error) {
	c.releaseCapacity(ctx, TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}, state.Generation)
	snapshot, transitionErr := c.transition(
		ctx, state, settings, PhaseError, errorCode, cause.Error(),
	)
	return snapshot, errors.Join(cause, transitionErr)
}
