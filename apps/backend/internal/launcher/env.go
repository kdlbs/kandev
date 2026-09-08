package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

const healthTokenBytes = 32
const desktopNativeNotificationsEnabled = "true"
const backendPIDFileEnv = "KANDEV_BACKEND_PID_FILE"

func newHealthToken() (string, error) {
	token := make([]byte, healthTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate backend health token: %w", err)
	}
	return hex.EncodeToString(token), nil
}

func launchHealthToken() (string, error) {
	if os.Getenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS") == desktopNativeNotificationsEnabled {
		if token := os.Getenv("KANDEV_DESKTOP_HEALTH_TOKEN"); token != "" {
			return token, nil
		}
	}
	return newHealthToken()
}

func backendEnv(ports portConfig, logLevel, consoleLogLevel string, debug bool, healthToken string, extra []string, configs ...*config.Config) []string {
	if len(configs) > 0 {
		return backendEnvForConfig(ports, logLevel, consoleLogLevel, debug, healthToken, extra, configs[0])
	}
	return backendEnvForConfig(ports, logLevel, consoleLogLevel, debug, healthToken, extra, nil)
}

func backendEnvForConfig(ports portConfig, logLevel, consoleLogLevel string, debug bool, healthToken string, extra []string, cfg *config.Config) []string {
	env := stripAppliedProfileEnvironment(os.Environ())
	env = upsertEnv(env, "KANDEV_SERVER_PORT", fmt.Sprint(ports.BackendPort))
	env = upsertEnv(env, "KANDEV_AGENT_STANDALONE_PORT", fmt.Sprint(ports.AgentctlPort))
	if cfg != nil && cfg.SourceFor("database.path") == config.SourceEnvironment && strings.TrimSpace(cfg.Database.Path) != "" {
		env = upsertEnv(env, "KANDEV_DATABASE_PATH", resolveDatabasePathForConfig(cfg))
	}
	if configPath := configFileForChild(cfg); configPath != "" {
		env = upsertEnv(env, config.InternalConfigFileEnv, configPath)
		if cfg.Source.HomeFile {
			env = upsertEnv(env, config.InternalConfigHomeFileEnv, "1")
		}
	}
	env = upsertEnv(env, "KANDEV_DESKTOP_HEALTH_TOKEN", healthToken)
	if ports.WebPort != 0 {
		env = upsertEnv(env, "KANDEV_WEB_INTERNAL_URL", fmt.Sprintf("http://localhost:%d", ports.WebPort))
	}
	if logLevel != "" && (cfg == nil || cfg.SourceFor("logging.level") != config.SourceConfiguration || cfg.Logging.Level != logLevel) {
		env = upsertEnv(env, "KANDEV_LOG_LEVEL", logLevel)
	}
	env = upsertEnv(env, "KANDEV_CONSOLE_LOG_LEVEL", consoleLogLevel)
	if debug {
		env = upsertEnv(env, "KANDEV_DEBUG_AGENT_MESSAGES", "true")
		env = upsertEnv(env, "KANDEV_DEBUG_PPROF_ENABLED", "true")
		env = setEnvIfUnset(env, "KANDEV_WEB_TITLE_PREFIX", "Debug")
	}
	for _, item := range extra {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env = upsertEnv(env, key, value)
		}
	}
	return env
}

func stripAppliedProfileEnvironment(env []string) []string {
	defaults, err := profiles.EnvironmentDefaults()
	if err != nil {
		return env
	}
	filtered := make([]string, 0, len(env))
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok && profiles.WasApplied(key) && defaults[key] == value {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func processEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func setEnvIfUnset(env []string, key, value string) []string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return env
		}
	}
	return append(env, prefix+value)
}
