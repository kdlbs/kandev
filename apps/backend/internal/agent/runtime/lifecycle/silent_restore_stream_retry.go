package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// RecoverSilentRestoreCandidateStream reconnects the initialized candidate
// without starting another native session or dispatching a prompt.
func (m *Manager) RecoverSilentRestoreCandidateStream(ctx context.Context, sessionID, executionID, nativeID string) error {
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists || execution.ID != executionID || nativeID == "" ||
		execution.RequiredNativeConversationID != nativeID || execution.ACPSessionID != nativeID ||
		!execution.isSessionInitialized() {
		return ErrDeliveryOwnerMismatch
	}
	execution.bindSuccessorGenerationOwner(ctx, &LaunchRequest{
		RequiredNativeConversationID: nativeID, CandidateExecutionID: executionID,
	})
	return m.recoverAgentPromptStream(ctx, execution)
}

// An uncommitted successor has no receipts yet. Its complete, unacknowledged
// setup prefix can be replayed; event receipt still waits for the SQL commit.
func canReplaySilentRestoreSetup(
	ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, status *agentctl.DeliveryStatus,
) bool {
	if !execution.isSessionInitialized() || execution.RequiredNativeConversationID == "" ||
		execution.ACPSessionID != execution.RequiredNativeConversationID ||
		status.Stream == nil || status.Stream.FirstRetained != 1 || status.Stream.Acknowledged != 0 ||
		status.SubmissionCount != 0 || len(status.Submissions) != 0 || status.SubmissionsTruncated {
		return false
	}
	reader, supported := delivery.(harnessGenerationReader)
	if !supported {
		return false
	}
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, execution.SessionID, execution.DeliveryIncarnationID)
	return err == nil && silentRestoreGenerationPending(execution, current)
}
