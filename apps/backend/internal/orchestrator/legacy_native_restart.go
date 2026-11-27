package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	runtimekind "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
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
	if _, exists := models.LoadAgentDeliveryRecovery(session.Metadata); exists {
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
	if !legacyNativeRestartEvidenceMatches(latest, selected, block) || !sameRestartDescriptor(evidence.Descriptor, latest.Descriptor) ||
		*evidence.ControlIdentity != *latest.ControlIdentity || source.terminalSequence > 0 && retainedCancellationSequence(latest.StoredSubmission) != source.terminalSequence {
		return false, errors.New("legacy native restart evidence changed")
	}
	var cancellationReservation *sessionKeyedCeilingReservation
	if source.terminalSequence > 0 {
		var admitted bool
		cancellationReservation, admitted = s.reserveSilentRestoreCapacity(ctx, session.TaskID, session.ID)
		if !admitted {
			return false, nil
		}
		defer cancellationReservation.releaseIfNotConsumed()
	}
	if err := s.pauseSilentRestoreQueue(ctx, session.TaskID, session.ID); err != nil {
		return false, err
	}
	if source.terminalSequence > 0 {
		identity := *latest.ControlIdentity
		identity.PayloadHash = selected.PayloadHash
		attempt := s.runSessionDeliveryRecovery(ctx, identity)
		if attempt.outcome != SessionDeliveryRecoverySettled {
			return false, nil
		}
		if err := s.reconcileAgentDeliverySettlements(ctx, session.ID); err != nil {
			return false, err
		}
		settled, err := s.legacyNativeCancellationSettled(ctx, source)
		if err != nil || !settled {
			return false, err
		}
		return s.resumeLegacyNativeCancellation(ctx, session, source, cancellationReservation)
	}
	response, err := s.RecoverSession(ctx, session.TaskID, session.ID, "resume")
	return err == nil && response != nil && response.Success, err
}

func legacyNativeRestartEvidenceMatches(evidence *agentruntime.DeliveryRecordEvidence, row *models.AgentDeliverySubmission, block *models.SessionRecoveryBlock) bool {
	if evidence == nil || evidence.ControlIdentity == nil || evidence.ProcessTerminated == nil || !*evidence.ProcessTerminated {
		return false
	}
	d, peer, identity := evidence.Descriptor, evidence.StoredSubmission, evidence.ControlIdentity
	if peer == nil || peer.Retired {
		return false
	}
	if peer.State == journal.SubmissionCancelled && peer.TerminalEventRetained && peer.TerminalSequence > 0 {
		return legacyNativeCancellationDescriptorMatches(d, peer, row) && legacyNativeCancellationEvidenceMatches(evidence, row, block)
	}
	if !legacyNativeDescriptorIdle(d) {
		return false
	}
	if peer.State == journal.SubmissionInterruptedUnknown || !journal.HasUnprovenOutcome(*peer) {
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

func legacyRestartCancellationCursorCovered(ctx context.Context, repository taskrepo.AgentDeliveryRepository, session *models.TaskSession, generation *models.HarnessSessionGeneration, d journal.RecoveryDescriptor, terminalSequence uint64) bool {
	if d.Stream == nil || terminalSequence == 0 || terminalSequence > d.Stream.HighWater {
		return false
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, d.StreamID)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil || cursor == nil || cursor.StreamID != d.StreamID || cursor.SessionID != session.ID ||
		cursor.IncarnationID != session.QueueIncarnationID || cursor.HarnessGeneration != generation.Generation ||
		cursor.ProjectedSequence < 0 || cursor.ReceivedSequence < cursor.ProjectedSequence {
		return false
	}
	return uint64(cursor.ProjectedSequence) <= terminalSequence && uint64(cursor.ReceivedSequence) <= d.Stream.HighWater
}

type legacyRestartSource struct {
	client           deliveryRecordEvidenceClient
	request          agentruntime.DeliveryRecordEvidenceRequest
	evidence         *agentruntime.DeliveryRecordEvidence
	selected         *models.AgentDeliverySubmission
	block            *models.SessionRecoveryBlock
	nativeSessionID  string
	terminalSequence uint64
	provenStreamID   string
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
	if source.terminalSequence > 0 {
		stopped, err := legacyNativeRestartStoppedWorktree(ctx, s.repo, session, generation, evidence.ControlIdentity)
		if err != nil || !stopped {
			return nil, err
		}
		if !legacyRestartCancellationCursorCovered(ctx, repository, session, generation, evidence.Descriptor, source.terminalSequence) {
			return nil, nil
		}
		return source, nil
	}
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
		selected = &legacyRestartSource{client: client, request: request, evidence: evidence, selected: row, block: block,
			nativeSessionID: generation.NativeSessionID, terminalSequence: retainedCancellationSequence(evidence.StoredSubmission),
			provenStreamID: evidence.StoredSubmission.StreamID}
	}
	return selected, nil
}

func retainedCancellationSequence(peer *journal.Submission) uint64 {
	if peer != nil && peer.State == journal.SubmissionCancelled && peer.TerminalEventRetained {
		return peer.TerminalSequence
	}
	return 0
}

func legacyNativeCancellationEvidenceMatches(evidence *agentruntime.DeliveryRecordEvidence, row *models.AgentDeliverySubmission, block *models.SessionRecoveryBlock) bool {
	if evidence == nil || row == nil || block == nil || evidence.StoredSubmission == nil || evidence.ControlIdentity == nil {
		return false
	}
	peer := evidence.StoredSubmission
	return legacyNativeCancellationSubmissionMatches(row, peer) &&
		legacyNativeCancellationBlockMatches(block, row, peer) &&
		legacyNativeRecordMatches(peer, row, evidence.Descriptor) &&
		legacyNativeOwnerMatches(evidence.ControlIdentity, peer, block)
}

func legacyNativeCancellationSubmissionMatches(row *models.AgentDeliverySubmission, peer *journal.Submission) bool {
	return row.State == models.DeliverySubmissionInterruptedUnknown && row.TerminalSequence == 0 &&
		peer.State == journal.SubmissionCancelled && peer.TerminalEventRetained && peer.TerminalSequence > 0
}

func legacyNativeCancellationBlockMatches(block *models.SessionRecoveryBlock, row *models.AgentDeliverySubmission, peer *journal.Submission) bool {
	return block.State == models.RecoveryBlockOpen && block.ConsumerReference == agentDeliveryConsumer &&
		block.SessionID == row.SessionID && block.IncarnationID == row.IncarnationID &&
		block.ExpectedGeneration == row.HarnessGeneration && block.DeliverySubmissionID == peer.ID &&
		(block.DeliveryStreamID == "" || block.DeliveryStreamID == peer.StreamID)
}

func legacyNativeCancellationDescriptorMatches(d journal.RecoveryDescriptor, peer *journal.Submission, row *models.AgentDeliverySubmission) bool {
	return d.Durable && journal.SupportsDeliveryVersion(d.Version) && !d.SubmissionsTruncated &&
		d.SubmissionCount == 0 && len(d.Submissions) == 0 && d.Stream != nil &&
		d.Stream.StreamID == peer.StreamID && d.Stream.SessionID == row.SessionID &&
		d.Stream.IncarnationID == row.IncarnationID && d.Stream.HarnessGeneration == uint64(row.HarnessGeneration) &&
		d.Stream.HighWater >= peer.TerminalSequence
}

func legacyNativeRestartStoppedWorktree(ctx context.Context, repository any, session *models.TaskSession, generation *models.HarnessSessionGeneration, identity *agentruntime.AgentDeliveryRecoveryIdentity) (bool, error) {
	reader, ok := repository.(interface {
		GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	})
	if !ok || identity == nil || session == nil || generation == nil {
		return false, nil
	}
	running, err := reader.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil || running == nil {
		return false, err
	}
	return legacyNativeStoppedWorktreeIdentityMatches(running, session, generation, identity) &&
		legacyNativeStoppedWorktreeStateMatches(running, generation), nil
}

func legacyNativeStoppedWorktreeIdentityMatches(running *models.ExecutorRunning, session *models.TaskSession, generation *models.HarnessSessionGeneration, identity *agentruntime.AgentDeliveryRecoveryIdentity) bool {
	return running.TaskID == session.TaskID && running.SessionID == session.ID &&
		running.ExecutorID == models.ExecutorIDWorktree && running.Runtime == runtimekind.RuntimeStandalone &&
		running.AgentExecutionID == identity.ExecutionID && generation.NativeSessionID != ""
}

func legacyNativeStoppedWorktreeStateMatches(running *models.ExecutorRunning, generation *models.HarnessSessionGeneration) bool {
	return running.Status == models.ExecutorRunningStatusStopped && running.LocalPID == 0 && running.Resumable &&
		running.ResumeToken == generation.NativeSessionID
}

func (s *Service) legacyNativeCancellationSettled(ctx context.Context, source *legacyRestartSource) (bool, error) {
	repository, ok := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok || source == nil || source.selected == nil || source.block == nil || source.terminalSequence == 0 {
		return false, nil
	}
	row, err := repository.GetAgentDeliverySubmission(ctx, source.selected.ID)
	if err != nil {
		return false, err
	}
	block, err := s.GetSessionRecoveryBlock(ctx, source.block.ID)
	if err != nil {
		return false, err
	}
	return legacyNativeCancellationSettlementMatches(row, block, source), nil
}

func legacyNativeCancellationSettlementMatches(row *models.AgentDeliverySubmission, block *models.SessionRecoveryBlock, source *legacyRestartSource) bool {
	return legacyNativeCancellationSettledSubmissionMatches(row, source) &&
		legacyNativeCancellationSettledBlockMatches(block, source)
}

func legacyNativeCancellationSettledSubmissionMatches(row *models.AgentDeliverySubmission, source *legacyRestartSource) bool {
	return row != nil && row.ID == source.selected.ID && row.State == models.DeliverySubmissionCancelled &&
		row.Outcome == string(models.DeliverySubmissionCancelled)
}

func legacyNativeCancellationSettledBlockMatches(block *models.SessionRecoveryBlock, source *legacyRestartSource) bool {
	return block != nil && source != nil && source.provenStreamID != "" && block.State == models.RecoveryBlockResolved &&
		block.ID == source.block.ID && block.DeliverySubmissionID == source.selected.ID &&
		block.DeliveryStreamID == source.provenStreamID &&
		block.DeliverySequence == int64(source.terminalSequence) &&
		block.DeliveryOutcome == string(models.DeliverySubmissionCancelled)
}

func (s *Service) resumeLegacyNativeCancellation(ctx context.Context, session *models.TaskSession, source *legacyRestartSource, reservation *sessionKeyedCeilingReservation) (bool, error) {
	current, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil || current == nil || current.TaskID != session.TaskID ||
		current.State != models.TaskSessionStateWaitingForInput || !silentRestoreSessionRouteEligible(current) {
		return false, err
	}
	generation, err := s.restartRecoveryNativeGeneration(ctx, current)
	if err != nil || generation == nil || generation.NativeSessionID != source.nativeSessionID {
		return false, err
	}
	stopped, err := legacyNativeRestartStoppedWorktree(ctx, s.repo, current, generation, source.evidence.ControlIdentity)
	if err != nil || !stopped {
		return false, err
	}
	if err := s.checkSessionRecoveryBlock(ctx, current.ID); err != nil {
		return false, err
	}
	remainingBlock, err := s.GetOpenSessionRecoveryBlock(ctx, current.ID)
	if err != nil {
		return false, err
	}
	if remainingBlock != nil {
		return false, &sessionRecoveryRequiredError{Block: remainingBlock}
	}
	execution, err := s.resumeTaskSessionWithContinuationAndReservation(ctx, current.TaskID, current.ID, executor.ResumeOptions{
		RequiredNativeConversationID: generation.NativeSessionID,
		NoInitialPrompt:              true,
		Origin:                       string(launchOriginAutomatic),
	}, nil, reservation)
	return err == nil && execution != nil, err
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
