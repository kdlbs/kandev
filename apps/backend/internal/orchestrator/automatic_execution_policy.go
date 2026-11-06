package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type automaticExecutionCandidateReader interface {
	ListAutomaticExecutionCandidates(context.Context, string, int) ([]models.SilentRestoreCandidate, error)
}

type automaticRestoreAttemptReader interface {
	GetRestoreAttempt(context.Context, string) (*models.RestoreAttempt, error)
}

type automaticHistoryAttemptKey struct{}

func automaticHistoryAttempt(ctx context.Context) string {
	id, _ := ctx.Value(automaticHistoryAttemptKey{}).(string)
	return id
}

// Scheduling, cancellation and worker limits belong to the restart pass.
func (s *Service) runAutomaticExecutionPolicyPass(ctx context.Context) {
	if !s.config.AutoStartPendingWork && !s.config.AutoContinueFromHistory {
		return
	}
	reader, ok := s.repo.(automaticExecutionCandidateReader)
	if !ok {
		return
	}
	queue := s.restartRestoreScheduler(ctx)
	after := ""
	for ctx.Err() == nil {
		candidates, err := reader.ListAutomaticExecutionCandidates(ctx, after, silentRestoreCandidateLimit)
		if err != nil {
			s.logger.Debug("automatic execution inventory unavailable", zap.Error(err))
			return
		}
		for _, candidate := range candidates {
			work := queue.submitWork(candidate, restartRestoreAutomatic, s.applyAutomaticExecutionPolicy)
			if !queue.waitForWork(ctx, work) {
				return
			}
			after = candidate.SessionID
		}
		if len(candidates) < silentRestoreCandidateLimit {
			return
		}
	}
}

func (s *Service) automaticExecutionOwner(ctx context.Context, candidate models.SilentRestoreCandidate) (*silentRestoreOwner, *models.TaskSession, error) {
	if s.silentRestoreOwnerContext == nil || s.messageQueue == nil {
		return nil, nil, nil
	}
	owner, eligible, err := s.loadSilentRestoreOwner(ctx, candidate)
	if err != nil || !eligible {
		return nil, nil, err
	}
	task := owner.task
	if !automaticTaskMayRecover(task) {
		return nil, nil, nil
	}
	if blocked, _ := s.dependencyBlocksAutoStart(owner.ctx, task.ID, "automatic_execution_policy"); blocked {
		return nil, nil, nil
	}
	session, err := s.repo.GetTaskSession(owner.ctx, candidate.SessionID)
	if err != nil || !automaticSessionMatchesOwner(session, task.ID) {
		return nil, nil, err
	}
	if busy, busyErr := s.automaticTaskHasOtherExecution(owner.ctx, task.ID, session.ID); busy || busyErr != nil {
		return nil, nil, busyErr
	}
	status, err := s.messageQueue.Snapshot(owner.ctx, automaticExecutionQueueIdentity(session))
	if err != nil || !status.AutoRun {
		return nil, nil, err
	}
	return owner, session, nil
}

func automaticTaskMayRecover(task *models.Task) bool {
	return task != nil && task.QueuedForStepID == "" && task.State != "COMPLETED" && task.State != "CANCELLED"
}

func automaticSessionMatchesOwner(session *models.TaskSession, taskID string) bool {
	return session != nil && session.TaskID == taskID && session.IsPrimary && !session.IsPassthrough
}

func automaticPreparedTaskMayStart(task *models.Task) bool {
	return task.Autopilot && task.WIPAdmitted
}

func (s *Service) automaticTaskHasOtherExecution(ctx context.Context, taskID, sessionID string) (bool, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return true, err
	}
	for _, other := range sessions {
		if other == nil || other.ID == sessionID {
			continue
		}
		if other.State == models.TaskSessionStateStarting || other.State == models.TaskSessionStateRunning || s.agentManager.IsAgentRunningForSession(ctx, other.ID) {
			return true, nil
		}
	}
	return false, nil
}

func automaticExecutionQueueIdentity(session *models.TaskSession) messagequeue.QueueSessionIdentity {
	incarnation := session.QueueIncarnationID
	if incarnation == "" {
		incarnation = session.ID
	}
	return messagequeue.QueueSessionIdentity{TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: incarnation}
}

func (s *Service) applyAutomaticExecutionPolicy(ctx context.Context, candidate models.SilentRestoreCandidate) error {
	owner, session, err := s.automaticExecutionOwner(ctx, candidate)
	if err != nil || owner == nil {
		return err
	}
	if s.agentManager.IsAgentRunningForSession(owner.ctx, session.ID) {
		return nil
	}
	if session.State == models.TaskSessionStateCreated {
		return s.startAutomaticPendingSession(owner.ctx, owner.task, session)
	}
	return s.continueAutomaticHistory(owner.ctx, session)
}

func (s *Service) startAutomaticPendingSession(ctx context.Context, task *models.Task, session *models.TaskSession) error {
	if !s.config.AutoStartPendingWork || !automaticPreparedTaskMayStart(task) || s.workflowStepGetter == nil {
		return nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil || !step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		return err
	}
	block, err := s.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil || block != nil {
		return err
	}
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil || !found || submission.State != models.InitialPromptSubmissionPending {
		return err
	}
	replayable, err := validateInitialSubmissionReplayState(submission)
	if err != nil || !replayable {
		return err
	}
	if err := s.validateInitialSubmissionAttachments(ctx, task.ID, session.ID, submission); err != nil {
		return err
	}
	_, err = s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: task.ID, SessionID: session.ID, Intent: IntentStartCreated,
		AgentProfileID: session.AgentProfileID, Prompt: submission.Content,
		PlanMode: submission.PlanMode, Attachments: submission.Attachments,
		AutoStart: true, Automatic: true,
	})
	return err
}

func (s *Service) continueAutomaticHistory(ctx context.Context, session *models.TaskSession) error {
	if !s.config.AutoContinueFromHistory ||
		(session.State != models.TaskSessionStateWaitingForInput && session.State != models.TaskSessionStateFailed) {
		return nil
	}
	attemptID, err := s.automaticHistoryRecoveryID(ctx, session)
	if err != nil || attemptID == "" {
		return err
	}
	allowed, err := s.automaticHistoryAttemptAllowed(ctx, session.ID, attemptID)
	if err != nil || !allowed {
		return err
	}
	reservation, admitted := s.reserveSilentRestoreCapacity(ctx, session.TaskID, session.ID)
	if !admitted {
		return nil
	}
	s.automaticHistoryRetries.attempted(session.ID, time.Now())
	defer reservation.releaseIfNotConsumed()
	ctx = context.WithValue(ctx, automaticHistoryAttemptKey{}, attemptID)
	return s.recoverAutomaticHistory(ctx, session)
}

func (s *Service) automaticHistoryAttemptAllowed(ctx context.Context, sessionID, attemptID string) (bool, error) {
	reader, supported := s.repo.(automaticRestoreAttemptReader)
	if !supported {
		return false, nil
	}
	previous, err := reader.GetRestoreAttempt(ctx, attemptID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if previous != nil {
		return false, nil
	}
	if !s.automaticHistoryRetries.due(sessionID, time.Now()) {
		return false, nil
	}
	return true, nil
}

func (s *Service) recoverAutomaticHistory(ctx context.Context, session *models.TaskSession) error {
	response, nativeErr := s.RecoverSessionWithOptions(ctx, session.TaskID, session.ID, recoveryActionResume, RecoverSessionOptions{
		SettingsPolicy: executor.ResumeSettingsPolicyStrict, Automatic: true,
	})
	if nativeErr == nil {
		if response != nil && response.Success {
			return nil
		}
		return errors.New("automatic native resume did not settle")
	}
	var signal recoveryRequiredSignal
	if !errors.As(nativeErr, &signal) || (signal.RecoveryReason() != models.AgentErrorCauseCodeNativeStateMissing && signal.RecoveryReason() != models.AgentErrorCauseCodeNativeResumeUnsupported) {
		return nativeErr
	}
	_, err := s.RecoverSessionWithOptions(ctx, session.TaskID, session.ID, recoveryActionContinueHistory, RecoverSessionOptions{
		SettingsPolicy: executor.ResumeSettingsPolicyStrict, Automatic: true,
	})
	return err
}

func nativeBootstrapRecoveryReason(session *models.TaskSession) string {
	last, found := models.LoadLastAgentError(session.Metadata)
	if !found || last.IsDismissed() || last.Phase != models.LaunchErrorPhaseBootstrap || last.AttemptID == "" {
		return ""
	}
	for _, cause := range last.Causes {
		if cause.Operation == models.AgentErrorCauseOperationResume && cause.PromptNotSent != nil && *cause.PromptNotSent &&
			(cause.Code == models.AgentErrorCauseCodeNativeStateMissing || cause.Code == models.AgentErrorCauseCodeNativeResumeUnsupported) {
			return cause.Code
		}
	}
	return ""
}

func (s *Service) automaticHistoryRecoveryID(ctx context.Context, session *models.TaskSession) (string, error) {
	block, err := s.automaticHistoryBlock(ctx, session.ID)
	if err != nil {
		return "", err
	}
	if block != nil {
		return "automatic-history:" + block.ID, nil
	}
	if session.State != models.TaskSessionStateFailed || nativeBootstrapRecoveryReason(session) == "" {
		return "", nil
	}
	open, err := s.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil || open != nil {
		return "", err
	}
	incarnation, generation, err := s.recoverySessionIdentity(ctx, session.ID)
	if err != nil || generation < 1 {
		return "", err
	}
	return fmt.Sprintf("automatic-history:%s:%s:%d", session.ID, incarnation, generation), nil
}

func (s *Service) automaticHistoryBlock(ctx context.Context, sessionID string) (*models.SessionRecoveryBlock, error) {
	block, err := s.GetOpenSessionRecoveryBlock(ctx, sessionID)
	if err != nil || block == nil {
		return nil, err
	}
	if block.Reason != models.AgentErrorCauseCodeNativeStateMissing && block.Reason != models.AgentErrorCauseCodeNativeResumeUnsupported {
		return nil, nil
	}
	reader, ok := s.repo.(interface {
		ListOpenSessionRecoveryBlocks(context.Context, string, string, int64) ([]*models.SessionRecoveryBlock, error)
	})
	if !ok {
		return nil, nil
	}
	blocks, err := reader.ListOpenSessionRecoveryBlocks(ctx, sessionID, block.IncarnationID, block.ExpectedGeneration)
	if err != nil || len(blocks) != 1 || blocks[0].ID != block.ID {
		return nil, err
	}
	return block, nil
}
