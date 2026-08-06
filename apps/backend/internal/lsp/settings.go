package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"time"
)

const settingsRetryDelay = time.Second

// NotifySettingsChanged schedules one task-level reconciliation. The settings
// event handler may call this synchronously; duplicate notifications coalesce.
func (c *Controller) NotifySettingsChanged() {
	select {
	case c.settingsSignal <- struct{}{}:
	default:
	}
}

// ApplySettings pushes changed configuration into live generations, then
// reconciles inherited task policies. It never creates a second ownership path.
func (c *Controller) ApplySettings(ctx context.Context) error {
	c.settingsApplyMu.Lock()
	defer c.settingsApplyMu.Unlock()

	loaded, err := c.loadSettings(ctx)
	if err != nil {
		return err
	}
	next := normalizeTaskSettings(loaded)
	if c.appliedSettings != nil && taskSettingsEqual(*c.appliedSettings, next) {
		return nil
	}

	applyErrors := c.pushSettingsToLiveGenerations(ctx, next)
	if reconcileErr := c.ReconcileAll(ctx); reconcileErr != nil {
		applyErrors = append(applyErrors, reconcileErr)
	}
	if err := errors.Join(applyErrors...); err != nil {
		return err
	}
	remembered := normalizeTaskSettings(next)
	c.appliedSettings = &remembered
	return nil
}

func (c *Controller) pushSettingsToLiveGenerations(ctx context.Context, next TaskSettings) []error {
	states, err := c.store.ListAllTaskLSPLanguages(ctx)
	if err != nil {
		return []error{err}
	}
	var applyErrors []error
	for _, state := range states {
		if !phaseHasServer(state.Phase) || state.Generation == 0 ||
			!serverConfigurationChanged(c.appliedSettings, next, state.Language) {
			continue
		}
		host, hostErr := c.resolveExistingHost(ctx, state.TaskID)
		if hostErr != nil {
			applyErrors = append(applyErrors, hostErr)
			continue
		}
		configuration := append(json.RawMessage(nil), next.ServerConfigs[state.Language]...)
		if len(configuration) == 0 {
			configuration = json.RawMessage(`{}`)
		}
		_, configErr := host.UpdateTaskLSPConfiguration(ctx, TaskHostConfigurationRequest{
			Language: state.Language, Generation: state.Generation, Configuration: configuration,
		})
		if configErr != nil {
			applyErrors = append(applyErrors, configErr)
		}
	}
	return applyErrors
}

func (c *Controller) runSettingsWorker(ctx context.Context) {
	defer c.lifecycleWG.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.settingsSignal:
		}
		for c.ApplySettings(ctx) != nil {
			timer := time.NewTimer(settingsRetryDelay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}
}

func (c *Controller) rememberSettings(settings TaskSettings) {
	normalized := normalizeTaskSettings(settings)
	c.settingsApplyMu.Lock()
	c.appliedSettings = &normalized
	c.settingsApplyMu.Unlock()
}

func normalizeTaskSettings(settings TaskSettings) TaskSettings {
	normalized := TaskSettings{
		AutoStartLanguages:   normalizedLanguages(settings.AutoStartLanguages),
		AutoInstallLanguages: normalizedLanguages(settings.AutoInstallLanguages),
		ServerConfigs:        make(map[string]json.RawMessage, len(settings.ServerConfigs)),
	}
	for language, configuration := range settings.ServerConfigs {
		if !json.Valid(configuration) {
			continue
		}
		var compact bytes.Buffer
		if json.Compact(&compact, configuration) == nil {
			normalized.ServerConfigs[language] = append(json.RawMessage(nil), compact.Bytes()...)
		}
	}
	return normalized
}

func normalizedLanguages(languages []string) []string {
	unique := make(map[string]struct{}, len(languages))
	for _, language := range languages {
		unique[language] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for language := range unique {
		result = append(result, language)
	}
	sort.Strings(result)
	return result
}

func taskSettingsEqual(left, right TaskSettings) bool {
	if !slices.Equal(left.AutoStartLanguages, right.AutoStartLanguages) ||
		!slices.Equal(left.AutoInstallLanguages, right.AutoInstallLanguages) ||
		len(left.ServerConfigs) != len(right.ServerConfigs) {
		return false
	}
	for language, configuration := range left.ServerConfigs {
		if !bytes.Equal(configuration, right.ServerConfigs[language]) {
			return false
		}
	}
	return true
}

func serverConfigurationChanged(previous *TaskSettings, next TaskSettings, language string) bool {
	nextConfiguration, nextConfigured := next.ServerConfigs[language]
	if previous == nil {
		return nextConfigured
	}
	previousConfiguration, previouslyConfigured := previous.ServerConfigs[language]
	return nextConfigured != previouslyConfigured || !bytes.Equal(previousConfiguration, nextConfiguration)
}
