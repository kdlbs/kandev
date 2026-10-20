package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// GetNativeResumeDeliveryEvidence revalidates retirement before SQL clears an
// admission fence. The snapshot cannot authorize dispatch or alter old outcomes.
func (m *Manager) GetNativeResumeDeliveryEvidence(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	owner := captureNativeResumeProofOwner(execution, client)
	release()
	return m.waitForNativeResumeProjection(ctx, owner)
}

func (m *Manager) nativeResumeDeliveryProof(ctx context.Context, execution *AgentExecution, client *agentctl.Client) (*models.NativeResumeDeliveryEvidence, error) {
	status, err := m.nativeResumeDeliveryStatus(ctx, execution, client)
	if err != nil || status == nil {
		return nil, err
	}
	if m.streamManager == nil || m.streamManager.deliveryRepository() == nil {
		return nil, ErrDurableAdoptionEvidenceUnavailable
	}
	repository := m.streamManager.deliveryRepository()
	if _, err := authenticatedDeliveryReplayCursor(ctx, execution, repository, status); err != nil {
		return nil, err
	}
	if status.Unresolved {
		return nil, fmt.Errorf("%w: native resume journal work remains unresolved", ErrDurableAdoptionBlocked)
	}
	rows, streams, err := m.nativeResumeProofRows(ctx, execution, status, client, repository)
	if err != nil {
		return nil, err
	}
	proof := &models.NativeResumeDeliveryEvidence{SessionID: execution.SessionID, IncarnationID: status.IncarnationID,
		HarnessGeneration: int64(status.HarnessGeneration), StreamID: status.StreamID,
		ExecutionID: execution.ID, NativeSessionID: execution.ACPSessionID}
	proof.Cursor, err = nativeResumeProofCursor(ctx, repository, status)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		proof.Submissions = append(proof.Submissions, models.NativeResumeSubmissionEvidence{ID: row.ID, State: row.State, Outcome: row.Outcome,
			HarnessGeneration: row.HarnessGeneration, PayloadHash: row.PayloadHash, StreamID: streams[row.ID],
			TerminalSequence: row.TerminalSequence, TerminalStreamID: row.TerminalStreamID})
	}
	after, err := client.GetDeliveryStatus(ctx, execution.DeliveryStreamID)
	if err != nil {
		return nil, err
	}
	if !sameNativeResumeProofStatus(status, after) {
		if nativeResumeStreamAdvanced(status, after) {
			return nil, errNativeResumeStreamProgress
		}
		return nil, ErrDeliveryRecordEvidenceStale
	}
	return proof, nil
}

func (m *Manager) nativeResumeProofRows(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, client *agentctl.Client, repository AgentDeliveryRepository) ([]*models.AgentDeliverySubmission, map[string]string, error) {
	peer, err := client.ListDeliverySubmissions(ctx, execution.SessionID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := nativeResumeSQLRows(ctx, execution, status, peer, repository)
	if err != nil {
		return nil, nil, err
	}
	if err := validateNativeResumeSQLRows(execution, status, peer, rows, true); err != nil {
		return nil, nil, err
	}
	ids, err := nativeResumeRetirementIDs(execution, status, peer, rows)
	if err != nil {
		return nil, nil, err
	}
	if len(ids) != 0 {
		return nil, nil, fmt.Errorf("%w: native resume inventory has %d unsettled submissions", ErrDurableAdoptionBlocked, len(ids))
	}
	streams := make(map[string]string, len(peer))
	for _, submission := range peer {
		streams[submission.ID] = submission.StreamID
	}
	return rows, streams, nil
}

func nativeResumeProofCursor(ctx context.Context, repository AgentDeliveryRepository, status *agentctl.DeliveryStatus) (*models.AgentDeliveryCursor, error) {
	cursor, err := repository.GetAgentDeliveryCursor(ctx, status.StreamID)
	if errors.Is(err, sql.ErrNoRows) && (status.Stream == nil || status.Stream.HighWater == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validateAuthenticatedSQLDeliveryCursor(cursor, status); err != nil {
		return nil, err
	}
	if _, err := projectedCursorCoveredByPeer(cursor, status); err != nil {
		return nil, err
	}
	if cursor.ReceivedSequence != cursor.ProjectedSequence {
		return nil, errNativeResumeProjectionLag
	}
	return cursor, nil
}

func sameNativeResumeProofStatus(before, after *agentctl.DeliveryStatus) bool {
	if before == nil || after == nil || !sameDeliveryRecordDescriptor(*before, *after) || before.StreamID != after.StreamID || before.Unresolved != after.Unresolved {
		return false
	}
	if before.Stream == nil || after.Stream == nil {
		return before.Stream == nil && after.Stream == nil
	}
	return *before.Stream == *after.Stream
}
