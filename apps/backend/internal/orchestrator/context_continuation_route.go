package orchestrator

import (
	"context"
	"errors"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// Explicit context recovery may claim the already-selected failed route without
// invoking the route-action launcher or replaying its predecessor prompt.
func (s *Service) claimContextContinuationRoute(ctx context.Context, session *models.TaskSession) (bool, error) {
	if session.RouteState != dynamicRouteStatusActionRequired || session.RouteGeneration <= 0 ||
		session.ExecutionProfileID == "" || s.profileExecutionResolver == nil {
		return false, nil
	}
	releaseRoute := s.acquireSessionRouteOperationLock(session.ID)
	defer releaseRoute()
	releaseLifecycle := s.acquireSessionLifecycleLock(session.ID)
	defer releaseLifecycle()
	current, err := s.loadContextContinuationRoute(ctx, session)
	if err != nil {
		return false, err
	}
	resolved, err := s.profileExecutionResolver.ResolveRouteAction(ctx, current.ID, current.AgentProfileID,
		current.ExecutionProfileID, current.RouteGeneration, "retry")
	if err != nil {
		return false, err
	}
	if resolved.Generation != current.RouteGeneration || resolved.ExecutionProfileID != current.ExecutionProfileID ||
		resolved.Decision.Status != dynamicRouteStatusRetrying {
		return false, dynamicruntime.ErrStaleGeneration
	}
	selected := *current
	selected.RouteState, selected.RouteReason = resolved.Decision.Status, resolved.Decision.Reason
	changed, err := s.persistContextContinuationRoute(ctx, current, &selected)
	if err != nil || !changed {
		if err == nil {
			err = dynamicruntime.ErrStaleGeneration
		}
		return false, errors.Join(err, s.profileExecutionResolver.MarkRouteRecoveryActionRequired(context.WithoutCancel(ctx), current.ID, current.RouteGeneration))
	}
	*session = selected
	return true, nil
}

type contextContinuationRouteUpdater interface {
	UpdateTaskSessionIfCurrentSnapshot(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}) (bool, error)
}

// The claim keeps the same concrete candidate, so no candidate runtime reset
// applies. The write is guarded by the observed session revision and shares the
// settings snapshot lock with other route projections.
func (s *Service) persistContextContinuationRoute(ctx context.Context, observed, selected *models.TaskSession) (bool, error) {
	settingsLock := s.sessionSettingsSnapshotLock(selected.ID)
	settingsLock.Lock()
	defer settingsLock.Unlock()
	updater, ok := s.repo.(contextContinuationRouteUpdater)
	if !ok {
		return false, errors.New("context recovery route selection requires guarded metadata updates")
	}
	return updater.UpdateTaskSessionIfCurrentSnapshot(ctx, selected, observed.State, observed.UpdatedAt, selected.Metadata)
}

func (s *Service) loadContextContinuationRoute(ctx context.Context, expected *models.TaskSession) (*models.TaskSession, error) {
	current, err := s.repo.GetTaskSession(ctx, expected.ID)
	if err != nil {
		return nil, err
	}
	if !sameFailedContextRoute(current, expected) {
		return nil, dynamicruntime.ErrStaleGeneration
	}
	if s.agentManager != nil && s.agentManager.IsAgentRunningForSession(ctx, current.ID) {
		return nil, &sessionOpenRecoveryBlockedError{reason: autoResumeBlockedDynamicRoute}
	}
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return nil, errors.New("context recovery route state is unavailable")
	}
	state, err := loader.LoadRouteState(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Status != dynamicRouteStatusActionRequired || state.Generation != current.RouteGeneration ||
		state.ExecutionProfileID != current.ExecutionProfileID || state.LogicalProfileID != current.AgentProfileID {
		return nil, dynamicruntime.ErrStaleGeneration
	}
	return current, nil
}

func sameFailedContextRoute(current, expected *models.TaskSession) bool {
	return current != nil && current.RouteState == dynamicRouteStatusActionRequired &&
		current.RouteGeneration == expected.RouteGeneration && current.ExecutionProfileID == expected.ExecutionProfileID &&
		current.TaskID == expected.TaskID
}

// retainedContextContinuationRoute reports a manual retry claim that a route
// action kept behind the recovery fence for this explicit recovery.
func (s *Service) retainedContextContinuationRoute(ctx context.Context, session *models.TaskSession) bool {
	return s.profileExecutionResolver != nil && session != nil && session.RouteState == dynamicRouteStatusRetrying &&
		session.RouteGeneration > 0 && session.ExecutionProfileID != "" &&
		(s.agentManager == nil || !s.agentManager.IsAgentRunningForSession(ctx, session.ID))
}

func (s *Service) releaseAbandonedContextContinuationRoute(ctx context.Context, sessionID string, generation int64, succeeded *bool) {
	if *succeeded {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err := s.profileExecutionResolver.MarkRouteRecoveryActionRequired(cleanupCtx, sessionID, generation); err != nil {
		s.logger.Warn("failed to release abandoned context recovery route", zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	s.mirrorReleasedContextContinuationRoute(cleanupCtx, sessionID, generation)
}

// The session projection follows the engine only when the engine returned this
// generation to action_required and the projection still shows the claim, so a
// newer route transition is never overwritten.
func (s *Service) mirrorReleasedContextContinuationRoute(ctx context.Context, sessionID string, generation int64) {
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return
	}
	state, err := loader.LoadRouteState(ctx, sessionID)
	if err != nil || state == nil || state.Status != dynamicRouteStatusActionRequired || state.Generation != generation {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteState != dynamicRouteStatusRetrying {
		return
	}
	s.mirrorDynamicRouteProjection(ctx, session, generation, dynamicRouteStatusActionRequired, RouteActionLaunchFailedReason)
}
