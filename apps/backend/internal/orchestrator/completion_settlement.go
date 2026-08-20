package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

const completionIntentReconcileBatchSize = 32

// completionIntentReconciliationStore is deliberately narrow so the
// orchestrator can be constructed with older test repositories while the
// production repository provides durable, indexed recovery.
type completionIntentReconciliationStore interface {
	ListDueCompletionIntents(ctx context.Context, now time.Time, limit int) ([]*models.CompletionIntent, error)
	TransitionCompletionIntent(ctx context.Context, id string, from, to models.CompletionIntentState, settledAt time.Time) (bool, error)
}

// reconcileDueCompletionIntents settles only a claimed intent's captured
// active turn. A different successor turn always wins; the old intent becomes
// superseded and cannot drive the task backwards.
func (s *Service) reconcileDueCompletionIntents(ctx context.Context) {
	store, ok := s.repo.(completionIntentReconciliationStore)
	if !ok || s.turnService == nil {
		return
	}
	intents, err := store.ListDueCompletionIntents(ctx, time.Now().UTC(), completionIntentReconcileBatchSize)
	if err != nil {
		s.logger.Warn("failed to list due completion intents", zap.Error(err))
		return
	}
	for _, intent := range intents {
		s.reconcileCompletionIntent(ctx, store, intent)
	}
}

func (s *Service) reconcileCompletionIntent(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent) {
	if intent == nil {
		return
	}
	claimed, err := store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, time.Time{})
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("failed to claim completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		}
		return
	}
	now := time.Now().UTC()
	task, err := s.repo.GetTask(ctx, intent.TaskID)
	if err != nil || task == nil || task.WorkflowStepID != intent.WorkflowStepID {
		s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now)
		return
	}
	turn, err := s.turnService.GetActiveTurn(ctx, intent.SessionID)
	if err != nil && !isNoActiveTurnError(err) {
		s.logger.Warn("failed to load active turn for completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		return
	}
	if turn != nil && turn.ID != intent.TurnID {
		s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now)
		return
	}
	if turn != nil {
		if err := s.turnService.CompleteTurn(ctx, intent.TurnID); err != nil {
			s.logger.Warn("failed to settle completion intent turn", zap.String("intent_id", intent.ID), zap.Error(err))
			return
		}
		s.activeTurns.CompareAndDelete(intent.SessionID, intent.TurnID)
	}
	session, err := s.repo.GetTaskSession(ctx, intent.SessionID)
	if err != nil {
		s.logger.Warn("failed to load completion intent session", zap.String("intent_id", intent.ID), zap.Error(err))
		return
	}
	// Release coarse RUNNING ownership before workflow evaluation. The normal
	// provider-ready path does this implicitly; reconciliation has no later
	// lifecycle callback to perform that release for us.
	s.setSessionWaitingForInput(ctx, intent.TaskID, intent.SessionID, session)
	s.processOnTurnCompleteViaEngine(ctx, intent.TaskID, session)
	s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSettled, now)
}

func (s *Service) finishCompletionIntent(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent, state models.CompletionIntentState, at time.Time) {
	if _, err := store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStateSettling, state, at); err != nil {
		s.logger.Warn("failed to finish completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}
