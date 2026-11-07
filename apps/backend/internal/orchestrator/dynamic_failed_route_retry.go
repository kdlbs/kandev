package orchestrator

import (
	"context"
	"errors"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// Automatic failed-route retry is the workspace opt-in that replaces the
// person who would otherwise press Retry on a dynamic task route that a
// classified provider-capacity failure left in manual recovery:
//
//   - action_required: the failed candidate's policy stopped the route, or
//   - waiting (no_eligible_candidate): every candidate was tried or its
//     circuit was open.
//
// With DynamicFailedRouteAutoRetryEnabled on, such a failure durably
// schedules one automatic Retry in the route's policy state, due no earlier
// than one minute and no earlier than the earliest expiry of the open circuit
// of a candidate the retry could use. The shared dynamic recovery timer then
// submits the Retry through the same route-action lock, settled-turn check,
// and backend route-action transaction as a manual Retry, so the successor
// receives the same continuation package and the failed prompt and its tools
// are not replayed. The chain is capped by DynamicFailedRouteAutoRetryMax
// until a turn completes successfully, and stops when the same candidate fails
// with the same code again. Every declined or stopped case keeps today's
// recovery surface. The schedule, the attempt count, and the failures survive
// a restart; the timer is re-armed from them. A run that cannot act yet (an
// active turn or a session that has not settled) re-arms the same schedule
// instead of dropping it, so the retry still fires once the turn ends. A
// terminal session, or one whose logical profile changed, stops the schedule.
// A retry that never runs because the turn never settles is a turn-settlement
// problem rather than something this loop can fix.

// failedRouteAutoRetryMinDelay is the shortest wait before an automatic
// failed-route retry, even when no candidate circuit is open.
const failedRouteAutoRetryMinDelay = time.Minute

// failedRouteAutoRetryDeferredDelay is how long a run that cannot act yet (an
// active turn or a session that has not settled) waits before the schedule is
// re-armed. The durable schedule is left untouched, so the next run still
// checks the original due time.
const failedRouteAutoRetryDeferredDelay = time.Minute

type dynamicRecoveryRouteLister interface {
	ListRecoveryRouteStates(context.Context) ([]dynamicruntime.RouteState, error)
}

// failedRouteAutoRetryStatus reports whether a durable route status is one a
// person would resolve with Retry after a provider failure.
func failedRouteAutoRetryStatus(status string) bool {
	return status == dynamicRouteStatusActionRequired || status == dynamicRouteStatusWaiting
}

// workspaceFailedRouteAutoRetryLimit returns the automatic retry limit of the
// task's workspace, or zero when the opt-in is off or cannot be read.
func (s *Service) workspaceFailedRouteAutoRetryLimit(ctx context.Context, taskID string) int {
	return s.taskWorkspace(ctx, taskID).FailedRouteAutoRetryLimit()
}

// scheduleFailedRouteAutoRetryAfterFailure runs when a classified failure has
// finished routing. Only a provider-capacity failure, the same set mid-turn
// failover accepts, can recover by waiting; credential, account, user,
// runtime, and unclassified failures keep manual recovery.
func (s *Service) scheduleFailedRouteAutoRetryAfterFailure(
	ctx context.Context,
	sessionID, failedExecutionProfileID string,
	classified *routingerr.Error,
) {
	if !midTurnProviderCapacityFailure(classified) {
		return
	}
	s.scheduleFailedRouteAutoRetry(ctx, sessionID, dynamicruntime.FailedRouteFailure{
		ExecutionProfileID: failedExecutionProfileID, Code: classified.Code,
	})
}

// scheduleFailedRouteAutoRetry records and arms one automatic Retry when the
// session's durable route is in manual recovery and its workspace opted in.
// It reports whether a new schedule was recorded; a schedule already recorded
// for the same generation is only re-armed.
func (s *Service) scheduleFailedRouteAutoRetry(
	ctx context.Context,
	sessionID string,
	failure dynamicruntime.FailedRouteFailure,
) bool {
	if failure.ExecutionProfileID == "" {
		return false
	}
	state, session, ok := s.failedRouteAutoRetryTarget(ctx, sessionID)
	if !ok {
		return false
	}
	limit := s.workspaceFailedRouteAutoRetryLimit(ctx, session.TaskID)
	if limit == 0 {
		return false
	}
	pending, scheduled, err := s.profileExecutionResolver.ScheduleFailedRouteRetry(
		ctx, sessionID, session.AgentProfileID, state.Generation, failure, limit, failedRouteAutoRetryMinDelay,
	)
	if err != nil {
		s.logger.Info("automatic failed-route retry declined; route left for manual recovery",
			zap.String("session_id", sessionID), zap.Int64("route_generation", state.Generation),
			zap.String("error_code", string(failure.Code)), zap.Error(err))
		return false
	}
	s.armFailedRouteAutoRetry(sessionID, pending)
	return scheduled
}

// failedRouteAutoRetryTarget returns the durable route and the session when
// the route is in a manual recovery state and the session is a live,
// non-Office task session of that route.
func (s *Service) failedRouteAutoRetryTarget(
	ctx context.Context,
	sessionID string,
) (*dynamicruntime.RouteState, *models.TaskSession, bool) {
	if sessionID == "" || s.profileExecutionResolver == nil || !s.profileExecutionResolver.Enabled() {
		return nil, nil, false
	}
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return nil, nil, false
	}
	state, err := loader.LoadRouteState(ctx, sessionID)
	if err != nil || state == nil || !failedRouteAutoRetryStatus(state.Status) {
		return nil, nil, false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.AgentProfileID != state.LogicalProfileID ||
		isTerminalSessionState(session.State) || s.isOfficeSession(ctx, sessionID) {
		return nil, nil, false
	}
	return state, session, true
}

func (s *Service) armFailedRouteAutoRetry(sessionID string, pending dynamicruntime.PendingFailedRouteRetry) {
	generation := pending.Generation
	s.armDynamicRecoveryTimer(sessionID, generation, pending.Due, func(ctx context.Context) {
		s.runFailedRouteAutoRetry(ctx, sessionID, generation)
	})
}

// restoreFailedRouteAutoRetries re-arms the automatic retries recorded before
// a restart. An overdue schedule fires at once; its run re-validates the
// route, the session, and the workspace setting.
func (s *Service) restoreFailedRouteAutoRetries(ctx context.Context) {
	lister, ok := s.repo.(dynamicRecoveryRouteLister)
	if !ok {
		return
	}
	states, err := lister.ListRecoveryRouteStates(ctx)
	if err != nil {
		s.logger.Warn("failed to restore automatic failed-route retries", zap.Error(err))
		return
	}
	for _, state := range states {
		if pending, scheduled := dynamicruntime.FailedRouteRetryPending(state); scheduled {
			s.armFailedRouteAutoRetry(state.SessionID, pending)
		}
	}
}

// runFailedRouteAutoRetry submits the scheduled Retry. The schedule is
// consumed under the route-action lock just before the backend transaction
// runs, so a timer that fires twice, a replaced timer, or a restart between
// the two never submits a second Retry. A route that moved, a session that
// is busy or settled differently, or a workspace that turned the setting off
// leaves the recovery surface as it is.
func (s *Service) runFailedRouteAutoRetry(ctx context.Context, sessionID string, generation int64) {
	s.removeDynamicPolicyRecoveryTimer(sessionID)
	if !s.dynamicPolicyRecoveryActive(ctx) {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || s.workspaceFailedRouteAutoRetryLimit(ctx, session.TaskID) == 0 {
		return
	}
	request := RouteActionRequest{
		SessionID: sessionID, Action: RouteActionRetry, ExpectedGeneration: generation, Automatic: true,
	}
	result, err := s.applyAutomaticRouteAction(ctx, request, func(ctx context.Context) bool {
		return s.claimFailedRouteAutoRetry(ctx, sessionID, generation)
	})
	if err != nil {
		if errors.Is(err, ErrRouteActionActiveTurn) {
			s.deferFailedRouteAutoRetryLogged(ctx, sessionID, generation)
			return
		}
		s.logger.Info("automatic failed-route retry did not run; route left for manual recovery",
			zap.String("session_id", sessionID), zap.Int64("route_generation", generation), zap.Error(err))
		return
	}
	if result != nil {
		s.logger.Info("automatic failed-route retry submitted",
			zap.String("session_id", sessionID), zap.Int64("failed_route_generation", generation),
			zap.Int64("route_generation", result.RouteGeneration), zap.String("route_state", result.State))
	}
}

// claimFailedRouteAutoRetry admits the retry under the route-action lock: the
// session must be settled and waiting for input, and the durable schedule of
// generation must still be due and unconsumed. A session that has not settled
// yet is re-armed; a terminal session stops the schedule instead of repeating.
func (s *Service) claimFailedRouteAutoRetry(ctx context.Context, sessionID string, generation int64) bool {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return false
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		s.deferFailedRouteAutoRetryLogged(ctx, sessionID, generation)
		return false
	}
	pending, err := s.profileExecutionResolver.ClaimFailedRouteRetry(ctx, sessionID, generation)
	if errors.Is(err, dynamicruntime.ErrRecoveryNotDue) {
		s.armFailedRouteAutoRetry(sessionID, pending)
		return false
	}
	if err == nil {
		s.clearFailedRouteRetryDeferral(sessionID)
	}
	return err == nil
}

func (s *Service) deferFailedRouteAutoRetryLogged(ctx context.Context, sessionID string, generation int64) {
	if !s.deferFailedRouteAutoRetry(ctx, sessionID, generation) {
		return
	}
	if s.noteFailedRouteRetryDeferral(sessionID, generation) {
		s.logger.Info("automatic failed-route retry deferred until the session can take it",
			zap.String("session_id", sessionID), zap.Int64("route_generation", generation))
		return
	}
	s.logger.Debug("automatic failed-route retry is still deferred",
		zap.String("session_id", sessionID), zap.Int64("route_generation", generation))
}

// deferFailedRouteAutoRetry re-arms the retry recorded for generation after a
// run that could not act yet. It reuses failedRouteAutoRetryTarget, so a
// terminal session, a session whose profile changed, or a route that is no
// longer in manual recovery stops the schedule instead of repeating it. Only a
// schedule still pending for the same generation is re-armed.
func (s *Service) deferFailedRouteAutoRetry(ctx context.Context, sessionID string, generation int64) bool {
	state, _, ok := s.failedRouteAutoRetryTarget(ctx, sessionID)
	if !ok || state.Generation != generation {
		return false
	}
	if _, pending := dynamicruntime.FailedRouteRetryPending(*state); !pending {
		return false
	}
	s.armFailedRouteAutoRetry(sessionID, dynamicruntime.PendingFailedRouteRetry{
		Generation: generation,
		Due:        time.Now().UTC().Add(failedRouteAutoRetryDeferredDelay),
	})
	return true
}

// noteFailedRouteRetryDeferral reports whether this is the first deferral of
// the generation, so an unchanged turn that keeps the retry waiting logs once
// at Info and then at Debug.
func (s *Service) noteFailedRouteRetryDeferral(sessionID string, generation int64) bool {
	s.dynamicRecoveryMu.Lock()
	defer s.dynamicRecoveryMu.Unlock()
	if s.failedRouteRetryDeferrals == nil {
		s.failedRouteRetryDeferrals = make(map[string]int64)
	}
	if s.failedRouteRetryDeferrals[sessionID] == generation {
		return false
	}
	s.failedRouteRetryDeferrals[sessionID] = generation
	return true
}

func (s *Service) clearFailedRouteRetryDeferral(sessionID string) {
	s.dynamicRecoveryMu.Lock()
	defer s.dynamicRecoveryMu.Unlock()
	delete(s.failedRouteRetryDeferrals, sessionID)
}

// resetFailedRouteAutoRetryChain ends a session's automatic retry chain after
// a successful turn, so the next failure may retry again up to the limit.
func (s *Service) resetFailedRouteAutoRetryChain(ctx context.Context, session *models.TaskSession) {
	if session == nil || session.RouteGeneration <= 0 || s.profileExecutionResolver == nil {
		return
	}
	if err := s.profileExecutionResolver.ClearFailedRouteRetry(ctx, session.ID); err != nil &&
		!errors.Is(err, dynamicruntime.ErrStaleGeneration) {
		s.logger.Warn("failed to reset automatic failed-route retry chain",
			zap.String("session_id", session.ID), zap.Error(err))
		return
	}
	s.clearFailedRouteRetryDeferral(session.ID)
}
