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
