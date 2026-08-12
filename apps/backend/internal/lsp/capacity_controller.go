package lsp

import (
	"context"
	"errors"
)

// releaseCapacity frees one proven server generation and starts the oldest
// accepted queued generation in the slot reserved for it by Capacity.Release.
func (c *Controller) releaseCapacity(
	ctx context.Context,
	key TaskLanguageKey,
	generation uint64,
) {
	next := c.capacity.Release(key, generation)
	if next == nil {
		return
	}
	_, _ = c.commands.submit(context.WithoutCancel(ctx), next.Key, ActionReconcile, "",
		func(workCtx context.Context) (*LanguageSnapshot, error) {
			return c.promoteCapacityEntry(workCtx, *next)
		})
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
