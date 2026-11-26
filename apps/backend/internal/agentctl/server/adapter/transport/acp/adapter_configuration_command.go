package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const configurationCommandStatusApplied = "applied"

type configurationCommandSpec struct {
	configID             string
	requestedID          string
	requested            string
	partialConfiguration bool
}

type configurationCommandTracker struct {
	sessionID        string
	promptGeneration uint64
	spec             configurationCommandSpec
	baseline         string
	configGeneration uint64
	conflicted       bool
	observed         map[string]string
}

func (a *Adapter) configurationCommandForPrompt(
	sessionID, message string,
	promptGeneration uint64,
) *configurationCommandTracker {
	conflicted := !a.configChangeMu.TryLock()
	if !conflicted {
		defer a.configChangeMu.Unlock()
	}
	return a.buildConfigurationCommandTracker(sessionID, message, promptGeneration, conflicted)
}

func (a *Adapter) buildConfigurationCommandTracker(
	sessionID, message string,
	promptGeneration uint64,
	conflicted bool,
) *configurationCommandTracker {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed || a.sessionID != sessionID {
		return nil
	}
	spec := a.dialect.configurationCommand
	if spec == nil {
		spec = modelConfigurationCommand
	}
	request := spec(message, a.availableModels, a.availableConfigOptions, a.availableCommandsForSession(sessionID))
	if request == nil {
		return nil
	}
	tracker := &configurationCommandTracker{
		sessionID:        sessionID,
		promptGeneration: promptGeneration,
		spec:             *request,
		baseline:         configOptionValue(a.availableConfigOptions, request.configID),
		configGeneration: a.configGeneration,
		observed:         make(map[string]string),
	}
	if conflicted {
		tracker.conflicted = true
	}
	return tracker
}

func (a *Adapter) availableCommandsForSession(sessionID string) []streams.AvailableCommand {
	return a.availableCommandsBySession[sessionID]
}

func (a *Adapter) retainAvailableCommandsForSessionLocked(sessionID string) {
	commands := a.availableCommandsBySession[sessionID]
	a.availableCommandsBySession = make(map[string][]streams.AvailableCommand)
	if len(commands) > 0 {
		a.availableCommandsBySession[sessionID] = commands
	}
}

func modelConfigurationCommand(
	message string,
	models []modelInfo,
	options []streams.ConfigOption,
	commands []streams.AvailableCommand,
) *configurationCommandSpec {
	if request := validatedCommandAction(message, options, commands); request != nil {
		return request
	}
	parts := strings.Fields(message)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "/model") || !modelIsAdvertised(parts[1], models, options) {
		return nil
	}
	return &configurationCommandSpec{
		configID:    modelConfigOptionID(options),
		requestedID: parts[1],
		requested:   modelDisplayName(parts[1], models, options),
	}
}

func validatedCommandAction(
	message string,
	options []streams.ConfigOption,
	commands []streams.AvailableCommand,
) *configurationCommandSpec {
	parts := strings.Fields(message)
	if len(parts) != 1 || !strings.HasPrefix(parts[0], "/") {
		return nil
	}
	name := strings.TrimPrefix(parts[0], "/")
	for _, command := range commands {
		if !strings.EqualFold(command.Name, name) || command.Action == nil ||
			command.Action.Kind != normalizedSetConfigOptionActionKind ||
			command.Action.ConfigID == "" || command.Action.Value == "" {
			continue
		}
		requestedID := command.Action.Value
		if command.Action.ResetValue != "" && configOptionValue(options, command.Action.ConfigID) == requestedID {
			requestedID = command.Action.ResetValue
		}
		return &configurationCommandSpec{
			configID:    command.Action.ConfigID,
			requestedID: requestedID,
			requested:   configValueName(options, command.Action.ConfigID, requestedID),
		}
	}
	return nil
}

func modelIsAdvertised(value string, models []modelInfo, options []streams.ConfigOption) bool {
	for _, model := range models {
		if model.ModelId == value {
			return true
		}
	}
	for _, option := range options {
		if option.ID != modelConfigOptionID(options) && option.Category != configOptionIDModel {
			continue
		}
		for _, item := range option.Options {
			if item.Value == value {
				return true
			}
		}
	}
	return false
}

func modelDisplayName(value string, models []modelInfo, options []streams.ConfigOption) string {
	for _, model := range models {
		if model.ModelId == value && model.Name != "" {
			return model.Name
		}
	}
	return configValueName(options, modelConfigOptionID(options), value)
}

func configOptionValue(options []streams.ConfigOption, configID string) string {
	for _, option := range options {
		if option.ID == configID {
			return option.CurrentValue
		}
	}
	return ""
}

func configValueName(options []streams.ConfigOption, configID, value string) string {
	for _, option := range options {
		if option.ID != configID {
			continue
		}
		for _, item := range option.Options {
			if item.Value == value && item.Name != "" {
				return item.Name
			}
		}
	}
	return value
}

func configurationCommandResult(
	tracker *configurationCommandTracker,
	configGeneration uint64,
) *streams.ConfigurationCommandResult {
	if tracker == nil {
		return nil
	}
	result := &streams.ConfigurationCommandResult{
		Status:    "unconfirmed",
		ConfigID:  tracker.spec.configID,
		Requested: tracker.spec.requested,
	}
	if len(tracker.observed) != 1 {
		return result
	}
	for effectiveID, effectiveName := range tracker.observed {
		result.Effective = effectiveName
		if tracker.conflicted || tracker.configGeneration == 0 || tracker.configGeneration != configGeneration ||
			effectiveID == tracker.baseline {
			return result
		}
		if effectiveID != tracker.spec.requestedID {
			return result
		}
		if tracker.spec.partialConfiguration {
			result.Status = "clamped"
		} else {
			result.Confirmed = true
			result.Status = configurationCommandStatusApplied
		}
	}
	return result
}

func (a *Adapter) observeConfigurationCommandValue(
	sessionID string,
	promptGeneration uint64,
	configID, value, display string,
) {
	turn := a.currentPromptTurn()
	if turn == nil || promptGeneration == 0 || turn.promptGeneration != promptGeneration || value == "" {
		return
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	tracker := turn.configurationCommand
	if tracker == nil || tracker.sessionID != sessionID || tracker.spec.configID != configID {
		return
	}
	tracker.observed[value] = display
}

func (a *Adapter) observeConfigurationCommandOptions(
	sessionID string,
	promptGeneration uint64,
	options []streams.ConfigOption,
) {
	for _, option := range options {
		if option.CurrentValue == "" {
			continue
		}
		name := configValueName(options, option.ID, option.CurrentValue)
		a.observeConfigurationCommandValue(sessionID, promptGeneration, option.ID, option.CurrentValue, name)
	}
}

func (a *Adapter) observeConfigurationCommandMode(sessionID string, promptGeneration uint64, value string) {
	a.observeConfigurationCommandValue(
		sessionID,
		promptGeneration,
		codexCollaborationModeConfigID,
		value,
		value,
	)
}

func (a *Adapter) configurationCommandResult(turn *promptTurnState, sessionID string) *streams.ConfigurationCommandResult {
	if turn == nil {
		return nil
	}
	a.mu.RLock()
	active := a.sessionID == sessionID && !a.closed
	configGeneration := a.configGeneration
	options := cloneConfigOptions(a.availableConfigOptions)
	a.mu.RUnlock()
	if !active {
		return nil
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	tracker := turn.configurationCommand
	if tracker == nil || tracker.sessionID != sessionID || tracker.promptGeneration != turn.promptGeneration {
		return nil
	}
	for value := range tracker.observed {
		tracker.observed[value] = configValueName(options, tracker.spec.configID, value)
	}
	return configurationCommandResult(tracker, configGeneration)
}
