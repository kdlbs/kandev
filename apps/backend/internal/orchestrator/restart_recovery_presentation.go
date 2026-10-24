package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) repairRestartRecoveryPresentation(ctx context.Context, session *models.TaskSession) (*models.TaskSession, error) {
	release := s.acquireSessionLifecycleLock(session.ID)
	defer release()
	session, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if s.automationOwnsRetrySession(ctx, session.TaskID, session.ID) {
		return session, nil
	}
	generation, err := s.restartRecoveryNativeGeneration(ctx, session)
	if err != nil {
		return nil, err
	}
	if generation == nil {
		return session, nil
	}
	blocks, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return session, nil
	}
	if open, readErr := blocks.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, generation.Generation); readErr == nil && open != nil {
		if resolved, _ := s.finishLoadedNativeDeliveryBlock(ctx, open); resolved {
			return s.repo.GetTaskSession(ctx, session.ID)
		}
	}
	block, reason, err := s.deliveryReconstructionBlock(ctx, session.ID, session.QueueIncarnationID, generation.Generation, nil)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return session, nil
	}
	recovery, exists, err := s.currentRestartRecoverySnapshot(ctx, session, generation)
	if err != nil {
		return nil, err
	}
	if exists && !silentRestoreRecoveryPhase(recovery.Phase) {
		return session, nil
	}
	if !s.restartRecoveryPresentationRepairAllowed(ctx, session, generation, recovery, exists) {
		return session, nil
	}
	if block == nil {
		block, err = s.recreateRestartRecoveryBlock(ctx, blocks, session, generation, recovery, exists)
		if err != nil {
			return nil, err
		}
		if block == nil {
			return session, nil
		}
	}
	return s.finishRestartPresentationRepair(ctx, session, generation, recovery, exists, block)
}

func (s *Service) finishRestartPresentationRepair(ctx context.Context, session *models.TaskSession, generation *models.HarnessSessionGeneration, recovery models.AgentDeliveryRecovery, exists bool, block *models.SessionRecoveryBlock) (*models.TaskSession, error) {
	if exists && !incompleteReconstructedRecovery(recovery) && !silentRestoreBlockMatches(block, recovery) {
		if err := s.repairKnownRestartBlock(ctx, session, recovery, block); err != nil {
			return nil, err
		}
		return s.repo.GetTaskSession(ctx, session.ID)
	}
	if !exists || incompleteReconstructedRecovery(recovery) || s.knownRestartHistoryGap(ctx, session, recovery) {
		if _, err := s.retryMissingCanonicalDeliverySubmission(ctx, session, session.QueueIncarnationID, generation.Generation, block); err != nil {
			return nil, err
		}
		return s.repo.GetTaskSession(ctx, session.ID)
	}
	return session, nil
}

func (s *Service) hasRestartSubmission(ctx context.Context, session *models.TaskSession, generation int64) (bool, error) {
	lister, ok := s.repo.(nativeResumeDeliverySubmissionReader)
	if !ok {
		return false, nil
	}
	rows, err := lister.ListAgentDeliverySubmissions(ctx, session.ID)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row.IncarnationID == session.QueueIncarnationID && row.HarnessGeneration == generation &&
			(row.State == models.DeliverySubmissionInterruptedUnknown || row.State == models.DeliverySubmissionDispatching) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) restartRecoveryNativeGeneration(ctx context.Context, session *models.TaskSession) (*models.HarnessSessionGeneration, error) {
	continuity, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return nil, nil
	}
	generation, err := continuity.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if generation == nil || generation.NativeSessionID == "" {
		return nil, nil
	}
	return generation, nil
}

func (s *Service) recreateRestartRecoveryBlock(ctx context.Context, blocks sessionRecoveryBlockStore, session *models.TaskSession, generation *models.HarnessSessionGeneration, recovery models.AgentDeliveryRecovery, exists bool) (*models.SessionRecoveryBlock, error) {
	eligible, err := s.hasRestartSubmission(ctx, session, generation.Generation)
	if err != nil || !eligible {
		return nil, err
	}
	block := &models.SessionRecoveryBlock{SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: generation.Generation, ConsumerReference: agentDeliveryConsumer,
		Reason: durableDeliveryUnresolvedReason, State: models.RecoveryBlockOpen}
	if exists && silentRestoreRecoveryMatchesSession(recovery, session) && recovery.HarnessGeneration == generation.Generation {
		block.DeliverySubmissionID, block.DeliveryStreamID = recovery.SubmissionID, recovery.StreamID
		block.Reason = unknownPromptOutcome
	}
	if err := blocks.UpsertSessionRecoveryBlock(ctx, block); err != nil {
		return nil, err
	}
	return block, nil
}

func (s *Service) restartMissingOwnerProvenDead(ctx context.Context, session *models.TaskSession, generation *models.HarnessSessionGeneration) bool {
	inspector, ok := s.agentManager.(deliveryRecordEvidenceClient)
	if !ok {
		return false
	}
	evidence, err := inspector.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: session.TaskID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: uint64(generation.Generation)})
	return err == nil && evidence != nil && evidence.ProcessTerminated != nil && *evidence.ProcessTerminated
}

func (s *Service) restartRecoveryPresentationRepairAllowed(ctx context.Context, session *models.TaskSession, generation *models.HarnessSessionGeneration, recovery models.AgentDeliveryRecovery, exists bool) bool {
	if exists && !incompleteReconstructedRecovery(recovery) {
		return true
	}
	return s.restartMissingOwnerProvenDead(ctx, session, generation)
}

func (s *Service) currentRestartRecoverySnapshot(ctx context.Context, session *models.TaskSession, generation *models.HarnessSessionGeneration) (models.AgentDeliveryRecovery, bool, error) {
	recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !exists || recovery.Phase != models.AgentDeliveryRecoveryRestored {
		return recovery, exists, nil
	}
	if !s.hasNewRestartJournalSubmission(ctx, session, generation, recovery.SubmissionID) {
		return recovery, true, nil
	}
	verifier, supported := s.repo.(historicalNativeResumeRecoveryVerifier)
	if !supported {
		return recovery, exists, nil
	}
	resolved, err := verifier.HasResolvedHistoricalNativeResumeRecovery(ctx, recovery)
	return recovery, !resolved, err
}
