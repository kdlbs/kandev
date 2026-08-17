package service

import (
	"context"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// TaskStatusSummaryEventProjector is the synchronous status-summary boundary
// used when a committed state transition must be acknowledged before return.
type TaskStatusSummaryEventProjector interface {
	HandleEvent(context.Context, *bus.Event) error
}

// SetTaskStatusSummaryEventProjector wires the live projector for acknowledged
// pending-state convergence. Ordinary source events still flow through the bus.
func (s *Service) SetTaskStatusSummaryEventProjector(projector TaskStatusSummaryEventProjector) {
	if s != nil {
		s.statusSummaryProjector = projector
	}
}

// GetTaskStatusSummaries batch-loads the bounded task projections used by all
// list/snapshot DTO builders. A nil repository is a safe compatibility state
// for focused tests and installations that have not materialized projections.
func (s *Service) GetTaskStatusSummaries(ctx context.Context, taskIDs []string) (map[string]*statussummary.TaskStatusSummary, error) {
	if s == nil || s.statusSummaries == nil {
		return map[string]*statussummary.TaskStatusSummary{}, nil
	}
	return s.statusSummaries.LoadTaskStatusSummaries(ctx, taskIDs)
}
