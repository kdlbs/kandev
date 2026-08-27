package queuesettings

import (
	"fmt"
	"strconv"
	"strings"
)

// Resolve combines persisted settings with startup configuration and an
// optional environment override. A malformed environment value is reported
// but ignored so the configuration/persisted/default setting remains
// effective. Environment takes precedence over startup configuration, which
// takes precedence over the persisted setting.
func Resolve(configured *Settings, environment Environment, startup ...Configuration) (Resolution, error) {
	settings := DefaultSettings()
	source := SourceDefault
	var configuration Configuration
	if len(startup) > 0 {
		configuration = startup[0]
	}
	if configuration.Present && configuration.Value < 0 {
		return Resolution{}, fmt.Errorf("%w: max_per_session must be zero or greater", ErrValidation)
	}
	if configured != nil {
		if err := Validate(*configured); err != nil {
			return Resolution{}, err
		}
		settings = *configured
		source = SourceSetting
	}
	resolution := Resolution{Response: Response{
		Settings: settings,
		Effective: Effective{
			MaxPerSession:    settings.MaxPerSession,
			Source:           source,
			MergeEnabled:     settings.MergeEnabled,
			AutoMergeEnabled: settings.AutoMergeEnabled,
		},
	}}
	raw := strings.TrimSpace(environment.Value)
	if !environment.Present || raw == "" {
		return resolveConfiguration(resolution, configuration), nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		resolution.InvalidEnvironment = true
		return resolveConfiguration(resolution, configuration), nil
	}
	if value <= 0 {
		value = 0
	}
	resolution.Effective = Effective{
		MaxPerSession:    value,
		Source:           SourceEnvironment,
		Locked:           true,
		MergeEnabled:     settings.MergeEnabled,
		AutoMergeEnabled: settings.AutoMergeEnabled,
	}
	return resolution, nil
}

func resolveConfiguration(resolution Resolution, configuration Configuration) Resolution {
	if !configuration.Present {
		return resolution
	}
	resolution.Effective = Effective{
		MaxPerSession:    configuration.Value,
		Source:           SourceConfiguration,
		Locked:           true,
		MergeEnabled:     resolution.Settings.MergeEnabled,
		AutoMergeEnabled: resolution.Settings.AutoMergeEnabled,
	}
	return resolution
}
