package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

type taskStatusSummaryQuotaWaitReader interface {
	LoadTaskStatusSummaryQuotaWaits(context.Context, []string) (map[string]*statussummary.QuotaWaitSummary, error)
}

// reconcileSummaryQuotaWait publishes a new revision for a changed wait, so
// an older HTTP response cannot replace a newer status-summary event.
func (s *Service) reconcileSummaryQuotaWait(ctx context.Context, task *models.Task, current *statussummary.TaskStatusSummary, wait *statussummary.QuotaWaitSummary) (*statussummary.TaskStatusSummary, error) {
	for attempt := 0; attempt < maxSummaryReconcileAttempts && current != nil; attempt++ {
		if equalSummaryQuotaWait(current.QuotaWait, wait) {
			return current, nil
		}
		if attempt == 0 {
			// The batch read can predate a writer that created or cleared
			// this summary; confirm the wait before replacing the stored one.
			waits, err := s.loadSummaryQuotaWaits(ctx, []string{task.ID})
			if err != nil {
				return current, err
			}
			if waits == nil {
				return current, nil
			}
			wait = waits[task.ID]
			if equalSummaryQuotaWait(current.QuotaWait, wait) {
				return current, nil
			}
		}
		next := *current
		next.QuotaWait = wait
		next.Revision++
		next.UpdatedAt = advancedSummaryTime(current.UpdatedAt, time.Now().UTC())
		if err := next.Validate(); err != nil {
			return current, err
		}
		accepted, err := s.persistReconciledSummary(ctx, task, next)
		if err != nil {
			return current, err
		}
		if accepted {
			return &next, nil
		}
		rows, err := s.statusSummaries.LoadTaskStatusSummaries(ctx, []string{task.ID})
		if err != nil {
			return current, err
		}
		current = rows[task.ID]
		waits, err := s.loadSummaryQuotaWaits(ctx, []string{task.ID})
		if err != nil {
			return current, err
		}
		if waits == nil {
			return current, nil
		}
		wait = waits[task.ID]
	}
	if current == nil {
		return nil, nil
	}
	return current, fmt.Errorf("quota wait summary update exceeded retry limit")
}

func equalSummaryQuotaWait(left, right *statussummary.QuotaWaitSummary) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.SessionID == right.SessionID && left.Reason == right.Reason && left.Deadline.Equal(right.Deadline)
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
