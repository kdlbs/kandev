package lifecycle

import (
	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
)

// agentctlStartupConfigForExecutor keeps the host-only survival reaper out of
// managed remote/container environments. The backend owns the renewal loop for
// its host control server; a remote agentctl has no such loop and would reap
// its own instances after the unowned timeout.
func agentctlStartupConfigForExecutor(
	startup commonconfig.AgentctlStartupConfig,
	executorType string,
) commonconfig.AgentctlStartupConfig {
	if models.IsRemoteExecutorType(models.ExecutorType(executorType)) {
		startup.AgentSurvivalEnabled = false
	}
	return startup
}

func validateAgentctlStartupConfig(startup commonconfig.AgentctlStartupConfig) error {
	if !startup.Configured {
		return nil
	}
	return startup.Validate()
}

func agentctlStartupEnvironment(startup commonconfig.AgentctlStartupConfig) map[string]string {
	if !startup.Configured {
		return nil
	}
	raw, err := commonconfig.EncodeAgentctlStartupConfig(startup)
	if err != nil {
		// Executor entry points validate the contract before calling this helper.
		// Keep the helper total for test-only direct environment builders.
		return nil
	}
	return map[string]string{commonconfig.InternalAgentctlStartupConfigEnv: raw}
}

func mergeAgentctlStartupEnvironment(base map[string]string, startup commonconfig.AgentctlStartupConfig) map[string]string {
	result := make(map[string]string, len(base)+1)
	for key, value := range base {
		result[key] = value
	}
	for key, value := range agentctlStartupEnvironment(startup) {
		result[key] = value
	}
	return result
}
