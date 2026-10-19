package lifecycle

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// Include only terminal SQL records needed to explain retained unknown journal
// entries. Completed history outside that inventory is not recovery authority.
func nativeResumeSQLRows(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, peer []journal.Submission, repository AgentDeliveryRepository) ([]*models.AgentDeliverySubmission, error) {
	rows, err := activeBackendSubmissions(ctx, repository, execution.SessionID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[row.ID] = true
	}
	for _, submission := range peer {
		if seen[submission.ID] || submission.State != journal.SubmissionInterruptedUnknown {
			continue
		}
		row, err := nativeResumeSubmissionMetadata(ctx, repository, submission.ID)
		if err != nil {
			return nil, err
		}
		if !peerMatchesTerminalHistory(execution, status, submission, row) {
			return nil, fmt.Errorf("%w: native resume terminal evidence disagrees", ErrDurableAdoptionBlocked)
		}
		cursor, err := repository.GetAgentDeliveryCursor(ctx, submission.StreamID)
		if err != nil {
			return nil, err
		}
		if !nativeTerminalCursorMatches(cursor, submission, row.TerminalSequence) {
			return nil, fmt.Errorf("%w: native resume terminal is not projected", ErrDurableAdoptionBlocked)
		}
		seen[row.ID] = true
		rows = append(rows, row)
	}
	return rows, nil
}

func peerMatchesTerminalHistory(execution *AgentExecution, status *agentctl.DeliveryStatus, peer journal.Submission, row *models.AgentDeliverySubmission) bool {
	if row == nil || !nativeResumeTerminalState(row.State) || row.Outcome != string(row.State) {
		return false
	}
	return peer.State == journal.SubmissionInterruptedUnknown && peer.TerminalEventRetained && peer.TerminalSequence > 0 &&
		nativeTerminalOwnerMatches(execution, status, peer, row) && row.TerminalStreamID == peer.StreamID &&
		row.TerminalSequence > 0 && uint64(row.TerminalSequence) == peer.TerminalSequence && peer.Hash != "" && row.PayloadHash == peer.Hash
}

func nativeTerminalOwnerMatches(execution *AgentExecution, status *agentctl.DeliveryStatus, peer journal.Submission, row *models.AgentDeliverySubmission) bool {
	return peer.SessionID == execution.SessionID && row.SessionID == peer.SessionID && peer.ID == row.ID &&
		peer.IncarnationID == status.IncarnationID && row.IncarnationID == peer.IncarnationID &&
		peer.HarnessGeneration > 0 && peer.HarnessGeneration <= status.HarnessGeneration && row.HarnessGeneration == int64(peer.HarnessGeneration) &&
		peer.StreamID != "" && (peer.HarnessGeneration < status.HarnessGeneration || peer.StreamID == status.StreamID)
}

func nativeResumeTerminalState(state models.DeliverySubmissionState) bool {
	return state == models.DeliverySubmissionCompleted || state == models.DeliverySubmissionFailed || state == models.DeliverySubmissionCancelled
}

func (m *Manager) hasProjectedTerminalRetirement(ctx context.Context, execution *AgentExecution) (bool, error) {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil || m.streamManager == nil || m.streamManager.deliveryRepository() == nil {
		return false, ErrDurableAdoptionEvidenceUnavailable
	}
	status, err := m.nativeResumeDeliveryStatus(ctx, execution, client)
	if err != nil || status == nil {
		return false, err
	}
	peer, err := client.ListDeliverySubmissions(ctx, execution.SessionID)
	if err != nil {
		return false, err
	}
	rows, err := nativeResumeSQLRows(ctx, execution, status, peer, m.streamManager.deliveryRepository())
	if err != nil {
		return false, err
	}
	if err := validateNativeResumeSQLRows(execution, status, peer, rows, false); err != nil {
		return false, err
	}
	ids, err := nativeResumeRetirementIDs(execution, status, peer, rows)
	if err != nil {
		return false, err
	}
	terminals := make(map[string]bool, len(rows))
	for _, row := range rows {
		terminals[row.ID] = nativeResumeTerminalState(row.State)
	}
	for _, id := range ids {
		if !terminals[id] {
			return false, nil
		}
	}
	return len(ids) > 0, nil
}

func nativeResumeSubmissionMetadata(ctx context.Context, repository AgentDeliveryRepository, id string) (*models.AgentDeliverySubmission, error) {
	if reader, ok := repository.(agentDeliverySubmissionMetadataReader); ok {
		return reader.GetAgentDeliverySubmissionMetadata(ctx, id)
	}
	if reader, ok := repository.(agentDeliverySubmissionReader); ok {
		return reader.GetAgentDeliverySubmission(ctx, id)
	}
	return nil, ErrDurableAdoptionEvidenceUnavailable
}

func nativeTerminalCursorMatches(cursor *models.AgentDeliveryCursor, peer journal.Submission, sequence int64) bool {
	return cursor != nil && cursor.StreamID == peer.StreamID && cursor.SessionID == peer.SessionID &&
		cursor.IncarnationID == peer.IncarnationID && cursor.HarnessGeneration == int64(peer.HarnessGeneration) &&
		cursor.ReceivedSequence == cursor.ProjectedSequence && cursor.RetiredSequence >= 0 && cursor.RetiredSequence <= cursor.ProjectedSequence && cursor.ProjectedSequence >= sequence
}
