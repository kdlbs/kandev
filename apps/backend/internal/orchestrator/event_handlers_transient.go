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

// transientMaxAttempts caps how many times a transient provider error (529
// Overloaded) is auto-retried with backoff before falling through to the
// manual recovery banner.
const transientMaxAttempts = 3

// recoverActionCancelRetry is the session.recover action that stops an
// in-progress transient retry loop and surfaces manual recovery.
const recoverActionCancelRetry = "cancel_retry"

const recoveryCancelRetryButtonTestID = "recovery-cancel-retry-button"

// Shared status-message metadata keys. Defined as constants because the same
// keys are built in more than one place in this package (recovery + retry
// status messages), which otherwise trips goconst on new code.
const (
	metaKeyVariant   = "variant"
	metaKeySessionID = "session_id"
	metaKeyTaskID    = "task_id"
)

// metaVariantWarning is the status-message variant that drives the frontend's
// yellow (non-alarming) styling, as opposed to the red "error" variant.
const metaVariantWarning = "warning"

// transientRetryBackoff is the per-attempt delay before re-driving a turn that
// failed transiently. Index is attempt-1 (5s → 15s → 30s).
var transientRetryBackoff = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
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

// handleTransientFailure routes a transient provider error (529 Overloaded)
// into a paced, visible retry-with-backoff instead of the red recovery banner.
// Returns true when it takes ownership (caller must NOT fall through to
// handleRecoverableFailure); false for non-transient errors, office tasks,
// or an exhausted retry budget.
func (s *Service) handleTransientFailure(ctx context.Context, data watcher.AgentEventData) bool {
	if data.SessionID == "" || !routingerr.IsTransientProviderError(data.ErrorMessage) {
		return false
	}
	// Genuine office tasks (those with an assignee agent profile) render their
	// own structured error UI — keep them on the existing path rather than the
	// kanban-style yellow retry card. Note: we intentionally check the task's
	// assignee, NOT isOfficeSession (session.AgentProfileID), which is also set
	// for ordinary kanban tasks started with an agent profile.
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

	delay := transientRetryDelay(attempt)
	s.logger.Info("scheduling transient provider-error retry",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.Int("attempt", attempt),
		zap.Int("max_attempts", transientMaxAttempts),
		zap.Duration("delay", delay))

	// Emit the yellow status (against the failed turn) before completing it.
	s.createTransientRetryStatusMessage(ctx, data, attempt, delay)
	s.completeTurnForSession(ctx, data.SessionID)

	// Park the session in WAITING_FOR_INPUT (a calm, banner-less state that
	// also lets the yellow retry card render — ActionMessage hides itself while
	// the session is RUNNING). Deliberately NOT FAILED and NOT task→REVIEW.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)

	s.scheduleTransientRetry(data.TaskID, data.SessionID, data.AgentExecutionID, attempt, delay)
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
			ErrorMessage:     "Provider overloaded — automatic retry was not possible. Resume or start fresh to continue.",
		})
		return
	}
	cp, _ := v.(capturedPrompt)

	if execID != "" {
		if err := s.executor.StopExecution(ctx, execID, "transient retry: relaunching agent", true); err != nil {
			s.logger.Debug("failed to stop failed execution before transient retry",
				zap.String("session_id", sessionID),
				zap.String("execution_id", execID),
				zap.Error(err))
		}
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
			ErrorMessage:     "Provider overloaded — automatic retry could not be started. Resume or start fresh to continue.",
		})
	}
}

// createTransientRetryStatusMessage emits the calm yellow "retrying" status
// (variant=warning) with a Cancel action, driving the frontend's
// AgentWarningStatus instead of the red AgentErrorStatus.
func (s *Service) createTransientRetryStatusMessage(ctx context.Context, data watcher.AgentEventData, attempt int, delay time.Duration) {
	if s.messageCreator == nil {
		return
	}
	secs := int(delay.Seconds())
	content := fmt.Sprintf("Provider overloaded — retrying in %ds (attempt %d/%d)", secs, attempt, transientMaxAttempts)
	cancelAction := wsRecoveryAction(data.TaskID, data.SessionID, recoverActionCancelRetry,
		"Cancel", "x", "Stop retrying and choose how to recover", recoveryCancelRetryButtonTestID)
	meta := map[string]interface{}{
		metaKeyVariant:     metaVariantWarning,
		"retrying":         true,
		"attempt":          attempt,
		"max_attempts":     transientMaxAttempts,
		"retry_in_seconds": secs,
		metaKeySessionID:   data.SessionID,
		metaKeyTaskID:      data.TaskID,
		"actions":          []map[string]interface{}{cancelAction},
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
		ErrorMessage:     "Provider overloaded — retries cancelled. Resume or start fresh to continue.",
	})
	return true
}
