package backendapp

import (
	"os"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

var claudeProviderSelectorKeys = [...]string{
	"CLAUDE_CODE_USE_ANTHROPIC_AWS",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_FOUNDRY",
	"CLAUDE_CODE_USE_MANTLE",
	"CLAUDE_CODE_USE_VERTEX",
}

func isClaudeProviderSelector(key string) bool {
	for _, selector := range claudeProviderSelectorKeys {
		if key == selector {
			return true
		}
	}
	return false
}

// A nonempty provider selector does not establish the host subscription.
// Actual execution snapshots can explicitly unset a process value with "".
func snapshotClaudeProviderUncertain(profile *settingsmodels.AgentProfile, actual ...map[string]string) bool {
	for _, selector := range claudeProviderSelectorKeys {
		value := os.Getenv(selector)
		if len(actual) > 0 {
			for key, override := range actual[0] {
				if snapshotEnvKey(key) != selector {
					continue
				}
				if override != "" {
					return true
				}
				value = ""
			}
		} else if profile != nil {
			for _, entry := range profile.EnvVars {
				if snapshotEnvKey(entry.Key) == selector && (entry.SecretID != "" || entry.Value != "") {
					return true
				}
			}
		}
		if value != "" {
			return true
		}
	}
	return false
}
