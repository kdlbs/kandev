package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const healthTokenBytes = 32

func newHealthToken() (string, error) {
	token := make([]byte, healthTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate backend health token: %w", err)
	}
	return hex.EncodeToString(token), nil
}

func backendEnv(ports portConfig, logLevel, consoleLogLevel string, debug bool, healthToken string, extra []string) []string {
	env := os.Environ()
	env = upsertEnv(env, "KANDEV_SERVER_PORT", fmt.Sprint(ports.BackendPort))
	env = upsertEnv(env, "KANDEV_AGENT_STANDALONE_PORT", fmt.Sprint(ports.AgentctlPort))
	env = upsertEnv(env, "KANDEV_DATABASE_PATH", resolveDatabasePath())
	env = upsertEnv(env, "KANDEV_DESKTOP_HEALTH_TOKEN", healthToken)
	if ports.WebPort != 0 {
		env = upsertEnv(env, "KANDEV_WEB_INTERNAL_URL", fmt.Sprintf("http://localhost:%d", ports.WebPort))
	}
	if logLevel != "" {
		env = upsertEnv(env, "KANDEV_LOG_LEVEL", logLevel)
	}
	env = upsertEnv(env, "KANDEV_CONSOLE_LOG_LEVEL", consoleLogLevel)
	if debug {
		env = upsertEnv(env, "KANDEV_DEBUG_AGENT_MESSAGES", "true")
		env = upsertEnv(env, "KANDEV_DEBUG_PPROF_ENABLED", "true")
	}
	for _, item := range extra {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env = upsertEnv(env, key, value)
		}
	}
	return env
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
