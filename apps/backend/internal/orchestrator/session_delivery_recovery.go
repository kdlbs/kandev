package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// RetrySessionDelivery reconciles one durable submission without dispatching a
// prompt. A missing or ambiguous durable owner remains visible as blocked.
const (
	agentDeliveryConsumer               = "agent_delivery"
	missingCanonicalSubmissionReason    = "missing_canonical_submission"
	unknownPromptOutcome                = "unknown_prompt_outcome"
	recoveryUnavailableReason           = "recovery_unavailable"
	silentRestartRestoredCreationReason = "silent_restart_restored"
)

func (s *Service) RetrySessionDelivery(
	ctx context.Context,
	taskID, sessionID string,
) (*SessionDeliveryRecoveryResponse, error) {
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return nil, err
	}
	if err := s.ensureTaskNotArchived(ctx, taskID); err != nil {
		return nil, err
	}
	session, err := s.loadDeliveryRetrySession(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	currentGeneration, err := s.currentDeliveryGeneration(ctx, session, incarnationID)
	if err != nil {
		return nil, err
	}
	blocks, hasBlocks := s.repo.(sessionRecoveryBlockStore)
	if !hasBlocks {
		return sessionDeliveryRecoveryResponse(taskID, sessionID, SessionDeliveryRecoveryBlocked,
			"recovery_state_unavailable", 0, nil), nil
	}
	block, err := blocks.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, currentGeneration)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	recovery, hasRecovery := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !hasRecovery {
		if block != nil {
			reason := "independent_recovery_block"
			if block.ConsumerReference == agentDeliveryConsumer {
				reason = missingCanonicalSubmissionReason
			}
			return sessionDeliveryRecoveryResponse(taskID, sessionID, SessionDeliveryRecoveryBlocked,
				reason, 0, nil), nil
		}
		return s.retryLegacySessionDelivery(ctx, taskID, sessionID)
	}
	if response, handled, terminalErr := s.retryRecoverySuccessor(ctx, session, recovery, currentGeneration, block); handled || terminalErr != nil {
		return response, terminalErr
	}
	return s.retryKnownSessionDelivery(ctx, session, block, recovery, currentGeneration)
}

func (s *Service) retryRecoverySuccessor(
	ctx context.Context,
	session *models.TaskSession,
	recovery models.AgentDeliveryRecovery,
	currentGeneration int64,
	block *models.SessionRecoveryBlock,
) (*SessionDeliveryRecoveryResponse, bool, error) {
	if !hasRecoverySuccessorPhase(recovery.Phase) {
		return nil, false, nil
	}
	identity := recoverySuccessorIdentity(recovery)
	if block != nil {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"independent_recovery_block", recovery.Revision, identity), true, nil
	}
	generation, available, err := s.loadRecoverySuccessorGeneration(ctx, session.ID, recovery.IncarnationID)
	if err != nil {
		return nil, true, err
	}
	if !available {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"recovery_state_unavailable", recovery.Revision, identity), true, nil
	}
	creationReason, outcome := recoverySuccessorOutcome(recovery.Phase)
	if !sameRecoverySuccessorGeneration(generation, recovery, creationReason) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"recovery_identity_mismatch", recovery.Revision, identity), true, nil
	}
	reason, err := s.verifyRecoverySuccessorSubmission(ctx, session, recovery)
	if err != nil {
		return nil, true, err
	}
	if reason != "" {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			reason, recovery.Revision, identity), true, nil
	}
	return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, outcome, "", recovery.Revision, identity), true, nil
}

func hasRecoverySuccessorPhase(phase string) bool {
	return phase == models.AgentDeliveryRecoveryRestored || phase == models.AgentDeliveryRecoveryContinued
}

func recoverySuccessorIdentity(recovery models.AgentDeliveryRecovery) *SessionDeliveryRecoveryIdentity {
	return &SessionDeliveryRecoveryIdentity{
		SubmissionID: recovery.SubmissionID, StreamID: recovery.StreamID,
		IncarnationID: recovery.IncarnationID, HarnessGeneration: recovery.HarnessGeneration,
		PromptGeneration: recovery.PromptGeneration,
	}
}

func (s *Service) loadRecoverySuccessorGeneration(
	ctx context.Context,
	sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, bool, error) {
	continuity, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return nil, false, nil
	}
	generation, err := continuity.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	return generation, true, err
}

func recoverySuccessorOutcome(phase string) (string, SessionDeliveryRecoveryOutcome) {
	if phase == models.AgentDeliveryRecoveryContinued {
		return "interrupted_continued", SessionDeliveryRecoveryContinued
	}
	return silentRestartRestoredCreationReason, SessionDeliveryRecoveryAttached
}

func sameRecoverySuccessorGeneration(
	generation *models.HarnessSessionGeneration,
	recovery models.AgentDeliveryRecovery,
	creationReason string,
) bool {
	return generation != nil && generation.Generation == recovery.HarnessGeneration+1 &&
		generation.PredecessorGeneration == recovery.HarnessGeneration &&
		generation.NativeSessionID != "" && generation.CreationReason == creationReason
}

func (s *Service) verifyRecoverySuccessorSubmission(
	ctx context.Context,
	session *models.TaskSession,
	recovery models.AgentDeliveryRecovery,
) (string, error) {
	submissions, ok := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok {
		return "recovery_state_unavailable", nil
	}
	submission, err := submissions.GetAgentDeliverySubmission(ctx, recovery.SubmissionID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrTaskSessionNotFound) {
		return missingCanonicalSubmissionReason, nil
	}
	if err != nil {
		return "", err
	}
	if !canonicalRecoverySubmissionMatches(session, recovery, submission, recovery.HarnessGeneration) {
		return "recovery_identity_mismatch", nil
	}
	return "", nil
}

func (s *Service) retryKnownSessionDelivery(ctx context.Context, session *models.TaskSession, block *models.SessionRecoveryBlock, recovery models.AgentDeliveryRecovery, currentGeneration int64) (*SessionDeliveryRecoveryResponse, error) {
	if !recoverySessionMatches(session, recovery, currentGeneration) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"recovery_identity_mismatch", recovery.Revision, nil), nil
	}
	submissions, ok := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"recovery_state_unavailable", recovery.Revision, nil), nil
	}
	submission, err := submissions.GetAgentDeliverySubmission(ctx, recovery.SubmissionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrTaskSessionNotFound) {
			return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
				missingCanonicalSubmissionReason, recovery.Revision, nil), nil
		}
		return nil, err
	}
	if !canonicalRecoverySubmissionMatches(session, recovery, submission, currentGeneration) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"recovery_identity_mismatch", recovery.Revision, nil), nil
	}
	if !recoveryBlockMatches(block, recovery) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"independent_recovery_block", recovery.Revision, nil), nil
	}
	if block == nil && recovery.Phase != models.AgentDeliveryRecoveryRecovered &&
		recovery.Phase != models.AgentDeliveryRecoverySettled && !terminalDeliverySubmission(submission.State) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"delivery_recovery_block_missing", recovery.Revision, nil), nil
	}
	return s.retryVerifiedSessionDelivery(ctx, session, recovery, submission)
}

func (s *Service) retryVerifiedSessionDelivery(ctx context.Context, session *models.TaskSession, recovery models.AgentDeliveryRecovery, submission *models.AgentDeliverySubmission) (*SessionDeliveryRecoveryResponse, error) {
	identity := &SessionDeliveryRecoveryIdentity{
		SubmissionID: recovery.SubmissionID, StreamID: recovery.StreamID,
		IncarnationID: recovery.IncarnationID, HarnessGeneration: recovery.HarnessGeneration,
		PromptGeneration: recovery.PromptGeneration,
	}
	if recovery.Phase == models.AgentDeliveryRecoverySettled {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoverySettled,
			"", recovery.Revision, identity), nil
	}
	if terminalDeliverySubmission(submission.State) {
		return s.retryTerminalSessionDelivery(ctx, session, recovery, identity)
	}

	if recovery.Phase == models.AgentDeliveryRecoveryRecovered {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryAttached,
			"", recovery.Revision, identity), nil
	}
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	attempt := s.runSessionDeliveryRecovery(retryCtx, agentruntime.AgentDeliveryRecoveryIdentity{
		OriginalRuntime: recovery.OriginalRuntime,
		TaskID:          session.TaskID, SessionID: session.ID, ExecutionID: recovery.AgentExecutionID,
		SubmissionID: recovery.SubmissionID, StreamID: recovery.StreamID,
		IncarnationID: recovery.IncarnationID, HarnessGeneration: uint64(recovery.HarnessGeneration),
		PromptGeneration: recovery.PromptGeneration,
	})
	response := sessionDeliveryRecoveryResponse(session.TaskID, session.ID, attempt.outcome,
		attempt.reason, recovery.Revision, identity)
	if attempt.outcome == SessionDeliveryRecoverySettled {
		if err := s.reconcileAgentDeliverySettlements(retryCtx, session.ID); err != nil {
			response.Outcome = SessionDeliveryRecoveryBlocked
			response.Reason = "terminal_settlement_pending"
		}
	}
	return response, nil
}

func (s *Service) retryLegacySessionDelivery(
	ctx context.Context,
	taskID, sessionID string,
) (*SessionDeliveryRecoveryResponse, error) {
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	attempt := s.runLegacySessionDeliveryRecovery(retryCtx, sessionID)
	return sessionDeliveryRecoveryResponse(taskID, sessionID, attempt.outcome,
		attempt.reason, 0, nil), nil
}

func (s *Service) runLegacySessionDeliveryRecovery(
	ctx context.Context,
	sessionID string,
) sessionDeliveryRecoveryAttempt {
	recoverer, ok := s.agentManager.(agentPromptStreamRecoverer)
	if !ok {
		return sessionDeliveryRecoveryAttempt{
			outcome: SessionDeliveryRecoveryUnavailable, reason: recoveryUnavailableReason,
		}
	}
	if err := recoverer.RecoverAgentPromptStream(ctx, sessionID); err != nil {
		if errors.Is(err, agentruntime.ErrDeliveryRecoveryBlocked) || errors.Is(err, agentruntime.ErrDeliveryOwnerMismatch) {
			return sessionDeliveryRecoveryAttempt{
				outcome: SessionDeliveryRecoveryBlocked, reason: "delivery_identity_mismatch",
			}
		}
		if errors.Is(err, agentruntime.ErrUncertainPromptDelivery) {
			return sessionDeliveryRecoveryAttempt{
				outcome: SessionDeliveryRecoveryUncertain, reason: unknownPromptOutcome,
			}
		}
		return sessionDeliveryRecoveryAttempt{
			outcome: SessionDeliveryRecoveryUnavailable, reason: recoveryUnavailableReason,
		}
	}
	return sessionDeliveryRecoveryAttempt{outcome: SessionDeliveryRecoveryAttached}
}

func (s *Service) runSessionDeliveryRecovery(
	ctx context.Context,
	identity agentruntime.AgentDeliveryRecoveryIdentity,
) sessionDeliveryRecoveryAttempt {
	if recoverer, ok := s.agentManager.(agentPromptStreamIdentityRecoverer); ok {
		result := recoverer.RecoverAgentPromptStreamWithIdentity(ctx, identity)
		attempt := sessionDeliveryRecoveryAttempt{
			outcome:           mapDeliveryRecoveryOutcome(result.Outcome),
			reason:            publicDeliveryRecoveryReason(result.Reason),
			processTerminated: result.ProcessTerminated,
		}
		if attempt.reason == "" && attempt.outcome == SessionDeliveryRecoveryUncertain {
			attempt.reason = unknownPromptOutcome
		}
		return attempt
	}
	recoverer, ok := s.agentManager.(agentPromptStreamRecoverer)
	if !ok {
		return sessionDeliveryRecoveryAttempt{
			outcome: SessionDeliveryRecoveryUnavailable, reason: recoveryUnavailableReason,
		}
	}
	if err := recoverer.RecoverAgentPromptStream(ctx, identity.SessionID); err != nil {
		if errors.Is(err, agentruntime.ErrDeliveryRecoveryBlocked) || errors.Is(err, agentruntime.ErrDeliveryOwnerMismatch) {
			return sessionDeliveryRecoveryAttempt{
				outcome: SessionDeliveryRecoveryBlocked, reason: "delivery_identity_mismatch",
			}
		}
		if errors.Is(err, agentruntime.ErrUncertainPromptDelivery) {
			return sessionDeliveryRecoveryAttempt{
				outcome: SessionDeliveryRecoveryUncertain, reason: unknownPromptOutcome,
			}
		}
		return sessionDeliveryRecoveryAttempt{
			outcome: SessionDeliveryRecoveryUnavailable, reason: recoveryUnavailableReason,
		}
	}
	return sessionDeliveryRecoveryAttempt{outcome: SessionDeliveryRecoveryAttached}
}

func mapDeliveryRecoveryOutcome(outcome agentruntime.DeliveryReconciliationOutcome) SessionDeliveryRecoveryOutcome {
	switch outcome {
	case agentruntime.DeliveryReconciliationRunningAttached:
		return SessionDeliveryRecoveryAttached
	case agentruntime.DeliveryReconciliationTerminalSettled:
		return SessionDeliveryRecoverySettled
	case agentruntime.DeliveryReconciliationUncertain:
		return SessionDeliveryRecoveryUncertain
	case agentruntime.DeliveryReconciliationBlocked, agentruntime.DeliveryReconciliationOwnerMismatch:
		return SessionDeliveryRecoveryBlocked
	case agentruntime.DeliveryReconciliationTransportUnavailable:
		return SessionDeliveryRecoveryUnavailable
	default:
		return SessionDeliveryRecoveryBlocked
	}
}

func publicDeliveryRecoveryReason(reason string) string {
	if reason == "" {
		return ""
	}
	switch reason {
	case unknownPromptOutcome, "delivery_output_paused", "delivery_cancellation_pending", "delivery_storage_pressure", "invalid_continuation_request", "stale_recovery", "continuation_checkpoint_failed", "idempotency_conflict", "continuation_outcome_unknown", "recovery_state_unavailable", missingCanonicalSubmissionReason, "recovery_identity_incomplete", "recovery_identity_mismatch",
		"execution_identity_changed", "instance_not_found", "instance_identity_mismatch",
		"delivery_evidence_unavailable", "submission_evidence_unavailable", "delivery_identity_mismatch",
		"terminal_evidence_incomplete", "delivery_projection_unavailable", "retained_replay_failed",
		"terminal_projection_incomplete", "terminal_settlement_unavailable", "terminal_settlement_failed",
		"terminal_settlement_pending", "delivery_recovery_block_missing", "independent_recovery_block":
		return reason
	case "terminal_outcome_missing", "execution_missing_active":
		return unknownPromptOutcome
	case "runtime_unavailable", "instance_unavailable", recoveryUnavailableReason, "delivery_recovery_unavailable",
		"runtime_owner_changed":
		return recoveryUnavailableReason
	default:
		return recoveryUnavailableReason
	}
}

func sessionDeliveryRecoveryResponse(
	taskID, sessionID string,
	outcome SessionDeliveryRecoveryOutcome,
	reason string,
	revision int64,
	identity *SessionDeliveryRecoveryIdentity,
) *SessionDeliveryRecoveryResponse {
	return &SessionDeliveryRecoveryResponse{
		TaskID: taskID, SessionID: sessionID, Outcome: outcome,
		Reason: publicDeliveryRecoveryReason(reason), RecoveryRevision: revision,
		RecoveryIdentity: identity,
	}
}

func recoverySessionMatches(session *models.TaskSession, recovery models.AgentDeliveryRecovery, currentGeneration int64) bool {
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	return recovery.SessionID == session.ID && recovery.AgentExecutionID != "" &&
		(session.AgentExecutionID == "" || recovery.AgentExecutionID == session.AgentExecutionID) && recovery.IncarnationID == incarnationID &&
		recovery.HarnessGeneration == currentGeneration && recovery.SubmissionID != "" && recovery.StreamID != "" && recovery.PromptGeneration > 0
}

func canonicalRecoverySubmissionMatches(session *models.TaskSession, recovery models.AgentDeliveryRecovery, submission *models.AgentDeliverySubmission, currentGeneration int64) bool {
	return submission != nil && submission.SessionID == session.ID && submission.IncarnationID == recovery.IncarnationID && submission.HarnessGeneration == currentGeneration
}

func recoveryBlockMatches(block *models.SessionRecoveryBlock, recovery models.AgentDeliveryRecovery) bool {
	return block == nil || (block.ConsumerReference == agentDeliveryConsumer && block.DeliverySubmissionID == recovery.SubmissionID && block.DeliveryStreamID == recovery.StreamID && block.IncarnationID == recovery.IncarnationID && block.ExpectedGeneration == recovery.HarnessGeneration)
}

func (s *Service) retryTerminalSessionDelivery(ctx context.Context, session *models.TaskSession, recovery models.AgentDeliveryRecovery, identity *SessionDeliveryRecoveryIdentity) (*SessionDeliveryRecoveryResponse, error) {
	blocks := s.repo.(sessionRecoveryBlockStore)
	currentGeneration := recovery.HarnessGeneration
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.reconcileAgentDeliverySettlements(retryCtx, session.ID); err != nil {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"terminal_settlement_pending", recovery.Revision, identity), nil
	}
	remaining, err := blocks.GetOpenSessionRecoveryBlock(retryCtx, session.ID, recovery.IncarnationID, currentGeneration)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if remaining != nil && remaining.ConsumerReference == agentDeliveryConsumer &&
		remaining.DeliverySubmissionID == recovery.SubmissionID {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
			"terminal_settlement_pending", recovery.Revision, identity), nil
	}
	return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoverySettled,
		"", recovery.Revision, identity), nil
}

func (s *Service) loadDeliveryRetrySession(ctx context.Context, taskID, sessionID string) (*models.TaskSession, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err == nil {
			err = errors.New("task not found")
		}
		return nil, err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		if err == nil {
			err = models.ErrTaskSessionNotFound
		}
		return nil, err
	}
	if session.TaskID != taskID {
		return nil, errors.New("session task ownership changed")
	}
	return session, nil
}
