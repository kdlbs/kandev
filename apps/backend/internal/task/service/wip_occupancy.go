package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrWIPOccupancyUnavailable reports a task store that cannot explain WIP
// occupancy.
var ErrWIPOccupancyUnavailable = errors.New("workflow step WIP occupancy is unavailable")

type workflowStepWIPOccupancyReader interface {
	GetWorkflowStepWIPOccupancy(ctx context.Context, stepID string) (*models.WIPStepOccupancy, error)
}

// GetWorkflowStepWIPOccupancy lists the admitted tasks of a workflow step and
// whether each one holds a WIP slot, under the same step access rule as the
// step's task count.
func (s *Service) GetWorkflowStepWIPOccupancy(ctx context.Context, stepID string) (*models.WIPStepOccupancy, error) {
	if err := s.AuthorizeWorkflowStepAccess(ctx, stepID); err != nil {
		return nil, err
	}
	reader, ok := s.tasks.(workflowStepWIPOccupancyReader)
	if !ok {
		return nil, ErrWIPOccupancyUnavailable
	}
	return reader.GetWorkflowStepWIPOccupancy(ctx, stepID)
}
