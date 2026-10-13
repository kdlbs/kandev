package lifecycle

import (
	"context"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// A journal unknown state is irreversible. Only an exact authenticated match
// can classify its stale SQL dispatch before native resume retires that work.
func (m *Manager) reconcileNativeResumeSQLDispatch(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, peer []journal.Submission) error {
	repository := m.streamManager.deliveryRepository()
	rows, err := activeBackendSubmissions(ctx, repository, execution.SessionID)
	if err != nil {
		return err
	}
	retained := make(map[string]journal.Submission, len(peer))
	for _, submission := range peer {
		if _, duplicate := retained[submission.ID]; duplicate {
			return fmt.Errorf("%w: duplicate native resume evidence", ErrDurableAdoptionBlocked)
		}
		retained[submission.ID] = submission
	}
	var abandoned []*models.AgentDeliverySubmission
	for _, row := range rows {
		candidate := *row
		if candidate.State == models.DeliverySubmissionDispatching {
			if candidate.TerminalSequence != 0 {
				return ErrDurableAdoptionBlocked
			}
			candidate.State = models.DeliverySubmissionInterruptedUnknown
			abandoned = append(abandoned, row)
		}
		evidence, found := retained[row.ID]
		if !found || (row.State == models.DeliverySubmissionDispatching && evidence.State != journal.SubmissionInterruptedUnknown) || !peerMatchesUnknownHistory(execution, status, evidence, &candidate) {
			return fmt.Errorf("%w: native resume peer and SQL submission evidence disagree", ErrDurableAdoptionBlocked)
		}
	}
	if len(abandoned) == 0 {
		return nil
	}
	if _, err := authenticatedDeliveryReplayCursor(ctx, execution, repository, status); err != nil {
		return err
	}
	writer, supported := repository.(initialDeliveryAdmissionStore)
	if !supported {
		return ErrDurableAdoptionEvidenceUnavailable
	}
	for _, row := range abandoned {
		changed, err := writer.TransitionAgentDeliverySubmission(ctx, row.ID, models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown, "unknown", time.Now().UTC())
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("%w: native resume submission changed", ErrDurableAdoptionBlocked)
		}
	}
	return nil
}
