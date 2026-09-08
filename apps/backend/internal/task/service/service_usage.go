package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// GetTaskUsageTotals returns the task-cost-ledger aggregate for taskID
// (docs/specs/task-cost-ledger/spec.md AC-18, AC-19), including rows whose
// session_id was cleared by session deletion. authorizeTaskID alone does not
// prove the task exists - it is a no-op for unscoped (internal/synthetic)
// callers - so an explicit GetTask call supplies the 404 for an unknown task
// in every caller scope.
func (s *Service) GetTaskUsageTotals(ctx context.Context, taskID string) (*models.TaskUsageTotals, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	if _, err := s.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.usage.GetTaskUsageTotals(ctx, taskID)
}

// GetTaskSessionUsageTotals returns the task-cost-ledger aggregate scoped to
// sessionID (AC-18, AC-19). AuthorizeTaskSessionAccess always fetches the
// session and checks that it belongs to taskID regardless of caller scope,
// so it alone supplies both the unknown-session and mismatched-pair 404s.
func (s *Service) GetTaskSessionUsageTotals(ctx context.Context, taskID, sessionID string) (*models.TaskUsageTotals, error) {
	if err := s.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	return s.usage.GetSessionUsageTotals(ctx, sessionID)
}
