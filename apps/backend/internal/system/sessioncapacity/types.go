package sessioncapacity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	SettingsKey         = "session_capacity"
	EnvironmentVariable = "KANDEV_MAX_CONCURRENT_SESSIONS"
	DefaultMaxSessions  = 5
	maxSessionsLimit    = 2147483647
)

type Source string

const (
	SourceDefault     Source = "default"
	SourceSetting     Source = "setting"
	SourceEnvironment Source = "environment"
)

var (
	ErrValidation        = errors.New("session capacity settings validation")
	ErrInvalidPersisted  = errors.New("invalid persisted session capacity settings")
	ErrEnvironmentLocked = errors.New("session capacity is controlled by the environment")
	ErrTargetUnavailable = errors.New("session capacity live target is unavailable")
)

type Settings struct {
	Enabled     bool `json:"enabled"`
	MaxSessions int  `json:"max_sessions"`
}

// SettingsPatch is a partial update. A nil field means that the field was
// omitted from the request and must retain its saved value.
type SettingsPatch struct {
	Enabled     *bool `json:"enabled"`
	MaxSessions *int  `json:"max_sessions"`
}

// UnmarshalJSON rejects null values because null cannot represent an omitted
// field once the patch has been decoded into pointers.
func (p *SettingsPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("session capacity settings patch must be an object")
	}

	var decoded SettingsPatch
	for field, raw := range fields {
		switch field {
		case "enabled":
			if isJSONNull(raw) {
				return errors.New("enabled must be a boolean")
			}
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("enabled must be a boolean: %w", err)
			}
			decoded.Enabled = &value
		case "max_sessions":
			if isJSONNull(raw) {
				return errors.New("max_sessions must be an integer")
			}
			var value int
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("max_sessions must be an integer: %w", err)
			}
			decoded.MaxSessions = &value
		default:
			return fmt.Errorf("unknown session capacity setting %q", field)
		}
	}
	*p = decoded
	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// Apply overlays every field present in the patch onto base.
func (p SettingsPatch) Apply(base Settings) Settings {
	if p.Enabled != nil {
		base.Enabled = *p.Enabled
	}
	if p.MaxSessions != nil {
		base.MaxSessions = *p.MaxSessions
	}
	return base
}

type Response struct {
	Settings  Settings  `json:"settings"`
	Effective Effective `json:"effective"`
}

type Effective struct {
	Enabled     bool   `json:"enabled"`
	MaxSessions int    `json:"max_sessions"`
	Source      Source `json:"source"`
	Locked      bool   `json:"locked"`
}

type Environment struct {
	Value   string
	Present bool
}

type Resolution struct {
	Response
	InvalidEnvironment bool
}

// DefaultSettings returns the disabled setting with its editable default
// maximum retained for a later opt-in.
func DefaultSettings() Settings {
	return Settings{Enabled: false, MaxSessions: DefaultMaxSessions}
}

func Validate(settings Settings) error {
	if settings.MaxSessions < 1 || settings.MaxSessions > maxSessionsLimit {
		return fmt.Errorf(
			"%w: max_sessions must be between 1 and %d",
			ErrValidation,
			maxSessionsLimit,
		)
	}
	return nil
}
