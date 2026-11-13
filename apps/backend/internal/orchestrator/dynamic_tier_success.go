package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"go.uber.org/zap"
)

// Chain completion is independent of the unclassified-counter reset episode:
// current output may already have cleared that counter before turn completion.
func (s *Service) clearTierChainForCurrentSuccess(ctx context.Context, data watcher.AgentEventData) {
	if s.repo == nil || !s.profileExecutionResolver.OrderedTiersEnabled() || !s.currentUnclassifiedStreakEvent(data, false) {
		return
	}
	observed := s.withPromptAttemptEvidence(data)
	if !observed.EvidenceKnown || (!observed.OutputObserved && !observed.EffectObserved) {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || !validUnclassifiedStreakEventSession(session, data) {
		return
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil || task.ID != session.TaskID {
		return
	}
	err = s.profileExecutionResolver.ClearSelectionChain(ctx, session.ID, session.RouteGeneration, session.ExecutionProfileID)
	if err != nil && !errors.Is(err, dynamic.ErrStaleGeneration) && !errors.Is(err, dynamic.ErrRouteStateNotFound) {
		s.logger.Debug("could not end dynamic selection chain after current success", zap.String("session_id", session.ID), zap.Error(err))
	}
}
