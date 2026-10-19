package coordinator

import (
	"context"
	"errors"
	"expvar"

	"go.uber.org/zap"
)

const abortReasonProjectsRead = "projects_read_failed"

var deliveryAbortedTotal = expvar.NewMap("coordinator_delivery_aborted_total")

func deliveryAbortedCounter(reason string) int64 { return expvarMapValue(deliveryAbortedTotal, reason) }

// wakeRepos is the repositories of the pending wakes' tasks, read once; err is
// set when that read failed.
type wakeRepos struct {
	byTask map[string][]string
	err    error
}

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
	g, err := s.newWatchGate(ctx, set, coord.WorkspaceID)
	if err != nil {
		deliveryAbortedTotal.Add(abortReasonProjectsRead, 1)
		s.logger.Warn("coordinator delivery: project scope read failed; delivery aborted",
			zap.String("coordinator_id", coord.ID), zap.Error(err))
		return nil, err
	}
	own, err := s.store.ListOwnTasks(ctx, coord.ID)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(pending))
	for _, w := range pending {
		taskIDs = append(taskIDs, w.TaskID)
	}
	byTask, repoErr := s.taskRepositoryIDs(ctx, g, taskIDs)
	repos := wakeRepos{byTask: byTask, err: repoErr}
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
	keep, superseded := s.partitionWakes(ctx, src, coord, g, repos, workflows, pending)
	if superseded {
		s.publishCoordinatorUpdatedWith(ctx, coord.WorkspaceID, coord.ID, true)
	}
	return keep, nil
}

// partitionWakes returns the first deliveryWakeLimit wakes that still hold and
// supersedes the ones that no longer do; superseded reports whether any row
// changed.
func (s *Service) partitionWakes(ctx context.Context, src WakeSources, coord *Coordinator, g *watchGate, repos wakeRepos, workflows map[string]string, pending []pendingWake) ([]pendingWake, bool) {
	var keep []pendingWake
	superseded := false
	for _, w := range pending {
		if len(keep) >= deliveryWakeLimit {
			break
		}
		holds, err := s.wakeHolds(ctx, src, coord, g, repos, workflows, w)
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
	return keep, superseded
}

// wakeHolds reports whether the wake's task is still owned, still in the watch
// set, and still shows the wake's exact episode.
func (s *Service) wakeHolds(ctx context.Context, src WakeSources, coord *Coordinator, g *watchGate, repos wakeRepos, workflows map[string]string, w pendingWake) (bool, error) {
	workflowID, owned := workflows[w.TaskID]
	if !owned || !g.set.Contains(workflowID) {
		return false, nil
	}
	if g.needsRepositories() && repos.err != nil {
		return false, repos.err
	}
	if !g.task(workflowID, repos.byTask[w.TaskID]) {
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
