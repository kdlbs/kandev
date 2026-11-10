package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	clarificationInputPauseTimeout = 30 * time.Second
	// Detached answers may first cold-resume the runtime. Keep that startup
	// bounded while preserving the synchronous accept-or-restore contract; a
	// 202 response could falsely acknowledge a dispatch that never reached it.
	detachedClarificationDispatchTimeout = 30 * time.Second
)

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
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	writeCtx = withWorkflowProfileSwitchGuardHeld(writeCtx, data.SessionID, "")
	if s.isCancelInFlight(data.SessionID) {
		s.logger.Debug("ignoring stale clarification dismissal while cancellation is in progress",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return nil
	}

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
	s.markAutomationRunTerminalForTurn(writeCtx, data.TaskID, data.SessionID, data.ClarificationTurnID, true, "")
	transitioned := s.processOnTurnCompleteViaEngine(writeCtx, data.TaskID, session)
	if !transitioned {
		s.writeTaskReviewState(writeCtx, data.TaskID, data.SessionID)
	}
	return nil
}

// clarificationAnsweredData is the event payload for ClarificationAnswered events.
type clarificationAnsweredData struct {
	SessionID           string                                 `json:"session_id"`
	TaskID              string                                 `json:"task_id"`
	PendingID           string                                 `json:"pending_id"`
	ClarificationTurnID string                                 `json:"clarification_turn_id"`
	Question            string                                 `json:"question"`
	AnswerText          string                                 `json:"answer_text"`
	Attachments         []models.ClarificationAnswerAttachment `json:"attachments,omitempty"`
	Rejected            bool                                   `json:"rejected"`
	RejectReason        string                                 `json:"reject_reason"`
	HandledInline       bool                                   `json:"handled_inline"`
}

type clarificationWatchdogEntry struct {
	cancel func()
	// Recovery's silent cancellation can synchronously emit stream activity.
	// Keep that activity from cancelling this entry's own recovery context.
	recoveryCancellationActive atomic.Bool
}

func (e *clarificationWatchdogEntry) beginRecoveryCancellation() {
	if e != nil {
		e.recoveryCancellationActive.Store(true)
	}
}

func (e *clarificationWatchdogEntry) endRecoveryCancellation() {
	if e != nil {
		e.recoveryCancellationActive.Store(false)
	}
}

func (e *clarificationWatchdogEntry) isRecoveryCancellationActive() bool {
	return e != nil && e.recoveryCancellationActive.Load()
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

	if err := s.resumeDetachedClarification(ctx, data); err != nil {
		s.logger.Error("failed to resume agent with clarification answer",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
	}
	return nil
}

// ResumeDetachedClarification synchronously reports whether the orchestrator
// accepted a detached answer. The HTTP handler keeps the durable bundle
// retryable unless this call succeeds.
func (s *Service) ResumeDetachedClarification(
	ctx context.Context,
	request clarification.DetachedClarificationResume,
) error {
	// The HTTP boundary already supplies a fresh bounded persistence context.
	// Preserve its remaining total budget and explicit caller cancellation here.
	resumeCtx, cancel := context.WithTimeout(ctx, detachedClarificationDispatchTimeout)
	defer cancel()
	return s.resumeDetachedClarificationWithPrompt(resumeCtx, clarificationAnsweredData{
		SessionID:           request.SessionID,
		TaskID:              request.TaskID,
		PendingID:           request.PendingID,
		ClarificationTurnID: request.ClarificationTurnID,
		Question:            request.Question,
		AnswerText:          request.AnswerText,
		Attachments:         request.Attachments,
		Rejected:            request.Rejected,
		RejectReason:        request.RejectReason,
	}, true, promptTaskOptions{
		preservePromptContext:    true,
		reserveTurnUntilDispatch: true,
		promptDispatchRecovery: &models.PromptDispatchRecovery{
			PendingID:  request.PendingID,
			TurnID:     request.ClarificationTurnID,
			MessageIDs: request.ClaimedMessageIDs,
		},
	})
}

func (s *Service) resumeDetachedClarification(ctx context.Context, data clarificationAnsweredData) error {
	// Legacy bus events do not carry a claimed turn ID. Their active-bundle
	// producer is gated by FindActiveClarificationMessagesBySessionID and its
	// turnAuthorityPredicate, so this path intentionally leaves
	// expectedCurrentTurnID unset. Keep that SQL authority boundary intact.
	return s.resumeDetachedClarificationWithPrompt(ctx, data, false, promptTaskOptions{})
}

func (s *Service) resumeDetachedClarificationWithPrompt(
	ctx context.Context,
	data clarificationAnsweredData,
	dispatchOnly bool,
	options promptTaskOptions,
) error {
	if data.SessionID == "" || data.TaskID == "" {
		return errors.New("detached clarification resume missing session_id or task_id")
	}
	if err := s.authorizeTaskSessionPair(ctx, data.TaskID, data.SessionID); err != nil {
		return err
	}
	if options.deliverySubmissionID == "" {
		options.deliverySubmissionID = "clarification:" + uuid.NewString()
	}
	// A clarification answer continues an already-accepted turn.
	options.capacityIntent = turnCapacityOverride
	prompt := buildClarificationPrompt(data)

	s.logger.Info("resuming agent with clarification answer",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.Bool("rejected", data.Rejected))

	// The primary MCP request can keep the session RUNNING while the task was
	// moved to REVIEW for the pending question. Reassert runtime ownership at
	// the answer boundary; terminal and archived sessions remain protected by
	// reconcileTaskStateForRuntime.
	s.writeTaskInProgressForRuntime(ctx, data.TaskID, data.SessionID)

	if _, err := s.promptTask(
		ctx, data.TaskID, data.SessionID, prompt, "", false,
		clarificationAnswerAttachmentsV1(data.Attachments), dispatchOnly, launchOriginManual, options,
	); err != nil {
		// The synchronous HTTP path must not turn an asynchronous queue handoff
		// into false acknowledgement. Its handler restores the claimed bundle on
		// any error so the user can retry. Event/watchdog recovery keeps the
		// existing cancel-and-queue fallback.
		if !dispatchOnly && s.retryClarificationAfterCancel(ctx, data, prompt, err) {
			return nil
		}
		return fmt.Errorf("prompt task with clarification answer: %w", err)
	}
	return nil
}

func (s *Service) handleClarificationPrimaryAnswered(ctx context.Context, event *bus.Event) error {
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
	if data.HandledInline {
		return nil
	}
	return s.handleClarificationPrimaryAnsweredData(ctx, data)
}

func (s *Service) handleClarificationPrimaryAnsweredData(ctx context.Context, data clarificationAnsweredData) error {
	if data.SessionID == "" || data.TaskID == "" || data.PendingID == "" {
		return nil
	}

	// A directly answered MCP request does not transition the session through
	// WAITING_FOR_INPUT, so no ordinary turn-start event exists to move a task
	// out of REVIEW. The active runtime still owns that projection.
	s.writeTaskInProgressForRuntime(ctx, data.TaskID, data.SessionID)
	s.scheduleClarificationWatchdog(data)
	return nil
}

// HandleClarificationPrimaryAnswered is the synchronous local notification
// used by clarification.Resolver to arm the watchdog before releasing a live
// clarification waiter. The bus event remains available for projections.
func (s *Service) HandleClarificationPrimaryAnswered(ctx context.Context, answered clarification.PrimaryAnswered) {
	_ = s.handleClarificationPrimaryAnsweredData(ctx, clarificationAnsweredData{
		SessionID:           answered.SessionID,
		TaskID:              answered.TaskID,
		PendingID:           answered.PendingID,
		ClarificationTurnID: answered.ClarificationTurnID,
		Question:            answered.Question,
		AnswerText:          answered.AnswerText,
		Attachments:         answered.Attachments,
		Rejected:            answered.Rejected,
		RejectReason:        answered.RejectReason,
	})
}

func (s *Service) clarificationWatchdogKey(sessionID, pendingID string) string {
	return sessionID + "::" + pendingID
}

func (s *Service) loadClarificationWatchdogEntry(sessionID, pendingID string) *clarificationWatchdogEntry {
	if sessionID == "" || pendingID == "" {
		return nil
	}
	value, ok := s.clarificationWatchdogs.Load(s.clarificationWatchdogKey(sessionID, pendingID))
	if !ok {
		return nil
	}
	entry, ok := value.(*clarificationWatchdogEntry)
	if !ok {
		return nil
	}
	return entry
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
		current, ok := s.clarificationWatchdogs.Load(key)
		if !ok || current != entry {
			return
		}
		defer s.clarificationWatchdogs.CompareAndDelete(key, entry)
		if entry.cancel != nil {
			defer entry.cancel()
		}
		s.resumeClarificationViaFallback(watchCtx, data)
	}
}

func (s *Service) resumeClarificationViaFallback(ctx context.Context, data clarificationAnsweredData) {
	prompt := buildClarificationPrompt(data)
	s.logger.Warn("clarification resume watchdog expired; triggering fallback resume",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("pending_id", data.PendingID))

	if !s.clarificationTurnStillCurrent(ctx, data) {
		return
	}
	if _, err := s.promptTask(
		ctx,
		data.TaskID,
		data.SessionID,
		prompt,
		"",
		false,
		clarificationAnswerAttachmentsV1(data.Attachments),
		false,
		launchOriginAutomatic,
		promptTaskOptions{
			capacityIntent:        turnCapacityOverride,
			expectedCurrentTurnID: data.ClarificationTurnID,
			deliverySubmissionID:  "clarification:" + uuid.NewString(),
		},
	); err != nil {
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
	guard := s.lockCancelInFlightGuard(data.SessionID)
	defer guard.release()

	// Coordinator stop may have won while this recovery waited for the shared
	// guard. Re-read inside the critical section and never revive a terminal
	// session or queue replacement work for it.
	session, sessionErr := s.repo.GetTaskSession(ctx, data.SessionID)
	if sessionErr != nil || session == nil {
		s.logger.Warn("cannot confirm live session for clarification recovery",
			zap.String("session_id", data.SessionID),
			zap.Error(sessionErr))
		return false
	}
	if isTerminalSessionState(session.State) {
		s.logger.Debug("skipping clarification recovery for terminal session",
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return false
	}
	if !s.clarificationTurnStillCurrent(ctx, data) {
		return false
	}

	watchdogEntry := s.loadClarificationWatchdogEntry(data.SessionID, data.PendingID)
	if watchdogEntry != nil {
		watchdogEntry.beginRecoveryCancellation()
	}
	expectedTurnID := data.ClarificationTurnID
	cancelErr := s.cancelAgentSilentExpectedWithGuard(
		ctx,
		data.TaskID,
		data.SessionID,
		&expectedTurnID,
		guard.unlock,
		guard.relockWithContext,
	)
	if watchdogEntry != nil {
		watchdogEntry.endRecoveryCancellation()
	}
	if !guard.locked {
		s.logger.Warn("clarification recovery lost the cancellation guard; skipping recovery",
			zap.String("session_id", data.SessionID),
			zap.Error(cancelErr))
		return false
	}
	if cancelErr != nil {
		s.logger.Warn("cancel failed (agent likely dead), force-transitioning session state",
			zap.String("session_id", data.SessionID),
			zap.Error(cancelErr))
		// Revert through the terminal-safe state writer. Production uses a
		// compare-and-set, and the shared guard keeps coordinator cancellation
		// outside this narrow mutation.
		reverted := s.updateTaskSessionState(
			ctx,
			data.TaskID,
			data.SessionID,
			models.TaskSessionStateWaitingForInput,
			"",
			true,
			session,
		)
		if reverted == nil || reverted.State != models.TaskSessionStateWaitingForInput {
			s.logger.Error("failed to force-revert session state for clarification recovery",
				zap.String("session_id", data.SessionID))
			return false
		}
		s.completeTurnForSession(ctx, data.SessionID)
	}
	// The joined cancellation may have been owned by the user-facing cancel
	// path. Re-read after the shared operation settles before queueing a
	// clarification replacement; a stale pre-cancel RUNNING snapshot must not
	// revive a session that the explicit source already marked terminal.
	session, sessionErr = s.repo.GetTaskSession(ctx, data.SessionID)
	if sessionErr != nil || session == nil {
		s.logger.Debug("cannot confirm session after clarification cancellation",
			zap.String("session_id", data.SessionID),
			zap.Error(sessionErr))
		return false
	}
	if isTerminalSessionState(session.State) {
		s.logger.Debug("skipping clarification replacement for terminal session after cancellation",
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return false
	}
	if !s.clarificationTurnStillCurrentAfterRecovery(ctx, data) {
		return false
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
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, data.TaskID, data.SessionID)
	if err != nil {
		return fmt.Errorf("resolve clarification resume session: %w", err)
	}
	queued, err := s.messageQueue.QueueMessageWithMetadataForSession(
		ctx, identity, prompt, "", messagequeue.QueuedByAgent, false,
		clarificationAnswerAttachmentsQueue(data.Attachments),
		map[string]interface{}{
			metaKeyUserMessageRecorded: true,
			metaKeyCapacityOverride:    true,
		},
	)
	if err != nil {
		return fmt.Errorf("queue clarification resume prompt: %w", err)
	}
	// Queue insertion is observable even when a concurrent drain prevents the
	// targeted take, so publish before attempting dispatch.
	s.publishQueueStatusEvent(ctx, data.SessionID)
	dispatched, err := s.takeAndDispatchEntryLocked(ctx, identity, queued.ID)
	if err != nil {
		return fmt.Errorf("dispatch clarification resume prompt: %w", err)
	}
	if !dispatched {
		return errClarificationResumeQueuedForDrain
	}
	return nil
}

func (s *Service) cancelClarificationWatchdogsForSession(
	sessionID, reason string,
	payload *lifecycle.AgentStreamEventPayload,
) {
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
		// Silent cancellation may synchronously emit a frame for the captured
		// execution/prompt. Ignore only that operation-owned identity. A newer
		// execution or prompt generation remains authoritative and cancels the
		// watchdog even while the fallback cancellation is blocked.
		if entry, ok := value.(*clarificationWatchdogEntry); ok &&
			entry.isRecoveryCancellationActive() &&
			s.clarificationRecoveryOwnsStreamEvent(sessionID, payload) {
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

func (s *Service) clarificationRecoveryOwnsStreamEvent(
	sessionID string,
	payload *lifecycle.AgentStreamEventPayload,
) bool {
	if payload == nil || payload.Data == nil {
		return false
	}
	operation := s.currentCancellation(sessionID)
	if operation == nil || operation.kind != cancellationKindSilent {
		return false
	}
	if !clarificationRecoveryCancellationFrame(payload) {
		// Message, thinking, and tool frames are always live activity. Even an
		// exact execution/generation match cannot prove that those frames came
		// from the cancellation rather than the agent's normal work.
		return false
	}
	identity, ready := s.cancellationIdentitySnapshot(operation)
	if !ready || identity.executionID == "" || identity.promptGeneration == 0 {
		// Recovery may ignore a frame only when both immutable parts of the
		// cancellation identity are present on the frame. Missing identity is
		// independent activity by definition, not evidence of cancellation.
		return false
	}
	eventExecutionID := payload.ExecutionID
	if eventExecutionID == "" {
		eventExecutionID = payload.AgentID
	}
	return eventExecutionID != "" &&
		eventExecutionID == identity.executionID &&
		payload.Data.PromptGeneration == identity.promptGeneration
}

func clarificationRecoveryCancellationFrame(payload *lifecycle.AgentStreamEventPayload) bool {
	if payload == nil || payload.Data == nil {
		return false
	}
	switch payload.Data.Type {
	case "session_info", "session_status":
		return true
	case agentEventComplete:
		return extractStopReason(payload) == stopReasonCancelled
	default:
		return false
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

func clarificationAnswerAttachmentsV1(
	attachments []models.ClarificationAnswerAttachment,
) []v1.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]v1.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, v1.MessageAttachment{
			Type:         attachment.Type,
			AttachmentID: attachment.AttachmentID,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: "path",
		})
	}
	return out
}

func clarificationAnswerAttachmentsQueue(
	attachments []models.ClarificationAnswerAttachment,
) []messagequeue.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]messagequeue.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, messagequeue.MessageAttachment{
			Type:         attachment.Type,
			AttachmentID: attachment.AttachmentID,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: "path",
		})
	}
	return out
}
