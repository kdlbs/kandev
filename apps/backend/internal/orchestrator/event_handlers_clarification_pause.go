package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// ClarificationPauseOptions controls the queue policy for a clarification
// pause. Human no-answer pauses drain an already queued prompt, while an
// autopilot parent question keeps unrelated child prompts parked until the
// parent answers.
type ClarificationPauseOptions struct {
	DrainQueuedMessages bool
	// Force permits a pause for an explicit async question gate even though
	// the session has no blocking clarification message to detach.
	Force bool
	// ExpectedTurnID binds the pause to the turn that registered the wait. A
	// successor turn makes the pause stale instead of cancelling it.
	ExpectedTurnID *string
}

// ErrStaleQuestionWaitPause reports an explicit wait whose captured turn was
// replaced before the pause could settle. No turn was cancelled.
var ErrStaleQuestionWaitPause = errors.New("question wait turn was superseded before pause")

// PauseForClarificationInput converts a no-answer ask_user_question outcome
// into a platform pause. It detaches the pending clarification so a late user
// answer resumes through the event fallback path, then silently cancels the
// active agent turn without evaluating workflow turn-complete actions. It
// returns the number of clarification bundles detached.
func clarificationInputPhaseContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), clarificationInputPauseTimeout)
}

// subscribeClarificationEvents subscribes to clarification-related events.
func (s *Service) PauseForClarificationInput(ctx context.Context, sessionID string) (int, error) {
	return s.PauseForClarificationInputWithOptions(ctx, sessionID, ClarificationPauseOptions{
		DrainQueuedMessages: true,
	})
}

// PauseForClarificationInputWithOptions is the explicit pause policy used by
// callers whose queue semantics differ from the human clarification timeout.
func (s *Service) PauseForClarificationInputWithOptions(
	ctx context.Context,
	sessionID string,
	options ClarificationPauseOptions,
) (int, error) {
	if sessionID == "" {
		return 0, fmt.Errorf("session_id is required")
	}
	writeCtx, cancel := clarificationInputPhaseContext(ctx)
	defer cancel()
	guard := s.lockCancelInFlightGuard(sessionID)
	defer guard.release()
	guardedPersistenceStartedAt := time.Now()
	session, expectedTurnID, expectedTurn, err := s.loadClarificationPauseState(writeCtx, sessionID, options.ExpectedTurnID)
	if err != nil {
		return 0, err
	}
	if session == nil {
		return 0, nil
	}

	hasPendingClarification := s.sessionHasPendingClarification(writeCtx, sessionID)
	detached := 0
	var detachErr error
	if s.clarificationCanceller != nil {
		detached, detachErr = s.clarificationCanceller.DetachSessionAndNotify(writeCtx, sessionID)
		if detachErr != nil {
			detachErr = fmt.Errorf("detach clarification before pause: %w", detachErr)
		}
	}
	s.logger.Debug("clarification pause persistence completed under cancellation guard",
		zap.String("task_id", session.TaskID),
		zap.String("session_id", sessionID),
		zap.Duration("guard_hold_duration", time.Since(guardedPersistenceStartedAt)))
	if isTerminalSessionState(session.State) {
		return detached, detachErr
	}
	if !options.Force && !hasPendingClarification && detached == 0 && detachErr == nil {
		return detached, nil
	}
	pauseCtx, cancelPause := clarificationInputPhaseContext(writeCtx)
	defer cancelPause()
	if _, has := models.LoadPendingStepSignal(session.Metadata); has {
		s.clearPendingStepSignal(pauseCtx, session)
	}

	// Detach and cancellation registration share the same guard as prompt turn
	// creation. The backend wait path and agentctl timeout notification can race for the
	// same ask_user_question call. A duplicate cancel is safe: lifecycle returns
	// ErrCancelEscalated/ErrNoExecutionForSession once the first pause wins, and
	// completeTurnForSession is idempotent when there is no active turn left.
	if err := s.cancelAgentSilentExpectedWithGuard(
		pauseCtx,
		session.TaskID,
		sessionID,
		expectedTurn,
		guard.unlock,
		guard.relockWithContext,
	); errors.Is(err, ErrSendNowTurnChanged) {
		if options.ExpectedTurnID != nil {
			return detached, ErrStaleQuestionWaitPause
		}
		s.logger.Debug("skipping stale clarification pause after successor turn",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", sessionID),
			zap.String("expected_turn_id", expectedTurnID))
		return detached, detachErr
	} else if err != nil {
		return detached, errors.Join(detachErr, err)
	}
	if !options.DrainQueuedMessages {
		return detached, detachErr
	}
	if detachErr != nil {
		// The durable clarification remains the input authority when detachment
		// failed. Starting a successor here would make the row look stale and
		// allow work to continue without a recoverable question.
		return detached, detachErr
	}
	// Clarification park is a turn boundary, mirroring the
	// pre-#677 contract where handleAgentReady always drained after returning
	// from a turn. PauseForClarificationInput holds the cancelInFlight guard
	// (line 533) for the entire function, so we must use the *Locked* drain
	// variant — the public one would try to re-acquire the same non-reentrant
	// sync.Mutex and deadlock. The detached bundle remains in the UI for the
	// user to answer, but the workflow will no longer block on it for the new
	// turn (PR description flags this trade-off for maintainer review).
	s.drainQueuedMessageForPromptableSessionLocked(pauseCtx, sessionID)
	return detached, detachErr
}

func (s *Service) loadClarificationPauseState(
	ctx context.Context,
	sessionID string,
	capturedTurnID *string,
) (*models.TaskSession, string, *string, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("load session for clarification pause: %w", err)
	}
	if session == nil || s.turnService == nil {
		return session, "", nil, nil
	}
	if capturedTurnID != nil {
		return session, *capturedTurnID, capturedTurnID, nil
	}
	expectedTurnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("inspect clarification turn before pause: %w", err)
	}
	return session, expectedTurnID, &expectedTurnID, nil
}

// cancelAgentSilent cancels the agent turn without creating a visible message
// in the chat. Used by clarification recovery so the cancel-and-retry is seamless.
//
// If the agent manager reports no live execution for the session, the session may be stuck
// (agent crashed mid-turn). In that case, skip the cancel signal but still reconcile the
// session's state so clarification recovery can proceed with a fresh prompt.
func (s *Service) cancelAgentSilent(ctx context.Context, taskID, sessionID string) error {
	_, err := s.cancelAgentSilentAction(ctx, taskID, sessionID, nil)
	return err
}

func (s *Service) cancelAgentSilentAction(
	ctx context.Context,
	taskID, sessionID string,
	action func(context.Context) (bool, error),
) (bool, error) {
	return s.cancelAgentSilentActionWithKind(ctx, taskID, sessionID, action, cancellationKindSilent)
}

func (s *Service) cancelAgentSilentActionWithKind(
	ctx context.Context,
	taskID, sessionID string,
	action func(context.Context) (bool, error),
	kind cancellationKind,
) (bool, error) {
	if s.repo == nil {
		return false, errors.New("cancel agent silently: repository is not configured")
	}
	var registeredAction func(context.Context, *cancelOperation) (bool, error)
	if action != nil {
		registeredAction = func(actionCtx context.Context, _ *cancelOperation) (bool, error) {
			return action(actionCtx)
		}
	}
	operation, owner, registered := s.claimCancellationWithAction(sessionID, kind, registeredAction)
	if owner {
		go s.runSilentCancellation(ctx, taskID, sessionID, operation)
	}
	if err := operation.wait(ctx); err != nil {
		return false, err
	}
	if registered == nil {
		return false, nil
	}
	return registered.wait(ctx)
}

// cancelAgentSilentActionWithKindExclusive is the non-joining cancellation
// path used by Send Now and workflow context reset. A second Send Now click or
// another cancellation that already owns the session is reported as a conflict
// instead of joining and inheriting the first operation's reconciliation
// semantics.
func (s *Service) cancelAgentSilentActionWithKindExclusive(
	ctx context.Context,
	taskID, sessionID string,
	action func(context.Context) (bool, error),
	kind cancellationKind,
	expectedTurnID string,
) (bool, error) {
	return s.cancelAgentSilentActionWithKindExclusiveConflict(
		ctx, taskID, sessionID, action, kind, expectedTurnID, ErrSendNowConflict,
	)
}

func (s *Service) cancelAgentSilentActionWithKindExclusiveConflict(
	ctx context.Context,
	taskID, sessionID string,
	action func(context.Context) (bool, error),
	kind cancellationKind,
	expectedTurnID string,
	conflictErr error,
) (bool, error) {
	operation, registered, err := s.startExclusiveSilentCancellation(
		ctx, taskID, sessionID, action, kind, expectedTurnID, conflictErr,
	)
	if err != nil {
		return false, err
	}
	if err := operation.wait(ctx); err != nil {
		return false, err
	}
	if registered == nil {
		return false, nil
	}
	return registered.wait(ctx)
}

// startExclusiveSilentCancellation registers an exclusive cancellation before
// the caller releases a session guard. Reset uses this hand-off to make its
// reset marker and cancellation claim one admission boundary.
func (s *Service) startExclusiveSilentCancellation(
	ctx context.Context,
	taskID, sessionID string,
	action func(context.Context) (bool, error),
	kind cancellationKind,
	expectedTurnID string,
	conflictErr error,
) (*cancelOperation, *cancellationAction, error) {
	if s.repo == nil {
		return nil, nil, errors.New("cancel agent silently: repository is not configured")
	}
	var registeredAction func(context.Context, *cancelOperation) (bool, error)
	if action != nil {
		registeredAction = func(actionCtx context.Context, _ *cancelOperation) (bool, error) {
			return action(actionCtx)
		}
	}
	operation, owner, registered, accepted := s.claimCancellationWithActionExclusive(
		sessionID, kind, registeredAction,
	)
	if !accepted {
		return nil, nil, conflictErr
	}
	if owner {
		s.setCancellationExpectedTurn(sessionID, operation, expectedTurnID)
		go s.runSilentCancellation(ctx, taskID, sessionID, operation)
	}
	return operation, registered, nil
}

func (s *Service) runSilentCancellation(requestCtx context.Context, taskID, sessionID string, operation *cancelOperation) {
	endProjection := s.beginCancellationProjection(sessionID)
	operation.projectionRelease = endProjection
	defer endProjection()
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), cancellationOperationTTL)
	defer cancel()
	err := s.runSilentCancellationOwned(operationCtx, taskID, sessionID, operation)
	s.finishCancellationWithActions(operationCtx, sessionID, operation, err)
}

func (s *Service) runSilentCancellationOwned(ctx context.Context, taskID, sessionID string, operation *cancelOperation) error {
	guard, err := s.lockCancelInFlightGuardWithContext(ctx, sessionID)
	if err != nil {
		return err
	}
	defer guard.release()

	// Capture only the execution/turn identity before yielding the guard. The
	// peer-interrupt path can race a ready handler that is blocked in its first
	// session read; loading the session before lifecycle cancellation would
	// strand both callers behind that read and prevent the cancel signal from
	// reaching the agent. Silent cancellation never evaluates workflow
	// completion, so its authoritative session reload can happen after the
	// lifecycle wait, immediately before source-specific reconciliation.
	identity, err := s.captureCancellationIdentity(ctx, sessionID)
	if err != nil {
		return err
	}
	if expectedTurnID, expectedReady := s.cancellationExpectedTurnSnapshot(operation); expectedReady && identity.turnID != expectedTurnID {
		return ErrSendNowTurnChanged
	}
	s.setCancellationIdentity(sessionID, operation, identity)
	if err := s.cancelAgentWhileUnlocked(ctx, sessionID, operation, guard.unlock, guard.relockWithContext); err != nil {
		return err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session after silent cancel: %w", err)
	}
	completionEligible, err := s.cancelTurnCompletionEligible(ctx, session, sessionID)
	if err != nil {
		return err
	}
	s.setCancellationCompletionEligible(sessionID, operation, completionEligible)
	prepared := cancelAgentPreparation{session: session, identity: identity}
	prepared.completionEligible = completionEligible
	return s.finishSilentCancelledAgentTurn(ctx, taskID, sessionID, prepared)
}

func (s *Service) finishSilentCancelledAgentTurn(
	ctx context.Context,
	taskID, sessionID string,
	prepared cancelAgentPreparation,
) error {
	if prepared.session != nil {
		if _, err := s.reconcileCancelledTurnOwned(
			ctx,
			taskID,
			sessionID,
			prepared.session,
			true,
			prepared.identity.turnID,
		); err != nil {
			return fmt.Errorf("reconcile silent cancelled turn: %w", err)
		}
		return nil
	}
	if _, err := s.reconcileCancelledTurnOwned(ctx, taskID, sessionID, nil, false, prepared.identity.turnID); err != nil {
		return fmt.Errorf("reconcile silent cancelled turn: %w", err)
	}
	return nil
}

// cancelAgentSilentWithGuard releases the caller's guard while the unified
// cancellation coordinator owns the lifecycle wait. It reacquires the guard
// before returning so callers can continue their source-specific queue or
// clarification action under the same serialization boundary.
func (s *Service) cancelAgentSilentWithGuard(
	ctx context.Context,
	taskID string,
	sessionID string,
	unlockGuard func(),
	relockGuard func(context.Context) error,
) error {
	return s.cancelAgentSilentExpectedWithGuard(
		ctx, taskID, sessionID, nil, unlockGuard, relockGuard,
	)
}

// cancelAgentSilentExpectedWithGuard registers cancellation before releasing
// the caller's guard. A prompt claim therefore cannot slip into the gap, and a
// turn created outside the orchestrator is rejected by the expected identity.
// A non-nil expectedTurnID records either one specific turn or an explicit
// no-turn expectation; nil preserves the fallback when turnService is unwired.
// unlockGuard and relockGuard must be non-nil paired callbacks for the
// caller-held guard. A joining non-owner keeps the original owner's
// expected-turn snapshot instead of replacing it.
func (s *Service) cancelAgentSilentExpectedWithGuard(
	ctx context.Context,
	taskID, sessionID string,
	expectedTurnID *string,
	unlockGuard func(),
	relockGuard func(context.Context) error,
) error {
	if s.repo == nil {
		return errors.New("cancel agent silently: repository is not configured")
	}
	operation, owner, _ := s.claimCancellationWithAction(
		sessionID,
		cancellationKindSilent,
		nil,
	)
	if owner && expectedTurnID != nil {
		s.setCancellationExpectedTurn(sessionID, operation, *expectedTurnID)
	}
	if owner {
		go s.runSilentCancellation(ctx, taskID, sessionID, operation)
	}
	unlockGuard()
	waitErr := operation.wait(ctx)
	if relockErr := relockGuard(ctx); relockErr != nil {
		return errors.Join(waitErr, fmt.Errorf("reacquire cancellation guard: %w", relockErr))
	}
	return waitErr
}

func (s *Service) cancelAgentSilentWithGuardActionKind(
	ctx context.Context,
	taskID string,
	sessionID string,
	unlockGuard func(),
	relockGuard func(context.Context) error,
	action func(context.Context) (bool, error),
	kind cancellationKind,
) (bool, error) {
	if unlockGuard != nil {
		unlockGuard()
		dispatched, actionErr := s.cancelAgentSilentActionWithKind(ctx, taskID, sessionID, action, kind)
		if err := relockGuard(ctx); err != nil {
			return dispatched, errors.Join(actionErr, fmt.Errorf("reacquire cancellation guard: %w", err))
		}
		return dispatched, actionErr
	}
	return s.cancelAgentSilentActionWithKind(ctx, taskID, sessionID, action, kind)
}

func (s *Service) cancelAgentSilentWithGuardActionKindExclusive(
	ctx context.Context,
	taskID, sessionID string,
	unlockGuard func(),
	relockGuard func(context.Context) error,
	action func(context.Context) (bool, error),
	kind cancellationKind,
	expectedTurnID string,
) (bool, error) {
	if unlockGuard != nil {
		unlockGuard()
		dispatched, actionErr := s.cancelAgentSilentActionWithKindExclusive(ctx, taskID, sessionID, action, kind, expectedTurnID)
		if err := relockGuard(ctx); err != nil {
			return dispatched, errors.Join(actionErr, fmt.Errorf("reacquire cancellation guard: %w", err))
		}
		return dispatched, actionErr
	}
	return s.cancelAgentSilentActionWithKindExclusive(ctx, taskID, sessionID, action, kind, expectedTurnID)
}

// cancelAgentSilentWithGuardActionKindExclusiveConflict claims cancellation
// while the caller still owns the session guard, then releases that guard
// while the lifecycle cancellation waits. This is the reset variant: a
// prompt cannot enter between reset-marker publication and the exclusive
// cancellation claim, and the cancellation owner can still reacquire the
// guard for its lifecycle reconciliation.
func (s *Service) cancelAgentSilentWithGuardActionKindExclusiveConflict(
	ctx context.Context,
	taskID, sessionID string,
	unlockGuard func(),
	relockGuard func(context.Context) error,
	action func(context.Context) (bool, error),
	kind cancellationKind,
	expectedTurnID string,
	conflictErr error,
) (*cancelOperation, bool, error) {
	operation, registered, err := s.startExclusiveSilentCancellation(
		ctx, taskID, sessionID, action, kind, expectedTurnID, conflictErr,
	)
	if err != nil {
		return nil, false, err
	}
	if unlockGuard != nil {
		unlockGuard()
	}
	waitErr := operation.wait(ctx)
	var dispatched bool
	if waitErr == nil && registered != nil {
		dispatched, waitErr = registered.wait(ctx)
	}
	if unlockGuard != nil {
		if relockErr := relockGuard(ctx); relockErr != nil {
			waitErr = errors.Join(waitErr, fmt.Errorf("reacquire cancellation guard: %w", relockErr))
		}
	}
	return operation, dispatched, waitErr
}
