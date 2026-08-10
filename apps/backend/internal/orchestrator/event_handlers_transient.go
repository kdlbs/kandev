package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// transientMaxAttempts caps how many times a high-confidence short provider
// failure is auto-retried with backoff before falling through to the manual
// recovery banner.
const transientMaxAttempts = 5

const transientRetryStopTimeout = 30 * time.Second

// recoverActionCancelRetry is the session.recover action that stops an
// in-progress transient retry loop and surfaces manual recovery.
const recoverActionCancelRetry = "cancel_retry"

const recoveryCancelRetryButtonTestID = "recovery-cancel-retry-button"

// Shared status-message metadata keys. Defined as constants because the same
// keys are built in more than one place in this package (recovery + retry
// status messages), which otherwise trips goconst on new code.
const (
	metaKeyVariant        = "variant"
	metaKeySessionID      = "session_id"
	metaKeyTaskID         = "task_id"
	metaKeyNewState       = "new_state"
	metaKeyAgentProfileID = "agent_profile_id"
	metaKeyUpdatedAt      = "updated_at"
)

// metaVariantWarning is the status-message variant that drives the frontend's
// yellow (non-alarming) styling, as opposed to the red "error" variant.
const metaVariantWarning = "warning"

// transientRetryBackoff is the per-attempt delay before re-driving a turn that
// failed transiently. Index is attempt-1 (5s → 10s → 20s → 40s → 60s).
var transientRetryBackoff = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	40 * time.Second,
	60 * time.Second,
}

// transientRetryDelay returns the backoff for a 1-based attempt, clamping to
// the longest step so an over-count never panics.
func transientRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(transientRetryBackoff) {
		attempt = len(transientRetryBackoff)
	}
	return transientRetryBackoff[attempt-1]
}

// transientRetryDelayFor honors a validated provider reset deadline when it
// is close enough to be useful. Longer or stale hints fall back to the stable
// local ladder so a provider cannot keep the session parked indefinitely.
func transientRetryDelayFor(classified *routingerr.Error, attempt int, now time.Time) time.Duration {
	if classified != nil && classified.ResetHint != nil && !classified.ResetHint.Before(now) {
		untilReset := classified.ResetHint.Sub(now)
		if untilReset <= time.Minute {
			return untilReset
		}
	}
	return transientRetryDelay(attempt)
}

// capturedPrompt is the minimal context needed to re-drive a failed turn.
type capturedPrompt struct {
	text        string
	model       string
	planMode    bool
	attachments []v1.MessageAttachment
}

// transientRetryEntry tracks one session's in-progress retry loop: the current
// attempt count and the cancel func for the armed backoff timer.
type transientRetryEntry struct {
	attempt int
	cancel  func()
	mu      sync.Mutex
	claimed bool
}

func (e *transientRetryEntry) claim() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.claimed {
		return false
	}
	e.claimed = true
	return true
}

// rememberTurnPrompt caches the raw outbound prompt so a transient retry can
// re-drive the same turn without the original caller's context.
func (s *Service) rememberTurnPrompt(sessionID, text, model string, planMode bool, attachments []v1.MessageAttachment) {
	if sessionID == "" {
		return
	}
	s.lastTurnPrompt.Store(sessionID, capturedPrompt{
		text:        text,
		model:       model,
		planMode:    planMode,
		attachments: attachments,
	})
}

// handleTransientFailure routes a high-confidence short provider error
// into a paced, visible retry-with-backoff instead of the red recovery banner.
// Returns true when it takes ownership (caller must NOT fall through to
// handleRecoverableFailure); false for non-transient errors, office tasks,
// or an exhausted retry budget.
func (s *Service) handleTransientFailure(ctx context.Context, data watcher.AgentEventData) bool {
	classified := classifyKanbanFailure(data)
	if data.SessionID == "" || routingerr.Decide(routingerr.ContextKanban, classified, time.Now().UTC()) != routingerr.DecisionShortRetry {
		return false
	}
	// Genuine Office-owned tasks render their
	// own structured error UI — keep them on the existing path rather than the
	// kanban-style yellow retry card. The canonical task projection avoids
	// treating ordinary Kanban tasks with an assigned profile as Office tasks.
	if s.isOfficeTask(ctx, data.TaskID) {
		return false
	}

	attempt := s.nextTransientAttempt(data.SessionID)
	if attempt > transientMaxAttempts {
		s.logger.Warn("transient retry budget exhausted; falling through to recovery banner",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Int("attempts", attempt-1))
		s.resetTransientRetry(data.SessionID)
		return false
	}

	now := time.Now().UTC()
	delay := transientRetryDelayFor(classified, attempt, now)
	retryAt := now.Add(delay)
	s.logger.Info("scheduling transient provider-error retry",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.Int("attempt", attempt),
		zap.Int("max_attempts", transientMaxAttempts),
		zap.Duration("delay", delay))

	// Emit the yellow status (against the failed turn) before completing it.
	s.createTransientRetryStatusMessage(ctx, data, classified, attempt, delay, retryAt)
	s.completeTurnForSession(ctx, data.SessionID)

	// Park the session in WAITING_FOR_INPUT (a calm, banner-less state that
	// also lets the yellow retry card render — ActionMessage hides itself while
	// the session is RUNNING). Deliberately NOT FAILED and NOT task→REVIEW.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)

	s.scheduleTransientRetryWithMetadata(
		data.TaskID,
		data.SessionID,
		data.AgentExecutionID,
		attempt,
		delay,
		retryAt,
		classified,
	)
	return true
}

// nextTransientAttempt returns the next 1-based attempt number for a session,
// cancelling any still-armed timer from a prior attempt.
func (s *Service) nextTransientAttempt(sessionID string) int {
	prev := 0
	if v, ok := s.transientRetries.Load(sessionID); ok {
		if entry, ok := v.(*transientRetryEntry); ok {
			prev = entry.attempt
			if entry.cancel != nil {
				entry.cancel()
			}
		}
	}
	return prev + 1
}

// scheduleTransientRetry stores a fresh retry entry and arms its backoff timer.
func (s *Service) scheduleTransientRetry(taskID, sessionID, execID string, attempt int, delay time.Duration) {
	s.scheduleTransientRetryWithMetadata(taskID, sessionID, execID, attempt, delay, time.Now().UTC().Add(delay), nil)
}

func (s *Service) scheduleTransientRetryWithMetadata(
	taskID, sessionID, execID string,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
	classified *routingerr.Error,
) {
	retryCtx, cancel := context.WithCancel(context.Background())
	entry := &transientRetryEntry{attempt: attempt, cancel: cancel}
	s.transientRetries.Store(sessionID, entry)
	go s.runTransientRetry(retryCtx, taskID, sessionID, execID, entry, delay)
}

// runTransientRetry waits out the backoff (or cancellation) then re-drives the
// turn. Mirrors the clarification-watchdog goroutine ownership pattern.
func (s *Service) runTransientRetry(retryCtx context.Context, taskID, sessionID, execID string, entry *transientRetryEntry, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-retryCtx.Done():
		return
	case <-timer.C:
		// Only fire if this entry is still the active one for the session.
		if cur, ok := s.transientRetries.Load(sessionID); !ok || cur != entry || !entry.claim() {
			return
		}
		s.retryTransientPrompt(retryCtx, taskID, sessionID, execID)
	}
}

// retryTransientPrompt re-drives the failed turn after backoff. The failed
// execution is torn down first so PromptTask's ensureSessionRunning resumes a
// fresh agent via the resume token (re-establishing the ACP session) rather
// than reusing the FAILED execution, which rejects prompts. The session was
// parked in WAITING_FOR_INPUT by handleTransientFailure so PromptTask accepts
// the re-send straight away.
func (s *Service) retryTransientPrompt(ctx context.Context, taskID, sessionID, execID string) {
	if ctx.Err() != nil {
		return
	}
	v, ok := s.lastTurnPrompt.Load(sessionID)
	if !ok {
		if ctx.Err() != nil {
			return
		}
		// No prompt to re-drive (e.g. an uncached launch path). Don't leave the
		// retry loop parked behind a stuck yellow card — clear it and surface
		// the manual recovery banner so the user can resume or start fresh.
		s.logger.Warn("transient retry has no cached prompt; surfacing recovery banner",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		s.resetTransientRetry(sessionID)
		s.handleRecoverableFailure(context.Background(), watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: execID,
			ErrorMessage:     "Automatic provider retry was not possible. Resume or start fresh to continue.",
		})
		return
	}
	cp, _ := v.(capturedPrompt)

	if execID != "" {
		if !s.claimForcedExecutionCleanup(sessionID, execID) {
			s.logger.Debug("skipping transient retry because execution teardown is already owned",
				zap.String("session_id", sessionID),
				zap.String("execution_id", execID))
			s.resetTransientRetry(sessionID)
			return
		}
		if err := s.stopTransientRetryExecution(ctx, execID); err != nil {
			s.logger.Debug("failed to stop failed execution before transient retry",
				zap.String("session_id", sessionID),
				zap.String("execution_id", execID),
				zap.Error(err))
		}
		// handleAgentFailed terminal-marked this exact execution before the
		// retry was scheduled, so no later frame may reclaim activity even when
		// runtime teardown times out. Retirement is safe and idempotent on both
		// stop outcomes.
		s.retireExecutionActivityAndPublish(
			context.WithoutCancel(ctx),
			taskID,
			sessionID,
			execID,
		)
	}
	if ctx.Err() != nil {
		return
	}

	if _, err := s.PromptTask(ctx, taskID, sessionID, cp.text, cp.model, cp.planMode, cp.attachments, false); err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger.Error("transient retry prompt failed synchronously; surfacing recovery banner",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		s.resetTransientRetry(sessionID)
		s.handleRecoverableFailure(context.Background(), watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: execID,
			ErrorMessage:     "Automatic provider retry could not be started. Resume or start fresh to continue.",
		})
	}
}

func (s *Service) stopTransientRetryExecution(ctx context.Context, executionID string) error {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), transientRetryStopTimeout)
	defer cancel()
	return s.executor.StopExecution(stopCtx, executionID, "transient retry: relaunching agent", true)
}

// createTransientRetryStatusMessage emits the calm yellow "retrying" status
// (variant=warning) with a Cancel action, driving the frontend's
// AgentWarningStatus instead of the red AgentErrorStatus.
func (s *Service) createTransientRetryStatusMessage(
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
) {
	if s.messageCreator == nil {
		return
	}
	secs := int(delay.Seconds())
	label := transientFailureLabel(classified)
	content := fmt.Sprintf("%s — retrying in %ds (attempt %d/%d)", label, secs, attempt, transientMaxAttempts)
	cancelAction := wsRecoveryAction(data.TaskID, data.SessionID, recoverActionCancelRetry,
		"Cancel", "x", "Stop retrying and choose how to recover", recoveryCancelRetryButtonTestID)
	meta := map[string]interface{}{
		metaKeyVariant:     metaVariantWarning,
		"retrying":         true,
		"attempt":          attempt,
		"max_attempts":     transientMaxAttempts,
		"retry_in_seconds": secs,
		"retry_at":         retryAt.UTC().Format(time.RFC3339Nano),
		metaKeySessionID:   data.SessionID,
		metaKeyTaskID:      data.TaskID,
		"actions":          []map[string]interface{}{cancelAction},
	}
	if classified != nil {
		meta["failure_code"] = string(classified.Code)
	}
	providerID := data.AgentID
	if providerError := data.ProviderError; providerError != nil {
		if providerError.ProviderID != "" {
			providerID = providerError.ProviderID
		}
		if modelID := routingerr.Sanitize(providerError.ModelID); modelID != "" {
			meta["model_id"] = modelID
		}
	}
	if providerID = routingerr.Sanitize(providerID); providerID != "" {
		meta["provider_name"] = providerID
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx,
		data.TaskID,
		content,
		data.SessionID,
		string(v1.MessageTypeStatus),
		s.getActiveTurnID(data.SessionID),
		meta,
		false,
	); err != nil {
		s.logger.Warn("failed to create transient retry status message",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}
}

func classifyKanbanFailure(data watcher.AgentEventData) *routingerr.Error {
	providerID := data.AgentID
	message := data.ErrorMessage
	var resetHint *time.Time
	if providerError := data.ProviderError; providerError != nil {
		if providerError.ProviderID != "" {
			providerID = providerError.ProviderID
		}
		if providerError.Message != "" {
			message = providerError.Message
		}
		resetHint = providerError.ResetAt
	}
	return routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhasePromptSend,
		ProviderID: providerID,
		ResetHint:  resetHint,
		Stderr:     message,
	})
}

func transientFailureLabel(classified *routingerr.Error) string {
	if classified == nil {
		return "Provider temporarily unavailable"
	}
	switch classified.Code {
	case routingerr.CodeModelCapacity:
		return "Model at capacity"
	case routingerr.CodeNetworkUnavailable:
		return "Network unavailable"
	case routingerr.CodeProviderOverloaded:
		return "Provider overloaded"
	case routingerr.CodeRateLimited:
		return "Rate limited"
	default:
		return "Provider temporarily unavailable"
	}
}

func transientFailureExhaustedMessage(classified *routingerr.Error) string {
	condition := "The provider remained unavailable"
	if classified != nil {
		switch classified.Code {
		case routingerr.CodeModelCapacity:
			condition = "The selected model remained at capacity"
		case routingerr.CodeNetworkUnavailable:
			condition = "The network remained unavailable"
		case routingerr.CodeProviderOverloaded:
			condition = "The provider remained overloaded"
		case routingerr.CodeRateLimited:
			condition = "The rate limit remained active"
		case routingerr.CodeProviderUnavailable:
			condition = "The provider remained unavailable"
		}
	}
	return condition + " after several retries. Resume to try again, or start a fresh session."
}

// resetTransientRetry clears a session's retry entry, cancels its timer, and
// drops the cached prompt (which may hold large/sensitive attachment data).
// Called on a successful turn, on cancel, and on exhaustion.
func (s *Service) resetTransientRetry(sessionID string) {
	s.lastTurnPrompt.Delete(sessionID)
	if v, ok := s.transientRetries.LoadAndDelete(sessionID); ok {
		if entry, ok := v.(*transientRetryEntry); ok && entry.cancel != nil {
			entry.cancel()
		}
	}
}

// cancelAllTransientRetries drains every armed retry timer at shutdown.
func (s *Service) cancelAllTransientRetries() {
	s.transientRetries.Range(func(key, _ interface{}) bool {
		if keyStr, ok := key.(string); ok {
			s.resetTransientRetry(keyStr)
		}
		return true
	})
}

// CancelTransientRetry stops an in-progress retry loop (user clicked Cancel)
// and surfaces the manual recovery banner so they can Resume or Start fresh.
// Returns true if a retry loop was active.
func (s *Service) CancelTransientRetry(ctx context.Context, taskID, sessionID string) bool {
	// Reports "nothing to cancel" on denial: the bool return carries no error
	// channel, and a foreign session must not be distinguishable from an idle
	// one. Guard first — resetTransientRetry below mutates retry state.
	//
	// Both IDs: taskID is handed to handleRecoverableFailure, which writes
	// against that task, so the session check alone would leave it free to
	// point at someone else's.
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return false
	}
	_, active := s.transientRetries.Load(sessionID)
	s.resetTransientRetry(sessionID)
	if !active {
		return false
	}
	s.logger.Info("user cancelled transient retry loop",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	execID, _ := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	s.handleRecoverableFailure(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: execID,
		ErrorMessage:     "Automatic provider retries cancelled. Resume or start fresh to continue.",
	})
	return true
}
