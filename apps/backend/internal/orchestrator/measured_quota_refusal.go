package orchestrator

import (
	"context"
	"errors"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

type measuredQuotaDispatch struct {
	session           *models.TaskSession
	admissionObserved bool
}

// Measured usage can reject a new input before its admission callback runs.
// Routing that refusal must retain the exact prepared owner and never settle
// an admitted delivery or manufacture a provider terminal event.
func (s *Service) routeMeasuredQuotaRefusal(
	ctx context.Context, dispatch measuredQuotaDispatch, rollback promptClaimRollback,
	options promptTaskOptions, resumeAttempt *resumeAttempt, foreground *foregroundDispatch,
	releaseDispatchGuard func(), failure error,
) (bool, error) {
	classified := measuredQuotaFailure(failure)
	expected := dispatch.session
	if classified == nil || !s.measuredQuotaDispatchEligible(dispatch, rollback, options) {
		return false, nil
	}
	if releaseDispatchGuard != nil {
		releaseDispatchGuard()
	}
	guard, err := s.lockCancelInFlightGuardWithContext(ctx, expected.ID)
	if err != nil {
		return true, err
	}
	defer guard.release()
	if err := s.validatePromptDispatchOwnership(ctx, expected.TaskID, expected.ID, expected, rollback, options, resumeAttempt); err != nil {
		return true, err
	}
	if err := s.checkSessionRecoveryBlock(ctx, expected.ID); err != nil {
		return true, err
	}
	current, err := s.repo.GetTaskSession(ctx, expected.ID)
	if err != nil || !sameMeasuredQuotaOwner(current, expected) {
		return true, errors.Join(err, dynamicruntime.ErrStaleGeneration)
	}
	data, ok := s.measuredQuotaAttempt(expected)
	if !ok {
		return true, errors.New("measured quota attempt ownership is no longer current")
	}
	defer s.clearPromptAttemptEvidence(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
	s.rollbackForegroundDispatchOnFailure(ctx, expected.TaskID, expected.ID, foreground)
	rollback.dispatchGuard, rollback.dispatchGuardRelease = nil, nil
	s.rollbackPromptClaim(ctx, expected.TaskID, expected.ID, rollback)
	if err := s.completeTurnForTaskSessionCheckedOwned(ctx, expected.TaskID, expected.ID, rollback.turnID); err != nil {
		return true, err
	}
	return true, s.applyMeasuredQuotaPolicy(ctx, current, data, classified)
}

func (s *Service) measuredQuotaDispatchEligible(dispatch measuredQuotaDispatch, rollback promptClaimRollback, options promptTaskOptions) bool {
	expected := dispatch.session
	return !dispatch.admissionObserved && expected != nil && rollback.turnID != "" &&
		options.recoveryAction == "" && !options.lifecyclePrompt && !options.internalContinuation &&
		s.profileExecutionResolver != nil && expected.QueueIncarnationID != "" &&
		expected.RouteGeneration > 0 && expected.ExecutionProfileID != ""
}

func measuredQuotaFailure(err error) *routingerr.Error {
	var failure *routingerr.Error
	if !errors.As(err, &failure) || failure == nil || failure.Code != routingerr.CodeQuotaLimited ||
		failure.ClassifierRule != "native_measured_usage_limit" || failure.Phase != routingerr.PhasePromptSend ||
		failure.Confidence != routingerr.ConfHigh || failure.Class != routingerr.ClassHard || !failure.FallbackAllowed {
		return nil
	}
	return failure
}

func sameMeasuredQuotaOwner(current, expected *models.TaskSession) bool {
	return current != nil && current.ID == expected.ID && current.TaskID == expected.TaskID &&
		current.QueueIncarnationID == expected.QueueIncarnationID && current.AgentExecutionID != "" &&
		current.AgentExecutionID == expected.AgentExecutionID && current.RouteGeneration == expected.RouteGeneration &&
		current.ExecutionProfileID == expected.ExecutionProfileID && current.AgentProfileID == expected.AgentProfileID
}

func (s *Service) measuredQuotaAttempt(session *models.TaskSession) (watcher.AgentEventData, bool) {
	attempt, ok := s.promptAttemptForSession(session.ID)
	if !ok {
		return watcher.AgentEventData{}, false
	}
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	if !attempt.dynamic || !attempt.evidenceKnown || attempt.executionID != session.AgentExecutionID ||
		attempt.promptGeneration == 0 || attempt.output || attempt.effect {
		return watcher.AgentEventData{}, false
	}
	return watcher.AgentEventData{
		TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: session.AgentExecutionID,
		ExecutionProfileID: session.ExecutionProfileID, PromptGeneration: attempt.promptGeneration,
	}, true
}

func (s *Service) applyMeasuredQuotaPolicy(ctx context.Context, session *models.TaskSession, data watcher.AgentEventData, failure *routingerr.Error) error {
	defer s.scheduleFailedRouteAutoRetryAfterFailure(ctx, session.ID, session.ExecutionProfileID, failure)
	if err := s.profileExecutionResolver.ClearUnclassifiedStreak(ctx, session.ID, session.RouteGeneration, session.ExecutionProfileID); err != nil {
		return err
	}
	conductor := s.profileExecutionResolver.NewConductor(nil)
	task, err := s.scheduler.GetTask(ctx, session.TaskID)
	if err != nil {
		return err
	}
	decision, input, err := s.routeDynamicFailureDecision(ctx, session, task, conductor, failure, dynamicruntime.UnclassifiedFailureEvidence{}, false)
	if err != nil {
		if !s.persistPendingDynamicRecovery(ctx, session, decision, err) {
			s.markDynamicRouteActionRequired(ctx, session.ID, session.RouteGeneration, string(failure.Code))
		}
		return nil
	}
	if !s.launchDynamicSuccessorAfterFailure(ctx, data, session, conductor, decision, input, nil) {
		s.markDynamicRouteActionRequired(ctx, session.ID, decision.Generation, string(failure.Code))
		return errors.New("measured quota successor was not scheduled")
	}
	return nil
}
