package lifecycle

import (
	"context"
	"encoding/json"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/processidentity"
)

// withAgentProcessGroup adds the running agent child's process group, as
// reported by agentctl's authenticated delivery descriptor, to the runtime
// identity. A shared runtime outlives one session's agent, so the group lets
// recovery prove that this session's writer exited. Missing evidence keeps the
// runtime-only identity.
func withAgentProcessGroup(
	ctx context.Context, client *agentctl.Client, streamID string, runtime processidentity.Identity,
) processidentity.Identity {
	if client == nil || runtime.Validate() != nil {
		return runtime
	}
	status, err := client.GetDeliveryStatus(ctx, streamID)
	if err != nil || status == nil || status.AgentProcess == nil {
		return runtime
	}
	return runtime.WithAgentGroup(*status.AgentProcess)
}

// boundAgentProcessGroup reuses the agent group recorded by the current prompt
// binding when it names the same runtime and submission. Reconciliation runs
// after a disconnect, when the child may already have exited.
func boundAgentProcessGroup(execution *AgentExecution, runtime processidentity.Identity, submissionID string) processidentity.Identity {
	value := execution.MetadataSnapshot()[deliveryPromptTurnBindingMetadataKey]
	if value == nil {
		return runtime
	}
	encoded, err := json.Marshal(value)
	var binding deliveryPromptTurnBinding
	if err != nil || json.Unmarshal(encoded, &binding) != nil || binding.ExecutionID != execution.ID ||
		binding.SubmissionID != deliverySubmissionIdentityForPrompt(submissionID, "") {
		return runtime
	}
	recorded := binding.OriginalRuntime
	if recorded.PID != runtime.PID || recorded.BirthToken != runtime.BirthToken || recorded.AgentGroupID <= 0 {
		return runtime
	}
	runtime.AgentGroupID, runtime.AgentBirthToken = recorded.AgentGroupID, recorded.AgentBirthToken
	return runtime
}
