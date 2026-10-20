package lifecycle

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

// Control identity comes from the exact admitted prompt or its persisted
// binding. The current replacement process is never historical owner proof.
func (m *Manager) deliveryRecordControlIdentity(ctx context.Context, request DeliveryRecordEvidenceRequest, descriptor journal.RecoveryDescriptor) *AgentDeliveryRecoveryIdentity {
	if descriptor.SubmissionsTruncated || descriptor.SubmissionCount != 1 || len(descriptor.Submissions) != 1 {
		return nil
	}
	if live := m.liveDeliveryRecordControlIdentity(request, descriptor); live != nil {
		return live
	}
	return m.persistedDeliveryRecordControlIdentity(ctx, request, descriptor.Submissions[0])
}

func (m *Manager) liveDeliveryRecordControlIdentity(request DeliveryRecordEvidenceRequest, descriptor journal.RecoveryDescriptor) *AgentDeliveryRecoveryIdentity {
	execution, exists := m.GetExecutionBySessionID(request.SessionID)
	if !exists || execution.TaskID != request.TaskID {
		return nil
	}
	captured, err := captureDeliveryReconciliationIdentity(execution)
	if err != nil || captured.PromptGeneration == 0 || captured.OriginalRuntime.Validate() != nil ||
		!deliveryRecordPromptGenerationKnown(execution, captured.PromptGeneration) {
		return nil
	}
	selected := descriptor.Submissions[0]
	if captured.SessionID != selected.SessionID || captured.IncarnationID != selected.IncarnationID ||
		captured.HarnessGeneration != selected.HarnessGeneration || captured.StreamID != selected.StreamID ||
		captured.SubmissionID != selected.ID {
		return nil
	}
	return &AgentDeliveryRecoveryIdentity{
		OriginalRuntime: captured.OriginalRuntime, TaskID: request.TaskID,
		SessionID: captured.SessionID, ExecutionID: captured.ExecutionID,
		SubmissionID: captured.SubmissionID, StreamID: captured.StreamID,
		IncarnationID: captured.IncarnationID, HarnessGeneration: captured.HarnessGeneration,
		PromptGeneration: captured.PromptGeneration,
	}
}

func deliveryRecordPromptGenerationKnown(execution *AgentExecution, generation uint64) bool {
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	return !execution.recoveredPromptGenerationPending.Load() &&
		execution.promptGeneration == generation && execution.dispatchedPromptGeneration == generation
}

func (m *Manager) persistedDeliveryRecordControlIdentity(ctx context.Context, request DeliveryRecordEvidenceRequest, selected journal.SubmissionSummary) *AgentDeliveryRecoveryIdentity {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, request.SessionID)
	if err != nil || row == nil || row.TaskID != request.TaskID || row.SessionID != request.SessionID {
		return nil
	}
	encoded, err := json.Marshal(row.Metadata[deliveryPromptTurnBindingMetadataKey])
	var binding deliveryPromptTurnBinding
	if err != nil || json.Unmarshal(encoded, &binding) != nil || !persistedPromptBindingMatches(binding, request, selected) {
		return nil
	}
	return &AgentDeliveryRecoveryIdentity{OriginalRuntime: binding.OriginalRuntime, TaskID: binding.TaskID,
		SessionID: binding.SessionID, ExecutionID: binding.ExecutionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: binding.HarnessGeneration, StreamID: binding.StreamID, SubmissionID: binding.SubmissionID,
		PromptGeneration: binding.PromptGeneration}
}

func persistedPromptBindingMatches(binding deliveryPromptTurnBinding, request DeliveryRecordEvidenceRequest, selected journal.SubmissionSummary) bool {
	return binding.ExecutionID != "" && binding.TurnID != "" && binding.PromptGeneration > 0 &&
		binding.OriginalRuntime.Validate() == nil && binding.TaskID == request.TaskID &&
		binding.SessionID == selected.SessionID && binding.IncarnationID == selected.IncarnationID &&
		binding.HarnessGeneration == selected.HarnessGeneration && binding.StreamID == selected.StreamID &&
		binding.SubmissionID == selected.ID
}
