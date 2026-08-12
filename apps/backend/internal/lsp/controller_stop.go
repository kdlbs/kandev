package lsp

import "context"

func (c *Controller) stop(
	ctx context.Context,
	taskID, language string,
	origin Origin,
	action Action,
) (*LanguageSnapshot, error) {
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	settings, err := c.loadSettings(ctx, taskID)
	if err != nil {
		return nil, err
	}
	state, err := c.markStopping(ctx, taskID, language, origin, action)
	if err != nil {
		return nil, err
	}
	return c.stopRuntime(ctx, *state, settings, origin)
}

func (c *Controller) markStopping(
	ctx context.Context,
	taskID, language string,
	origin Origin,
	action Action,
) (*TaskLanguageState, error) {
	now := c.clock()
	return c.updateState(ctx, taskID, language, func(next *TaskLanguageState) {
		if action == ActionStop {
			next.Policy = PolicyDisabled
		}
		next.LastAction = action
		next.LastActionAt = &now
		next.LastStopReason = origin.Reason
		next.LastInitiator = origin.Initiator
		next.LastTransitionAt = now
		if next.Generation == 0 || next.Phase == PhaseOff || next.Phase == PhaseQueued {
			next.Phase = PhaseOff
		} else {
			next.Phase = PhaseStopping
		}
	})
}

func (c *Controller) stopRuntime(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	origin Origin,
) (*LanguageSnapshot, error) {
	taskID, language := state.TaskID, state.Language
	key := TaskLanguageKey{TaskID: taskID, Language: language}
	if c.capacity.CancelQueued(key) || state.Generation == 0 || state.Phase == PhaseOff {
		return c.transition(ctx, state, settings, PhaseOff, "", "")
	}
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if environment == nil || !ExecutorSupportsLSP(environment.ExecutorType) {
		c.releaseCapacity(ctx, key, state.Generation)
		return c.transition(ctx, state, settings, PhaseOff, "", "")
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, environment.ID)
	if err != nil {
		return c.transition(ctx, state, settings, PhaseError, "task_host_control_failed", err.Error())
	}
	if !exists || host == nil {
		c.releaseCapacity(ctx, key, state.Generation)
		return c.transition(ctx, state, settings, PhaseOff, "", "")
	}
	runtimeSnapshot, err := host.StopTaskLSP(ctx, TaskHostStopRequest{
		Language: language, Generation: state.Generation, Reason: origin.Reason,
	})
	if err != nil {
		return c.transition(ctx, state, settings, PhaseError, "task_host_stop_failed", err.Error())
	}
	c.releaseCapacity(ctx, key, state.Generation)
	c.cancelWatch(key)
	stored, accepted, err := c.persistRuntime(ctx, state, *runtimeSnapshot)
	if err != nil {
		return nil, err
	}
	if !accepted {
		runtimeSnapshot = nil
	}
	snapshot := c.languageSnapshot(*stored, settings, runtimeSnapshot)
	return &snapshot, nil
}
