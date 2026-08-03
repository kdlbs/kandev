package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/statussummary"
)

// GetTaskStatusSummaries batch-loads the bounded task projections used by all
// list/snapshot DTO builders. A nil repository is a safe compatibility state
// for focused tests and installations that have not materialized projections.
func (s *Service) GetTaskStatusSummaries(ctx context.Context, taskIDs []string) (map[string]*statussummary.TaskStatusSummary, error) {
	if s == nil || s.statusSummaries == nil {
		return map[string]*statussummary.TaskStatusSummary{}, nil
	}
	return s.statusSummaries.LoadTaskStatusSummaries(ctx, taskIDs)
}
