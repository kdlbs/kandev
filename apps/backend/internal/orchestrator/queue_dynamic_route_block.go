package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// dynamicRouteOrphanedStartingReason is the route reason recorded for a
// WAITING_FOR_INPUT session whose "starting" route has no executors_running row.
const dynamicRouteOrphanedStartingReason = "orphaned_starting_route"

// dynamicRouteAwaitsUserAction reports whether queued work for the session can
// only fail until the user acts on its dynamic route. The route is
// action_required and no executors_running row exists, so ensureSessionRunning
// has nothing to resume and no launch owner will create one. The session's
// route recovery banner (Retry / Try next / Stop) is the visible block; a
// successful route action launches the session again and drains its queue.
func (s *Service) dynamicRouteAwaitsUserAction(ctx context.Context, session *models.TaskSession) bool {
	if session == nil || session.RouteState != dynamicRouteStatusActionRequired {
		return false
	}
	_, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	return errors.Is(err, models.ErrExecutorRunningNotFound)
}

// queueDrainBlockedByDynamicRoute reports whether an automatic queue drain
// must leave the head entry in place. A session whose route is action_required
// and which has no executors_running row cannot run queued work until a route
// action applies; a successful route action launches the session and drains the
// queue.
func (s *Service) queueDrainBlockedByDynamicRoute(ctx context.Context, sessionID string) bool {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || !s.dynamicRouteAwaitsUserAction(ctx, session) {
		return false
	}
	s.logger.Info("skipping queue drain while the dynamic route awaits a recovery action",
		zap.String("session_id", sessionID),
		zap.String("route_state", session.RouteState),
		zap.String("route_reason", session.RouteReason))
	return true
}

// retainQueuedMessageForDynamicRouteAction settles a queued dispatch that
// cannot run because the session has no executor record while its dynamic
// route awaits a user action. A WAITING_FOR_INPUT session whose starting route
// is stranded is released to action_required with the orphaned-starting reason,
// and the entry is made visible for that recovery: a retained durable delivery
// is released in place, while an entry removed on reserve is restored. Every
// other failure returns false so the existing retry path keeps handling it.
func (s *Service) retainQueuedMessageForDynamicRouteAction(
	ctx context.Context,
	callerSessionID string,
	queuedMsg *messagequeue.QueuedMessage,
	reservation *queuedDispatchReservation,
	userMessageRecorded bool,
	err error,
) bool {
	if queuedMsg == nil || !errors.Is(err, errSessionAwaitingRuntimeLaunch) {
		return false
	}
	releaseRouteAction := s.acquireRouteActionOperationLock(queuedMsg.SessionID)
	defer releaseRouteAction()

	session, ok := s.releaseStrandedQueuedRoute(ctx, queuedMsg.SessionID, reservation)
	if !ok {
		return false
	}
	if !s.dynamicRouteAwaitsUserAction(ctx, session) {
		return false
	}
	if userMessageRecorded {
		markQueuedUserMessageRecorded(queuedMsg)
	}
	s.logger.Warn("queued message retained until the dynamic route recovery action runs",
		zap.String("session_id", callerSessionID),
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("queue_id", queuedMsg.ID),
		zap.String("route_state", session.RouteState),
		zap.String("route_reason", session.RouteReason))
	if queuedMsg.IsReservedDelivery() {
		return s.releaseRetainedDynamicRouteDelivery(ctx, callerSessionID, queuedMsg, reservation)
	}
	if reservation != nil && reservation.identity.SessionIncarnationID != "" {
		s.restoreQueuedMessageForSession(ctx, reservation.identity, queuedMsg)
	} else {
		s.restoreQueuedMessage(ctx, queuedMsg)
	}
	return true
}

func (s *Service) releaseStrandedQueuedRoute(
	ctx context.Context,
	sessionID string,
	reservation *queuedDispatchReservation,
) (*models.TaskSession, bool) {
	// Deletion and identity replacement cannot cross the route settlement.
	guard := s.lockCancelInFlightGuard(sessionID)
	defer guard.release()

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil, false
	}
	if !reservation.matchesSessionIdentity(session.TaskID, session.ID, session.QueueIncarnationID) {
		return nil, false
	}
	return s.releaseStrandedStartingRoute(ctx, session), true
}

func (s *Service) releaseStrandedStartingRoute(
	ctx context.Context,
	session *models.TaskSession,
) *models.TaskSession {
	if !isStartingRouteClaim(session) || !s.dynamicStartingRouteIsStranded(ctx, session) {
		return session
	}
	s.markDynamicRouteActionRequired(ctx, session.ID, session.RouteGeneration, dynamicRouteOrphanedStartingReason)
	reloaded, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil || reloaded == nil {
		return session
	}
	return reloaded
}

func (s *Service) releaseRetainedDynamicRouteDelivery(
	ctx context.Context,
	callerSessionID string,
	queuedMsg *messagequeue.QueuedMessage,
	reservation *queuedDispatchReservation,
) bool {
	if queuedMsg.IsDeliveryAttempted() || s.messageQueue == nil ||
		reservation == nil || reservation.identity.SessionIncarnationID == "" {
		return false
	}
	if releaseErr := s.messageQueue.ReleaseQueuedDeliveryForSession(
		ctx, reservation.identity, queuedMsg,
	); releaseErr != nil {
		if errors.Is(releaseErr, messagequeue.ErrSessionIdentityMismatch) {
			s.discardLifecycleReservationForReplacedSession(ctx, queuedMsg, reservation.identity)
			return true
		}
		s.logger.Warn("failed to release queued delivery for dynamic route recovery",
			zap.String("session_id", callerSessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID),
			zap.Error(releaseErr))
		return false
	}
	s.publishQueueStatusEventForIdentity(ctx, reservation.identity)
	return true
}
