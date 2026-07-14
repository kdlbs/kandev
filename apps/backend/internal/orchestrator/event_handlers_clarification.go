package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

const clarificationInputPauseTimeout = 30 * time.Second

// subscribeClarificationEvents subscribes to clarification-related events.
func (s *Service) subscribeClarificationEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationAnswered, s.handleClarificationAnswered); err != nil {
		s.logger.Error("failed to subscribe to clarification.answered events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationPrimaryAnswered, s.handleClarificationPrimaryAnswered); err != nil {
		s.logger.Error("failed to subscribe to clarification.primary_answered events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationCancelled, s.handleClarificationAnswered); err != nil {
		s.logger.Error("failed to subscribe to clarification.cancelled events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationStaleDismissed, s.handleClarificationStaleDismissed); err != nil {
		s.logger.Error("failed to subscribe to clarification.stale_dismissed events", zap.Error(err))
	}
}

// handleClarificationStaleDismissed runs session cleanup when the user dismisses
// a detached clarification overlay without starting a new agent turn.
func (s *Service) handleClarificationStaleDismissed(ctx context.Context, event *bus.Event) error {
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		s.logger.Error("failed to marshal stale-dismissed clarification event data", zap.Error(err))
		return nil
	}
	var data clarificationAnsweredData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		s.logger.Error("failed to parse stale-dismissed clarification event data", zap.Error(err))
		return nil
	}
	if data.SessionID == "" || data.TaskID == "" {
		s.logger.Warn("stale-dismissed clarification event missing session_id or task_id",
			zap.String("session_id", data.SessionID),
			zap.String("task_id", data.TaskID))
		return nil
	}

	writeCtx := context.WithoutCancel(ctx)
	if s.sessionHasPendingClarification(writeCtx, data.SessionID) {
		return nil
	}

	session, err := s.repo.GetTaskSession(writeCtx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for stale-dismissed clarification cleanup",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return nil
	}
	if isTerminalSessionState(session.State) {
		s.logger.Debug("ignoring stale-dismissed clarification for terminal session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return nil
	}

	s.captureGitStatusSnapshot(writeCtx, data.SessionID)
	s.finalizeAutomationRunIfEphemeral(writeCtx, data.TaskID, data.SessionID, true, "")
	transitioned := s.processOnTurnCompleteViaEngine(writeCtx, data.TaskID, session)
	if !transitioned {
		s.writeTaskReviewState(writeCtx, data.TaskID, data.SessionID)
	}
	return nil
}

// clarificationAnsweredData is the event payload for ClarificationAnswered events.
type clarificationAnsweredData struct {
	SessionID    string `json:"session_id"`
	TaskID       string `json:"task_id"`
	PendingID    string `json:"pending_id"`
	Question     string `json:"question"`
	AnswerText   string `json:"answer_text"`
	Rejected     bool   `json:"rejected"`
	RejectReason string `json:"reject_reason"`
}

type clarificationWatchdogEntry struct {
	cancel func()
}

// handleClarificationAnswered handles user responses to agent clarification questions.
// It constructs a follow-up prompt with the answer and sends it to the agent.
func (s *Service) handleClarificationAnswered(ctx context.Context, event *bus.Event) error {
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		s.logger.Error("failed to marshal clarification event data", zap.Error(err))
		return nil
	}
	var data clarificationAnsweredData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		s.logger.Error("failed to parse clarification event data", zap.Error(err))
		return nil
	}

	if data.SessionID == "" || data.TaskID == "" {
		s.logger.Warn("clarification answered event missing session_id or task_id",
			zap.String("session_id", data.SessionID),
			zap.String("task_id", data.TaskID))
		return nil
	}

	prompt := buildClarificationPrompt(data)

	s.logger.Info("resuming agent with clarification answer",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.Bool("rejected", data.Rejected))

	if _, err := s.PromptTask(ctx, data.TaskID, data.SessionID, prompt, "", false, nil, false); err != nil {
		if !s.retryClarificationAfterCancel(ctx, data, prompt, err) {
			s.logger.Error("failed to resume agent with clarification answer",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.Error(err))
		}
	}
	return nil
}

func (s *Service) handleClarificationPrimaryAnswered(_ context.Context, event *bus.Event) error {
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		s.logger.Error("failed to marshal primary clarification event data", zap.Error(err))
		return nil
	}
	var data clarificationAnsweredData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		s.logger.Error("failed to parse primary clarification event data", zap.Error(err))
		return nil
	}
	if data.SessionID == "" || data.TaskID == "" || data.PendingID == "" {
		s.logger.Warn("primary clarification event missing identifiers",
			zap.String("session_id", data.SessionID),
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return nil
	}

	s.scheduleClarificationWatchdog(data)
	return nil
}

func (s *Service) clarificationWatchdogKey(sessionID, pendingID string) string {
	return sessionID + "::" + pendingID
}

func (s *Service) getClarificationWatchdogTimeout() time.Duration {
	if s.clarificationWatchdogTimeout > 0 {
		return s.clarificationWatchdogTimeout
	}
	// After primary path delivery, if the agent doesn't send events within 15s,
	// its MCP client has timed out and the response was dropped. Trigger fallback.
	return 15 * time.Second
}

func (s *Service) scheduleClarificationWatchdog(data clarificationAnsweredData) {
	key := s.clarificationWatchdogKey(data.SessionID, data.PendingID)
	timeout := s.getClarificationWatchdogTimeout()

	if old, ok := s.clarificationWatchdogs.LoadAndDelete(key); ok {
		if oldEntry, ok := old.(*clarificationWatchdogEntry); ok && oldEntry.cancel != nil {
			oldEntry.cancel()
		}
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	entry := &clarificationWatchdogEntry{cancel: cancel}
	s.clarificationWatchdogs.Store(key, entry)

	s.logger.Info("scheduled clarification resume watchdog",
		zap.String("session_id", data.SessionID),
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.Duration("timeout", timeout))

	go s.runClarificationWatchdog(watchCtx, key, entry, data, timeout)
}

func (s *Service) runClarificationWatchdog(
	watchCtx context.Context,
	key string,
	entry *clarificationWatchdogEntry,
	data clarificationAnsweredData,
	timeout time.Duration,
) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-watchCtx.Done():
		return
	case <-timer.C:
		current, ok := s.clarificationWatchdogs.LoadAndDelete(key)
		if !ok || current != entry {
			return
		}
		if entry.cancel != nil {
			entry.cancel()
		}
		s.resumeClarificationViaFallback(data)
	}
}

func (s *Service) resumeClarificationViaFallback(data clarificationAnsweredData) {
	prompt := buildClarificationPrompt(data)
	s.logger.Warn("clarification resume watchdog expired; triggering fallback resume",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("pending_id", data.PendingID))

	ctx := context.Background()
	if _, err := s.PromptTask(ctx, data.TaskID, data.SessionID, prompt, "", false, nil, false); err != nil {
		if !s.retryClarificationAfterCancel(ctx, data, prompt, err) {
			s.logger.Error("failed to resume agent via clarification watchdog fallback",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		}
	}
}

// retryClarificationAfterCancel handles the case where PromptTask fails because
// the agent is stuck in RUNNING state (MCP client timed out during clarification).
// It silently cancels the stuck turn and retries the prompt so the recovery is
// seamless for the user (no "Turn cancelled" separator in the chat).
// Returns true if recovery succeeded.
func (s *Service) retryClarificationAfterCancel(ctx context.Context, data clarificationAnsweredData, prompt string, promptErr error) bool {
	if !isAgentPromptInProgressError(promptErr) {
		return false
	}

	s.logger.Warn("agent stuck in RUNNING state during clarification recovery; cancelling turn",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))

	// Claim the shared per-session guard across the cancel-then-hand-off
	// sequence — see the Service.cancelInFlight field doc comment. Releasing
	// between the cancel and the hand-off would let a concurrent
	// QueueAndInterruptForPeerMessage (or another drain) take-and-dispatch a
	// queued entry in that gap, only for this retry to then prompt over top of
	// it.
	//
	// The retry prompt itself is NOT sent under the guard: dispatching it
	// through the queue's take-and-dispatch path hands the (potentially
	// long-blocking) executor.Prompt call to a background goroutine
	// (executeQueuedMessage). Sending it inline here instead — the previous
	// behavior — held the guard across executor.Prompt, which blocks for as
	// long as a jammed agent takes to accept the prompt (observed: minutes,
	// stuck inside an MCP call). While it blocked, the user's Cancel button —
	// which TryLocks this same guard — was permanently starved, leaving the
	// session unstoppable. markQueuedDispatchInFlight (inside
	// dispatchTakenQueuedMessage) makes the session "busy" under the guard, so
	// a concurrent interrupt/drain still backs off exactly as an inline retry
	// would have.
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	if err := s.cancelAgentSilent(ctx, data.TaskID, data.SessionID); err != nil {
		s.logger.Warn("cancel failed (agent likely dead), force-transitioning session state",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		// Force-revert session state so the retry prompt can proceed
		if revertErr := s.repo.UpdateTaskSessionState(ctx, data.SessionID, models.TaskSessionStateWaitingForInput, ""); revertErr != nil {
			s.logger.Error("failed to force-revert session state for clarification recovery",
				zap.String("session_id", data.SessionID),
				zap.Error(revertErr))
			return false
		}
		s.completeTurnForSession(ctx, data.SessionID)
	}

	if err := s.dispatchClarificationResumeLocked(ctx, data, prompt); err != nil {
		if errors.Is(err, errClarificationResumeQueuedForDrain) {
			// Not a failure: the answer is safely queued and a future drain
			// (the next agent.ready) will dispatch it. Logging this at error
			// level produced misleading "failed to resume agent" noise even
			// though recovery still succeeds.
			s.logger.Info("clarification answer queued; awaiting drain to dispatch",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID))
			return true
		}
		s.logger.Error("failed to resume agent after cancel in clarification recovery",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return false
	}

	s.logger.Info("recovered stuck agent; dispatching clarification answer",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))
	return true
}

// errClarificationResumeQueuedForDrain signals that the clarification resume
// prompt was safely queued but not immediately dispatched (a concurrent
// dispatch was already settling for this session, so take-and-dispatch backed
// off). The entry stays in the queue and the next drain will pick it up, so
// this is a success case for recovery — not an error — and must not be logged
// as a failure.
var errClarificationResumeQueuedForDrain = errors.New("clarification resume queued for future drain")

// dispatchClarificationResumeLocked enqueues the clarification resume prompt and
// hands it to the async take-and-dispatch path so the blocking executor.Prompt
// call runs off the cancelInFlight guard (see retryClarificationAfterCancel).
// The entry is tagged user_message_recorded so executeQueuedMessage does not
// insert a spurious user chat message for the system-built resume prompt. The
// caller MUST already hold sessionID's cancelInFlight lock.
//
// Returns nil when the entry was dispatched immediately,
// errClarificationResumeQueuedForDrain when it was safely queued for a future
// drain (a non-failure the caller must not treat as an error), and any other
// error when the resume genuinely could not be handed off.
func (s *Service) dispatchClarificationResumeLocked(ctx context.Context, data clarificationAnsweredData, prompt string) error {
	if s.messageQueue == nil {
		// The queue is the only hand-off path now that the retry prompt no
		// longer runs inline under the guard. A nil queue is a wiring bug, not
		// a runtime condition, so surface it with context rather than failing
		// silently the way a bare false return did.
		return fmt.Errorf("cannot resume clarification: message queue is not configured")
	}
	queued, err := s.messageQueue.QueueMessageWithMetadata(
		ctx, data.SessionID, data.TaskID, prompt, "", messagequeue.QueuedByAgent, false, nil,
		map[string]interface{}{metaKeyUserMessageRecorded: true},
	)
	if err != nil {
		return fmt.Errorf("queue clarification resume prompt: %w", err)
	}
	dispatched, err := s.takeAndDispatchEntryLocked(ctx, data.SessionID, queued.ID)
	if err != nil {
		return fmt.Errorf("dispatch clarification resume prompt: %w", err)
	}
	if !dispatched {
		return errClarificationResumeQueuedForDrain
	}
	return nil
}

// PauseForClarificationInput converts a no-answer ask_user_question outcome
// into a platform pause. It detaches the pending clarification so a late user
// answer resumes through the event fallback path, then silently cancels the
// active agent turn without evaluating workflow turn-complete actions. It
// returns the number of clarification bundles detached.
func (s *Service) PauseForClarificationInput(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, fmt.Errorf("session_id is required")
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clarificationInputPauseTimeout)
	defer cancel()
	session, err := s.repo.GetTaskSession(writeCtx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("load session for clarification pause: %w", err)
	}
	if session == nil {
		return 0, nil
	}

	hasPendingClarification := s.sessionHasPendingClarification(writeCtx, sessionID)
	detached := 0
	if s.clarificationCanceller != nil {
		detached = s.clarificationCanceller.DetachSessionAndNotify(writeCtx, sessionID)
	}
	if isTerminalSessionState(session.State) {
		return detached, nil
	}
	if !hasPendingClarification && detached == 0 {
		return detached, nil
	}
	if _, has := models.LoadPendingStepSignal(session.Metadata); has {
		s.clearPendingStepSignal(writeCtx, session)
	}

	// The backend wait path and agentctl timeout notification can race for the
	// same ask_user_question call. A duplicate cancel is safe: lifecycle returns
	// ErrCancelEscalated/ErrNoExecutionForSession once the first pause wins, and
	// completeTurnForSession is idempotent when there is no active turn left.
	//
	// Claim the shared per-session guard around the cancel itself — see the
	// Service.cancelInFlight field doc comment: every cancel/take-and-dispatch
	// decision for a session must serialize through this one guard, including
	// this clarification-timeout cancel, or it can race a concurrent parent
	// interrupt (or another drain) for the same session.
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if err := s.cancelAgentSilent(writeCtx, session.TaskID, sessionID); err != nil {
		return detached, err
	}
	return detached, nil
}

// cancelAgentSilent cancels the agent turn without creating a visible message
// in the chat. Used by clarification recovery so the cancel-and-retry is seamless.
//
// If the agent manager reports no live execution for the session, the session may be stuck
// (agent crashed mid-turn). In that case, skip the cancel signal but still reconcile the
// session's state so clarification recovery can proceed with a fresh prompt.
func (s *Service) cancelAgentSilent(ctx context.Context, taskID, sessionID string) error {
	if s.agentManager == nil {
		s.logger.Debug("skipping silent clarification cancel because agent manager is not configured",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
	} else if err := s.agentManager.CancelAgent(ctx, sessionID); err != nil {
		if !errors.Is(err, lifecycle.ErrNoExecutionForSession) && !errors.Is(err, lifecycle.ErrCancelEscalated) {
			return fmt.Errorf("cancel agent: %w", err)
		}
		s.logSilentCancelReconciled(taskID, sessionID, err)
	}
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", true, nil)
	s.completeTurnForSession(ctx, sessionID)
	return nil
}

func (s *Service) logSilentCancelReconciled(taskID, sessionID string, err error) {
	if errors.Is(err, lifecycle.ErrCancelEscalated) {
		s.logger.Warn("agent did not acknowledge silent clarification cancel; reconciling session state",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	// Agent crashed or exited mid-turn — clarification recovery cannot signal a cancel,
	// but we still reconcile state below so a fresh prompt can run. Error level so this
	// surfaces for root-cause investigation of the crash.
	s.logger.Error("agent process appears to have crashed during clarification recovery",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Error(err))
}

func (s *Service) cancelClarificationWatchdogsForSession(sessionID, reason string) {
	if sessionID == "" {
		return
	}

	prefix := sessionID + "::"
	cancelled := 0
	s.clarificationWatchdogs.Range(func(key, value interface{}) bool {
		keyStr, ok := key.(string)
		if !ok || !strings.HasPrefix(keyStr, prefix) {
			return true
		}
		s.clarificationWatchdogs.Delete(keyStr)
		if entry, ok := value.(*clarificationWatchdogEntry); ok && entry.cancel != nil {
			entry.cancel()
		}
		cancelled++
		return true
	})

	if cancelled > 0 {
		s.logger.Debug("cancelled clarification watchdogs after session activity",
			zap.String("session_id", sessionID),
			zap.String("reason", reason),
			zap.Int("count", cancelled))
	}
}

func (s *Service) cancelAllClarificationWatchdogs() {
	s.clarificationWatchdogs.Range(func(key, value interface{}) bool {
		keyStr, ok := key.(string)
		if ok {
			s.clarificationWatchdogs.Delete(keyStr)
		}
		if entry, ok := value.(*clarificationWatchdogEntry); ok && entry.cancel != nil {
			entry.cancel()
		}
		return true
	})
}

// buildClarificationPrompt constructs the resume prompt from a clarification answer.
// Handles both single- and multi-question bundles: when data.Question contains
// newlines it is treated as a pre-formatted multi-line summary and embedded
// as-is rather than quoted.
func buildClarificationPrompt(data clarificationAnsweredData) string {
	multiQuestion := strings.Contains(data.Question, "\n")

	if data.Rejected {
		reason := data.RejectReason
		if reason == "" {
			reason = "No reason provided"
		}
		if multiQuestion {
			return fmt.Sprintf("The user declined to answer your questions:\n%s\nReason: %s\nPlease continue without this information.",
				data.Question, reason)
		}
		return fmt.Sprintf("The user declined to answer your question: %q\nReason: %s\nPlease continue without this information.",
			data.Question, reason)
	}
	if multiQuestion {
		return fmt.Sprintf("You previously asked the user:\n%s\n\n%s\nPlease continue with this information.",
			data.Question, data.AnswerText)
	}
	return fmt.Sprintf("You previously asked the user: %q\n%s\nPlease continue with this information.",
		data.Question, data.AnswerText)
}
