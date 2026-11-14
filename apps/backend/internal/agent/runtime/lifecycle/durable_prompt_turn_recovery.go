package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
)

const deliveryPromptTurnBindingMetadataKey = models.SessionMetaKeyDeliveryPromptTurnBinding

type deliveryPromptTurnBinding = models.AgentDeliveryPromptTurnBinding

func (sm *SessionManager) persistDurablePromptTurnBinding(ctx context.Context, execution *AgentExecution, submissionID string) error {
	// Clear a predecessor's binding even when the next prompt is legacy or
	// has no Kandev turn. Missing evidence must not resurrect an older turn.
	execution.setMetadataValue(deliveryPromptTurnBindingMetadataKey, nil)
	turnID := execution.promptTurnIDSnapshot()
	if execution.DeliveryMode != DurableDeliveryV1 || submissionID == "" {
		return nil
	}
	if turnID != "" {
		var original processidentity.Identity
		client, release := execution.AcquireAgentCtlClient()
		if client != nil {
			original = withAgentProcessGroup(ctx, client, execution.DeliveryStreamID, client.RuntimeProcessIdentity())
		}
		release()
		execution.setMetadataValue(deliveryPromptTurnBindingMetadataKey, deliveryPromptTurnBinding{
			OriginalRuntime: original, PromptGeneration: execution.promptGenerationSnapshot(),
			TaskID: execution.TaskID, SessionID: execution.SessionID, ExecutionID: execution.ID,
			IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration,
			StreamID: execution.DeliveryStreamID, SubmissionID: deliverySubmissionIdentityForPrompt(submissionID, ""),
			TurnID: turnID,
		})
	}
	if sm.persistPromptTurnBinding != nil {
		return sm.persistPromptTurnBinding(ctx, execution)
	}
	return nil
}

func restoreRecoveredPromptTurnBinding(execution *AgentExecution, peer journal.SubmissionSummary) error {
	value := execution.MetadataSnapshot()[deliveryPromptTurnBindingMetadataKey]
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	var binding deliveryPromptTurnBinding
	if err != nil || json.Unmarshal(encoded, &binding) != nil || binding.TurnID == "" ||
		binding.TaskID != execution.TaskID || binding.SessionID != execution.SessionID ||
		binding.ExecutionID != execution.ID || binding.IncarnationID != execution.DeliveryIncarnationID ||
		binding.HarnessGeneration != execution.DeliveryHarnessGeneration ||
		binding.StreamID != execution.DeliveryStreamID || binding.SubmissionID != peer.ID {
		return fmt.Errorf("%w: %w: recovered prompt turn does not match the admitted delivery owner",
			ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	applyRecoveredPromptTurnBinding(execution, binding, peer)
	return nil
}

func applyRecoveredPromptTurnBinding(execution *AgentExecution, binding deliveryPromptTurnBinding, peer journal.SubmissionSummary) {
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	if execution.promptGeneration != 0 && execution.promptGeneration != binding.PromptGeneration {
		return
	}
	execution.promptTurnID = binding.TurnID
	if binding.PromptGeneration == 0 || execution.promptCompletionGeneration == binding.PromptGeneration ||
		peer.StreamID != binding.StreamID || peer.HarnessGeneration != binding.HarnessGeneration ||
		(peer.State != journal.SubmissionAccepted && peer.State != journal.SubmissionDispatching) {
		return
	}
	execution.promptGeneration = binding.PromptGeneration
	execution.dispatchedPromptGeneration = binding.PromptGeneration
	if execution.promptTurnIDs == nil {
		execution.promptTurnIDs = make(map[uint64]string)
	}
	execution.promptTurnIDs[binding.PromptGeneration] = binding.TurnID
	execution.armAcceptedPromptActivity()
}
