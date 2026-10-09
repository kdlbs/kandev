package lifecycle

import (
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/githubauth"
)

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
