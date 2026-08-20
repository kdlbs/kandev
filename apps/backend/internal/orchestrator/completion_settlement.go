package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	completionIntentReconcileBatchSize = 32
	completionIntentReconcileInterval  = 30 * time.Second
)

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

// resetCompletionIntentReconciler creates the owner context used by the
// periodic recovery worker. It is called at startup only after a previous
// owner has stopped, so no old worker can observe the new context.
func (s *Service) resetCompletionIntentReconciler() {
	s.completionIntentReconcileMu.Lock()
	defer s.completionIntentReconcileMu.Unlock()
	s.completionIntentReconcileStopped = false
	s.completionIntentReconcileStarted = false
	s.completionIntentReconcileCtx, s.completionIntentReconcileCancel = context.WithCancel(context.Background())
}

// startCompletionIntentReconciler keeps bounded due scans independent from a
// provider terminal event. Claiming remains compare-and-set in the repository,
// so duplicate ticks and instances cannot settle the same intent twice.
func (s *Service) startCompletionIntentReconciler() {
	s.completionIntentReconcileMu.Lock()
	if s.completionIntentReconcileStopped || s.completionIntentReconcileStarted {
		s.completionIntentReconcileMu.Unlock()
		return
	}
	if s.completionIntentReconcileCtx == nil {
		s.completionIntentReconcileCtx, s.completionIntentReconcileCancel = context.WithCancel(context.Background())
	}
	workerCtx := s.completionIntentReconcileCtx
	interval := s.completionIntentReconcileInterval
	if interval <= 0 {
		interval = completionIntentReconcileInterval
	}
	s.completionIntentReconcileStarted = true
	s.completionIntentReconcileWorkers.Add(1)
	s.completionIntentReconcileMu.Unlock()

	go func() {
		defer s.completionIntentReconcileWorkers.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				s.reconcileDueCompletionIntents(workerCtx)
			}
		}
	}()
}

// stopCompletionIntentReconciler prevents shutdown from leaving a background
// settlement path that could race a successor turn after the service stops.
func (s *Service) stopCompletionIntentReconciler() {
	s.completionIntentReconcileMu.Lock()
	s.completionIntentReconcileStopped = true
	cancel := s.completionIntentReconcileCancel
	s.completionIntentReconcileMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.completionIntentReconcileWorkers.Wait()
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
	// A later workflow move must never replay this old completion, but it does
	// not own the captured stale turn. Release that exact ownership first, then
	// mark the old intent superseded without evaluating its source step.
	task, err := s.repo.GetTask(ctx, intent.TaskID)
	if err != nil || task == nil || task.WorkflowStepID != intent.WorkflowStepID {
		s.settleMovedCompletionIntent(ctx, store, intent, now)
		return
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

func (s *Service) settleMovedCompletionIntent(
	ctx context.Context,
	store completionIntentReconciliationStore,
	intent *models.CompletionIntent,
	now time.Time,
) {
	// The source turn is now terminal, so it must no longer leave the reused
	// session coarse-RUNNING behind the newer task step. Do not run the old
	// completion evaluation here: task.moved owns current-step entry and this
	// intent has been superseded.
	session, err := s.repo.GetTaskSession(ctx, intent.SessionID)
	if err != nil {
		s.logger.Warn("failed to load moved completion intent session", zap.String("intent_id", intent.ID), zap.Error(err))
	} else {
		s.setSessionWaitingForInput(ctx, intent.TaskID, intent.SessionID, session)
		// task.moved may already have recorded the destination handoff while
		// the source turn was still RUNNING. Now that this exact old turn is
		// terminal, no provider-ready callback remains to drain it.
		s.drainQueuedMessageForPromptableSession(ctx, intent.SessionID)
	}
	s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now)
}

func (s *Service) finishCompletionIntent(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent, state models.CompletionIntentState, at time.Time) {
	if _, err := store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStateSettling, state, at); err != nil {
		s.logger.Warn("failed to finish completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}
