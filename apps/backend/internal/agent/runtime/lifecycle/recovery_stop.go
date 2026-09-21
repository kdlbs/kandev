package lifecycle

import "github.com/kandev/kandev/internal/agent/executor"

// StopReasonRecoverableAgentFailure stops a failed agent while retaining the
// Kubernetes runtime and credentials needed to resume its session.
const StopReasonRecoverableAgentFailure = "recoverable agent failure"

// shouldPreserveKubernetesRuntime retains established Kubernetes resources for
// recovery while keeping fresh bootstrap rollback and explicit cleanup destructive.
func shouldPreserveKubernetesRuntime(execution *AgentExecution, reason string) bool {
	if execution == nil || execution.RuntimeName != executor.NameKubernetes {
		return false
	}
	return reason == StopReasonRecoverableAgentFailure ||
		(execution.isResumedSession && reason == StopReasonAgentBootstrapFailed)
}
