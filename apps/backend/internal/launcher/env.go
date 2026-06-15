package launcher

import (
	"fmt"
	"os"
)

func backendEnv(ports portConfig, logLevel string, debug bool) []string {
	env := os.Environ()
	env = upsertEnv(env, "KANDEV_SERVER_PORT", fmt.Sprint(ports.BackendPort))
	env = upsertEnv(env, "KANDEV_WEB_INTERNAL_URL", fmt.Sprintf("http://localhost:%d", ports.WebPort))
	env = upsertEnv(env, "KANDEV_AGENT_STANDALONE_PORT", fmt.Sprint(ports.AgentctlPort))
	env = upsertEnv(env, "KANDEV_DATABASE_PATH", resolveDatabasePath())
	if logLevel != "" {
		env = upsertEnv(env, "KANDEV_LOG_LEVEL", logLevel)
	}
	if debug {
		env = upsertEnv(env, "KANDEV_DEBUG_AGENT_MESSAGES", "true")
		env = upsertEnv(env, "KANDEV_DEBUG_PPROF_ENABLED", "true")
	}
	return env
}

func webEnv(ports portConfig, production bool, debug bool) []string {
	env := os.Environ()
	env = upsertEnv(env, "KANDEV_API_BASE_URL", ports.BackendURL)
	env = upsertEnv(env, "PORT", fmt.Sprint(ports.WebPort))
	env = upsertEnv(env, "HOSTNAME", "127.0.0.1")
	if production {
		env = upsertEnv(env, "NODE_ENV", "production")
		env = removeEnv(env, "NEXT_PUBLIC_KANDEV_API_PORT")
	} else {
		env = upsertEnv(env, "NEXT_PUBLIC_KANDEV_API_PORT", fmt.Sprint(ports.BackendPort))
	}
	if debug {
		env = upsertEnv(env, "KANDEV_DEBUG", "true")
		env = upsertEnv(env, "NEXT_PUBLIC_KANDEV_DEBUG", "true")
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

func removeEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0]
	for _, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			continue
		}
		out = append(out, item)
	}
	return out
}
