package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

var errUsageEventReaderUnavailable = errors.New("usage event detail is unavailable")

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

func (s *Service) ListTaskSessionUsageTurns(ctx context.Context, taskID, sessionID string, afterID int64, limit int) ([]models.TaskUsageTurnEvents, int64, error) {
	if err := s.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return nil, afterID, err
	}
	reader, ok := s.usage.(repository.UsageEventReader)
	if !ok {
		return nil, afterID, errUsageEventReaderUnavailable
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	turns, err := reader.ListSessionUsageTurnCursors(ctx, sessionID, afterID, limit)
	if err != nil {
		return nil, afterID, err
	}
	result := make([]models.TaskUsageTurnEvents, 0, len(turns))
	nextCursor := afterID
	for _, turn := range turns {
		events, err := reader.ListSessionUsageEventsByTurn(ctx, sessionID, turn.TurnID)
		if err != nil {
			return nil, afterID, err
		}
		result = append(result, models.TaskUsageTurnEvents{TurnID: turn.TurnID, Cursor: turn.Cursor, Events: events})
		nextCursor = turn.Cursor
	}
	return result, nextCursor, nil
}

func (s *Service) GetTaskSessionUsageTurn(ctx context.Context, taskID, sessionID, turnID string) ([]*models.TaskUsageEvent, error) {
	if err := s.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	reader, ok := s.usage.(repository.UsageEventReader)
	if !ok {
		return nil, errUsageEventReaderUnavailable
	}
	return reader.ListSessionUsageEventsByTurn(ctx, sessionID, turnID)
}
