package lsp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/lsp/installer"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (c *Controller) startAllocatedWithEnvironment(
	ctx context.Context,
	task *taskmodels.Task,
	state TaskLanguageState,
	settings TaskSettings,
	acceptedAt time.Time,
	action Action,
	environment *taskmodels.TaskEnvironment,
	previousMayHaveProcess bool,
) (*LanguageSnapshot, error) {
	if task.ArchivedAt != nil || !readyTaskEnvironment(environment) {
		return c.transition(ctx, state, settings, PhaseWaitingForTask, "", "")
	}
	if !ExecutorSupportsLSP(environment.ExecutorType) {
		return c.transition(ctx, state, settings, PhaseUnsupported, "unsupported_executor", "")
	}
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	if !c.capacity.Admit(key, state.Generation, acceptedAt) {
		return c.transition(ctx, state, settings, PhaseQueued, "", "")
	}
	return c.launchReserved(ctx, state, settings, environment, action, previousMayHaveProcess)
}

func (c *Controller) launchReserved(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	action Action,
	previousMayHaveProcess bool,
) (*LanguageSnapshot, error) {
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	host, err := c.runtimes.EnsureTaskHost(ctx, state.TaskID, environment.ID)
	if err != nil {
		processAbsent := !previousMayHaveProcess
		if processAbsent {
			c.releaseCapacity(ctx, key, state.Generation)
		}
		snapshot, transitionErr := c.transitionRuntimeFailure(
			ctx, state, settings, "task_host_unavailable", err, processAbsent,
		)
		c.scheduleDesiredRecovery(key, snapshot)
		return snapshot, transitionErr
	}
	request := TaskHostStartRequest{
		Language: state.Language, Generation: state.Generation,
		AutoInstall: contains(settings.AutoInstallLanguages, state.Language) &&
			installer.SupportsAutoInstall(state.Language),
		Configuration: append(json.RawMessage(nil), settings.ServerConfigs[state.Language]...),
	}
	var runtimeSnapshot *RuntimeSnapshot
	if action == ActionRestart {
		runtimeSnapshot, err = host.RestartTaskLSP(ctx, request)
	} else {
		runtimeSnapshot, err = host.StartTaskLSP(ctx, request)
	}
	if err != nil {
		return c.handleLaunchFailure(ctx, key, state, settings, runtimeSnapshot, err)
	}
	stored, accepted, err := c.persistRuntime(ctx, state, *runtimeSnapshot)
	if err != nil {
		return nil, err
	}
	c.ensureWatch(key)
	if !accepted {
		runtimeSnapshot = nil
	}
	snapshot := c.languageSnapshot(*stored, settings, runtimeSnapshot)
	return &snapshot, nil
}

func (c *Controller) handleLaunchFailure(
	ctx context.Context,
	key TaskLanguageKey,
	state TaskLanguageState,
	settings TaskSettings,
	runtimeSnapshot *RuntimeSnapshot,
	controlErr error,
) (*LanguageSnapshot, error) {
	if runtimeSnapshot == nil {
		return c.transitionLaunchFailure(ctx, key, state, settings, controlErr)
	}
	persisted, accepted, err := c.persistRuntime(ctx, state, *runtimeSnapshot)
	if err != nil {
		return nil, err
	}
	state = *persisted
	if !accepted {
		// Replacement cleanup failures are reported with the previous live
		// generation. That is not newer watch evidence: the requested generation
		// still needs an actionable error while capacity remains reserved for the
		// process tree whose cleanup is unresolved.
		if runtimeSnapshot.Generation < state.Generation {
			return c.transitionLaunchFailure(ctx, key, state, settings, controlErr)
		}
		// A watch persisted newer evidence while the HTTP request was in flight.
		c.ensureWatch(key)
		snapshot := c.languageSnapshot(state, settings, nil)
		return &snapshot, nil
	}
	if runtimeFailureProvesNoProcess(runtimeSnapshot, state.Generation) {
		c.releaseCapacity(ctx, key, state.Generation)
	}
	return c.transitionLaunchFailure(ctx, key, state, settings, controlErr)
}

func (c *Controller) transitionLaunchFailure(
	ctx context.Context,
	key TaskLanguageKey,
	state TaskLanguageState,
	settings TaskSettings,
	controlErr error,
) (*LanguageSnapshot, error) {
	snapshot, err := c.transitionRuntimeFailure(
		ctx, state, settings, "task_host_control_failed", controlErr, false,
	)
	c.scheduleDesiredRecovery(key, snapshot)
	return snapshot, err
}

func (c *Controller) transitionRuntimeFailure(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	errorCode string,
	cause error,
	processAbsent bool,
) (*LanguageSnapshot, error) {
	stored, err := c.updateState(ctx, state.TaskID, state.Language, func(next *TaskLanguageState) {
		next.Phase = PhaseError
		next.ErrorCode = errorCode
		next.ErrorMessage = cause.Error()
		next.LastTransitionAt = c.clock()
		if processAbsent {
			next.ProcessAbsentGeneration = next.Generation
		}
	})
	if err != nil {
		return nil, err
	}
	snapshot := c.languageSnapshot(*stored, settings, nil)
	return &snapshot, nil
}
