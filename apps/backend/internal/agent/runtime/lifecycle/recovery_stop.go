package lifecycle

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
)

// StopReasonRecoverableAgentFailure stops a failed agent while retaining the
// task runtime and credentials needed to resume its session.
const StopReasonRecoverableAgentFailure = "recoverable agent failure"

// errRecoverableStopUnconfirmed leaves a preserved runtime tracked after the
// agent process could not be confirmed stopped.
var errRecoverableStopUnconfirmed = errors.New("agent stop was not confirmed; runtime retained for retry")

// shouldPreserveRemoteDockerStopReason keeps an unconfirmed agent stop from
// stopping or removing a workspace that recovery still owns.
func shouldPreserveRemoteDockerStopReason(reason string) bool {
	return reason == StopReasonRecoverableAgentFailure || reason == StopReasonAgentBootstrapFailed
}

// shouldPreserveKubernetesRuntime retains established Kubernetes resources for
// recovery while keeping fresh bootstrap rollback and explicit cleanup destructive.
func shouldPreserveKubernetesRuntime(execution *AgentExecution, reason string) bool {
	if execution == nil || execution.RuntimeName != executor.NameKubernetes {
		return false
	}
	return reason == StopReasonRecoverableAgentFailure ||
		(execution.isResumedSession && reason == StopReasonAgentBootstrapFailed)
}

// shouldPreserveRemoteDockerRuntime retains an established remote workspace.
// Bootstrap failure only preserves compute this attempt actually attached to;
// a fresh allocation still rolls back destructively.
func shouldPreserveRemoteDockerRuntime(execution *AgentExecution, reason string) bool {
	if execution == nil || execution.RuntimeName != executor.NameRemoteDocker {
		return false
	}
	if reason == StopReasonRecoverableAgentFailure {
		return true
	}
	return reason == StopReasonAgentBootstrapFailed && execution.attachedExistingCompute
}

func shouldPreserveRecoverableRuntime(execution *AgentExecution, reason string) bool {
	if shouldPreserveKubernetesRuntime(execution, reason) {
		return true
	}
	if shouldPreserveRemoteDockerRuntime(execution, reason) {
		return true
	}
	return execution != nil && execution.RuntimeName == executor.NameDocker && reason == StopReasonRecoverableAgentFailure
}

// retainsRuntimeOnLaunchRollback reports whether a failed registered launch
// must keep the runtime it attached to instead of rolling it back.
func retainsRuntimeOnLaunchRollback(execution *AgentExecution) bool {
	if execution == nil {
		return false
	}
	if execution.RuntimeName == agentruntime.RuntimeKubernetes {
		return execution.isResumedSession || execution.attachedExistingCompute
	}
	return execution.RuntimeName == agentruntime.RuntimeRemoteDocker && execution.attachedExistingCompute
}
