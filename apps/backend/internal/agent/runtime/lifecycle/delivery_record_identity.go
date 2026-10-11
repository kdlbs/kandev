package lifecycle

import "github.com/kandev/kandev/internal/agentctl/journal"

// A retained descriptor names journal work, not its historical process. Only
// an execution still bound to that exact prompt can supply control identity.
func (m *Manager) deliveryRecordControlIdentity(request DeliveryRecordEvidenceRequest, descriptor journal.RecoveryDescriptor) *AgentDeliveryRecoveryIdentity {
	if descriptor.SubmissionsTruncated || descriptor.SubmissionCount != 1 || len(descriptor.Submissions) != 1 {
		return nil
	}
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
