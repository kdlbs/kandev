package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/common/processidentity"
)

const deliveryPromptTurnBindingMetadataKey = "delivery_prompt_turn_binding"

type deliveryPromptTurnBinding struct {
	OriginalRuntime   processidentity.Identity `json:"original_runtime"`
	PromptGeneration  uint64                   `json:"prompt_generation"`
	TaskID            string                   `json:"task_id"`
	SessionID         string                   `json:"session_id"`
	ExecutionID       string                   `json:"execution_id"`
	IncarnationID     string                   `json:"incarnation_id"`
	HarnessGeneration uint64                   `json:"harness_generation"`
	StreamID          string                   `json:"stream_id"`
	SubmissionID      string                   `json:"submission_id"`
	TurnID            string                   `json:"turn_id"`
}

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
			original = client.RuntimeProcessIdentity()
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

func restoreRecoveredPromptTurnBinding(execution *AgentExecution, submissionID string) error {
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
		binding.StreamID != execution.DeliveryStreamID || binding.SubmissionID != submissionID {
		return fmt.Errorf("%w: %w: recovered prompt turn does not match the admitted delivery owner",
			ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	execution.setPromptTurnID(binding.TurnID)
	return nil
}
