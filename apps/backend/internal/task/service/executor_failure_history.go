package service

import (
	"context"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) publishExecutorFailureHistory(ctx context.Context, taskID string) {
	store, ok := s.tasks.(executorFailureSettlementRepository)
	if !ok {
		return
	}
	refs, err := store.ListExecutorFailureAffectedSessions(ctx, taskID)
	if err != nil {
		return
	}
	for _, ref := range refs {
		id := models.ExecutorFailureMessageID(ref.EpisodeID, ref.Candidate.SessionID, ref.Candidate.ExpectedExecutorAgentExecutionID)
		message, err := s.GetMessage(ctx, id)
		if err != nil || message == nil {
			continue
		}
		turn, err := s.GetTurn(ctx, message.TurnID)
		if err != nil || turn == nil {
			continue
		}
		if lifecycleOnly, _ := turn.Metadata["lifecycle_only"].(bool); lifecycleOnly {
			_ = s.PublishTurnStarted(ctx, turn)
		}
		_ = s.PublishMessageEvent(ctx, events.MessageAdded, message)
	}
}
