package lifecycle

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
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
		return fmt.Errorf("%w: %w: SQL submission repository is unavailable", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	repository := m.streamManager.deliveryRepository()
	if _, supported := repository.(agentDeliverySubmissionLister); !supported {
		return fmt.Errorf("%w: %w: SQL submission history is unavailable", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	active, err := activeBackendSubmissions(ctx, repository, execution.SessionID)
	if err != nil {
		return err
	}
	retained := make(map[string]journal.Submission, len(peer))
	for _, submission := range peer {
		retained[submission.ID] = submission
	}
	for _, submission := range active {
		evidence, found := retained[submission.ID]
		if !found || !peerMatchesUnknownHistory(execution, status, evidence, submission) || retired && !evidence.Retired {
			return fmt.Errorf("%w: native resume peer and SQL submission evidence disagree", ErrDurableAdoptionBlocked)
		}
	}
	return nil
}
