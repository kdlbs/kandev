package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/statussummary"
)

type taskStatusSummaryQuotaWaitReader interface {
	LoadTaskStatusSummaryQuotaWaits(context.Context, []string) (map[string]*statussummary.QuotaWaitSummary, error)
}

func (s *Service) loadSummaryQuotaWaits(
	ctx context.Context,
	taskIDs []string,
) (map[string]*statussummary.QuotaWaitSummary, error) {
	reader, ok := s.tasks.(taskStatusSummaryQuotaWaitReader)
	if !ok {
		return nil, nil
	}
	return reader.LoadTaskStatusSummaryQuotaWaits(ctx, taskIDs)
}
