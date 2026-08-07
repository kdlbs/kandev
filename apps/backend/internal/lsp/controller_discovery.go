package lsp

import (
	"context"
	"errors"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type discoveryCall struct {
	done chan struct{}
	err  error
}

const workspaceRootsChangedReason = "workspace_roots_changed"

// RefreshDiscovery explicitly refreshes bounded task-host evidence. It is a
// human-facing operation, so task authorization precedes environment lookup.
// Discovery is a bounded backend filesystem scan and never starts an execution
// or language server.
func (c *Controller) RefreshDiscovery(ctx context.Context, taskID string) error {
	if err := c.authorize(ctx, taskID); err != nil {
		return err
	}
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return err
	}
	return c.discoverTask(ctx, taskID, true)
}

// WorkspaceSourcesChanged reconciles one task's ordered source roots without
// restarting a generation. Dynamic-capable servers update in place; every
// other live language receives an explicit restart-required overlay.
func (c *Controller) WorkspaceSourcesChanged(ctx context.Context, taskID string) error {
	if err := c.authorize(ctx, taskID); err != nil {
		return err
	}
	if _, err := c.tasks.GetTask(ctx, taskID); err != nil {
		return err
	}
	states, err := c.store.ListTaskLSPLanguages(ctx, taskID)
	if err != nil {
		return err
	}
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	dynamic, restartRequired, refreshErr := c.refreshTaskWorkspace(ctx, environment)
	updateErrors := c.recordWorkspaceRefresh(ctx, taskID, states, dynamic, restartRequired, refreshErr)
	discoveryErr := c.discoverTask(ctx, taskID, true)
	return errors.Join(refreshErr, errors.Join(updateErrors...), discoveryErr)
}

func (c *Controller) refreshTaskWorkspace(
	ctx context.Context,
	environment *taskmodels.TaskEnvironment,
) (map[string]bool, map[string]bool, error) {
	dynamic := make(map[string]bool)
	restartRequired := make(map[string]bool)
	if !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return dynamic, restartRequired, nil
	}
	host, exists, err := c.runtimes.ExistingTaskHost(ctx, environment.ID)
	if err != nil || !exists || host == nil {
		return dynamic, restartRequired, err
	}
	result, err := host.RefreshTaskLSPWorkspace(ctx)
	if result == nil {
		return dynamic, restartRequired, err
	}
	for _, language := range result.DynamicLanguages {
		dynamic[language] = true
	}
	for _, language := range result.RestartRequiredLanguages {
		restartRequired[language] = true
	}
	return dynamic, restartRequired, err
}

func (c *Controller) recordWorkspaceRefresh(
	ctx context.Context,
	taskID string,
	states []TaskLanguageState,
	dynamic, restartRequired map[string]bool,
	refreshErr error,
) []error {
	var updateErrors []error
	for _, state := range states {
		if !phaseHasServer(state.Phase) {
			continue
		}
		requiresRestart := restartRequired[state.Language] || (refreshErr != nil && !dynamic[state.Language])
		_, updateErr := c.updateState(ctx, taskID, state.Language, func(next *TaskLanguageState) {
			switch {
			case dynamic[state.Language]:
				next.RestartRequired = false
				next.RestartRequiredReason = ""
			case requiresRestart:
				next.RestartRequired = true
				next.RestartRequiredReason = workspaceRootsChangedReason
			}
		})
		if updateErr != nil {
			updateErrors = append(updateErrors, updateErr)
		}
	}
	return updateErrors
}

func (c *Controller) discoverTask(ctx context.Context, taskID string, force bool) error {
	if !force {
		states, err := c.store.ListTaskLSPLanguages(ctx, taskID)
		if err != nil {
			return err
		}
		if discoveryAlreadyRecorded(states) {
			return nil
		}
	}

	call, leader := c.beginDiscovery(taskID)
	if !leader {
		select {
		case <-call.done:
			return call.err
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	err := c.runDiscovery(ctx, taskID)
	c.finishDiscovery(taskID, call, err)
	return err
}

func (c *Controller) beginDiscovery(taskID string) (*discoveryCall, bool) {
	c.discoveryMu.Lock()
	defer c.discoveryMu.Unlock()
	if current := c.discoveries[taskID]; current != nil {
		return current, false
	}
	call := &discoveryCall{done: make(chan struct{})}
	c.discoveries[taskID] = call
	return call, true
}

func (c *Controller) finishDiscovery(taskID string, call *discoveryCall, err error) {
	c.discoveryMu.Lock()
	call.err = err
	delete(c.discoveries, taskID)
	close(call.done)
	c.discoveryMu.Unlock()
}

func (c *Controller) runDiscovery(ctx context.Context, taskID string) error {
	environment, err := c.tasks.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if !readyTaskEnvironment(environment) || !ExecutorSupportsLSP(environment.ExecutorType) {
		return nil
	}
	result, discoverErr := c.runtimes.DiscoverTaskLanguages(ctx, environment.ID)
	if result == nil {
		result = &DiscoveryResult{State: DetectionUnavailable, Truncated: true, ScannedAt: c.clock()}
	}
	if result.ScannedAt.IsZero() {
		result.ScannedAt = c.clock()
	}
	if discoverErr != nil {
		result.State = DetectionUnavailable
		result.Truncated = true
	}
	if err := c.persistDiscovery(ctx, taskID, *result); err != nil {
		return errors.Join(discoverErr, err)
	}
	return discoverErr
}

func (c *Controller) persistDiscovery(ctx context.Context, taskID string, result DiscoveryResult) error {
	detected := make(map[string]bool, len(result.Languages))
	for _, language := range result.Languages {
		if installerLanguageSupported(language) {
			detected[language] = true
		}
	}
	var persistErrors []error
	for _, language := range registeredLanguages() {
		_, err := c.updateState(ctx, taskID, language, func(next *TaskLanguageState) {
			if result.State != DetectionUnavailable {
				next.Detected = detected[language]
			}
			next.DetectionState = result.State
			scannedAt := result.ScannedAt.UTC()
			next.DetectionScannedAt = &scannedAt
			next.DetectionTruncated = result.Truncated
		})
		if err != nil {
			persistErrors = append(persistErrors, err)
		}
	}
	return errors.Join(persistErrors...)
}

func discoveryAlreadyRecorded(states []TaskLanguageState) bool {
	if len(states) < len(registeredLanguages()) {
		return false
	}
	for _, state := range states {
		if state.DetectionState == DetectionUnknown || state.DetectionState == DetectionScanning ||
			state.DetectionScannedAt == nil || state.DetectionScannedAt.Equal(time.Time{}) {
			return false
		}
	}
	return true
}

func installerLanguageSupported(language string) bool {
	for _, supported := range registeredLanguages() {
		if language == supported {
			return true
		}
	}
	return false
}
