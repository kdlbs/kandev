package config

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// TestCollectAgentEnvOmitsACPDebugSettings pins that no ACP entry point passes
// agentctl's debug-log knobs to an agent, even when the host process has them.
func TestCollectAgentEnvOmitsACPDebugSettings(t *testing.T) {
	debugSettings := map[string]string{
		"KANDEV_DEBUG_AGENT_MESSAGES":      "true",
		"KANDEV_DEBUG_LOG_DIR":             "/tmp/real-acp-logs",
		"KANDEV_DEBUG_ACP_MAX_FILES":       "5",
		"KANDEV_DEBUG_ACP_MAX_FILE_BYTES":  "1024",
		"KANDEV_DEBUG_ACP_RETENTION_HOURS": "1",
		"KANDEV_KEEP_ME":                   "yes",
	}
	for key, value := range debugSettings {
		t.Setenv(key, value)
	}

	for _, tc := range []struct {
		name string
		env  []string
	}{
		{name: "without additional", env: CollectAgentEnvWithoutAdditional()},
		{name: "with error api", env: mustCollectAgentEnv(t)},
		{name: "instance variant", env: mustCollectInstanceAgentEnv(t)},
		{name: "debug settings in overrides", env: CollectAgentEnv(map[string]string{
			"KANDEV_DEBUG_AGENT_MESSAGES": "true",
			"KANDEV_DEBUG_LOG_DIR":        "/tmp/override-logs",
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, entry := range tc.env {
				key, _, _ := strings.Cut(entry, "=")
				if launchwrapper.IsAgentDebugSetting(key) {
					t.Fatalf("agent environment leaked debug setting %q", entry)
				}
			}
			if envSliceValue(tc.env, "KANDEV_KEEP_ME") != "yes" {
				t.Fatal("unrelated environment variable was dropped")
			}
		})
	}
}

func mustCollectAgentEnv(t *testing.T) []string {
	t.Helper()
	env, err := CollectAgentEnvWithError(nil)
	if err != nil {
		t.Fatalf("collect agent env: %v", err)
	}
	return env
}

func mustCollectInstanceAgentEnv(t *testing.T) []string {
	t.Helper()
	env, err := collectAgentEnvWithError(nil, true)
	if err != nil {
		t.Fatalf("collect instance agent env: %v", err)
	}
	return env
}
