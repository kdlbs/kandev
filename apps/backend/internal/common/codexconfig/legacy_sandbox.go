// Package codexconfig validates Codex configuration that can override a
// Kandev-managed filesystem permission profile.
package codexconfig

import (
	"encoding/json"
	"errors"

	"github.com/pelletier/go-toml/v2"
)

var ErrLegacySandbox = errors.New("legacy Codex sandbox configuration conflicts with task filesystem policy")

// HasLegacySandbox reports whether a loaded configuration contains an older
// sandbox contract. Profiles are traversed because Codex applies sandbox_mode
// from a selected profile after default_permissions is resolved.
func HasLegacySandbox(config map[string]any) bool {
	for key, value := range config {
		if key == "sandbox_mode" || key == "sandbox_workspace_write" {
			return true
		}
		switch nested := value.(type) {
		case map[string]any:
			if HasLegacySandbox(nested) {
				return true
			}
		case []any:
			for _, item := range nested {
				if child, ok := item.(map[string]any); ok && HasLegacySandbox(child) {
					return true
				}
			}
		}
	}
	return false
}

func ValidateJSON(contents string) error {
	if contents == "" {
		return nil
	}
	config := make(map[string]any)
	if err := json.Unmarshal([]byte(contents), &config); err != nil || config == nil {
		return errors.New("unable to validate Codex session configuration")
	}
	if HasLegacySandbox(config) {
		return ErrLegacySandbox
	}
	return nil
}

func ValidateTOML(contents []byte) error {
	config := make(map[string]any)
	if err := toml.Unmarshal(contents, &config); err != nil {
		return errors.New("unable to validate Codex sandbox configuration")
	}
	if HasLegacySandbox(config) {
		return ErrLegacySandbox
	}
	return nil
}
