package lifecycle

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ReconcileLoadedNativeDelivery completes retirement for an idle native successor.
// The persisted prompt binding proves the original owner, independently of the
// replacement's current process and of missing UI recovery metadata.
func (m *Manager) ReconcileLoadedNativeDelivery(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists || execution == nil {
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	if !execution.promptMu.TryLock() {
		return nil, ErrDeliveryRecoveryBlocked
	}
	defer execution.promptMu.Unlock()
	if execution.Status != v1.AgentStatusReady || execution.dispatchedPromptPending.Load() {
		return nil, ErrDeliveryRecoveryBlocked
	}
	if proof, err := m.GetNativeResumeDeliveryEvidence(ctx, sessionID); err == nil && proof != nil {
		return proof, nil
	}
	historical, err := m.readLoadedNativeHistoricalOwner(ctx, execution)
	if err != nil {
		return nil, err
	}
	if historical == nil || historical.ExecutionID == execution.ID || !loadedRecoverySuccessorMatches(execution, *historical) {
		return nil, ErrDeliveryRecoveryBlocked
	}
	if err := proveHistoricalDeliveryOwnerDead(*historical); err != nil {
		return nil, err
	}
	if err := m.AcknowledgeNativeResumeDelivery(ctx, sessionID); err != nil {
		return nil, err
	}
	return m.GetNativeResumeDeliveryEvidence(ctx, sessionID)
}

func (m *Manager) readLoadedNativeHistoricalOwner(ctx context.Context, execution *AgentExecution) (*AgentDeliveryRecoveryIdentity, error) {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	status, err := m.nativeResumeDeliveryStatus(ctx, execution, client)
	if err != nil || status == nil {
		return nil, err
	}
	if m.streamManager == nil || m.streamManager.deliveryRepository() == nil {
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	peer, err := client.ListDeliverySubmissions(ctx, execution.SessionID)
	if err != nil {
		return nil, err
	}
	rows, err := activeBackendSubmissions(ctx, m.streamManager.deliveryRepository(), execution.SessionID)
	if err != nil {
		return nil, err
	}
	return m.loadedNativeHistoricalOwner(ctx, execution, status, peer, rows)
}
func (m *Manager) loadedNativeHistoricalOwner(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, peer []journal.Submission, rows []*models.AgentDeliverySubmission) (*AgentDeliveryRecoveryIdentity, error) {
	var historical *AgentDeliveryRecoveryIdentity
	for _, row := range rows {
		for _, submission := range peer {
			if submission.ID != row.ID || submission.Retired {
				continue
			}
			if historical != nil {
				return nil, fmt.Errorf("%w: independent unretired history", ErrDeliveryRecoveryBlocked)
			}
			candidate := *row
			if candidate.State == models.DeliverySubmissionDispatching && submission.State == journal.SubmissionInterruptedUnknown {
				candidate.State = models.DeliverySubmissionInterruptedUnknown
			}
			if !peerMatchesUnknownHistory(execution, status, submission, &candidate) {
				return nil, ErrDeliveryOwnerMismatch
			}
			historical = m.persistedDeliveryRecordControlIdentity(ctx, DeliveryRecordEvidenceRequest{TaskID: execution.TaskID, SessionID: execution.SessionID}, journal.SubmissionSummary{
				ID: submission.ID, SessionID: submission.SessionID, IncarnationID: submission.IncarnationID, HarnessGeneration: submission.HarnessGeneration, StreamID: submission.StreamID})
			if historical == nil {
				return nil, ErrDeliveryRecoveryBlocked
			}
		}
	}
	return historical, nil
}
