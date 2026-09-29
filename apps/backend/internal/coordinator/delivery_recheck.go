package coordinator

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// holdingWakes runs delivery step 2: it re-checks each pending wake's episode
// against stored state, supersedes the ones that no longer hold, and returns
// the first deliveryWakeLimit that do. A failed watch-set or own-task read
// aborts before any supersede; a per-wake read error leaves that wake pending
// and excluded.
func (s *Service) holdingWakes(ctx context.Context, coord *Coordinator) ([]pendingWake, error) {
	pending, err := s.store.listPendingWakes(ctx, coord.ID)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, coord.ID, coord.WorkspaceID)
	if err != nil {
		return nil, err
	}
	own, err := s.store.ListOwnTasks(ctx, coord.ID)
	if err != nil {
		return nil, err
	}
	workflows := make(map[string]string, len(own))
	for _, o := range own {
		workflows[o.TaskID] = o.WorkflowID
	}
	s.wakeMu.Lock()
	src := s.wakeSources
	s.wakeMu.Unlock()
	if src == nil {
		return nil, errors.New("coordinator delivery: wake sources are not wired")
	}
	var keep []pendingWake
	superseded := false
	for _, w := range pending {
		if len(keep) >= deliveryWakeLimit {
			break
		}
		holds, err := s.wakeHolds(ctx, src, coord, set, workflows, w)
		if err != nil {
			s.logger.Warn("coordinator delivery: episode read failed",
				zap.String("coordinator_id", coord.ID), zap.String("wake_id", w.ID), zap.Error(err))
			continue
		}
		if holds {
			keep = append(keep, w)
			continue
		}
		changed, err := s.store.supersedeWake(ctx, w.ID)
		if err != nil {
			s.logger.Warn("coordinator delivery: supersede failed",
				zap.String("coordinator_id", coord.ID), zap.String("wake_id", w.ID), zap.Error(err))
			continue
		}
		if changed {
			wakeSupersededTotal.Add(1)
			superseded = true
		}
	}
	if superseded {
		s.publishCoordinatorUpdatedWith(ctx, coord.WorkspaceID, coord.ID, true)
	}
	return keep, nil
}

// wakeHolds reports whether the wake's task is still owned, still in the watch
// set, and still shows the wake's exact episode.
func (s *Service) wakeHolds(ctx context.Context, src WakeSources, coord *Coordinator, set WatchSet, workflows map[string]string, w pendingWake) (bool, error) {
	workflowID, owned := workflows[w.TaskID]
	if !owned || !set.Contains(workflowID) {
		return false, nil
	}
	episodes, err := s.readEpisodes(ctx, src, coord.WorkspaceID, w.TaskID, []WakeKind{w.Kind})
	if err != nil {
		return false, err
	}
	for _, ep := range episodes {
		if ep.Kind == w.Kind && ep.Key == w.EpisodeKey {
			return true, nil
		}
	}
	return false, nil
}
