package lsp

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/lsp/installer"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (c *Controller) launchReserved(
	ctx context.Context,
	state TaskLanguageState,
	settings TaskSettings,
	environment *taskmodels.TaskEnvironment,
	action Action,
) (*LanguageSnapshot, error) {
	key := TaskLanguageKey{TaskID: state.TaskID, Language: state.Language}
	host, err := c.runtimes.EnsureTaskHost(ctx, environment.ID)
	if err != nil {
		c.releaseCapacity(ctx, key, state.Generation)
		snapshot, transitionErr := c.transition(ctx, state, settings, PhaseError, "task_host_unavailable", err.Error())
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
	snapshot, err := c.transition(ctx, state, settings, PhaseError, "task_host_control_failed", controlErr.Error())
	c.scheduleDesiredRecovery(key, snapshot)
	return snapshot, err
}
