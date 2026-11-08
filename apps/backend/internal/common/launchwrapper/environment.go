package launchwrapper

import "strings"

// StripControlCredentials removes control-plane credentials from child environments.
// These values belong to the host process and cannot be restored by profile overrides.
func StripControlCredentials(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "AGENTCTL_BOOTSTRAP_NONCE", "AGENTCTL_AUTH_TOKEN", "KANDEV_DESKTOP_HEALTH_TOKEN":
			continue
		}
		result = append(result, entry)
	}
	return result
}

// agentDebugSettingPrefix covers agentctl's ACP debug-log knobs
// (KANDEV_DEBUG_ACP_MAX_FILES, KANDEV_DEBUG_ACP_RETENTION_HOURS, ...). Agent
// subprocesses must not inherit them: a nested tool would otherwise write
// frames into the developer's real log directory and rotate the owning
// session's evidence away.
const agentDebugSettingPrefix = "KANDEV_DEBUG_ACP_"

// IsAgentDebugSetting reports whether key is one of agentctl's ACP debug-log
// settings.
func IsAgentDebugSetting(key string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(key))
	switch normalized {
	case "KANDEV_DEBUG_AGENT_MESSAGES", "KANDEV_DEBUG_LOG_DIR":
		return true
	}
	return strings.HasPrefix(normalized, agentDebugSettingPrefix)
}

// StripAgentDebugSettings removes ACP debug-log settings from a child
// environment. The owning agentctl keeps logging because it reads its own
// process environment.
func StripAgentDebugSettings(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if IsAgentDebugSetting(key) {
			continue
		}
		result = append(result, entry)
	}
	return result
}
