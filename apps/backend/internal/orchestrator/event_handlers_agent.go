package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// handleAgentRunning handles agent running events (user sent input in passthrough mode)
// This is called when the user sends input to the agent, indicating a new turn started.
func (s *Service) handleAgentRunning(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent running event",
			zap.String("task_id", data.TaskID))
		return
	}

	// agent.running fires whenever the agent process starts running — including
	// the boot of a silent resume after a backend restart (session/new fallback
	// for agents without native resume, or a session/load reconnect), where no
	// turn is actually in flight. ACP sessions drive RUNNING from the
	// prompt-dispatch path (PromptTask / dispatchPromptAsync) and stream
	// tool/message events, so reacting to the boot signal here would only flicker
	// a settled WAITING_FOR_INPUT task into the Running bucket during resume.
	// Passthrough sessions have no PromptTask, so agent.running IS their
	// turn-start signal: handle on_turn_start and move the session to RUNNING.
	if !s.agentManager.IsPassthroughSession(ctx, data.SessionID) {
		return
	}
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent running",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	if isTerminalSessionState(session.State) {
		s.logger.Debug("ignoring agent running event for terminal session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return
	}
	s.processOnTurnStartViaEngine(ctx, data.TaskID, session)

	// Move session to running and task to in progress.
	s.setSessionRunning(ctx, data.TaskID, data.SessionID, session)
}

// publishQueueStatusEvent publishes a queue status changed event for the given session
func (s *Service) publishQueueStatusEvent(ctx context.Context, sessionID string) {
	if s.eventBus == nil {
		return
	}

	queueStatus := s.messageQueue.GetStatus(ctx, sessionID)
	eventData := map[string]interface{}{
		"session_id": sessionID,
		"entries":    queueStatus.Entries,
		"count":      queueStatus.Count,
		"max":        queueStatus.Max,
	}

	s.logger.Debug("publishing queue status changed event",
		zap.String("session_id", sessionID),
		zap.Int("count", queueStatus.Count))

	_ = s.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"orchestrator",
		eventData,
	))
}

// requeueMessage re-enqueues a message that could not be delivered, publishing a queue status event on success.
// Preserves the original Metadata (e.g. sender_task_id from message_task_kandev)
// so attribution survives transient failures + retries.
func (s *Service) requeueMessage(ctx context.Context, queuedMsg *messagequeue.QueuedMessage, queuedBy string) {
	coalesceKey := messageCoalesceKey(queuedMsg)
	if queuedMsg.QueuedBy != "" && coalesceKey != "" {
		queuedBy = queuedMsg.QueuedBy
	}
	var (
		requeuedMsg *messagequeue.QueuedMessage
		replaced    bool
		queueErr    error
	)
	if coalesceKey != "" {
		requeuedMsg, replaced, queueErr = s.messageQueue.QueueMessageWithCoalesceKey(
			ctx,
			queuedMsg.SessionID,
			queuedMsg.TaskID,
			queuedMsg.Content,
			queuedMsg.Model,
			queuedBy,
			queuedMsg.PlanMode,
			queuedMsg.Attachments,
			queuedMsg.Metadata,
			coalesceKey,
			true,
		)
	} else {
		requeuedMsg, queueErr = s.messageQueue.QueueMessageWithMetadata(
			ctx,
			queuedMsg.SessionID,
			queuedMsg.TaskID,
			queuedMsg.Content,
			queuedMsg.Model,
			queuedBy,
			queuedMsg.PlanMode,
			queuedMsg.Attachments,
			queuedMsg.Metadata,
		)
	}
	if queueErr != nil {
		s.logger.Error("failed to requeue message",
			zap.String("session_id", queuedMsg.SessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID),
			zap.String("queued_by", queuedBy),
			zap.Error(queueErr))
		return
	}
	s.logger.Info("message requeued",
		zap.String("session_id", queuedMsg.SessionID),
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("old_queue_id", queuedMsg.ID),
		zap.String("new_queue_id", requeuedMsg.ID),
		zap.String("queued_by", queuedBy),
		zap.String("coalesce_key", coalesceKey),
		zap.Bool("replaced", replaced))
	s.publishQueueStatusEvent(ctx, queuedMsg.SessionID)
}

func messageCoalesceKey(queuedMsg *messagequeue.QueuedMessage) string {
	if queuedMsg == nil || len(queuedMsg.Metadata) == 0 {
		return ""
	}
	value, ok := queuedMsg.Metadata[messagequeue.MetadataCoalesceKey]
	if !ok {
		return ""
	}
	key, ok := value.(string)
	if !ok {
		return ""
	}
	return key
}

// handleAgentBootReady handles the boot signal: an agent's ACP session has
// finished initializing but no turn has run yet. This event is distinct from
// agent.ready (turn-end) so the orchestrator never has to disambiguate the
// two with race-prone flags.
//
// Two jobs here: (1) flip the session to WAITING_FOR_INPUT so callers
// that are gating on that state (e.g. PromptTask's waitForSessionReady after
// ensureSessionRunning kicked off ResumeSession) can proceed; (2) drain any
// orphaned queued message. Without the drain, a workflow auto-start prompt
// queued against a session that died mid-turn (or before its first prompt)
// would sit forever after the user resumed it — agent.ready (the usual drain
// trigger) never fires for a turn that never completed. Crucially we do
// NOT call processOnTurnCompleteViaEngine — there's no turn to complete, and
// stepping the workflow off a boot signal is what caused the production
// ping-pong bug.
func (s *Service) handleAgentBootReady(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent boot ready event",
			zap.String("task_id", data.TaskID))
		return
	}

	if s.isSessionResetInProgress(data.SessionID) {
		s.logger.Debug("ignoring agent.boot_ready while session reset is in progress",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return
	}
	if s.isCancelInFlight(data.SessionID) {
		s.logger.Debug("ignoring agent.boot_ready while cancel is in progress",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent.boot_ready",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	// Pre-refactor this branch dropped events from "non-active" executions by
	// comparing data.AgentExecutionID with session.AgentExecutionID. With the
	// in-memory ExecutionStore now the single source of truth (and persisted
	// in lockstep with executors_running), a live event arriving here means
	// the lifecycle manager already considers data.AgentExecutionID active
	// for this session — there's no "old execution" to drop. The check is gone.
	// If the in-memory store has been torn down, the event simply has nowhere
	// to land and the downstream session-state guard handles it.

	// Terminal sessions never need a boot signal — if a stale init event
	// arrives after the session was completed/cancelled, just drop it.
	if isTerminalSessionState(session.State) {
		s.logger.Debug("ignoring agent.boot_ready for terminal session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return
	}

	// Idempotent: if the session is already WAITING_FOR_INPUT (e.g. revived
	// from a previously launched session and the boot signal arrived faster
	// than persistResumeState wrote STARTING), skip the flip — but still
	// fall through to the drain below: an orphaned queued message would
	// otherwise sit forever.
	if session.State == models.TaskSessionStateWaitingForInput {
		s.logger.Debug("agent.boot_ready: session already WAITING_FOR_INPUT, skipping flip",
			zap.String("session_id", data.SessionID))
	} else {
		s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID, session)
	}

	// Drain any orphaned queued message. handleAgentReady drains on turn-end,
	// but a session that crashed mid-turn (or never started its first turn)
	// won't fire agent.ready — leaving e.g. workflow auto-start prompts stuck
	// on the queue until the user manually sends another message. After the
	// agent has booted and the session is back to WAITING_FOR_INPUT it's safe
	// to dispatch any pending message.
	// Claim the shared per-session lock first (see handleAgentReady's
	// analogous claim for the full race this closes): a concurrent
	// QueueAndInterruptForPeerMessage racing to deliver its own just-queued
	// message must never have this drain steal it before the interrupt's
	// own cancel+take runs. drainQueuedMessageForPromptableSession now owns
	// that acquisition itself (blocking, plus its own promptability
	// reload) — see its doc comment.
	s.drainQueuedMessageForPromptableSession(ctx, data.SessionID)
}

// handleAgentReady handles turn-end ready events: the agent finished processing
// a prompt and is waiting for the next input. This is the *only* event that
// should evaluate workflow on_turn_complete actions — boot signals route
// through handleAgentBootReady instead.
//
// Acquires the per-session cancelInFlight guard *before* any
// turn-completion/pending-move/on_turn_complete bookkeeping runs, not just
// before the final queue-take decision (as it did before). Without this, a
// ready event could pass the early checks, then a concurrent parent
// interrupt (QueueAndInterruptForPeerMessage) could acquire the guard and
// cancel-and-redispatch on this same session — starting a *new* turn — all
// before this event reaches its own completeTurnForSession /
// processOnTurnCompleteViaEngine, which would then wrongly complete and
// evaluate on_turn_complete against that new turn instead of the one this
// event actually reports the completion of, or apply a pending move while
// the interrupt is still targeting the "old" session.
//
// The acquisition is a genuine blocking Lock (mirroring
// QueueAndInterruptForPeerMessage's own precedent), not the previous
// TryLock-and-skip: skipping here would leave this event's own
// turn-completion/workflow bookkeeping undone forever — nothing else
// performs it — and cancelAndTakeForPeerMessage's own "cancel failed but
// already promptable" recovery explicitly relies on a *future* agent.ready
// completing that bookkeeping once the guard frees up; this event may be
// exactly that future ready event. Blocking here adds no new deadlock
// risk: every existing guard holder already bounds its own hold time (see
// QueueAndInterruptForPeerMessage's doc comment), and acquiring an
// uncontended mutex via Lock costs the same as via TryLock, so the common
// (non-racing) case is unaffected.
//
// Once the guard is held, the session and its active turn are re-validated
// against the snapshot taken before waiting (session state, plus turn
// identity via peekActiveTurnID when a TurnService is wired): if either
// changed, a concurrent interrupt (or another turn entirely) has already
// superseded this event, so it backs off without touching anything —
// whatever superseded it owns that turn's own eventual completion.
func (s *Service) handleAgentReady(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent ready event",
			zap.String("task_id", data.TaskID))
		return
	}

	if s.isSessionResetInProgress(data.SessionID) {
		s.logger.Debug("ignoring agent.ready while session reset is in progress",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent.ready",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	// See comment in handleAgentBootReady: the stale-execution drop is gone; the
	// in-memory ExecutionStore is the source of truth and a live event implies
	// the emitting execution is the active one for this session.

	if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
		s.logger.Debug("ignoring agent.ready while session is not running or starting",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return
	}

	// Snapshot which turn this event reports the completion of *before*
	// contending for the guard — re-checked below once it's held. See the
	// function doc comment for the race this closes.
	turnAtEventFire, turnSnapshotErr := s.peekActiveTurnID(ctx, data.SessionID)

	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	// Re-validate now that the guard is held: a concurrent interrupt (or
	// clarification recovery, or another drain) may have already resolved
	// this exact session while this event waited. isSessionResetInProgress
	// and the session state are re-checked in case either changed in that
	// window; the turn-identity comparison additionally catches the
	// specific case a plain state re-check can't — a *different* turn
	// (e.g. one the interrupt cancelled-and-redispatched) that also
	// happens to be RUNNING/STARTING.
	if s.isSessionResetInProgress(data.SessionID) {
		s.logger.Debug("stale agent.ready: session reset started while waiting for the guard",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return
	}
	session, err = s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to reload session for agent.ready once the guard was held",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
		s.logger.Debug("stale agent.ready: session no longer running/starting once the guard was held",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return
	}
	if data.PromptGeneration != 0 {
		generationOwner, ok := s.agentManager.(interface {
			OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
		})
		// Generation-bearing events fail closed: without an ownership validator,
		// the handler cannot prove that the event still belongs to this turn.
		if !ok || !generationOwner.OwnsPromptGeneration(
			data.SessionID, data.AgentExecutionID, data.PromptGeneration,
		) {
			s.logger.Debug("stale agent.ready: prompt generation no longer owns the session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.String("agent_execution_id", data.AgentExecutionID),
				zap.Uint64("event_prompt_generation", data.PromptGeneration))
			return
		}
	}
	if s.turnService != nil {
		if turnSnapshotErr != nil {
			s.logger.Warn("could not confirm this event's active turn before waiting for the guard; treating as stale rather than risk completing a possible successor turn",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.Error(turnSnapshotErr))
			return
		}
		turnNow, turnNowErr := s.peekActiveTurnID(ctx, data.SessionID)
		if turnNowErr != nil || turnNow != turnAtEventFire {
			s.logger.Debug("stale agent.ready: active turn changed (or could not be reconfirmed) while waiting for the guard",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.Error(turnNowErr))
			return
		}
	}

	// A turn completed successfully — clear any transient retry budget so a
	// later, unrelated provider overload starts its backoff fresh at attempt 1.
	s.resetTransientRetry(data.SessionID)

	// Complete the current turn
	s.completeTurnForSession(ctx, data.SessionID)

	// A move_task_kandev call during this turn deferred the actual move to
	// avoid racing on_enter against the running turn. Apply it now: the move
	// is the explicit transition the agent requested, so skip the regular
	// on_turn_complete evaluation against the (still old) step.
	if pendingMove, exists := s.messageQueue.TakePendingMove(ctx, data.SessionID); exists {
		s.applyPendingMove(ctx, data.TaskID, data.SessionID, session, pendingMove)
		return
	}

	// Explicit agent-requested moves (move_task_kandev) take precedence over pending clarifications.
	if s.sessionHasPendingClarification(ctx, data.SessionID) {
		s.logger.Info("deferring on_turn_complete while clarification is pending",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID, session)
		return
	}

	// Check for workflow transition based on session's current step.
	// Uses the engine when available; falls back to legacy evaluation.
	// The ViaEngine method handles setSessionWaitingForInput internally when no transition occurs.
	transitioned := s.processOnTurnCompleteViaEngine(ctx, data.TaskID, session)

	// When a workflow transition occurred (e.g. Work → Review), the new step's
	// on_enter actions handle the next prompt (auto_start_agent launches a goroutine).
	// Skip the queued-message check to avoid racing with that auto-start goroutine —
	// both would try to call PromptTask and the loser's queued message would be lost.
	if transitioned {
		s.logger.Debug("workflow transition occurred, skipping queued message check",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return
	}

	// Passthrough sessions: deliver queued messages via PTY stdin instead of ACP.
	if s.agentManager.IsPassthroughSession(ctx, data.SessionID) {
		queuedMsg, exists := s.messageQueue.TakeQueued(ctx, data.SessionID)
		if !exists {
			return
		}
		if queuedMsg.Content != "" {
			if err := s.deliverPassthroughPrompt(ctx, data.SessionID, queuedMsg.Content); err != nil {
				s.logger.Warn("failed to deliver queued message to passthrough",
					zap.String("session_id", data.SessionID),
					zap.Error(err))
			}
		}
		return
	}

	// Check for queued messages when no workflow transition occurred. Uses
	// the Locked variant directly: the guard above is held for this
	// entire function now, not just this final step.
	s.drainQueuedMessageForPromptableSessionLocked(ctx, data.SessionID)
}

func (s *Service) executeQueuedMessage(callerSessionID string, queuedMsg *messagequeue.QueuedMessage) {
	promptCtx := context.Background() // Use a fresh context for async execution

	// Safety net: guarantee the in-flight marker (set by
	// dispatchTakenQueuedMessage before spawning this goroutine — see the
	// Service.dispatchingQueued field doc comment) is cleared on every exit
	// path, including the early reset-in-progress return below, PROVIDED
	// this goroutine still owns it (compare-and-delete keyed on this
	// entry's own ID) — the primary claim-and-clear happens deterministically
	// inside promptTask's guarded claim step, right before setSessionRunning
	// further down; this defer only catches paths that return before
	// reaching it, or a losing claim that must not touch a newer dispatch's
	// marker.
	defer s.clearQueuedDispatchInFlightIfCurrent(queuedMsg.SessionID, queuedMsg.ID)

	if s.isSessionResetInProgress(queuedMsg.SessionID) {
		s.logger.Warn("queued message execution deferred due to context reset in progress",
			zap.String("session_id", callerSessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID))
		s.requeueMessage(promptCtx, queuedMsg, "workflow-auto-start-reset-retry")
		return
	}

	attachments := make([]v1.MessageAttachment, len(queuedMsg.Attachments))
	for i, att := range queuedMsg.Attachments {
		attachments[i] = v1.MessageAttachment{
			Type:         att.Type,
			Data:         att.Data,
			MimeType:     att.MimeType,
			Name:         att.Name,
			DeliveryMode: att.DeliveryMode,
		}
	}

	// Create user message for the queued message (so it appears in chat history).
	// Skip when the queued metadata is tagged user_message_recorded — that means
	// autoStartStepPrompt already inserted the chat row via recordAutoStartMessage
	// before queueing (the post-recordAutoStartMessage retry branches). Recording
	// here would produce the duplicate user message observed when a workflow
	// auto-start failed transiently and the queue drained on boot_ready.
	alreadyRecorded, _ := queuedMsg.Metadata[metaKeyUserMessageRecorded].(bool)
	if s.messageCreator != nil && !alreadyRecorded {
		turnID := s.getActiveTurnID(queuedMsg.SessionID)
		if turnID == "" {
			// Start a new turn if needed
			s.startTurnForSession(promptCtx, queuedMsg.SessionID)
			turnID = s.getActiveTurnID(queuedMsg.SessionID)
		}

		meta := NewUserMessageMeta().
			WithPlanMode(queuedMsg.PlanMode).
			WithAttachments(attachments)
		// Merge any extra metadata captured at queue time (e.g. sender_task_id
		// from message_task_kandev) so the resulting Message row carries the
		// full context.
		metaMap := mergeMetadata(meta.ToMap(), queuedMsg.Metadata)
		err := s.messageCreator.CreateUserMessage(promptCtx, queuedMsg.TaskID, queuedMsg.Content, queuedMsg.SessionID, turnID, metaMap)
		if err != nil {
			s.logger.Error("failed to create user message for queued message",
				zap.String("session_id", queuedMsg.SessionID),
				zap.Error(err))
			// Continue anyway - the prompt should still be sent
		}
	} else if s.messageCreator != nil && alreadyRecorded {
		s.logger.Debug("skipping CreateUserMessage for queued workflow auto-start; already recorded before queueing",
			zap.String("session_id", queuedMsg.SessionID),
			zap.String("queue_id", queuedMsg.ID))
	}

	// Process on_turn_start before sending the queued prompt, just like
	// dispatchPromptAsync does for user-initiated messages. This allows
	// workflow transitions (e.g. move_to_next) to fire on auto-started prompts.
	if session, sErr := s.repo.GetTaskSession(promptCtx, queuedMsg.SessionID); sErr == nil {
		s.processOnTurnStartViaEngine(promptCtx, queuedMsg.TaskID, session)
	}

	// Call the internal promptTask directly (not the public PromptTask
	// wrapper), passing this entry's own ID as the claim token — see
	// promptTask's doc comment and the Service.dispatchingQueued field doc
	// comment for the guarded claim-then-mark-RUNNING step this enables,
	// and for why a bare, unguarded check-then-clear would not be enough.
	_, err := s.promptTask(promptCtx, queuedMsg.TaskID, queuedMsg.SessionID,
		queuedMsg.Content, queuedMsg.Model, queuedMsg.PlanMode, attachments, false,
		queuedMsg.ID)
	if errors.Is(err, errQueuedDispatchSuperseded) {
		// A newer dispatch for this same session (e.g. a second parent
		// interrupt cancelling and re-taking while this one was still
		// settling) won the claim first. This entry's content still
		// matters — steering messages are not silently dropped — so
		// requeue it instead of losing it; it will be delivered once the
		// winning turn completes naturally.
		s.logger.Info("queued message superseded by a newer dispatch for the same session before it could be prompted; requeueing",
			zap.String("session_id", callerSessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID))
		s.requeueMessage(promptCtx, queuedMsg, "superseded-by-newer-dispatch")
		return
	}
	if err != nil {
		s.logger.Error("failed to execute queued message",
			zap.String("session_id", callerSessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID),
			zap.Error(err))

		if isSessionBusyError(err) || isTransientPromptError(err) ||
			errors.Is(err, lifecycle.ErrCancelEscalated) || isSessionResetInProgressError(err) {
			s.logger.Warn("queued message execution failed transiently; requeueing",
				zap.String("session_id", callerSessionID),
				zap.String("task_id", queuedMsg.TaskID),
				zap.String("queue_id", queuedMsg.ID))
			s.requeueMessage(promptCtx, queuedMsg, "workflow-auto-start-retry")
			return
		}

		// TODO: Implement dead letter queue for failed queued messages
		// Currently, failed messages are lost. Consider:
		// 1. Retry mechanism with exponential backoff
		// 2. Persist failed messages to database for manual intervention
		// 3. Notification to user about failed queue execution
		s.logger.Warn("queued message execution failed - message is lost (no retry/dead letter queue)",
			zap.String("session_id", callerSessionID),
			zap.String("queue_id", queuedMsg.ID),
			zap.String("content_preview", queuedMsg.Content[:min(50, len(queuedMsg.Content))]))
	}
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.handleAgentCompletedLocked(ctx, data)
		return
	}

	// Completion owns workflow advancement only while serialized with every
	// cancel/interrupt decision for this session. If coordinator stop won while
	// the event waited, the guarded state reload below observes CANCELLED and
	// suppresses all workflow/on_enter side effects.
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	s.handleAgentCompletedLocked(ctx, data)
}

func (s *Service) handleAgentCompletedLocked(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	s.markExecutionCompleted(data.SessionID, data.AgentExecutionID)

	// Check for workflow transition based on session's current step.
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent completed",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		return
	}

	// Skip transition logic when this event is the side-effect of a deliberate
	// stop (e.g. a workflow profile-switch calling completeAndStopSession). Two
	// signals identify that case:
	//   - The session's current live execution differs from the event's: the
	//     lifecycle manager has rotated the session to a new execution, so this
	//     event refers to a stopped run, not the current one. (Pre-refactor this
	//     compared session.AgentExecutionID; now the lifecycle store is the
	//     source of truth.)
	//   - Terminal session state: completeAndStopSession set state to COMPLETED
	//     before StopAgent fired this event.
	// Without this guard, processOnTurnCompleteViaEngine evaluates the *current*
	// task step (which has already moved past where this agent ran) and triggers
	// spurious transitions — manifesting as task-step ping-pong on profile switches.
	liveExecID, _ := s.agentManager.GetExecutionIDForSession(ctx, data.SessionID)
	if data.AgentExecutionID != "" && liveExecID != "" && liveExecID != data.AgentExecutionID {
		s.logger.Debug("ignoring agent.completed for non-active (rotated) execution",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("event_execution_id", data.AgentExecutionID),
			zap.String("live_execution_id", liveExecID))
		go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		return
	}
	if isTerminalSessionState(session.State) {
		s.logger.Debug("ignoring agent.completed; session already in terminal state (deliberate stop)",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		return
	}

	// A successful, still-live completion clears retry state and scheduler
	// ownership only after the guarded terminal/rotation checks above.
	s.resetTransientRetry(data.SessionID)
	s.scheduler.HandleTaskCompleted(data.TaskID, true)
	s.scheduler.RemoveTask(data.TaskID)

	s.completeTurnForSession(context.WithoutCancel(ctx), data.SessionID)

	if s.sessionHasPendingClarification(ctx, data.SessionID) {
		s.logger.Info("deferring on_turn_complete on agent.completed while clarification is pending",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID, session)
		go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		// captureGitStatusSnapshot and finalizeAutomationRunIfEphemeral are deferred
		// until a later agent turn completes without pending clarifications, or the
		// user dismisses a stale overlay (clarification.stale_dismissed).
		return
	}

	transitioned := s.processOnTurnCompleteViaEngine(ctx, data.TaskID, session)

	// Agent-exit path: processOnTurnCompleteViaEngine handles normal
	// on_turn_complete transitions. If it did not transition, ensure the
	// completed session leaves RUNNING and let setSessionWaitingForInput perform
	// the guarded task REVIEW reconciliation when needed.
	s.logger.Debug("agent.completed turn-complete decision",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("session_state", string(session.State)),
		zap.Bool("workflow_transitioned", transitioned))

	if !transitioned && session.State != models.TaskSessionStateWaitingForInput {
		s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID, session)
	}

	// Capture a git status snapshot before cleanup so it can be served
	// when clients subscribe to this session later (sidebar diff stats, etc.).
	s.captureGitStatusSnapshot(ctx, data.SessionID)

	// Clean up the agent execution (stop agentctl, release port)
	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)

	// Finalize run-mode automation runs: mark status=succeeded and reap
	// the ephemeral worktree right away (the 24h Office GC is too late).
	s.finalizeAutomationRunIfEphemeral(ctx, data.TaskID, data.SessionID, true, "")
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.handleAgentFailedLocked(ctx, data)
		return
	}

	// Linearize every session-backed failure decision with coordinator stop,
	// interrupt, and queued-dispatch ownership. The state is re-read inside
	// handleAgentFailedLocked after this lock is held: if stop won, failure
	// recovery must not create messages, arm retries, or force-clean the
	// execution that graceful teardown now owns.
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	s.handleAgentFailedLocked(ctx, data)
}

func (s *Service) handleAgentFailedLocked(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	if s.shouldDropSessionFailure(ctx, data, "agent.failed", true) {
		return
	}
	s.markExecutionFailed(data.SessionID, data.AgentExecutionID)

	// Transient provider errors (529 Overloaded) get a paced, visible
	// retry-with-backoff before any red banner. This is the ONLY non-terminal
	// failure path, so it runs before automation finalization below — otherwise
	// a transient 529 on a run-mode automation would mark the run failed and
	// reap its ephemeral worktree out from under the in-flight retry.
	// handleTransientFailure returns false (falling through) for non-transient
	// errors, office tasks, or an exhausted budget.
	if data.SessionID != "" && s.handleTransientFailure(ctx, data) {
		return
	}

	// Terminal from here. Finalize run-mode automation runs — every branch
	// below returns early (resume failure, session-backed recoverable failure,
	// no-session retry), and run-mode automations need their AutomationRun
	// flipped + worktree reaped on *every* terminal failure path.
	errMsg := data.ErrorMessage
	if errMsg == "" {
		errMsg = "agent failed"
	}
	s.finalizeAutomationRunIfEphemeral(ctx, data.TaskID, data.SessionID, false, errMsg)

	// Check if the agent was started with a resume token AND session init hadn't completed.
	// If init completed, this is a normal prompt failure (e.g. agent internal timeout),
	// not a resume failure — skip the resume cleanup path.
	if data.SessionID != "" && s.wasResumeAttempt(ctx, data.SessionID) &&
		!s.agentManager.WasSessionInitialized(data.AgentExecutionID) {
		if s.handleResumeFailure(ctx, data) {
			return // Resume token cleared, session set to WAITING_FOR_INPUT
		}
		// Fall through to normal failure handling if cleanup failed
	}

	// Make all agent CLI failures recoverable — let the user choose to resume or start fresh.
	if data.SessionID != "" {
		s.handleRecoverableFailure(ctx, data)
		return
	}

	// No session — fall back to scheduler retry + task to REVIEW unless another
	// session is still working.
	s.scheduler.HandleTaskCompleted(data.TaskID, false)
	s.scheduler.RetryTask(data.TaskID)
	s.writeTaskReviewState(ctx, data.TaskID, data.SessionID)

	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
}

func (s *Service) shouldDropSessionFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	source string,
	dropWhenUnavailable bool,
) bool {
	if data.SessionID == "" {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		s.logger.Warn("dropping session failure because current state is unavailable",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("failure_source", source),
			zap.Error(err))
		if dropWhenUnavailable {
			// The workflow state is unavailable, but the execution ID is still
			// authoritative enough for bounded runtime cleanup. Leaving it alive
			// here leaks agentctl/port state until restart reconciliation.
			go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		}
		return dropWhenUnavailable
	}
	if isTerminalSessionState(session.State) {
		s.resetTransientRetry(data.SessionID)
		s.logger.Debug("dropping session failure for terminal session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("failure_source", source),
			zap.String("session_state", string(session.State)))
		if session.State != models.TaskSessionStateCancelled {
			go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		}
		return true
	}
	liveExecutionID, _ := s.agentManager.GetExecutionIDForSession(ctx, data.SessionID)
	if data.AgentExecutionID != "" && liveExecutionID != "" && liveExecutionID != data.AgentExecutionID {
		s.logger.Debug("dropping session failure for rotated execution",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("failure_source", source),
			zap.String("event_execution_id", data.AgentExecutionID),
			zap.String("live_execution_id", liveExecutionID))
		go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
		return true
	}
	return false
}

type executionTeardownIntent uint8

const (
	executionTeardownIntentGraceful executionTeardownIntent = iota + 1
	executionTeardownIntentForce
)

type executionTeardownClaim struct {
	intent    executionTeardownIntent
	expiresAt time.Time
}

// claimExecutionTeardown accepts the first teardown intent for one concrete
// execution. Callers with a session ID must invoke it while holding that
// session's cancelInFlight guard, making the decision atomic with the state
// read/write that justified the intent. The blocking teardown itself happens
// only after that guard is released.
func (s *Service) claimExecutionTeardown(
	sessionID, executionID string,
	intent executionTeardownIntent,
) bool {
	if sessionID == "" || executionID == "" {
		return false
	}
	key := terminalExecutionKey(sessionID, executionID)
	for {
		now := time.Now()
		claim := executionTeardownClaim{
			intent:    intent,
			expiresAt: now.Add(completedExecutionRetention),
		}
		value, loaded := s.executionTeardownClaims.LoadOrStore(key, claim)
		if !loaded {
			time.AfterFunc(completedExecutionRetention, func() {
				s.deleteExecutionTeardownClaimIfExpired(key, claim.expiresAt)
			})
			return true
		}
		current, ok := value.(executionTeardownClaim)
		if !ok {
			s.executionTeardownClaims.Delete(key)
			continue
		}
		if now.Before(current.expiresAt) {
			return false
		}
		if s.executionTeardownClaims.CompareAndDelete(key, current) {
			continue
		}
	}
}

func (s *Service) deleteExecutionTeardownClaimIfExpired(key string, expiresAt time.Time) {
	value, ok := s.executionTeardownClaims.Load(key)
	if !ok {
		return
	}
	claim, ok := value.(executionTeardownClaim)
	if !ok || !claim.expiresAt.After(expiresAt) {
		s.executionTeardownClaims.Delete(key)
	}
}

// wasResumeAttempt checks whether the session's last execution used a resume token.
// If the token is still present in the DB, the agent was started with --resume.
func (s *Service) wasResumeAttempt(ctx context.Context, sessionID string) bool {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil {
		return false
	}
	return running.ResumeToken != ""
}

// clearResumeToken removes the resume token from the executor running record so
// the next agent start won't use --resume. Used by both automatic resume failure
// handling and user-initiated fresh start recovery.
//
// Unconditional clear: passes expectedExecID="" so the narrow update is not
// CAS-guarded — clearing a token is always intentional regardless of which
// execution is currently registered.
func (s *Service) clearResumeToken(ctx context.Context, sessionID string) {
	err := s.repo.UpdateResumeToken(ctx, sessionID, "", "", "")
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		s.logger.Error("failed to clear resume token",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// handleResumeFailure handles the case where an agent failed while using a resume token.
// It clears the token so the next attempt starts fresh, and notifies the user.
//
// The session is set to WAITING_FOR_INPUT so the user can send a new message
// (which triggers a fresh agent start without --resume).
//
// Returns true to signal that the caller should skip normal failure handling
// (scheduler retry, FAILED state) since we've handled the state transition ourselves.
func (s *Service) handleResumeFailure(ctx context.Context, data watcher.AgentEventData) bool {
	s.logger.Warn("detected resume failure, clearing token for fresh start on next user action",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("error", data.ErrorMessage))

	// 1. Clear the resume token so the next attempt won't use --resume.
	s.clearResumeToken(ctx, data.SessionID)

	// 2. Send a status message about the failed resume.
	if s.messageCreator != nil {
		statusMsg := fmt.Sprintf("Previous agent session could not be restored (%s). Send a new message to start a fresh session.", data.ErrorMessage)
		if err := s.messageCreator.CreateSessionMessage(
			ctx,
			data.TaskID,
			statusMsg,
			data.SessionID,
			string(v1.MessageTypeStatus),
			s.getActiveTurnID(data.SessionID),
			map[string]interface{}{
				"variant":       "warning",
				"resume_failed": true,
			},
			false,
		); err != nil {
			s.logger.Warn("failed to create resume failure status message",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		}
	}

	// 3. Set session to WAITING_FOR_INPUT (not FAILED) so the user can interact.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)

	// 4. Ensure task is in REVIEW state unless another session is still working.
	s.writeTaskReviewState(ctx, data.TaskID, data.SessionID)

	return true
}

// handleRecoverableFailure handles agent failures by keeping the session recoverable.
// Instead of marking the session FAILED (terminal), it sets WAITING_FOR_INPUT and
// creates an error message with recovery action buttons so the user can choose to
// resume the agent session or start fresh.
func (s *Service) handleRecoverableFailure(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling recoverable agent failure",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("error", data.ErrorMessage))

	// Complete the current turn.
	s.completeTurnForSession(ctx, data.SessionID)
	s.persistLastAgentError(ctx, data)

	// Create a status message with recovery action metadata.
	// Skipped for office sessions: the office task page renders its
	// own structured RunErrorEntry sourced from the FAILED session,
	// and including the legacy ActionMessage would double-show the
	// red banner (top-level + inside the embedded chat panel).
	if s.messageCreator != nil && !s.isOfficeSession(ctx, data.SessionID) {
		s.createRecoveryStatusMessage(ctx, data)
	}

	// Set session state. Office tasks (those with an assignee_agent_profile_id)
	// transition to FAILED so the chat correctly stops rendering "Agent
	// working" and the topbar spinner clears. Kanban / quick-chat tasks
	// keep the legacy WAITING_FOR_INPUT path so the user can resume via
	// the Resume / Start fresh recovery buttons in the existing chat
	// surface. (See docs/specs/office-agent-error-handling.)
	nextState := models.TaskSessionStateWaitingForInput
	if s.isOfficeSession(ctx, data.SessionID) {
		nextState = models.TaskSessionStateFailed
	}
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, nextState, data.ErrorMessage, false)

	// Ensure task is in REVIEW state unless another session is still working.
	s.writeTaskReviewState(ctx, data.TaskID, data.SessionID)

	// Clean up the agent execution.
	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
}

func (s *Service) persistLastAgentError(ctx context.Context, data watcher.AgentEventData) {
	errMsg := data.ErrorMessage
	if errMsg == "" {
		errMsg = "agent failed"
	}
	// Keep this metadata until the user dismisses the UI notice locally or a
	// later recoverable failure replaces it. A successful turn should not erase
	// the investigation breadcrumb that explains why the task was marked REVIEW.
	lastErr := models.LastAgentError{
		Message:          errMsg,
		OccurredAt:       time.Now().UTC(),
		AgentExecutionID: data.AgentExecutionID,
	}
	if err := s.repo.SetSessionMetadataKey(ctx, data.SessionID, models.SessionMetaKeyLastAgentError, lastErr); err != nil {
		s.logger.Warn("failed to persist last agent error",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
	}
}

// createRecoveryStatusMessage builds and persists the ActionMessage shown
// in the kanban chat surface after a recoverable agent failure. Must only
// be called for non-office sessions (office sessions render their own error UI).
func (s *Service) createRecoveryStatusMessage(ctx context.Context, data watcher.AgentEventData) {
	authErr := isAuthError(data.ErrorMessage)
	resumeCorrupted := routingerr.IsResumeCorrupted(data.ErrorMessage)
	displayMsg := data.ErrorMessage
	if authErr {
		if readable := extractReadableAuthError(data.ErrorMessage); readable != "" {
			displayMsg = readable
		}
	}

	// Resume-corrupted failures (poisoned extended-thinking state after a
	// session/load) can't be fixed by resuming — steer the user to a fresh
	// session instead of dumping the raw 400.
	statusMsg := fmt.Sprintf("Agent encountered an error: %s", displayMsg)
	if resumeCorrupted {
		statusMsg = "This agent session can't be resumed — its saved reasoning state is corrupted. Start a fresh session to continue."
	} else if routingerr.IsTransientProviderError(data.ErrorMessage) {
		// Reached after the transient retry budget is exhausted — show friendly
		// copy instead of dumping the raw 529 JSON envelope.
		statusMsg = "The provider stayed overloaded after several retries. Resume to try again, or start a fresh session."
	}
	hasResumeToken := s.wasResumeAttempt(ctx, data.SessionID)
	meta := map[string]interface{}{
		"variant":          "error",
		"recovery_actions": true,
		"session_id":       data.SessionID,
		"task_id":          data.TaskID,
		"has_resume_token": hasResumeToken,
		"is_auth_error":    authErr,
		"resume_corrupted": resumeCorrupted,
	}

	// Include cached auth methods so the frontend can show login options.
	if authErr {
		if methods := s.agentManager.GetSessionAuthMethods(data.SessionID); len(methods) > 0 {
			meta["auth_methods"] = methods
		}
	}

	meta["actions"] = buildRecoveryActions(data.TaskID, data.SessionID, hasResumeToken, authErr, resumeCorrupted)

	if err := s.messageCreator.CreateSessionMessage(
		ctx,
		data.TaskID,
		statusMsg,
		data.SessionID,
		string(v1.MessageTypeStatus),
		s.getActiveTurnID(data.SessionID),
		meta,
		false,
	); err != nil {
		s.logger.Warn("failed to create recovery status message",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}
}

// isOfficeSession returns true when the session row carries an
// agent_profile_id — the office indicator. Best-effort: a missing
// session falls back to the legacy kanban path.
func (s *Service) isOfficeSession(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return false
	}
	return session.AgentProfileID != ""
}

// handleAgentStartFailed is called by the executor when StartAgentProcess fails.
// It detects auth errors and routes them through the recoverable failure path so
// the frontend shows login guidance instead of a terminal failure. When the
// failure occurred during a background session resume (fromResume=true) and is
// not an auth error, it sets the suppressToast flag so the default FAILED
// transition does not surface a user-facing toast for a transient bootstrap
// error on focus / auto-resume.
// Returns true if the failure was handled (caller should skip default FAILED logic).
func (s *Service) handleAgentStartFailed(ctx context.Context, taskID, sessionID, agentExecutionID string, err error, fromResume bool) bool {
	if sessionID != "" {
		lock, release := s.acquireCancelInFlightGuard(sessionID)
		defer release()
		lock.Lock()
		defer lock.Unlock()

		if s.shouldDropSessionFailure(ctx, watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: agentExecutionID,
			ErrorMessage:     err.Error(),
		}, "agent process start", false) {
			// Tell the executor that the failure is fully owned. In particular,
			// a concurrent CANCELLED transition must skip its default FAILED
			// transition and forced cleanup after this callback returns.
			return true
		}
	}

	if !isAuthError(err.Error()) {
		if fromResume {
			s.logger.Info("suppressing toast for resume bootstrap failure",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			s.suppressToast.Store(sessionID, true)
		}
		return false
	}
	s.logger.Info("agent start failure is auth error, treating as recoverable",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	s.handleRecoverableFailure(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: agentExecutionID,
		ErrorMessage:     err.Error(),
	})
	return true
}

// actionMetaKey* are the shared keys of the frontend ActionMessage button
// descriptor (see apps/web .../messages/action-message.tsx). Defined as
// constants because the shape is built in more than one place in this package.
const (
	actionMetaKeyType    = "type"
	actionMetaKeyLabel   = "label"
	actionMetaKeyIcon    = "icon"
	actionMetaKeyTooltip = "tooltip"
	actionMetaKeyTestID  = "test_id"
)

const (
	recoveryFreshButtonTestID   = "recovery-fresh-button"
	recoveryRestartButtonTestID = "recovery-restart-button"
	recoveryResumeButtonTestID  = "recovery-resume-button"
)

// wsRecoveryAction builds a single session.recover button descriptor. Keeping
// the map keys in one place avoids drift between the buttons and keeps the
// metadata shape consistent.
func wsRecoveryAction(taskID, sessionID, recoverAction, label, icon, tooltip, testID string) map[string]interface{} {
	return map[string]interface{}{
		actionMetaKeyType:    "ws_request",
		actionMetaKeyLabel:   label,
		actionMetaKeyIcon:    icon,
		actionMetaKeyTooltip: tooltip,
		actionMetaKeyTestID:  testID,
		"params": map[string]interface{}{
			"method":  "session.recover",
			"payload": map[string]interface{}{"task_id": taskID, "session_id": sessionID, "action": recoverAction},
		},
	}
}

// buildRecoveryActions creates the generic actions array for agent error
// recovery. Ordinary failures list Resume first (cheapest recovery, keeps
// context) then Start fresh. For resume-corrupted failures the order flips:
// Start fresh becomes the primary action and Resume is kept but flagged as
// likely-to-fail, since the agent's persisted state is poisoned.
func buildRecoveryActions(taskID, sessionID string, hasResumeToken, isAuthError, resumeCorrupted bool) []map[string]interface{} {
	resumeTooltip := "Re-launch with resume flag — keeps all previous messages and context"
	if resumeCorrupted {
		resumeTooltip = "Resume will likely fail again — this session's saved state is corrupted. Prefer Start fresh."
	}
	resume := func() map[string]interface{} {
		return wsRecoveryAction(taskID, sessionID, "resume",
			"Resume session", "refresh", resumeTooltip, recoveryResumeButtonTestID)
	}

	freshLabel, freshTestID := "Start fresh session", recoveryFreshButtonTestID
	if isAuthError {
		freshLabel, freshTestID = "Restart session", recoveryRestartButtonTestID
	}
	fresh := wsRecoveryAction(taskID, sessionID, "fresh_start", freshLabel, "player-play",
		"New agent process on the same workspace — no previous conversation context", freshTestID)

	actions := []map[string]interface{}{}
	if resumeCorrupted {
		// Fresh is primary; resume kept but de-emphasized below it.
		actions = append(actions, fresh)
		if hasResumeToken {
			actions = append(actions, resume())
		}
		return actions
	}
	if hasResumeToken {
		actions = append(actions, resume())
	}
	actions = append(actions, fresh)
	return actions
}

// handleAgentStopped handles agent stopped events (manual stop or cancellation)
func (s *Service) handleAgentStopped(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent stopped",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// NOTE: we deliberately do NOT resetTransientRetry here — the transient
	// retry tears down the failed execution via StopExecution as part of its
	// own re-drive, which surfaces as an agent.stopped event; clearing the loop
	// on that self-inflicted stop would abort the retry. The loop is freed on
	// ready/completed (success), cancel, and exhaustion instead.

	// Drop stopped events that belong to a previous (rotated) execution. The
	// session might already be running a fresh resume cycle; flipping its state
	// to CANCELLED based on the corpse of the prior execution poisons the
	// recovery (the new cycle's session/load succeeds against a session that
	// looks terminal to the rest of the system). Mirrors the rotation guard in
	// handleAgentCompleted.
	if s.agentManager != nil && data.AgentExecutionID != "" && data.SessionID != "" {
		if liveExecID, _ := s.agentManager.GetExecutionIDForSession(ctx, data.SessionID); liveExecID != "" && liveExecID != data.AgentExecutionID {
			s.logger.Info("ignoring agent.stopped for non-active (rotated) execution",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.String("event_execution_id", data.AgentExecutionID),
				zap.String("live_execution_id", liveExecID))
			return
		}
	}

	// Complete the current turn if there is one
	s.completeTurnForSession(ctx, data.SessionID)

	// Don't override WAITING_FOR_INPUT or IDLE — these are "stopped on
	// purpose" states the caller already set. WAITING_FOR_INPUT comes from
	// the recovery path so the user can choose to resume; IDLE comes from
	// the office fire-and-forget turn-complete handler which intentionally
	// stops the agent and parks the session for the next run. Either
	// way, the AgentStopped event here is a side-effect of that stop —
	// clobbering the state to CANCELLED would mark the row terminal and
	// break the next office run (EnsureSessionForAgent then tries to
	// INSERT a new row and the partial unique index rejects it).
	if session, err := s.repo.GetTaskSession(ctx, data.SessionID); err == nil &&
		(session.State == models.TaskSessionStateWaitingForInput ||
			session.State == models.TaskSessionStateIdle) {
		s.logger.Info("skipping CANCELLED transition; session was stopped on purpose",
			zap.String("session_id", data.SessionID),
			zap.String("state", string(session.State)))
		return
	}

	// Update session state to cancelled (already done by executor, but ensure consistency)
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateCancelled, "", false)

	// NOTE: We do NOT update task state here because:
	// 1. If this is from CompleteTask(), the task state will be set to COMPLETED by the caller
	// 2. If this is from StopTask(), the task state should be set to REVIEW by the caller
	// 3. Updating here would create a race condition with the caller's state update
	//
	// The task state management is the responsibility of the operation that triggered the stop,
	// not the event handler. This handler only manages session-level cleanup.
}

// cleanupAgentExecution stops the agentctl instance and releases its port after
// the agent reaches a terminal state (completed/failed). This runs in a goroutine
// so it doesn't block the event handler.
func (s *Service) cleanupAgentExecution(executionID, taskID, sessionID string) {
	if executionID == "" {
		return
	}
	if sessionID != "" {
		lock, release := s.acquireCancelInFlightGuard(sessionID)
		lock.Lock()

		// Coordinator stop persists CANCELLED before launching its graceful,
		// detached teardown. A generic completion/failure cleanup racing after
		// that commit must not replace it with force=true teardown.
		session, sessionErr := s.repo.GetTaskSession(context.Background(), sessionID)
		cancelled := sessionErr == nil && session != nil && session.State == models.TaskSessionStateCancelled
		claimed := !cancelled && s.claimExecutionTeardown(
			sessionID,
			executionID,
			executionTeardownIntentForce,
		)
		lock.Unlock()
		release()
		if cancelled {
			s.logger.Debug("skipping forced cleanup for cancelled session",
				zap.String("execution_id", executionID),
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
			return
		}
		if !claimed {
			s.logger.Debug("skipping duplicate execution teardown",
				zap.String("execution_id", executionID),
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
			return
		}
	}
	ctx := context.Background()
	if err := s.executor.StopExecution(ctx, executionID, "agent completed", true); err != nil {
		s.logger.Debug("agent execution cleanup after terminal state",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}
