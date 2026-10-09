package orchestrator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

type InterruptedSessionResumeRequest struct {
	Acknowledge      bool                            `json:"acknowledge_interruption"`
	RecoveryRevision int64                           `json:"recovery_revision"`
	RecoveryIdentity SessionDeliveryRecoveryIdentity `json:"recovery_identity"`
	Instruction      string                          `json:"instruction"`
	IdempotencyKey   string                          `json:"idempotency_key"`
}

type interruptedContinuationStore interface {
	sessionContinuityStore
	GetContinuationSnapshot(context.Context, string) (*models.ContinuationSnapshot, error)
	CommitInterruptedContinuation(context.Context, *models.InterruptedContinuationCommit) (bool, error)
}

type interruptedContinuationCheckpoint struct {
	store        interruptedContinuationStore
	request      InterruptedSessionResumeRequest
	recovery     models.AgentDeliveryRecovery
	generation   models.HarnessSessionGeneration
	blockID      string
	snapshotID   string
	contentHash  string
	submissionID string
}

func (s *Service) ResumeInterruptedSession(ctx context.Context, taskID, sessionID string, request InterruptedSessionResumeRequest) (*SessionDeliveryRecoveryResponse, error) {
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return nil, err
	}
	if err := s.ensureTaskNotArchived(ctx, taskID); err != nil {
		return nil, err
	}
	result := sessionDeliveryRecoveryResponse(taskID, sessionID, SessionDeliveryRecoveryBlocked, "invalid_continuation_request", request.RecoveryRevision, &request.RecoveryIdentity)
	if !request.Acknowledge || strings.TrimSpace(request.Instruction) == "" || len(request.Instruction) > 32<<10 || strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 128 || request.RecoveryRevision < 1 {
		return result, nil
	}
	err := s.withSessionPromptAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var admissionErr error
		result, admissionErr = s.resumeInterruptedSessionAdmitted(admittedCtx, taskID, sessionID, request)
		return admissionErr
	})
	return result, err
}

func (s *Service) resumeInterruptedSessionAdmitted(ctx context.Context, taskID, sessionID string, request InterruptedSessionResumeRequest) (*SessionDeliveryRecoveryResponse, error) {
	store, ok := s.repo.(interruptedContinuationStore)
	if !ok {
		return sessionDeliveryRecoveryResponse(taskID, sessionID, SessionDeliveryRecoveryBlocked, "recovery_state_unavailable", request.RecoveryRevision, &request.RecoveryIdentity), nil
	}
	encoded, _ := json.Marshal(request)
	contentHash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	snapshotID := fmt.Sprintf("interrupted:%x", sha256.Sum256([]byte(taskID+"\x00"+sessionID+"\x00"+request.IdempotencyKey)))
	if response, err := s.reconcileInterruptedCheckpoint(ctx, taskID, sessionID, request, store, snapshotID, contentHash); response != nil || err != nil {
		return response, err
	}
	response, err := s.RetrySessionDelivery(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	if response.RecoveryRevision != request.RecoveryRevision || response.RecoveryIdentity == nil || *response.RecoveryIdentity != request.RecoveryIdentity {
		response.Outcome = SessionDeliveryRecoveryBlocked
		response.Reason = "stale_recovery"
		response.AllowedActions = nil
		return response, nil
	}
	if response.Outcome != SessionDeliveryRecoveryUncertain || len(response.AllowedActions) != 1 || response.AllowedActions[0] != SessionDeliveryRecoveryActionContinueInterrupted {
		return response, nil
	}
	checkpoint, err := s.prepareInterruptedContinuation(ctx, sessionID, request, store, snapshotID, contentHash)
	if err != nil {
		return sessionDeliveryRecoveryResponse(taskID, sessionID, SessionDeliveryRecoveryBlocked, "continuation_checkpoint_failed", request.RecoveryRevision, &request.RecoveryIdentity), nil
	}
	return s.launchInterruptedContinuation(ctx, taskID, sessionID, checkpoint), nil
}

func (s *Service) reconcileInterruptedCheckpoint(ctx context.Context, taskID, sessionID string, request InterruptedSessionResumeRequest, store interruptedContinuationStore, snapshotID, contentHash string) (*SessionDeliveryRecoveryResponse, error) {
	snapshot, err := store.GetContinuationSnapshot(ctx, snapshotID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, errors.New("continuation checkpoint unavailable")
	}
	response := interruptedCheckpointResponse(taskID, sessionID, request, snapshot, contentHash)
	if response.Outcome == SessionDeliveryRecoveryBlocked || snapshot.Status == models.ContinuitySnapshotConsumed {
		return response, nil
	}
	// Prepared snapshots precede the atomic native-generation commit and cannot
	// have dispatched an instruction. Retry still rechecks every admission gate.
	if snapshot.Status == models.ContinuitySnapshotPrepared {
		return nil, nil
	}
	return s.reconcileInterruptedAcceptance(ctx, taskID, sessionID, request, snapshot, store, response), nil
}

func interruptedCheckpointResponse(taskID, sessionID string, request InterruptedSessionResumeRequest, snapshot *models.ContinuationSnapshot, contentHash string) *SessionDeliveryRecoveryResponse {
	outcome := SessionDeliveryRecoveryRestoredBlocked
	reason := "continuation_outcome_unknown"
	if snapshot.SessionID != sessionID || snapshot.ContentHash != contentHash || snapshot.Content != request.Instruction || snapshot.TargetGeneration != request.RecoveryIdentity.HarnessGeneration+1 || snapshot.SubmissionID != "prompt:"+snapshot.ID {
		outcome = SessionDeliveryRecoveryBlocked
		reason = "idempotency_conflict"
	} else if snapshot.Status == models.ContinuitySnapshotConsumed {
		outcome = SessionDeliveryRecoveryContinued
		reason = ""
	}
	return sessionDeliveryRecoveryResponse(taskID, sessionID, outcome, reason, request.RecoveryRevision, &request.RecoveryIdentity)
}

func (s *Service) prepareInterruptedContinuation(ctx context.Context, sessionID string, request InterruptedSessionResumeRequest, store interruptedContinuationStore, snapshotID, contentHash string) (*interruptedContinuationCheckpoint, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !ok || recovery.Revision != request.RecoveryRevision {
		return nil, errors.New("stale recovery")
	}
	generation, err := store.GetCurrentHarnessSessionGeneration(ctx, sessionID, recovery.IncarnationID)
	if err != nil {
		return nil, err
	}
	block, err := s.repo.(sessionRecoveryBlockStore).GetOpenSessionRecoveryBlock(ctx, sessionID, recovery.IncarnationID, recovery.HarnessGeneration)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	submissionID := "prompt:" + snapshotID
	if _, snapshotErr := store.GetContinuationSnapshot(ctx, snapshotID); errors.Is(snapshotErr, sql.ErrNoRows) {
		if err = store.CreateRestoreAttempt(ctx, &models.RestoreAttempt{ID: snapshotID, SessionID: sessionID, IncarnationID: recovery.IncarnationID, ExpectedGeneration: recovery.HarnessGeneration, Action: string(SessionDeliveryRecoveryActionContinueInterrupted), Authorized: true, CreatedAt: now}); err != nil {
			return nil, err
		}
		if err = store.CreateContinuationSnapshot(ctx, &models.ContinuationSnapshot{ID: snapshotID, AttemptID: snapshotID, SessionID: sessionID, TargetGeneration: recovery.HarnessGeneration + 1, SubmissionID: submissionID, Content: request.Instruction, ByteCount: len(request.Instruction), ContentHash: contentHash, Status: models.ContinuitySnapshotPrepared, CreatedAt: now}); err != nil {
			return nil, err
		}
	} else if snapshotErr != nil {
		return nil, snapshotErr
	}
	return &interruptedContinuationCheckpoint{store: store, request: request, recovery: recovery, generation: *generation, blockID: block.ID, snapshotID: snapshotID, contentHash: contentHash, submissionID: submissionID}, nil
}

func interruptedResumeOptions(checkpoint *interruptedContinuationCheckpoint) executor.ResumeOptions {
	return executor.ResumeOptions{RequiredNativeConversationID: checkpoint.generation.NativeSessionID, InterruptedSubmissionID: checkpoint.recovery.SubmissionID, InterruptedStreamID: checkpoint.recovery.StreamID, InterruptedHarnessGeneration: uint64(checkpoint.recovery.HarnessGeneration), NoInitialPrompt: true, HoldForInitialPrompt: true, StartAgentSynchronously: true, RecoveryAction: string(SessionDeliveryRecoveryActionContinueInterrupted), Origin: string(launchOriginManual)}
}
