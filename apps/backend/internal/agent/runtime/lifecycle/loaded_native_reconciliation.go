package lifecycle

import (
	"context"
	"errors"
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
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalExecutionUnavailable, ErrDurableAdoptionEvidenceUnavailable)
	}
	if !execution.promptMu.TryLock() {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalLiveWork, ErrDeliveryRecoveryBlocked)
	}
	defer execution.promptMu.Unlock()
	if execution.Status != v1.AgentStatusReady || execution.dispatchedPromptPending.Load() {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalLiveWork, ErrDeliveryRecoveryBlocked)
	}
	if proof, err := m.GetNativeResumeDeliveryEvidence(ctx, sessionID); err == nil && proof != nil {
		return proof, nil
	}
	if settled, err := m.hasProjectedTerminalRetirement(ctx, execution); err == nil && settled {
		if err := m.AcknowledgeNativeResumeDelivery(ctx, sessionID); err != nil {
			return nil, withJournalUnresolvedRefusal(err)
		}
		proof, err := m.GetNativeResumeDeliveryEvidence(ctx, sessionID)
		return proof, classifyLoadedNativeRefusal(err)
	}
	historical, err := m.readLoadedNativeHistoricalOwner(ctx, execution)
	if err != nil {
		return nil, classifyLoadedNativeRefusal(err)
	}
	if historical == nil {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMissing, ErrDeliveryRecoveryBlocked)
	}
	if historical.ExecutionID == execution.ID {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalLiveWork, ErrDeliveryRecoveryBlocked)
	}
	if historical.HarnessGeneration < execution.DeliveryHarnessGeneration {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalGenerationSuperseded, ErrDeliveryRecoveryBlocked)
	}
	if !loadedRecoverySuccessorMatches(execution, *historical) {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalNativeIdentityMismatch, ErrDeliveryOwnerMismatch)
	}
	if err := proveHistoricalDeliveryOwnerDead(*historical); err != nil {
		return nil, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalLiveWork, err)
	}
	if err := m.AcknowledgeNativeResumeDelivery(ctx, sessionID); err != nil {
		return nil, withJournalUnresolvedRefusal(err)
	}
	proof, err := m.GetNativeResumeDeliveryEvidence(ctx, sessionID)
	return proof, classifyLoadedNativeRefusal(err)
}

func classifyLoadedNativeRefusal(err error) error {
	if err == nil {
		return nil
	}
	var reason string
	switch {
	case errors.Is(err, ErrDurableAdoptionIdentityMismatch), errors.Is(err, ErrDeliveryOwnerMismatch):
		reason = DeliveryRecoveryRefusalNativeIdentityMismatch
	case errors.Is(err, errNativeResumeProjectionLag):
		reason = DeliveryRecoveryRefusalProjectionPending
	case errors.Is(err, ErrDurableAdoptionEvidenceUnavailable):
		reason = DeliveryRecoveryRefusalEvidenceUnavailable
	case errors.Is(err, ErrDeliveryRecoveryBlocked):
		reason = DeliveryRecoveryRefusalJournalUnresolved
	default:
		reason = DeliveryRecoveryRefusalUnclassified
	}
	var existing interface{ DeliveryRecoveryRefusalReason() string }
	if errors.As(err, &existing) {
		return err
	}
	return withDeliveryRecoveryRefusal(reason, err)
}

func withJournalUnresolvedRefusal(err error) error {
	if err == nil {
		return nil
	}
	var reasoner interface{ DeliveryRecoveryRefusalReason() string }
	if errors.As(err, &reasoner) && IsDeliveryRecoveryRefusalReason(reasoner.DeliveryRecoveryRefusalReason()) {
		return err
	}
	return withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalJournalUnresolved, err)
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
	unproven, err := nativeResumeUnprovenBackendRows(
		ctx, execution, status, peer, rows, m.streamManager.deliveryRepository(),
	)
	if err != nil {
		return nil, err
	}
	return m.loadedNativeHistoricalOwner(ctx, execution, status, peer, append(rows, unproven...))
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
			switch {
			case candidate.State == models.DeliverySubmissionDispatching && submission.State == journal.SubmissionInterruptedUnknown:
				candidate.State = models.DeliverySubmissionInterruptedUnknown
				if !peerMatchesUnknownHistory(execution, status, submission, &candidate) {
					return nil, ErrDeliveryOwnerMismatch
				}
			case candidate.State == models.DeliverySubmissionCompleted:
				// A completed label without terminal proof still needs the full
				// dead-owner assessment; discovery only identifies the owner.
				if !peerMatchesUnprovenCompletion(execution, status, submission, &candidate) {
					return nil, ErrDeliveryOwnerMismatch
				}
			default:
				if !peerMatchesUnknownHistory(execution, status, submission, &candidate) {
					return nil, ErrDeliveryOwnerMismatch
				}
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
