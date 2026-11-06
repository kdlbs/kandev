package sessioncapacity

import (
	"maps"
	"os"
	"strconv"
	"strings"
)

// ReadEnvironment captures the process environment value and whether the
// variable exists. Presence is separate because an explicitly blank value is
// different from an unset variable while resolving diagnostics.
func ReadEnvironment() Environment {
	value, present := os.LookupEnv(EnvironmentVariable)
	return Environment{Value: value, Present: present}
}

// Resolve applies the precedence environment > saved setting > default.
// Invalid nonblank environment values are ignored and reported in the result.
func Resolve(configured *Settings, environment Environment) (Resolution, error) {
	settings := DefaultSettings()
	source := SourceDefault
	if configured != nil {
		if err := Validate(*configured); err != nil {
			return Resolution{}, err
		}
		settings = *configured
		settings.GradeLimits = maps.Clone(configured.GradeLimits)
		if settings.Policy == "" {
			settings.Policy = PolicyAutomatic
		}
		if settings.GradeLimits == nil {
			settings.GradeLimits = map[string]int{}
		}
		source = SourceSetting
	}

	resolution := Resolution{Response: Response{
		Settings:  settings,
		Effective: effectiveFor(settings, source),
	}}
	raw := strings.TrimSpace(environment.Value)
	if !environment.Present || raw == "" {
		return resolution, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 || value > maxSessionsLimit {
		resolution.InvalidEnvironment = true
		return resolution, nil
	}
	resolution.Effective = Effective{
		Enabled:     value > 0,
		MaxSessions: int(value),
		Source:      SourceEnvironment,
		Locked:      true,
		Policy:      PolicyAutomatic,
		GradeLimits: map[string]int{},
	}
	return resolution, nil
}

func effectiveFor(settings Settings, source Source) Effective {
	effective := Effective{Source: source, Policy: PolicyAutomatic, GradeLimits: map[string]int{}}
	if settings.Enabled {
		effective.Enabled = true
		effective.Policy = settings.Policy
		effective.MaxSessions = settings.MaxSessions
		effective.GradeLimits = maps.Clone(settings.GradeLimits)
	}
	return effective
}
