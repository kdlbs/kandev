package lifecycle

import (
	"context"
	"fmt"
	"math"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// Include only terminal SQL records needed to explain retained unknown journal
// entries. Eligible unproven completions are classified separately so they
// never enter the proven-terminal branch.
func nativeResumeSQLRows(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	peer []journal.Submission,
	repository AgentDeliveryRepository,
) ([]*models.AgentDeliverySubmission, map[string]*models.AgentDeliveryCursor, error) {
	rows, err := activeBackendSubmissions(ctx, repository, execution.SessionID)
	if err != nil {
		return nil, nil, err
	}
	unproven := make(map[string]*models.AgentDeliveryCursor)
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[row.ID] = true
	}
	for _, submission := range peer {
		if seen[submission.ID] || submission.State != journal.SubmissionInterruptedUnknown {
			continue
		}
		row, cursor, unprovenRow, err := nativeResumeUnlistedRow(ctx, execution, status, submission, repository)
		if err != nil {
			return nil, nil, err
		}
		if unprovenRow {
			unproven[row.ID] = cursor
		}
		seen[row.ID] = true
		rows = append(rows, row)
	}
	return rows, unproven, nil
}

func nativeResumeUnlistedRow(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	submission journal.Submission,
	repository AgentDeliveryRepository,
) (*models.AgentDeliverySubmission, *models.AgentDeliveryCursor, bool, error) {
	row, err := nativeResumeSubmissionMetadata(ctx, repository, submission.ID)
	if err != nil {
		return nil, nil, false, err
	}
	row, err = nativeResumePendingTerminal(ctx, execution, status, submission, row, repository)
	if err != nil {
		return nil, nil, false, err
	}
	if row == nil {
		return nil, nil, false, fmt.Errorf("%w: native resume submission metadata is unavailable", ErrDurableAdoptionBlocked)
	}
	cursor, err := nativeResumeRowCursor(ctx, repository, submission)
	if err != nil {
		return nil, nil, false, err
	}
	if !nativeTerminalCursorMatches(cursor, submission, row.TerminalSequence) {
		return nil, nil, false, fmt.Errorf("%w: native resume terminal is not projected", ErrDurableAdoptionBlocked)
	}
	switch {
	case peerMatchesTerminalHistory(execution, status, submission, row):
		return row, cursor, false, nil
	case peerMatchesUnprovenCompletion(execution, status, submission, row):
		if status.SubmissionsTruncated {
			return nil, nil, false, fmt.Errorf("%w: native resume submission inventory is truncated", ErrDurableAdoptionBlocked)
		}
		if cursor.ReceivedSequence < cursor.RemoteHighWater {
			return nil, nil, false, fmt.Errorf("%w: native resume historical stream high-water mark is not received", ErrDurableAdoptionBlocked)
		}
		valid, verifyErr := verifyNativeResumeUnprovenCompletion(ctx, repository, execution, status, submission, row)
		if verifyErr != nil {
			return nil, nil, false, verifyErr
		}
		if !valid {
			return nil, nil, false, fmt.Errorf("%w: native resume terminal evidence disagrees", ErrDurableAdoptionBlocked)
		}
		return row, cursor, true, nil
	default:
		return nil, nil, false, fmt.Errorf("%w: native resume terminal evidence disagrees", ErrDurableAdoptionBlocked)
	}
}

func nativeResumeRowCursor(ctx context.Context, repository AgentDeliveryRepository, submission journal.Submission) (*models.AgentDeliveryCursor, error) {
	return repository.GetAgentDeliveryCursor(ctx, submission.StreamID)
}

type pendingNativeResumeTerminalVerifier interface {
	VerifyPendingNativeResumeTerminal(context.Context, string, string, models.NativeResumeSubmissionEvidence) (bool, error)
}

// A projected terminal can outlive an interrupted workflow settlement. Derive
// only a recovery view; settlement markers and historical records stay intact.
func nativeResumePendingTerminal(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, peer journal.Submission, row *models.AgentDeliverySubmission, repository AgentDeliveryRepository) (*models.AgentDeliverySubmission, error) {
	if row == nil || row.TerminalStreamID != "" || row.TerminalSequence != 0 || row.TerminalTurnID != "" || row.FullySettledAt != nil || row.PayloadPrunedAt != nil || peer.TerminalSequence > math.MaxInt64 {
		return row, nil
	}
	verifier, ok := repository.(pendingNativeResumeTerminalVerifier)
	if !ok {
		return row, nil
	}
	derived := *row
	derived.TerminalStreamID, derived.TerminalSequence = peer.StreamID, int64(peer.TerminalSequence)
	if !peerMatchesTerminalHistory(execution, status, peer, &derived) {
		return row, nil
	}
	valid, err := verifier.VerifyPendingNativeResumeTerminal(ctx, row.SessionID, row.IncarnationID, models.NativeResumeSubmissionEvidence{
		ID: row.ID, HarnessGeneration: row.HarnessGeneration, PayloadHash: row.PayloadHash,
		State: row.State, Outcome: row.Outcome, StreamID: peer.StreamID,
		TerminalStreamID: derived.TerminalStreamID, TerminalSequence: derived.TerminalSequence,
	})
	if err != nil || !valid {
		return row, err
	}
	return &derived, nil
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
	rows, unproven, err := nativeResumeSQLRows(ctx, execution, status, peer, m.streamManager.deliveryRepository())
	if err != nil {
		return false, err
	}
	if err := validateNativeResumeSQLRows(execution, status, peer, rows, unproven, false); err != nil {
		return false, err
	}
	ids, err := nativeResumeRetirementIDs(execution, status, peer, rows, unproven)
	if err != nil {
		return false, err
	}
	terminals := make(map[string]bool, len(rows))
	for _, row := range rows {
		// An unproven completion keeps its historical label but is not
		// terminal authority; it requires the full dead-owner proof.
		terminals[row.ID] = nativeResumeTerminalState(row.State) && unproven[row.ID] == nil
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
