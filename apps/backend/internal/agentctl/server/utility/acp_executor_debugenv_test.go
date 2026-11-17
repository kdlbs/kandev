package utility

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// TestSanitizeEnvForAgentOmitsACPDebugSettings pins that one-shot probe and
// inference subprocesses never inherit agentctl's debug-log knobs, including
// through an explicit per-instance override.
func TestSanitizeEnvForAgentOmitsACPDebugSettings(t *testing.T) {
	t.Setenv("KANDEV_DEBUG_AGENT_MESSAGES", "true")
	t.Setenv("KANDEV_DEBUG_LOG_DIR", "/tmp/real-acp-logs")
	t.Setenv("KANDEV_DEBUG_ACP_MAX_FILES", "11")
	t.Setenv("KANDEV_KEEP_ME", "yes")

	env := sanitizeEnvForAgent(&InferenceConfigDTO{
		Env: map[string]string{
			"KANDEV_DEBUG_AGENT_MESSAGES": "true",
			"KANDEV_DEBUG_ACP_MAX_FILES":  "3",
		},
	})

	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if launchwrapper.IsAgentDebugSetting(key) {
			t.Fatalf("probe environment leaked debug setting %q", entry)
		}
	}
	found := false
	for _, entry := range env {
		if entry == "KANDEV_KEEP_ME=yes" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("unrelated environment variable was dropped")
	}
}
