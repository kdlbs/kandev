package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/githubauth"
)

func (m *Manager) managedGitEnvironmentForPromotion(
	ctx context.Context, execution *AgentExecution, launchEnv map[string]string,
) (map[string]string, error) {
	if launchEnv[githubauth.CredentialBrokerURLEnv] == "" || adoptionPreservesPeer(nil, execution) {
		return nil, nil
	}
	if !hasManagedGitCredentialBrokerEnv(launchEnv) {
		return nil, errors.New("managed Git credential lease is missing")
	}
	workspaceEnv, err := m.environmentForPreparedLaunch(ctx, execution)
	if err != nil {
		return nil, err
	}
	env, err := composeExecutionRuntimeEnvironment(workspaceEnv, launchEnv)
	if err != nil {
		return nil, fmt.Errorf("apply managed Git credentials during workspace promotion: %w", err)
	}
	return env, nil
}

func normalizeManagedGitHelperEnvironment(runtimeName agentruntime.Runtime, env map[string]string) {
	if !hasManagedGitCredentialBrokerEnv(env) {
		return
	}
	switch runtimeName {
	case agentruntime.RuntimeDocker, agentruntime.RuntimeRemoteDocker:
		env[githubauth.CredentialHelperPathEnv] = remoteAgentctlExecutablePath
	case agentruntime.RuntimeKubernetes:
		env[githubauth.CredentialHelperPathEnv] = kubernetesAgentctlPath
	}
}
