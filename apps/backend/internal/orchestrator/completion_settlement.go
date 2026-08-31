package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/adminmetrics"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	completionIntentReconcileBatchSize = 32
	completionIntentReconcileInterval  = 30 * time.Second
	// completionIntentSettlingLease bounds how long a pending -> settling
	// claim can go unfinished before ReclaimAbandonedSettlingCompletionIntents
	// treats it as abandoned (a crashed or killed process) and returns it to
	// pending. Settlement work is a handful of DB reads/writes plus in-memory
	// checks; this is generous relative to that but still short enough to
	// recover promptly — several multiples of the 30s reconcile tick.
	completionIntentSettlingLease = 2 * time.Minute
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
	ClaimCompletionIntentForSettlement(ctx context.Context, id string, now, leaseUntil time.Time) (bool, error)
	ReleaseCompletionIntentSettlingClaim(ctx context.Context, id string, now time.Time) (bool, error)
	ReclaimAbandonedSettlingCompletionIntents(ctx context.Context, now time.Time) (int, error)
	TransitionCompletionIntentWithControlEvent(
		ctx context.Context, id string, from, to models.CompletionIntentState, settledAt time.Time,
		event *models.SessionControlEvent,
	) (bool, error)
}

type completionIntentReopenStore interface {
	GetCompletionIntentForTurn(ctx context.Context, sessionID, turnID string) (*models.CompletionIntent, error)
	TransitionCompletionIntent(ctx context.Context, id string, from, to models.CompletionIntentState, settledAt time.Time) (bool, error)
}

// AdmitQueuedUserWork serializes a user-owned queue admission with exact-turn
// settlement. A valid request reopens pending completion before its queue write
// so the reconciler cannot close the captured turn between those operations.
func (s *Service) AdmitQueuedUserWork(
	ctx context.Context,
	taskID, sessionID string,
	admit func(context.Context) (*messagequeue.QueuedMessage, error),
) (*messagequeue.QueuedMessage, error) {
	if admit == nil {
		return nil, errors.New("queued user-work admission callback is nil")
	}
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	queued, err := admit(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.reopenCompletionIntentForQueuedWorkLocked(ctx, taskID, sessionID); err != nil {
		return queued, err
	}
	return queued, nil
}

func (s *Service) reopenCompletionIntentForQueuedWorkLocked(ctx context.Context, taskID, sessionID string) error {
	if sessionID == "" || s.turnService == nil {
		return nil
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) || turn == nil {
		return s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, nil)
	}
	if err != nil {
		return fmt.Errorf("load active turn before queued work: %w", err)
	}
	store, ok := s.repo.(completionIntentReopenStore)
	if !ok {
		return s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, nil)
	}
	intent, err := store.GetCompletionIntentForTurn(ctx, sessionID, turn.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, nil)
	}
	if err != nil {
		return fmt.Errorf("load completion intent before queued work: %w", err)
	}
	if intent.TaskID != taskID {
		return fmt.Errorf("completion intent task %q does not match queued task %q", intent.TaskID, taskID)
	}
	if intent.State == models.CompletionIntentStatePending || intent.State == models.CompletionIntentStateSettling {
		reopened, err := store.TransitionCompletionIntent(ctx, intent.ID, intent.State, models.CompletionIntentStateReopened, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("reopen completion intent for queued work: %w", err)
		}
		if !reopened {
			return errors.New("completion intent changed before queued work could reopen it")
		}
	}
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, nil); err != nil {
		return fmt.Errorf("clear pending completion signal for queued work: %w", err)
	}
	return nil
}

// CaptureCompletionIntentPromptIdentity returns the runtime-owned prompt
// identity that a completion signal is allowed to settle.
func (s *Service) CaptureCompletionIntentPromptIdentity(
	ctx context.Context,
	sessionID string,
) (string, uint64, error) {
	reader, ok := s.agentManager.(interface {
		GetPromptActivityForSession(context.Context, string) (string, uint64, uint64, time.Time, error)
	})
	if !ok {
		return "", 0, errors.New("agent runtime does not expose prompt identity")
	}
	executionID, generation, _, _, err := reader.GetPromptActivityForSession(ctx, sessionID)
	if err != nil {
		return "", 0, fmt.Errorf("get active prompt identity: %w", err)
	}
	if executionID == "" || generation == 0 {
		return "", 0, errors.New("active prompt identity is incomplete")
	}
	return executionID, generation, nil
}

// reconcileDueCompletionIntents settles only a claimed intent's captured
// active turn. A different successor turn always wins; the old intent becomes
// superseded and cannot drive the task backwards.
func (s *Service) reconcileDueCompletionIntents(ctx context.Context) {
	store, ok := s.repo.(completionIntentReconciliationStore)
	if !ok || s.turnService == nil {
		return
	}
	now := time.Now().UTC()
	// Recover abandoned settling claims before scanning for due pending
	// intents, on every call — the periodic ticker AND the one-shot startup
	// scan — so a crash mid-settlement (including one during startup
	// recovery itself) cannot leave a claim invisible to every future scan.
	if reclaimed, err := store.ReclaimAbandonedSettlingCompletionIntents(ctx, now); err != nil {
		s.logger.Warn("failed to reclaim abandoned settling completion intents", zap.Error(err))
	} else if reclaimed > 0 {
		s.logger.Warn("reclaimed abandoned settling completion intents", zap.Int("count", reclaimed))
	}
	intents, err := store.ListDueCompletionIntents(ctx, now, completionIntentReconcileBatchSize)
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
	if err := s.reconcileCompletionIntentLocked(ctx, store, intent, nil); err != nil {
		s.logger.Warn("failed to reconcile completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}

// reconcileCompletionIntentLocked keeps the final active-work proof and the
// pending-to-settling claim under the same per-session guard as cancellation,
// prompt reservation, and tool/background ownership changes. Callers that
// already own that guard (manual stale settlement) use this directly.
//
// auditEvent is nil for the periodic reconciler, which has no caller to
// attribute. Manual stale-session settlement supplies a partially-populated
// event (Result left empty); whichever branch below actually reaches
// finishCompletionIntent fills in Result and commits it atomically with the
// intent's terminal transition, so a crash between "the turn settled" and
// "the audit was recorded" can never happen for that caller.
// reconcileCompletionIntentWithExecutionCheck verifies that the captured
// execution is still the current one before settling. Returns true when the
// reconciler should return early (the intent was either superseded or retried).
func (s *Service) reconcileCompletionIntentWithExecutionCheck(
	ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent,
	now time.Time, auditEvent *models.SessionControlEvent,
) (bool, error) {
	replaced, err := s.completionIntentExecutionWasReplaced(ctx, intent)
	if err != nil {
		s.logger.Warn("failed to check if completion intent execution was replaced", zap.String("intent_id", intent.ID), zap.Error(err))
		s.retryCompletionIntent(ctx, store, intent)
		return true, nil
	}
	if replaced {
		return true, s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now, "successor_execution", auditEvent)
	}
	return false, nil
}

func (s *Service) reconcileCompletionIntentLocked(
	ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent,
	auditEvent *models.SessionControlEvent,
) error {
	if intent == nil {
		return nil
	}
	now := time.Now().UTC()
	turn, proceed := s.prepareCompletionIntentReconciliation(ctx, store, intent, now)
	if !proceed {
		return nil
	}
	claimed, err := store.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, now.Add(completionIntentSettlingLease))
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("failed to claim completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
			return fmt.Errorf("claim completion intent: %w", err)
		}
		return nil
	}
	if s.completionIntentHasQueuedUserWork(ctx, intent) {
		s.clearPendingStepSignalByID(ctx, intent.SessionID)
		return s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateReopened, now, "queued_user_work", auditEvent)
	}
	if turn != nil && turn.ID != intent.TurnID {
		return s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now, "successor_turn", auditEvent)
	}
	if done, err := s.reconcileCompletionIntentWithExecutionCheck(ctx, store, intent, now, auditEvent); done {
		return err
	}
	if turn != nil {
		if err := s.turnService.CompleteTurn(ctx, intent.TurnID); err != nil {
			s.logger.Warn("failed to settle completion intent turn", zap.String("intent_id", intent.ID), zap.Error(err))
			s.retryCompletionIntent(ctx, store, intent)
			return fmt.Errorf("settle completion intent turn: %w", err)
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
		return fmt.Errorf("load completion intent task: %w", err)
	}
	if task == nil || task.WorkflowStepID != intent.WorkflowStepID {
		return s.settleMovedCompletionIntent(ctx, store, intent, now, auditEvent)
	}
	session, err := s.repo.GetTaskSession(ctx, intent.SessionID)
	if err != nil {
		s.logger.Warn("failed to load completion intent session", zap.String("intent_id", intent.ID), zap.Error(err))
		s.retryCompletionIntent(ctx, store, intent)
		return fmt.Errorf("load completion intent session: %w", err)
	}
	// Release coarse RUNNING ownership before workflow evaluation. The normal
	// provider-ready path does this implicitly; reconciliation has no later
	// lifecycle callback to perform that release for us.
	s.setSessionWaitingForInput(ctx, intent.TaskID, intent.SessionID, session)
	s.processOnTurnCompleteViaEngine(ctx, intent.TaskID, session)
	return s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSettled, now, "quiet_grace", auditEvent)
}

func (s *Service) completionIntentHasQueuedUserWork(ctx context.Context, intent *models.CompletionIntent) bool {
	if intent == nil || s.messageQueue == nil {
		return false
	}
	status := s.messageQueue.GetStatus(ctx, intent.SessionID)
	for _, entry := range status.Entries {
		if entry.QueuedBy == messagequeue.QueuedByUser && !entry.QueuedAt.Before(intent.RequestedAt) {
			return true
		}
	}
	return false
}

// completionIntentExecutionWasReplaced rejects a completion signal from an
// execution that has been superseded in the durable runtime projection. The
// captured turn might still be open while a replacement execution starts, so
// turn identity alone cannot protect the successor from an old reconciliation.
// A transient database error is propagated so the caller retries rather than
// settling an intent while unable to verify execution identity.
func (s *Service) completionIntentExecutionWasReplaced(ctx context.Context, intent *models.CompletionIntent) (bool, error) {
	if intent.AgentExecutionID == "" {
		return false, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, intent.SessionID)
	if err != nil {
		return false, fmt.Errorf("look up running execution for session %s: %w", intent.SessionID, err)
	}
	if running != nil && running.AgentExecutionID != "" && running.AgentExecutionID != intent.AgentExecutionID {
		return true, nil
	}
	if intent.PromptGeneration > 0 {
		generationReader, ok := s.agentManager.(interface {
			GetPromptGenerationForSession(context.Context, string) (uint64, error)
		})
		if !ok {
			return false, errors.New("agent runtime does not expose recovered prompt generation")
		}
		currentGeneration, err := generationReader.GetPromptGenerationForSession(ctx, intent.SessionID)
		if err != nil {
			return false, fmt.Errorf("read recovered prompt generation: %w", err)
		}
		if currentGeneration == 0 {
			return false, errors.New("recovered prompt generation is not yet available")
		}
		if currentGeneration != uint64(intent.PromptGeneration) {
			return true, nil
		}
		generationOwner, ok := s.agentManager.(interface {
			OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
		})
		// A generation-bearing completion intent is only safe to settle while
		// the runtime still assigns that exact prompt cycle to its execution.
		if !ok || !generationOwner.OwnsPromptGeneration(intent.SessionID, intent.AgentExecutionID, uint64(intent.PromptGeneration)) {
			return true, nil
		}
	}
	if running == nil || running.AgentExecutionID == "" {
		return false, nil
	}
	return false, nil
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
	auditEvent *models.SessionControlEvent,
) error {
	// The source turn is now terminal, so it must no longer leave the reused
	// session coarse-RUNNING behind the newer task step. Do not run the old
	// completion evaluation here: task.moved owns current-step entry and this
	// intent has been superseded.
	session, err := s.repo.GetTaskSession(ctx, intent.SessionID)
	if err != nil {
		s.logger.Warn("failed to load moved completion intent session", zap.String("intent_id", intent.ID), zap.Error(err))
		s.retryCompletionIntent(ctx, store, intent)
		return fmt.Errorf("load moved completion intent session: %w", err)
	}
	s.setSessionWaitingForInput(ctx, intent.TaskID, intent.SessionID, session)
	// task.moved may already have recorded the destination handoff while
	// the source turn was still RUNNING. Now that this exact old turn is
	// terminal, no provider-ready callback remains to drain it.
	// reconcileCompletionIntentLocked already owns the per-session
	// cancellation/ready guard. Re-acquiring it through the public drain helper
	// would deadlock the moved-step recovery path, leaving the stale turn and
	// destination handoff stranded forever.
	s.drainQueuedMessageForPromptableSessionLocked(ctx, intent.SessionID)
	return s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSuperseded, now, "task_moved", auditEvent)
}

// retryCompletionIntent releases a transient reconciliation claim. The next
// bounded due scan can safely retry the same exact turn; a concurrent provider
// callback still has to win the same pending-to-settling compare-and-set.
func (s *Service) retryCompletionIntent(ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent) {
	// Reset eligible_at to now, not just the state: ClaimCompletionIntentForSettlement
	// stamped it with the settling lease deadline, and without resetting it here
	// the next due scan would otherwise wait out that multi-minute lease before
	// retrying a failure this process already knows about right now.
	if _, err := store.ReleaseCompletionIntentSettlingClaim(ctx, intent.ID, time.Now().UTC()); err != nil {
		s.logger.Warn("failed to release completion intent retry claim", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}

// finishCompletionIntent performs the settling -> state terminal transition.
// When auditEvent is non-nil (manual stale-session settlement), its Result
// is completed to match the actual outcome and the transition plus the audit
// insert commit atomically in one transaction — otherwise a crash between
// "the turn settled" and "the audit was recorded separately" could leave a
// durably settled turn with no trace of who authorized it.
func (s *Service) finishCompletionIntent(
	ctx context.Context, store completionIntentReconciliationStore, intent *models.CompletionIntent,
	state models.CompletionIntentState, at time.Time, cause string, auditEvent *models.SessionControlEvent,
) error {
	var settled bool
	var err error
	if auditEvent != nil {
		auditEvent.Result = string(state)
		settled, err = store.TransitionCompletionIntentWithControlEvent(ctx, intent.ID, models.CompletionIntentStateSettling, state, at, auditEvent)
	} else {
		settled, err = store.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStateSettling, state, at)
	}
	if err != nil {
		s.logger.Warn("failed to finish completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		return fmt.Errorf("finish completion intent: %w", err)
	}
	if settled {
		adminmetrics.RecordCompletionReconciled(string(state), cause)
		s.recordPendingCompletionIntentMetric(ctx, store)
	}
	return nil
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
	now := time.Now().UTC()
	claimed, err := store.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, now.Add(completionIntentSettlingLease))
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("failed to claim provider completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
		}
		return
	}
	if err := s.finishCompletionIntent(ctx, store, intent, models.CompletionIntentStateSettled, now, "provider_terminal", nil); err != nil {
		s.logger.Warn("failed to settle provider completion intent", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}

func (s *Service) recordPendingCompletionIntentMetric(ctx context.Context, store completionIntentReconciliationStore) {
	count, err := store.CountPendingCompletionIntents(ctx)
	if err != nil {
		s.logger.Warn("failed to count pending completion intents", zap.Error(err))
		return
	}
	adminmetrics.RecordCompletionPending(count)
}

// completionIntentRearmThrottleInterval bounds how often
// rearmCompletionIntentForActivity does real database work per session.
// message_streaming/thinking_streaming fire once per token chunk, and each
// call previously ran a GetActiveTurn read, a GetCompletionIntentForTurn
// read, and a RearmCompletionIntent write while holding the per-session
// cancelInFlight guard — three round trips per chunk on the hottest path,
// serialized against every other side effect for the session. A fraction of
// the quiet grace is a small enough window that a real gap in activity still
// reads as stale to the reconciler.
const completionIntentRearmThrottleInterval = models.CompletionIntentQuietGrace / 5

// rearmCompletionIntentForActivity moves a pending intent's quiet window
// forward whenever its captured turn emits more foreground or tool activity.
// It is deliberately best-effort: a failed activity touch must never block
// the stream event that supplied the stronger evidence.
func (s *Service) rearmCompletionIntentForActivity(ctx context.Context, sessionID string) {
	if sessionID == "" || s.turnService == nil {
		return
	}
	now := time.Now().UTC()
	if last, ok := s.completionIntentRearmThrottle.Load(sessionID); ok {
		if now.Sub(last.(time.Time)) < completionIntentRearmThrottleInterval {
			return
		}
	}
	// Record the attempt before doing any DB work so every early-return path
	// below (no store support, no active turn, no pending intent) is still
	// throttled — not just the path that reaches RearmCompletionIntent.
	s.completionIntentRearmThrottle.Store(sessionID, now)
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
	if _, err := store.RearmCompletionIntent(ctx, intent.ID, now, now.Add(models.CompletionIntentQuietGrace)); err != nil {
		s.logger.Warn("failed to rearm completion intent after activity", zap.String("intent_id", intent.ID), zap.Error(err))
	}
}
