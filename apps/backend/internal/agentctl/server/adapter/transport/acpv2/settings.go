package acpv2

import (
	"context"
	"errors"
	"slices"
	"strconv"

	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const modelOptionCategory = "model"

func (a *Adapter) GetSessionModelState() *streams.SessionModelState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session == "" {
		return nil
	}
	return a.sessionModelStateLocked()
}

func (a *Adapter) sessionModelStateLocked() *streams.SessionModelState {
	model := optionForCategory(a.settings, modelOptionCategory)
	return &streams.SessionModelState{
		SessionID: string(a.session), SessionSettingsGeneration: a.revision,
		CurrentModelID: model.CurrentValue, Models: sessionModels(model),
		ConfigOptions: slices.Clone(a.settings), ConfigOptionsSettled: true,
		ConfigOptionsComplete: a.settingsComplete, VerifiedConfigOptionIDs: slices.Clone(a.verifiedOptionIDs),
	}
}

func (a *Adapter) SetConfigOption(ctx context.Context, id, value string) error {
	_, err := a.SetConfigOptionWithResult(ctx, id, value)
	return err
}

func (a *Adapter) SetConfigOptionWithResult(ctx context.Context, id, value string) (streams.ConfigurationResult, error) {
	return a.setOption(ctx, id, "", value)
}

func (a *Adapter) SetModel(ctx context.Context, id string) error {
	_, err := a.SetModelWithResult(ctx, id)
	return err
}

func (a *Adapter) SetModelWithResult(ctx context.Context, id string) (streams.ConfigurationResult, error) {
	return a.setOption(ctx, "", modelOptionCategory, id)
}

func (a *Adapter) SetMode(ctx context.Context, id string) (streams.ModeResult, error) {
	value, err := a.setOption(ctx, "", "mode", id)
	return streams.ModeResult{Requested: id, Effective: value.Effective, Confirmed: value.Confirmed}, err
}

func (a *Adapter) setOption(ctx context.Context, id, category, value string) (streams.ConfigurationResult, error) {
	result := streams.ConfigurationResult{ConfigID: id, Requested: value}
	if err := a.acquire(ctx); err != nil {
		return result, err
	}
	defer a.release()
	session, option := a.configTarget(id, category)
	if session == "" || option.ID == "" {
		return result, streams.NewSessionConfigurationRejectedError(errors.New("ACP v2 config option is unavailable"))
	}
	result.ConfigID = option.ID
	requestValue, err := configValue(option, value)
	if err != nil {
		return result, streams.NewSessionConfigurationRejectedError(err)
	}
	reply, err := a.client.SetSessionConfigOption(ctx, wire.SetSessionConfigOptionRequest{
		SessionID: session, ConfigID: wire.ConfigID(option.ID), Value: requestValue,
	})
	if err != nil {
		return result, streams.NewSessionConfigurationRejectedError(safeRPCError("configuration change", err))
	}
	for _, returned := range reply.ConfigOptions {
		if string(returned.ConfigID) == option.ID {
			effective := projectConfigOption(returned)
			result.Effective, result.Confirmed = effective.CurrentValue, effective.Type != ""
			break
		}
	}
	var data map[string]any
	if result.Confirmed {
		data = map[string]any{
			streams.ConfigOptionsSourceKey:   streams.ConfigOptionsSourceProviderResponse,
			streams.ConfigOptionsConfigIDKey: result.ConfigID,
		}
	}
	a.project(func(batch *eventBatch) {
		a.projectSettings(reply.ConfigOptions, streams.SessionSettingsPolicyFromContext(ctx), data, batch)
		if result.Confirmed && (option.Category == modelOptionCategory || option.ID == modelOptionCategory) {
			state := a.sessionModelStateLocked()
			if state.HasConfigOptionEvidence() {
				result.ModelState = state
			}
		}
	})
	return result, nil
}

func (a *Adapter) configTarget(id, category string) (wire.SessionID, streams.ConfigOption) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.blocked {
		return "", streams.ConfigOption{}
	}
	if category != "" {
		return a.session, optionForCategory(a.settings, category)
	}
	for _, option := range a.settings {
		if option.ID == id {
			return a.session, option
		}
	}
	return "", streams.ConfigOption{}
}

func configValue(option streams.ConfigOption, value string) (wire.ConfigValue, error) {
	switch option.Type {
	case "select":
		for _, candidate := range option.Options {
			if candidate.Value == value {
				return wire.SelectConfigValue(value), nil
			}
		}
		return nil, errors.New("ACP v2 config choice is unavailable")
	case "boolean":
		if value == "true" || value == "false" {
			return wire.BooleanConfigValue(value == "true"), nil
		}
	}
	return nil, errors.New("invalid ACP v2 config value")
}

func (a *Adapter) projectSettings(
	options []wire.SessionConfigOption, policy streams.SessionSettingsPolicy, data map[string]any, batch *eventBatch,
) {
	a.settings = nil
	for _, wireOption := range options {
		if option := projectConfigOption(wireOption); option.ID != "" && option.Type != "" {
			a.settings = append(a.settings, option)
		}
	}
	a.revision++
	model, mode := optionForCategory(a.settings, modelOptionCategory), optionForCategory(a.settings, "mode")
	ids, valid := streams.ModelConfigOptionIDs(model.CurrentValue, a.settings)
	a.settingsComplete = valid && len(ids) == len(options)
	a.verifiedOptionIDs = nil
	if valid && !a.settingsComplete {
		a.verifiedOptionIDs = ids
	}
	if a.settingsComplete {
		if data == nil {
			data = make(map[string]any)
		}
		data[streams.ConfigOptionsCompleteKey] = true
	}
	event := streams.AgentEvent{
		Type: streams.EventTypeSessionModels, SessionID: string(a.session), SessionSettingsPolicy: policy,
		SessionSettingsGeneration: a.revision, CurrentModelID: model.CurrentValue, SessionModels: sessionModels(model),
		ConfigOptions: slices.Clone(a.settings), Data: data,
		VerifiedConfigOptionIDs: slices.Clone(a.verifiedOptionIDs),
	}
	batch.add(event)
	if mode.ID == "" {
		return
	}
	event = streams.AgentEvent{
		Type: streams.EventTypeSessionMode, SessionID: string(a.session), CurrentModeID: mode.CurrentValue,
		SessionSettingsPolicy: policy, SessionSettingsGeneration: a.revision,
	}
	for _, choice := range mode.Options {
		event.AvailableModes = append(event.AvailableModes, streams.SessionModeInfo{
			ID: choice.Value, Name: choice.Name, Description: choice.Description,
		})
	}
	batch.add(event)
}

func projectConfigOption(option wire.SessionConfigOption) streams.ConfigOption {
	projected := streams.ConfigOption{
		ID: string(option.ConfigID), Category: valueOrZero(option.Category),
		Name: option.Name, Description: valueOrZero(option.Description),
	}
	if option.Select != nil {
		projected.Type, projected.CurrentValue = "select", option.Select.CurrentValue
		projected.Options = appendChoices(projected.Options, option.Select.Options.Choices)
		for _, group := range option.Select.Options.Groups {
			projected.Options = appendChoices(projected.Options, group.Options)
		}
	}
	if option.Boolean != nil {
		projected.Type, projected.CurrentValue = "boolean", strconv.FormatBool(option.Boolean.CurrentValue)
	}
	return projected
}

func appendChoices(options []streams.ConfigOptionValue, choices []wire.ConfigSelectOption) []streams.ConfigOptionValue {
	for _, choice := range choices {
		options = append(options, streams.ConfigOptionValue{
			Value: choice.Value, Name: choice.Name, Description: valueOrZero(choice.Description),
		})
	}
	return options
}

func optionForCategory(options []streams.ConfigOption, category string) streams.ConfigOption {
	for _, option := range options {
		if option.Category == category || (category == modelOptionCategory && option.ID == category) {
			return option
		}
	}
	return streams.ConfigOption{}
}

func sessionModels(option streams.ConfigOption) []streams.SessionModelInfo {
	models := make([]streams.SessionModelInfo, 0, len(option.Options))
	for _, choice := range option.Options {
		models = append(models, streams.SessionModelInfo{
			ModelID: choice.Value, Name: choice.Name, Description: choice.Description,
		})
	}
	return models
}

func (a *Adapter) projectCommands(commands []wire.AvailableCommand, batch *eventBatch) {
	if commands == nil {
		return
	}
	out := make([]streams.AvailableCommand, 0, len(commands))
	for _, command := range commands {
		out = append(out, streams.AvailableCommand{Name: command.Name, Description: command.Description})
	}
	batch.add(streams.AgentEvent{Type: streams.EventTypeAvailableCommands, SessionID: string(a.session), AvailableCommands: out})
}
