package orchestrator

import (
	"context"
	"errors"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

var (
	// ErrTurnNotActive means the turn CancelTurn was asked to cancel is not the
	// session's active turn. Nothing was cancelled.
	ErrTurnNotActive = errors.New("turn is not the session's active turn")
	// ErrCancelInFlight means another cancellation of the session holds the
	// claim. CancelTurn never joins it: the fence would be skipped.
	ErrCancelInFlight = errors.New("another cancellation of the session is in flight")
)

// CancelTurn cancels the session's agent when expectedTurnID is its active
// turn, silently: no authorization, no message, no workflow completion. The
// agent-level cancel is narrowed to the prompt observed inside the
// cancellation guard, so a prompt that starts afterwards is never cancelled.
// A cancel the agent did not acknowledge (escalated) counts as confirmed.
func (s *Service) CancelTurn(ctx context.Context, sessionID, expectedTurnID string) error {
	if sessionID == "" || expectedTurnID == "" {
		return errors.New("cancel turn: session id and expected turn id are required")
	}
	if s.repo == nil || s.turnService == nil {
		return errors.New("cancel turn: turn authority is not configured")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("cancel turn: load session: %w", err)
	}
	active, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("cancel turn: inspect active turn: %w", err)
	}
	if active != expectedTurnID {
		return ErrTurnNotActive
	}
	operation, owner, _, accepted := s.claimCancellationWithActionExclusive(sessionID, cancellationKindInternal, nil)
	if !accepted || !owner {
		return ErrCancelInFlight
	}
	s.setCancellationExpectedTurn(sessionID, operation, expectedTurnID)
	go s.runTurnCancellation(ctx, session.TaskID, sessionID, operation)
	err = operation.wait(ctx)
	if errors.Is(err, ErrSendNowTurnChanged) || errors.Is(err, agentruntime.ErrPromptActivityNotOwned) {
		return ErrTurnNotActive
	}
	return err
}

func (s *Service) runTurnCancellation(requestCtx context.Context, taskID, sessionID string, operation *cancelOperation) {
	endProjection := s.beginCancellationProjection(sessionID)
	operation.projectionRelease = endProjection
	defer endProjection()
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), cancellationOperationTTL)
	defer cancel()
	err := s.runTurnCancellationOwned(operationCtx, taskID, sessionID, operation)
	s.finishCancellationWithActions(operationCtx, sessionID, operation, err)
}

func (s *Service) runTurnCancellationOwned(ctx context.Context, taskID, sessionID string, operation *cancelOperation) error {
	guard, err := s.lockCancelInFlightGuardWithContext(ctx, sessionID)
	if err != nil {
		return err
	}
	defer guard.release()

	identity, err := s.captureCancellationIdentity(ctx, sessionID)
	if err != nil {
		return err
	}
	if expectedTurnID, ready := s.cancellationExpectedTurnSnapshot(operation); ready && identity.turnID != expectedTurnID {
		return ErrTurnNotActive
	}
	if err := s.capturePromptActivity(ctx, sessionID, &identity); err != nil {
		return err
	}
	s.setCancellationIdentity(sessionID, operation, identity)
	if err := s.cancelAgentWhileUnlockedForPrompt(ctx, sessionID, identity, operation, guard.unlock, guard.relockWithContext); err != nil {
		return err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session after turn cancel: %w", err)
	}
	completionEligible, err := s.cancelTurnCompletionEligible(ctx, session, sessionID)
	if err != nil {
		return err
	}
	s.setCancellationCompletionEligible(sessionID, operation, completionEligible)
	prepared := cancelAgentPreparation{session: session, identity: identity, completionEligible: completionEligible}
	return s.finishSilentCancelledAgentTurn(ctx, taskID, sessionID, prepared)
}

// capturePromptActivity overlays the execution, prompt generation and
// activity epoch the cancel is fenced on. A session with no tracked execution
// keeps an empty execution id, which cancels through the plain path.
func (s *Service) capturePromptActivity(ctx context.Context, sessionID string, identity *cancellationIdentity) error {
	reader, ok := s.agentManager.(promptActivitySessionReader)
	if !ok {
		return errors.New("agent manager cannot report prompt activity")
	}
	executionID, generation, epoch, _, err := reader.GetPromptActivityForSession(ctx, sessionID)
	if err != nil {
		if executor.IsNoExecutionForSessionError(err) {
			identity.executionID, identity.promptGeneration, identity.activityEpoch = "", 0, 0
			return nil
		}
		return fmt.Errorf("read prompt activity: %w", err)
	}
	identity.executionID, identity.promptGeneration, identity.activityEpoch = executionID, generation, epoch
	return nil
}
