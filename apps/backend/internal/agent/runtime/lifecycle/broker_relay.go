package lifecycle

import (
	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

// sameHostBrokerUpstream returns the backend broker URL an instance on the
// backend's own host relays to. Containerized and SSH executors reach the
// broker directly and relay nothing, so they get an empty upstream.
func sameHostBrokerUpstream(executorType string, env map[string]string) string {
	t := models.ExecutorType(executorType)
	if models.IsContainerizedExecutorType(t) || t == models.ExecutorTypeSSH {
		return ""
	}
	return env[githubauth.CredentialBrokerURLEnv]
}

// withBrokerRelayURL returns env with the broker URL replaced by the instance
// relay when the execution runs on the backend host and its instance listens
// on port. The input is not modified.
func withBrokerRelayURL(executorType string, env map[string]string, port int) map[string]string {
	if port <= 0 || sameHostBrokerUpstream(executorType, env) == "" {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	out[githubauth.CredentialBrokerURLEnv] = agentctlconfig.BrokerRelayURL(port)
	return out
}
