package queuesettings

import (
	"strconv"
	"strings"
)

// Resolve combines persisted configuration with an optional environment
// override. A malformed environment value is reported but ignored so the
// persisted/default setting remains effective.
func Resolve(configured *Settings, environment Environment) (Resolution, error) {
	settings := DefaultSettings()
	source := SourceDefault
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
			MaxPerSession: settings.MaxPerSession,
			Source:        source,
		},
	}}
	raw := strings.TrimSpace(environment.Value)
	if !environment.Present || raw == "" {
		return resolution, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		resolution.InvalidEnvironment = true
		return resolution, nil
	}
	if value <= 0 {
		value = 0
	}
	resolution.Effective = Effective{
		MaxPerSession: value,
		Source:        SourceEnvironment,
		Locked:        true,
	}
	return resolution, nil
}
