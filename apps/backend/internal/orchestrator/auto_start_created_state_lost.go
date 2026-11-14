package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type autoStartStepPromptAdmissionKey struct{}

type autoStartStepPromptAdmission struct {
	check   func(context.Context) error
	restore func()
}

func autoStartStepPromptAdmissionFromContext(ctx context.Context) func() error {
	admission, _ := ctx.Value(autoStartStepPromptAdmissionKey{}).(*autoStartStepPromptAdmission)
	if admission == nil {
		return nil
	}
	return func() error { return admission.check(ctx) }
}

func restoreAutoStartStepPromptHandoff(ctx context.Context) bool {
	admission, _ := ctx.Value(autoStartStepPromptAdmissionKey{}).(*autoStartStepPromptAdmission)
	if admission == nil {
		return false
	}
	admission.restore()
	return true
}

// continueAutoStartAfterCreatedStateLost preserves the undispatched input while
// admitting it against the original workflow step and session incarnation.
func (s *Service) continueAutoStartAfterCreatedStateLost(
	state *autoStartStepPromptState,
	fresh *models.TaskSession,
) error {
	onStep, err := s.taskStillOnAutoStartStep(state)
	if err != nil || !onStep || fresh == nil || isTerminalSessionState(fresh.State) {
		s.requeueAutoStartStepMessage(state)
		return err
	}
	if fresh.ID != state.sessionID || fresh.TaskID != state.taskID ||
		(state.session.QueueIncarnationID != "" && fresh.QueueIncarnationID != state.session.QueueIncarnationID) {
		s.requeueAutoStartStepMessage(state)
		return nil
	}
	if err := s.prepareCreatedAutoStartContinuation(state, fresh); err != nil {
		s.requeueAutoStartStepMessage(state)
		if isCreatedAutoStartContinuationSuperseded(state.ctx, err) {
			return nil
		}
		return err
	}
	state.session = fresh
	if state.shouldQueueIfBusy && sessionMidTurn(fresh) {
		return s.queueOrRestoreAutoStartPrompt(state)
	}
	if fresh.State == models.TaskSessionStateWaitingForInput || sessionMidTurn(fresh) {
		return s.retryAutoStartStepPrompt(state)
	}
	s.requeueAutoStartStepMessage(state)
	return nil
}

func (s *Service) prepareCreatedAutoStartContinuation(state *autoStartStepPromptState, fresh *models.TaskSession) error {
	if state.createdAdmission == nil {
		identity, entry, captured, err := s.captureWorkflowStartPromptAdmission(state.ctx, state.taskID, fresh)
		if err != nil {
			return err
		}
		state.createdAdmission = newWorkflowStartPromptAttemptWithAdmission(
			state.taskID, state.sessionID, state.origin, state.prompt, state.planMode,
			state.attachments, state.references, state.handoffForQueue, state.userMessageRecorded,
			state.dispatchInputPresent, identity, entry, captured,
		)
		state.createdAdmission.configMode, _ = state.session.Metadata["config_mode"].(bool)
	}
	attemptCopy := *state.createdAdmission
	state.createdAdmission = &attemptCopy
	attempt := state.createdAdmission
	attempt.retire()
	if attempt.queueIdentity.SessionIncarnationID == "" {
		attempt.queueIdentity = messagequeue.QueueSessionIdentity{
			TaskID: state.taskID, SessionID: state.sessionID,
			SessionIncarnationID: state.session.QueueIncarnationID,
		}
	}
	if ceilingEntryBindingFromContext(state.ctx) == nil && attempt.workflowEntryCaptured {
		if binding, bound := s.workflowEntryBindingForStep(
			state.ctx, state.taskID, state.step, state.sessionID, attempt.workflowEntry.TransitionID,
		); bound {
			state.ctx = withCeilingEntryBinding(state.ctx, binding)
			state.queueCtx = withCeilingEntryBinding(state.queueCtx, binding)
		}
	}
	admission := &autoStartStepPromptAdmission{
		check:   func(ctx context.Context) error { return s.validateCreatedAutoStartContinuation(ctx, state) },
		restore: func() { s.requeueAutoStartStepMessage(state) },
	}
	state.ctx = context.WithValue(state.ctx, autoStartStepPromptAdmissionKey{}, admission)
	state.queueCtx = context.WithValue(state.queueCtx, autoStartStepPromptAdmissionKey{}, admission)
	return admission.check(state.ctx)
}

func (s *Service) validateCreatedAutoStartContinuation(ctx context.Context, state *autoStartStepPromptState) error {
	if err := s.validatePromptCancellationFence(state.sessionID, createdAutoStartCancellationFence(state)); err != nil {
		return err
	}
	onStep, err := s.taskStillOnAutoStartStepContext(ctx, state)
	if err != nil {
		return err
	}
	if !onStep {
		return ErrCeilingLaunchSuperseded
	}
	identity := createdAutoStartSessionIdentity(state)
	if identity != nil {
		fresh, err := s.repo.GetTaskSession(ctx, state.sessionID)
		if err != nil {
			return err
		}
		if fresh == nil || fresh.TaskID != identity.TaskID || fresh.QueueIncarnationID != identity.SessionIncarnationID {
			return ErrCeilingLaunchSuperseded
		}
		if isTerminalSessionState(fresh.State) {
			return newWorkflowAutoStartSessionTerminalizedError(fresh)
		}
	}
	if err := s.validateCreatedAutoStartWorkflowEntry(ctx, state); err != nil {
		return err
	}
	return s.validateContextCeilingEntry(ctx, state.taskID)
}

func (s *Service) validateCreatedAutoStartWorkflowEntry(ctx context.Context, state *autoStartStepPromptState) error {
	attempt := state.createdAdmission
	if attempt == nil || !attempt.workflowEntryCaptured {
		return nil
	}
	reader, ok := s.repo.(workflowStartPromptTransitionReader)
	if !ok {
		return fmt.Errorf("workflow prompt transition reader is unavailable")
	}
	transition, err := reader.GetLatestTaskStepTransitionID(ctx, state.taskID)
	if err != nil {
		return err
	}
	if transition != attempt.workflowEntry.TransitionID {
		return ErrCeilingLaunchSuperseded
	}
	generation, err := s.messageQueue.LifecycleGeneration(ctx, state.taskID)
	if err != nil {
		return err
	}
	if generation != attempt.workflowEntry.LifecycleGeneration {
		return ErrCeilingLaunchSuperseded
	}
	return nil
}

func (s *Service) taskStillOnAutoStartStep(state *autoStartStepPromptState) (bool, error) {
	return s.taskStillOnAutoStartStepContext(state.ctx, state)
}

func (s *Service) taskStillOnAutoStartStepContext(ctx context.Context, state *autoStartStepPromptState) (bool, error) {
	if state.step == nil {
		return false, nil
	}
	task, err := s.repo.GetTask(ctx, state.taskID)
	if err != nil {
		return false, err
	}
	return task != nil && !taskArchived(task) && task.WorkflowStepID == state.step.ID &&
		(task.WorkflowID == "" || state.step.WorkflowID == "" || task.WorkflowID == state.step.WorkflowID), nil
}

func (s *Service) lockCreatedAutoStartFallbackAdmission(ctx context.Context, taskID string) (context.Context, func(), error) {
	if autoStartStepPromptAdmissionFromContext(ctx) == nil {
		return ctx, func() {}, nil
	}
	admissionCtx, release := s.lockCeilingEntryAdmission(ctx, taskID)
	if err := autoStartStepPromptAdmissionFromContext(admissionCtx)(); err != nil {
		release()
		return ctx, nil, err
	}
	return admissionCtx, release, nil
}

// resetSessionForFreshFallbackWithAdmission keeps entry mutation and
// cancellation outside the original session's reset-to-CREATED boundary.
func (s *Service) resetSessionForFreshFallbackWithAdmission(
	ctx context.Context, taskID, sessionID string,
) (*models.TaskSession, error) {
	cancelLock, releaseCancelLock := s.acquireCancelInFlightGuard(sessionID)
	cancelLock.Lock()
	defer func() {
		cancelLock.Unlock()
		releaseCancelLock()
	}()
	if s.isCancelInFlight(sessionID) {
		return nil, &executor.SessionStateSupersededError{SessionID: sessionID, State: models.TaskSessionStateCancelled}
	}
	// Match completion admission: cancellation guard precedes task-entry admission.
	admissionCtx, releaseAdmission, err := s.lockCreatedAutoStartFallbackAdmission(ctx, taskID)
	if err != nil {
		return nil, err
	}
	defer releaseAdmission()
	if admit := autoStartStepPromptAdmissionFromContext(admissionCtx); admit != nil {
		if err := admit(); err != nil {
			return nil, err
		}
	}
	return s.resetSessionForFreshFallback(admissionCtx, sessionID)
}

func createdAutoStartSessionIdentity(state *autoStartStepPromptState) *messagequeue.QueueSessionIdentity {
	if state.createdAdmission == nil || state.createdAdmission.queueIdentity.SessionIncarnationID == "" {
		return nil
	}
	return &state.createdAdmission.queueIdentity
}

func createdAutoStartCancellationFence(state *autoStartStepPromptState) *promptCancellationFence {
	if state.createdAdmission == nil {
		return nil
	}
	if admission := workflowAutoStartAdmissionFromContext(state.ctx, state.taskID, state.sessionID); admission != nil {
		return &admission.cancellationFence
	}
	return nil
}

func isCreatedAutoStartContinuationSuperseded(ctx context.Context, err error) bool {
	return autoStartStepPromptAdmissionFromContext(ctx) != nil &&
		(errors.Is(err, ErrCeilingLaunchSuperseded) ||
			errors.Is(err, messagequeue.ErrWorkflowEntryMismatch) ||
			errors.Is(err, messagequeue.ErrSessionIdentityMismatch))
}

func (s *Service) queueCreatedAutoStartContinuation(state *autoStartStepPromptState) error {
	admit := autoStartStepPromptAdmissionFromContext(state.queueCtx)
	if admit != nil {
		if err := admit(); err != nil {
			return err
		}
	}
	attempt := state.createdAdmission
	if attempt == nil || !attempt.workflowEntryCaptured {
		return s.queueAutoStartPrompt(
			state.queueCtx, state.taskID, state.sessionID, state.prompt, state.planMode,
			state.attachments, state.origin, state.userMessageRecorded, state.references, state.handoffForQueue,
		)
	}
	queued, err := s.persistAutoStartPromptAtWorkflowEntry(
		state.queueCtx, state.taskID, state.sessionID, state.prompt, state.planMode,
		state.attachments, state.origin, state.userMessageRecorded, state.references, state.handoffForQueue,
		attempt.dispatchInputPresent, attempt.configMode, attempt.queueIdentity, attempt.workflowEntry,
	)
	if err != nil {
		return err
	}
	s.scheduleAutoResumeForWorkflowQueue(state.queueCtx, state.sessionID, queued.ID)
	return nil
}

func isCreatedStateLostError(err error) bool {
	return errors.Is(err, executor.ErrSessionStateChangedBeforeStart)
}
