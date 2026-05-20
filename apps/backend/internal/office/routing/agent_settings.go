package routing

import (
	"encoding/json"
	"fmt"
)

// agentSettingsKey is the top-level key under AgentProfile.Settings
// where routing overrides live. The Settings JSON blob is shared with
// other features (skills, etc.) so we keep our footprint to one key.
const agentSettingsKey = "routing"

// ReadAgentOverrides extracts routing overrides from an
// AgentProfile.Settings JSON string. Returns zero-value overrides when
// settingsJSON is empty, when the JSON has no "routing" key, or when
// the embedded value is null. Other parse errors are propagated so
// callers can decide whether to surface them.
func ReadAgentOverrides(settingsJSON string) (AgentOverrides, error) {
	if settingsJSON == "" {
		return AgentOverrides{}, nil
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(settingsJSON), &raw); err != nil {
		return AgentOverrides{}, fmt.Errorf("routing: parse agent settings: %w", err)
	}
	entry, ok := raw[agentSettingsKey]
	if !ok || len(entry) == 0 || string(entry) == "null" {
		return AgentOverrides{}, nil
	}
	var ov AgentOverrides
	if err := json.Unmarshal(entry, &ov); err != nil {
		return AgentOverrides{}, fmt.Errorf("routing: parse agent overrides: %w", err)
	}
	return ov, nil
}

// WriteAgentOverrides returns a new settings JSON string with the
// "routing" key updated. When ov is zero, the key is removed (idempotent
// — writing a zero blob to settings that already has no "routing" key
// returns the input unchanged when possible).
func WriteAgentOverrides(settingsJSON string, ov AgentOverrides) (string, error) {
	raw, err := decodeSettings(settingsJSON)
	if err != nil {
		return "", err
	}
	if ov.IsZero() {
		delete(raw, agentSettingsKey)
	} else {
		blob, err := json.Marshal(ov)
		if err != nil {
			return "", fmt.Errorf("routing: marshal overrides: %w", err)
		}
		raw[agentSettingsKey] = blob
	}
	if len(raw) == 0 {
		return "", nil
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("routing: marshal settings: %w", err)
	}
	return string(out), nil
}

// decodeSettings parses the settings string into a raw map so other
// keys (skills, etc.) round-trip untouched. Empty input → empty map.
func decodeSettings(settingsJSON string) (map[string]json.RawMessage, error) {
	if settingsJSON == "" {
		return map[string]json.RawMessage{}, nil
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(settingsJSON), &raw); err != nil {
		return nil, fmt.Errorf("routing: parse agent settings: %w", err)
	}
	return raw, nil
}
