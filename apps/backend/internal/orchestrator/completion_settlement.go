package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/adminmetrics"
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
	CountPendingCompletionIntents(ctx context.Context) (int, error)
	GetCompletionIntentForTurn(ctx context.Context, sessionID, turnID string) (*models.CompletionIntent, error)
	RearmCompletionIntent(ctx context.Context, id string, activityAt, eligibleAt time.Time) (bool, error)
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
	s.recordPendingCompletionIntentMetric(ctx, store)
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
	lock, release := s.acquireCancelInFlightGuard(intent.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	s.reconcileCompletionIntentLocked(ctx, store, intent)
}

// reconcileCompletionIntentLocked keeps the final active-work proof and the
// pending-to-settling claim under the same per-session guard as cancellation,
// prompt reservation, and tool/background ownership changes. Callers that
// already own that guard (manual stale settlement) use this directly.
func (s *Service) reconcileCompletionIntentLocked(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent) {
	if intent == nil {
		return
	}
	now := time.Now().UTC()
	turn, proceed := s.prepareCompletionIntentReconciliation(ctx, store, intent, now)
	if !proceed {
		return
	}
	claimed, err := store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, time.Time{})
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("failed to claim completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		}
		return
	}
	if turn != nil && turn.ID != intent.TurnID {
		s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now, "successor_turn")
		return
	}
	if turn != nil {
		if err := s.turnService.CompleteTurn(ctx, intent.TurnID); err != nil {
			s.logger.Warn("failed to settle completion intent turn", zap.String("intent_id", intent.ID), zap.Error(err))
			s.retryCompletionIntent(ctx, store, intent)
			return
		}
		s.activeTurns.CompareAndDelete(intent.SessionID, intent.TurnID)
	}
	// A later workflow move must never replay this old completion, but it does
	// not own the captured stale turn. Release that exact ownership first, then
	// mark the old intent superseded without evaluating its source step.
	task, err := s.repo.GetTask(ctx, intent.TaskID)
	if err != nil {
		s.logger.Warn("failed to load completion intent task", zap.String("intent_id", intent.ID), zap.Error(err))
		s.retryCompletionIntent(ctx, store, intent)
		return
	}
	if task == nil || task.WorkflowStepID != intent.WorkflowStepID {
		s.settleMovedCompletionIntent(ctx, store, intent, now)
		return
	}
	session, err := s.repo.GetTaskSession(ctx, intent.SessionID)
	if err != nil {
		s.logger.Warn("failed to load completion intent session", zap.String("intent_id", intent.ID), zap.Error(err))
		s.retryCompletionIntent(ctx, store, intent)
		return
	}
	// Release coarse RUNNING ownership before workflow evaluation. The normal
	// provider-ready path does this implicitly; reconciliation has no later
	// lifecycle callback to perform that release for us.
	s.setSessionWaitingForInput(ctx, intent.TaskID, intent.SessionID, session)
	s.processOnTurnCompleteViaEngine(ctx, intent.TaskID, session)
	s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSettled, now, "quiet_grace")
}

func (s *Service) prepareCompletionIntentReconciliation(
	ctx context.Context,
	store completionIntentReconciliationStore,
	intent *models.CompletionIntent,
	now time.Time,
) (*models.Turn, bool) {
	turn, err := s.turnService.GetActiveTurn(ctx, intent.SessionID)
	if err != nil && !isNoActiveTurnError(err) {
		s.logger.Warn("failed to load active turn for completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		return nil, false
	}
	evidence := s.completionIntentActiveWorkEvidence(ctx, intent, turn)
	if evidence == "" {
		return turn, true
	}
	if _, err := store.RearmCompletionIntent(ctx, intent.ID, now, now.Add(models.CompletionIntentQuietGrace)); err != nil {
		s.logger.Warn("failed to rearm completion intent behind active work", zap.String("intent_id", intent.ID), zap.String("evidence", evidence), zap.Error(err))
	}
	return nil, false
}

// completionIntentActiveWorkEvidence deliberately reuses the manual stale
// settlement barriers before an automatic reconciler claims a pending intent.
// A quiet timestamp alone is not authority to interrupt a cancellation,
// reserved successor prompt, unfinished tool, or adapter-attested background
// workload. Persisted background attestation closes the restart gap where the
// in-memory tracker has not yet received a fresh provider frame.
func (s *Service) completionIntentActiveWorkEvidence(ctx context.Context, intent *models.CompletionIntent, turn *models.Turn) string {
	if s.isCancelInFlight(intent.SessionID) {
		return "cancellation_in_flight"
	}
	if turn != nil && turn.ID == intent.TurnID && staleSettlementHasPromptReservation(turn.Metadata) {
		return "prompt_reservation"
	}
	if s.hasOutstandingBackgroundWork(intent.SessionID) {
		return "background_work"
	}
	session, err := s.repo.GetTaskSession(ctx, intent.SessionID)
	if err != nil {
		return "session_check_failed"
	}
	if session != nil {
		attested, _ := session.Metadata[models.SessionMetaKeyBackgroundWorkAttested].(bool)
		if attested {
			return "background_work_attested"
		}
	}
	tools, ok := s.repo.(pendingTurnToolCallStore)
	if !ok {
		return "pending_tool_check_unavailable"
	}
	pending, err := tools.HasPendingToolCallsForTurn(ctx, intent.TurnID)
	if err != nil {
		return "pending_tool_check_failed"
	}
	if pending {
		return "pending_tool_call"
	}
	return ""
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
		s.retryCompletionIntent(ctx, store, intent)
		return
	}
	s.setSessionWaitingForInput(ctx, intent.TaskID, intent.SessionID, session)
	// task.moved may already have recorded the destination handoff while
	// the source turn was still RUNNING. Now that this exact old turn is
	// terminal, no provider-ready callback remains to drain it.
	s.drainQueuedMessageForPromptableSession(ctx, intent.SessionID)
	s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now, "task_moved")
}

// retryCompletionIntent releases a transient reconciliation claim. The next
// bounded due scan can safely retry the same exact turn; a concurrent provider
// callback still has to win the same pending-to-settling compare-and-set.
func (s *Service) retryCompletionIntent(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent) {
	if _, err := store.TransitionCompletionIntent(
		ctx,
		intent.ID,
		models.CompletionIntentStateSettling,
		models.CompletionIntentStatePending,
		time.Time{},
	); err != nil {
		s.logger.Warn("failed to release completion intent retry claim", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}

func (s *Service) finishCompletionIntent(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent, state models.CompletionIntentState, at time.Time, cause string) {
	settled, err := store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStateSettling, state, at)
	if err != nil {
		s.logger.Warn("failed to finish completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		return
	}
	if settled {
		adminmetrics.RecordCompletionReconciled(string(state), cause)
		s.recordPendingCompletionIntentMetric(ctx, store)
	}
}

// settleCompletionIntentForProviderTurn records the normal provider-ready
// settlement path. The periodic worker remains responsible for a missing
// lifecycle event, but an observed event must not later make a successfully
// completed intent look superseded merely because the task has advanced.
func (s *Service) settleCompletionIntentForProviderTurn(ctx context.Context, sessionID, turnID string) {
	if sessionID == "" || turnID == "" {
		return
	}
	store, ok := s.repo.(completionIntentReconciliationStore)
	if !ok {
		return
	}
	intent, err := store.GetCompletionIntentForTurn(ctx, sessionID, turnID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			s.logger.Warn("failed to load completion intent for provider settlement", zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
		}
		return
	}
	claimed, err := store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, time.Time{})
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("failed to claim provider completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		}
		return
	}
	s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSettled, time.Now().UTC(), "provider_terminal")
}

func (s *Service) recordPendingCompletionIntentMetric(ctx context.Context, store completionIntentReconciliationStore) {
	count, err := store.CountPendingCompletionIntents(ctx)
	if err != nil {
		s.logger.Warn("failed to count pending completion intents", zap.Error(err))
		return
	}
	adminmetrics.RecordCompletionPending(count)
}

// rearmCompletionIntentForActivity moves a pending intent's quiet window
// forward whenever its captured turn emits more foreground or tool activity.
// It is deliberately best-effort: a failed activity touch must never block
// the stream event that supplied the stronger evidence.
func (s *Service) rearmCompletionIntentForActivity(ctx context.Context, sessionID string) {
	if sessionID == "" || s.turnService == nil {
		return
	}
	store, ok := s.repo.(completionIntentReconciliationStore)
	if !ok {
		return
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil || turn == nil {
		return
	}
	intent, err := store.GetCompletionIntentForTurn(ctx, sessionID, turn.ID)
	if err != nil || intent.State != models.CompletionIntentStatePending {
		return
	}
	now := time.Now().UTC()
	if _, err := store.RearmCompletionIntent(ctx, intent.ID, now, now.Add(models.CompletionIntentQuietGrace)); err != nil {
		s.logger.Warn("failed to rearm completion intent after activity", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}
