package lifecycle

import (
	"context"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// Only an exact authenticated match to unproven journal history can classify
// a stale SQL dispatch before native resume retires that work.
func (m *Manager) reconcileNativeResumeSQLDispatch(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, peer []journal.Submission) ([]*models.AgentDeliverySubmission, error) {
	repository := m.streamManager.deliveryRepository()
	rows, err := activeBackendSubmissions(ctx, repository, execution.SessionID)
	if err != nil {
		return nil, err
	}
	rows, err = contextContinuedNativeResumeHistory(ctx, execution, status, repository, peer, rows)
	if err != nil {
		return nil, err
	}
	retained, err := nativeResumePeerSubmissionIndex(peer)
	if err != nil {
		return nil, err
	}
	var abandoned []*models.AgentDeliverySubmission
	var missingPeer []*models.AgentDeliverySubmission
	for _, row := range rows {
		candidate, wasAbandoned, err := nativeResumeDispatchCandidate(row)
		if err != nil {
			return nil, err
		}
		if wasAbandoned {
			abandoned = append(abandoned, row)
		}
		evidence, found := retained[row.ID]
		missing, err := nativeResumeMissingPeerCandidate(execution, status, row, candidate, evidence, found)
		if err != nil {
			return nil, err
		}
		if missing {
			missingPeer = append(missingPeer, row)
		}
	}
	if len(abandoned) == 0 {
		return missingPeer, nil
	}
	if _, err := authenticatedDeliveryReplayCursor(ctx, execution, repository, status); err != nil {
		return nil, err
	}
	writer, supported := repository.(initialDeliveryAdmissionStore)
	if !supported {
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	for _, row := range abandoned {
		changed, err := writer.TransitionAgentDeliverySubmission(ctx, row.ID, models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown, "unknown", time.Now().UTC())
		if err != nil {
			return nil, err
		}
		if !changed {
			return nil, fmt.Errorf("%w: native resume submission changed", ErrDurableAdoptionBlocked)
		}
	}
	return missingPeer, nil
}

func nativeResumePeerSubmissionIndex(peer []journal.Submission) (map[string]journal.Submission, error) {
	retained := make(map[string]journal.Submission, len(peer))
	for _, submission := range peer {
		if _, duplicate := retained[submission.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate native resume evidence", ErrDurableAdoptionBlocked)
		}
		retained[submission.ID] = submission
	}
	return retained, nil
}

func nativeResumeDispatchCandidate(row *models.AgentDeliverySubmission) (models.AgentDeliverySubmission, bool, error) {
	candidate := *row
	if candidate.State != models.DeliverySubmissionDispatching {
		return candidate, false, nil
	}
	if candidate.TerminalSequence != 0 {
		return candidate, false, ErrDurableAdoptionBlocked
	}
	candidate.State = models.DeliverySubmissionInterruptedUnknown
	return candidate, true, nil
}

func nativeResumeMissingPeerCandidate(
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	row *models.AgentDeliverySubmission,
	candidate models.AgentDeliverySubmission,
	evidence journal.Submission,
	found bool,
) (bool, error) {
	if found {
		if peerMatchesUnknownHistory(execution, status, evidence, &candidate) {
			return false, nil
		}
		return false, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMismatch,
			fmt.Errorf("%w: native resume peer and SQL submission evidence disagree", ErrDurableAdoptionBlocked))
	}
	if row.State != models.DeliverySubmissionInterruptedUnknown {
		return false, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMissing,
			fmt.Errorf("%w: native resume peer record is missing for active SQL work", ErrDurableAdoptionBlocked))
	}
	if row.ID == "" || row.SessionID != execution.SessionID || row.IncarnationID != status.IncarnationID || row.PayloadHash == "" {
		return false, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMismatch,
			fmt.Errorf("%w: identity-less native resume SQL evidence is incomplete", ErrDurableAdoptionBlocked))
	}
	if row.HarnessGeneration <= 0 || row.HarnessGeneration > int64(status.HarnessGeneration) {
		return false, withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalGenerationSuperseded,
			fmt.Errorf("%w: identity-less native resume SQL evidence belongs to a newer generation", ErrDurableAdoptionBlocked))
	}
	return true, nil
}
