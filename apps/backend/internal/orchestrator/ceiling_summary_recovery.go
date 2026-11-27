package orchestrator

import (
	"context"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

type launchQueueSummaryLister interface {
	ListTaskIDsWithLaunchQueueSummary(context.Context) ([]string, error)
}

// The periodic sweep repairs lost enqueue and clear publications independently
// of replay admission. A cleared record remains discoverable through its
// persisted queue summary until an authoritative refresh commits the clear.
func (s *Service) refreshCeilingLaunchQueueSummaries(ctx context.Context, deferred []*models.Task) {
	ids := make(map[string]struct{}, len(deferred))
	for _, task := range deferred {
		if task != nil && task.ID != "" {
			ids[task.ID] = struct{}{}
		}
	}
	if lister, ok := s.repo.(launchQueueSummaryLister); ok {
		cached, err := lister.ListTaskIDsWithLaunchQueueSummary(ctx)
		if err != nil {
			s.logger.Zap().Warn("could not list launch queue summaries for recovery", zap.Error(err))
		} else {
			for _, id := range cached {
				ids[id] = struct{}{}
			}
		}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		if ctx.Err() != nil {
			return
		}
		s.publishTaskUpdatedByID(ctx, id)
	}
}
