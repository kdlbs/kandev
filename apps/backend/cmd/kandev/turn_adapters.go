package main

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// turnServiceAdapter adapts the task service to the orchestrator.TurnService interface.
type turnServiceAdapter struct {
	svc *taskservice.Service
}

func (a *turnServiceAdapter) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.StartTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) CompleteTurn(ctx context.Context, turnID string) error {
	return a.svc.CompleteTurn(ctx, turnID)
}

func (a *turnServiceAdapter) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.GetActiveTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) AbandonOpenTurns(ctx context.Context, sessionID string) error {
	return a.svc.AbandonOpenTurns(ctx, sessionID)
}

func newTurnServiceAdapter(svc *taskservice.Service) *turnServiceAdapter {
	return &turnServiceAdapter{svc: svc}
}

// taskSessionCheckerAdapter adapts the task repository for github.TaskSessionChecker.
type taskSessionCheckerAdapter struct {
	repo interface {
		ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	}
}

func (a *taskSessionCheckerAdapter) HasTaskSessions(ctx context.Context, taskID string) (bool, error) {
	sessions, err := a.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	return len(sessions) > 0, nil
}
