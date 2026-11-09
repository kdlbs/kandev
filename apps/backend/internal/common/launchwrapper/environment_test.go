package launchwrapper

import "testing"

func TestIsAgentDebugSetting(t *testing.T) {
	for _, key := range []string{
		"KANDEV_DEBUG_AGENT_MESSAGES",
		"KANDEV_DEBUG_LOG_DIR",
		"KANDEV_DEBUG_ACP_MAX_FILES",
		"KANDEV_DEBUG_ACP_RETENTION_HOURS",
		"KANDEV_DEBUG_ACP_MAX_FILE_BYTES",
		"kandev_debug_agent_messages",
	} {
		if !IsAgentDebugSetting(key) {
			t.Errorf("IsAgentDebugSetting(%q) = false, want true", key)
		}
	}
	for _, key := range []string{
		"KANDEV_KEEP_ME",
		"KANDEV_DEBUG_PPROF_ENABLED",
		"KANDEV_DEBUG_ACP",
		"PATH",
		"",
	} {
		if IsAgentDebugSetting(key) {
			t.Errorf("IsAgentDebugSetting(%q) = true, want false", key)
		}
	}
}

func TestStripAgentDebugSettings(t *testing.T) {
	env := []string{
		"KANDEV_DEBUG_AGENT_MESSAGES=true",
		"KANDEV_DEBUG_LOG_DIR=/tmp/logs",
		"KANDEV_DEBUG_ACP_MAX_FILES=5",
		"KANDEV_KEEP_ME=yes",
	}
	stripped := StripAgentDebugSettings(env)
	if len(stripped) != 1 || stripped[0] != "KANDEV_KEEP_ME=yes" {
		t.Fatalf("stripped environment = %#v, want only the unrelated variable", stripped)
	}
}
