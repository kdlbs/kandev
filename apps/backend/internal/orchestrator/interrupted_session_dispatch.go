package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

const continuationOutcomeUnknown = "continuation_outcome_unknown"

func (s *Service) launchInterruptedContinuation(ctx context.Context, taskID, sessionID string, checkpoint *interruptedContinuationCheckpoint) *SessionDeliveryRecoveryResponse {
	prepareCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	accepted := false
	defer func() {
		if !accepted {
			cancel()
		}
	}()
	timer := time.AfterFunc(2*time.Minute, cancel)
	defer timer.Stop()
	prepareCtx = context.WithValue(prepareCtx, continuationOwnedContextKey{}, true)
	if err := s.retireInterruptedExecutorRow(prepareCtx, sessionID, checkpoint); err != nil {
		return sessionDeliveryRecoveryResponse(taskID, sessionID, SessionDeliveryRecoveryBlocked, "stale_recovery", checkpoint.request.RecoveryRevision, &checkpoint.request.RecoveryIdentity)
	}
	options := interruptedResumeOptions(checkpoint)
	_, err := s.resumeTaskSessionWithContinuation(prepareCtx, taskID, sessionID, options, func(promptCtx context.Context, attempt *resumeAttempt, execution *executor.TaskExecution) error {
		if err := s.commitInterruptedRestore(promptCtx, sessionID, checkpoint, execution); err != nil {
			return err
		}
		var acceptanceErr error
		_, err := s.promptTask(promptCtx, taskID, sessionID, checkpoint.request.Instruction, "", false, nil, true, launchOriginManual, promptTaskOptions{
			resumeAttempt: attempt, preservePromptContext: true, disableDispatchRetry: true, requireNonterminalSession: true, reserveTurnUntilDispatch: true, deliverySubmissionID: checkpoint.submissionID, expectedDeliveryGeneration: checkpoint.recovery.HarnessGeneration + 1,
			onAccepted: func(turnID string) {
				now := time.Now().UTC()
				acceptanceErr = s.recordInterruptedInstruction(promptCtx, taskID, sessionID, turnID, checkpoint)
				if acceptanceErr == nil {
					acceptanceErr = checkpoint.store.CompleteContinuationSnapshot(promptCtx, checkpoint.snapshotID, models.ContinuitySnapshotConsumed, now)
				}
				if acceptanceErr == nil {
					acceptanceErr = checkpoint.store.CompleteRestoreAttempt(promptCtx, checkpoint.snapshotID, "interrupted_continued", now)
				}
				if acceptanceErr == nil {
					accepted = true
					timer.Stop()
				}
			},
		})
		return errors.Join(err, acceptanceErr)
	})
	if err != nil {
		s.logger.Warn("interrupted continuation remains blocked", zap.String("session_id", sessionID), zap.Error(err))
	}
	outcome := SessionDeliveryRecoveryRestoredBlocked
	reason := continuationOutcomeUnknown
	if accepted {
		outcome = SessionDeliveryRecoveryContinued
		reason = ""
	}
	s.publishAgentDeliveryRecoveryState(context.WithoutCancel(ctx), sessionID)
	return sessionDeliveryRecoveryResponse(taskID, sessionID, outcome, reason, checkpoint.request.RecoveryRevision, &checkpoint.request.RecoveryIdentity)
}

func (s *Service) recordInterruptedInstruction(ctx context.Context, taskID, sessionID, turnID string, checkpoint *interruptedContinuationCheckpoint) error {
	if s.messageCreator == nil {
		return errors.New("continuation message persistence unavailable")
	}
	messageID := interruptedInstructionMessageID(checkpoint.snapshotID)
	return s.messageCreator.CreateUserMessageIdempotent(ctx, messageID, taskID, checkpoint.request.Instruction,
		sessionID, turnID, NewUserMessageMeta().ToMap())
}

func (s *Service) commitInterruptedRestore(ctx context.Context, sessionID string, checkpoint *interruptedContinuationCheckpoint, execution *executor.TaskExecution) error {
	if execution == nil || s.currentACPSessionID(sessionID) != checkpoint.generation.NativeSessionID {
		return errors.New("native restore identity changed")
	}
	if err := s.authorizeTaskSessionPair(ctx, execution.TaskID, sessionID); err != nil {
		return err
	}
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return err
	}
	if err := s.ensureTaskNotArchived(ctx, execution.TaskID); err != nil {
		return err
	}
	generation := checkpoint.generation
	generation.PredecessorGeneration = generation.Generation
	generation.Generation++
	generation.CreationReason = "interrupted_continued"
	generation.CreatedAt = time.Now().UTC()
	generation.CommittedAt = generation.CreatedAt
	changed, err := checkpoint.store.CommitInterruptedContinuation(ctx, &models.InterruptedContinuationCommit{Recovery: checkpoint.recovery, Generation: generation, CandidateExecutionID: execution.AgentExecutionID, BlockID: checkpoint.blockID, SnapshotID: checkpoint.snapshotID, ContentHash: checkpoint.contentHash})
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("interrupted continuation owner changed")
	}
	return nil
}

func (s *Service) retireInterruptedExecutorRow(ctx context.Context, sessionID string, checkpoint *interruptedContinuationCheckpoint) error {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) || errors.Is(err, sql.ErrNoRows) || (err == nil && running == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	if running.AgentExecutionID != checkpoint.recovery.AgentExecutionID {
		return errors.New("interrupted execution owner changed")
	}
	deleter, ok := s.repo.(interface {
		DeleteExecutorRunningIfCurrent(context.Context, string, string, time.Time) error
	})
	if !ok {
		return errors.New("guarded execution retirement unavailable")
	}
	return deleter.DeleteExecutorRunningIfCurrent(ctx, sessionID, running.AgentExecutionID, running.UpdatedAt)
}
