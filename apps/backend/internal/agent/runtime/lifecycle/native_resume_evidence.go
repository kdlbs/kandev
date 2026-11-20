package lifecycle

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// Validate the complete SQL history before retiring any peer work and require
// retained retirement evidence before the orchestrator clears the recovery block.
func (m *Manager) validateNativeResumeSQLHistory(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	peer []journal.Submission,
	retired bool,
) error {
	if m.streamManager == nil || m.streamManager.deliveryRepository() == nil {
		return withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalEvidenceUnavailable,
			fmt.Errorf("%w: %w: SQL submission repository is unavailable", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable))
	}
	repository := m.streamManager.deliveryRepository()
	if _, supported := repository.(agentDeliverySubmissionLister); !supported {
		return withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalEvidenceUnavailable,
			fmt.Errorf("%w: %w: SQL submission history is unavailable", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable))
	}
	active, unproven, err := nativeResumeSQLRows(ctx, execution, status, peer, repository)
	if err != nil {
		return err
	}
	return validateNativeResumeSQLRows(execution, status, peer, active, unproven, retired)
}

func validateNativeResumeSQLRows(execution *AgentExecution, status *agentctl.DeliveryStatus, peer []journal.Submission, active []*models.AgentDeliverySubmission, unproven map[string]*models.AgentDeliveryCursor, retired bool) error {
	retained := make(map[string]journal.Submission, len(peer))
	for _, submission := range peer {
		if _, duplicate := retained[submission.ID]; duplicate {
			return fmt.Errorf("%w: duplicate native resume evidence", ErrDurableAdoptionBlocked)
		}
		retained[submission.ID] = submission
	}
	for _, submission := range active {
		evidence, found := retained[submission.ID]
		if !found {
			return withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMissing,
				fmt.Errorf("%w: native resume peer record is missing for a SQL submission", ErrDurableAdoptionBlocked))
		}
		if submission.HarnessGeneration > 0 && evidence.HarnessGeneration < uint64(submission.HarnessGeneration) {
			return withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalGenerationSuperseded,
				fmt.Errorf("%w: native resume peer record belongs to an older generation", ErrDurableAdoptionBlocked))
		}
		unprovenMatch := unproven[submission.ID] != nil && peerMatchesUnprovenCompletion(execution, status, evidence, submission)
		if (!peerMatchesUnknownHistory(execution, status, evidence, submission) && !peerMatchesTerminalHistory(execution, status, evidence, submission) && !unprovenMatch) || retired && !evidence.Retired {
			return withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMismatch,
				fmt.Errorf("%w: native resume peer and SQL submission evidence disagree", ErrDurableAdoptionBlocked))
		}
	}
	return nil
}
