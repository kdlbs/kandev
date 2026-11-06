package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// Legacy shutdown labels may have no unresolved descriptor or recovery notice.
// Native resume itself is repeatable: the persisted block remains the admission
// fence until authenticated retirement and SQL proof commit together.
func (s *Service) resumeLegacyNativeRestart(ctx context.Context, session *models.TaskSession) (bool, error) {
	if session == nil || session.State != models.TaskSessionStateWaitingForInput || !silentRestoreSessionRouteEligible(session) {
		return false, nil
	}
	source, err := s.legacyNativeRestartSource(ctx, session)
	if err != nil || source == nil {
		return false, err
	}
	client, request, evidence, selected, block := source.client, source.request, source.evidence, source.selected, source.block
	latest, err := client.InspectDeliveryRecordEvidence(ctx, request)
	if err != nil {
		return false, err
	}
	if !legacyNativeRestartEvidenceMatches(latest, selected, block) || !sameRestartDescriptor(evidence.Descriptor, latest.Descriptor) || *evidence.ControlIdentity != *latest.ControlIdentity {
		return false, errors.New("legacy native restart evidence changed")
	}
	if err := s.pauseSilentRestoreQueue(ctx, session.TaskID, session.ID); err != nil {
		return false, err
	}
	response, err := s.RecoverSession(ctx, session.TaskID, session.ID, "resume")
	return err == nil && response != nil && response.Success, err
}

func legacyNativeRestartEvidenceMatches(evidence *agentruntime.DeliveryRecordEvidence, row *models.AgentDeliverySubmission, block *models.SessionRecoveryBlock) bool {
	if evidence == nil || evidence.ControlIdentity == nil || evidence.ProcessTerminated == nil || !*evidence.ProcessTerminated {
		return false
	}
	d, peer, identity := evidence.Descriptor, evidence.StoredSubmission, evidence.ControlIdentity
	if !legacyNativeDescriptorIdle(d) {
		return false
	}
	if peer == nil || peer.State == journal.SubmissionInterruptedUnknown || !journal.HasUnprovenOutcome(*peer) || peer.Retired {
		return false
	}
	return legacyNativeRecordMatches(peer, row, d) && legacyNativeOwnerMatches(identity, peer, block)

}

func legacyRestartCursorCovered(ctx context.Context, repository taskrepo.AgentDeliveryRepository, session *models.TaskSession, generation *models.HarnessSessionGeneration, d journal.RecoveryDescriptor) bool {
	cursor, err := repository.GetAgentDeliveryCursor(ctx, d.StreamID)
	if errors.Is(err, sql.ErrNoRows) {
		return d.Stream == nil || d.Stream.HighWater == 0
	}
	if err != nil || cursor == nil || d.Stream == nil {
		return false
	}
	return cursor.SessionID == session.ID && cursor.IncarnationID == session.QueueIncarnationID && cursor.HarnessGeneration == generation.Generation && cursor.ReceivedSequence == cursor.ProjectedSequence && uint64(cursor.ProjectedSequence) == d.Stream.HighWater
}

type legacyRestartSource struct {
	client   deliveryRecordEvidenceClient
	request  agentruntime.DeliveryRecordEvidenceRequest
	evidence *agentruntime.DeliveryRecordEvidence
	selected *models.AgentDeliverySubmission
	block    *models.SessionRecoveryBlock
}

func (s *Service) legacyNativeRestartSource(ctx context.Context, session *models.TaskSession) (*legacyRestartSource, error) {
	client, ok := s.agentManager.(deliveryRecordEvidenceClient)
	rows, canList := s.repo.(nativeResumeDeliverySubmissionReader)
	repository, canRead := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok || !canList || !canRead {
		return nil, nil
	}
	generation, err := s.restartRecoveryNativeGeneration(ctx, session)
	if err != nil || generation == nil {
		return nil, err
	}
	block, err := s.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil || block == nil || block.ConsumerReference != agentDeliveryConsumer {
		return nil, err
	}
	submissions, err := rows.ListAgentDeliverySubmissions(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	source, err := readLegacyRestartInventory(ctx, client, session, generation, block, submissions)
	if err != nil || source == nil {
		return nil, err
	}
	evidence := source.evidence
	if !legacyRestartCursorCovered(ctx, repository, session, generation, evidence.Descriptor) {
		return nil, nil
	}
	return source, nil
}

func readLegacyRestartInventory(ctx context.Context, client deliveryRecordEvidenceClient, session *models.TaskSession, generation *models.HarnessSessionGeneration, block *models.SessionRecoveryBlock, rows []*models.AgentDeliverySubmission) (*legacyRestartSource, error) {
	var selected *legacyRestartSource
	var descriptor *journal.RecoveryDescriptor
	inputs := &silentRestoreInputs{session: session, recovery: models.AgentDeliveryRecovery{IncarnationID: session.QueueIncarnationID, HarnessGeneration: generation.Generation}}
	for _, row := range rows {
		if terminalDeliverySubmission(row.State) {
			continue
		}
		if row.State != models.DeliverySubmissionInterruptedUnknown {
			return nil, nil
		}
		request := agentruntime.DeliveryRecordEvidenceRequest{TaskID: session.TaskID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: uint64(generation.Generation), StoredSubmissionID: row.ID}
		evidence, err := client.InspectDeliveryRecordEvidence(ctx, request)
		if err != nil || evidence == nil {
			return nil, err
		}
		if descriptor != nil && !sameRestartDescriptor(*descriptor, evidence.Descriptor) {
			return nil, errors.New("legacy inventory changed")
		}
		descriptor = &evidence.Descriptor
		if matchingRetiredRestartSubmission(inputs, row, evidence.StoredSubmission) {
			continue
		}
		if selected != nil || row.TerminalSequence != 0 || !legacyNativeRestartEvidenceMatches(evidence, row, block) {
			return nil, nil
		}
		selected = &legacyRestartSource{client: client, request: request, evidence: evidence, selected: row, block: block}
	}
	return selected, nil
}

func legacyNativeRecordMatches(peer *journal.Submission, row *models.AgentDeliverySubmission, d journal.RecoveryDescriptor) bool {
	return peer.ID == row.ID && peer.Hash != "" && peer.Hash == row.PayloadHash && peer.SessionID == row.SessionID && peer.IncarnationID == row.IncarnationID && peer.HarnessGeneration == uint64(row.HarnessGeneration) &&
		peer.StreamID != "" && peer.StreamID == d.StreamID && d.SessionID == row.SessionID && d.IncarnationID == row.IncarnationID && d.HarnessGeneration == peer.HarnessGeneration
}
func legacyNativeOwnerMatches(identity *agentruntime.AgentDeliveryRecoveryIdentity, peer *journal.Submission, block *models.SessionRecoveryBlock) bool {
	return (block.DeliverySubmissionID == "" || block.DeliverySubmissionID == peer.ID) && (block.DeliveryStreamID == "" || block.DeliveryStreamID == peer.StreamID) && identity.SubmissionID == peer.ID && identity.SessionID == peer.SessionID && identity.IncarnationID == peer.IncarnationID && identity.HarnessGeneration == peer.HarnessGeneration && identity.StreamID == peer.StreamID && identity.OriginalRuntime.Validate() == nil && identity.ExecutionID != "" && identity.PromptGeneration > 0
}

func legacyNativeDescriptorIdle(d journal.RecoveryDescriptor) bool {
	return d.Durable && journal.SupportsDeliveryVersion(d.Version) && !d.SubmissionsTruncated && d.SubmissionCount == 0 && len(d.Submissions) == 0 && !d.Unresolved
}
