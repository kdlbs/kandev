package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
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

const defaultTransientRetryNoticeFenceTTL = 5 * time.Minute

// recoverActionCancelRetry is the session.recover action that stops an
// in-progress transient retry loop and surfaces manual recovery.
const recoverActionCancelRetry = "cancel_retry"

const recoveryCancelRetryButtonTestID = "recovery-cancel-retry-button"

// Shared status-message metadata keys. Defined as constants because the same
// keys are built in more than one place in this package (recovery + retry
// status messages), which otherwise trips goconst on new code.
const (
	metaKeyVariant         = "variant"
	metaKeySessionID       = "session_id"
	metaKeyTaskID          = "task_id"
	metaKeyAgentID         = "agent_id"
	metaKeyNewState        = "new_state"
	metaKeyAgentProfileID  = "agent_profile_id"
	metaKeyUpdatedAt       = "updated_at"
	metaKeyExecutorProfile = "executor_profile_id"
	metaKeyWorkflowStepID  = "workflow_step_id"
	metaKeyPrompt          = "prompt"
	metaKeyPlanMode        = "plan_mode"
	metaKeyAttachments     = "attachments"
)

// metaVariantWarning is the status-message variant that drives the frontend's
// yellow (non-alarming) styling, as opposed to the red "error" variant.
const metaVariantWarning = "warning"

// metaVariantCeiling is the status-message variant AC-49 requires for every
// session-ceiling card note, including AC-17c's drop note.
const metaVariantCeiling = "ceiling"

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
	onAccepted  func(turnID string)
}

// transientRetryEntry tracks one session's in-progress retry loop: the current
// attempt count and the cancel func for the armed backoff timer.
type transientRetryEntry struct {
	attempt            int
	mode               string
	continuationPolicy continuationPolicy
	continuation       *continuationBinding
	providerID         string
	modelID            string
	cancel             func()
	retryCtx           context.Context
	mu                 sync.Mutex
	claimed            bool
	armed              bool
	started            int
	predecessorStopped bool
	acceptedExecution  string
	acceptedGeneration uint64
	restoredExecution  string
	retainedRuntime    *retainedRuntimeRetry
}

type retainedRuntimeRetry struct {
	executionID string
	generation  uint64
	profileID   string
	nativeID    string
	identity    [32]byte
	failure     watcher.AgentEventData
}

type retainedRuntimeRetryDisposition uint8

const (
	retainedRuntimeRetryLost retainedRuntimeRetryDisposition = iota
	retainedRuntimeRetryUsable
	retainedRuntimeRetryBlocked
)

func (e *transientRetryEntry) claim() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.claimed {
		return false
	}
	e.claimed = true
	return true
}

func (e *transientRetryEntry) arm() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.armed {
		return false
	}
	e.armed = true
	return true
}

// rememberTurnPrompt caches the raw outbound prompt so a transient retry can
// re-drive the same turn without the original caller's context.
func (s *Service) rememberTurnPrompt(sessionID, text, model string, planMode bool, attachments []v1.MessageAttachment) {
	s.rememberTurnPromptWithAccepted(sessionID, text, model, planMode, attachments, nil)
}

func (s *Service) rememberTurnPromptWithAccepted(
	sessionID, text, model string, planMode bool, attachments []v1.MessageAttachment,
	onAccepted func(turnID string),
) {
	if sessionID == "" {
		return
	}
	s.lastTurnPrompt.Store(sessionID, capturedPrompt{
		text:        text,
		model:       model,
		planMode:    planMode,
		attachments: attachments,
		onAccepted:  onAccepted,
	})
}

// handleTransientFailure routes a high-confidence short provider error
// into a paced, visible retry-with-backoff instead of the red recovery banner.
// Returns true when it takes ownership (caller must NOT fall through to
// handleRecoverableFailure); false for non-transient errors, office tasks,
// or an exhausted retry budget.
func (s *Service) handleTransientFailure(ctx context.Context, data watcher.AgentEventData) bool {
	// Dynamic profiles own both error classes and their retry/reset policy. The
	// legacy Kanban retry ladder must not consume a configured dynamic retry
	// budget before the shared evaluator sees the failure.
	if data.DynamicRouteAttempt {
		return false
	}
	if data.SessionID == "" {
		return false
	}

	noticeState, releaseNoticeState := s.acquireTransientRetryNoticeState(data.SessionID)
	noticeState.mu.Lock()
	if noticeState.retired.Load() {
		noticeState.mu.Unlock()
		releaseNoticeState()
		s.logger.Debug("ignoring transient failure after retry lifecycle was retired",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return true
	}
	data = s.withPromptAttemptEvidenceLocked(data)
	mode := recoveryModeReplay
	var binding *continuationBinding
	previous, _ := s.transientRetries.Load(data.SessionID)
	previousEntry, _ := previous.(*transientRetryEntry)
	continuing := previousEntry != nil && previousEntry.mode == recoveryModeContinue
	if continuing || !s.promptAttemptPreResultSafe(data) {
		binding = s.continuationBindingForFailure(ctx, data)
		if continuing && binding != nil && *binding != *previousEntry.continuation {
			binding = nil
		}
		if binding != nil {
			mode = recoveryModeContinue
		}
	}
	if data.DynamicRouteAttempt || ((continuing || !s.promptAttemptPreResultSafe(data)) && binding == nil) {
		noticeState.mu.Unlock()
		releaseNoticeState()
		s.logger.Debug("refusing automatic transient retry without safe prompt-attempt evidence",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID),
			zap.Uint64("prompt_generation", data.PromptGeneration))
		return false
	}
	classified := classifyKanbanFailure(data)
	if routingerr.Decide(routingerr.ContextKanban, classified, time.Now().UTC()) != routingerr.DecisionShortRetry {
		noticeState.mu.Unlock()
		releaseNoticeState()
		return false
	}
	// Genuine Office-owned tasks render their own structured error UI. Keep them
	// on the existing path rather than the Kanban-style yellow retry card.
	if s.isOfficeTask(ctx, data.TaskID) {
		noticeState.mu.Unlock()
		releaseNoticeState()
		return false
	}
	attempt := s.nextTransientAttemptLocked(data.SessionID)
	if attempt > transientMaxAttempts {
		noticeState.mu.Unlock()
		releaseNoticeState()
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

	data.RecoveryMode = mode
	retainedRuntime := s.retainedRuntimeRetryForFailure(ctx, data)
	// Emit the yellow status (against the failed turn) before completing it.
	s.createTransientRetryStatusMessageLocked(noticeState, ctx, data, classified, attempt, delay, retryAt)
	// Reserve the next timer while the notice lifecycle is still serialized. A
	// concurrent failure can replace this reservation, but cannot arm it until
	// its own failed turn has been parked.
	entry := s.reserveTransientRetryWithMetadataLocked(noticeState, data.SessionID, attempt, func(entry *transientRetryEntry) {
		entry.mode = mode
		entry.continuation = binding
		if binding != nil {
			entry.continuationPolicy = binding.policy
		}
		entry.providerID = data.AgentID
		if data.ProviderError != nil {
			if data.ProviderError.ProviderID != "" {
				entry.providerID = data.ProviderError.ProviderID
			}
			entry.modelID = data.ProviderError.ModelID
		}
		entry.retainedRuntime = retainedRuntime
	})
	noticeState.mu.Unlock()
	releaseNoticeState()

	if mode == recoveryModeContinue {
		if err := s.settleContinuationInterruption(ctx, data); err != nil {
			if entry != nil {
				cleanup := s.settleContinuationFailureLocked(ctx, data, entry)
				go cleanup(context.WithoutCancel(ctx))
			}
			return true
		}
		s.lastTurnPrompt.Delete(data.SessionID)
	} else {
		s.reconcileCIAutoFixTurnBeforeCompletion(ctx, data.TaskID, data.SessionID, "")
		s.completeTurnForSession(ctx, data.SessionID)
	}

	// Park the session in WAITING_FOR_INPUT (a calm, banner-less state that
	// also lets the yellow retry card render — ActionMessage hides itself while
	// the session is RUNNING). Deliberately NOT FAILED and NOT task→REVIEW.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)

	// Parking must complete before a zero-delay retry can dispatch. Reacquiring
	// the notice mutex also lets cancellation or a later failure replace this
	// reservation before it is armed.
	if entry != nil {
		state, release := s.acquireTransientRetryNoticeState(data.SessionID)
		state.mu.Lock()
		if current, ok := s.transientRetries.Load(data.SessionID); ok && current == entry && !state.retired.Load() {
			s.armTransientRetryEntryLocked(data.TaskID, data.SessionID, data.AgentExecutionID, entry, delay)
		}
		state.mu.Unlock()
		release()
	}

	return true
}

// nextTransientAttempt returns the next 1-based attempt number for a session,
// cancelling any still-armed timer from a prior attempt.
func (s *Service) nextTransientAttempt(sessionID string) int {
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	if state == nil {
		return 1
	}
	state.mu.Lock()
	attempt := s.nextTransientAttemptLocked(sessionID)
	state.mu.Unlock()
	release()
	return attempt
}

func (s *Service) nextTransientAttemptLocked(sessionID string) int {
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
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	if state == nil {
		return
	}
	state.mu.Lock()
	s.scheduleTransientRetryWithMetadataLocked(state, taskID, sessionID, execID, attempt, delay, retryAt, classified)
	state.mu.Unlock()
	release()
}

func (s *Service) scheduleTransientRetryWithMetadataLocked(
	state *transientRetryNoticeState,
	taskID, sessionID, execID string,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
	classified *routingerr.Error,
) {
	entry := s.reserveTransientRetryWithMetadataLocked(state, sessionID, attempt, nil)
	if entry != nil {
		s.armTransientRetryEntryLocked(taskID, sessionID, execID, entry, delay)
	}
}

func (s *Service) reserveTransientRetryWithMetadataLocked(
	state *transientRetryNoticeState,
	sessionID string,
	attempt int,
	initialize func(*transientRetryEntry),
) *transientRetryEntry {
	if state.retired.Load() {
		return nil
	}
	retryCtx, cancel := context.WithCancel(context.Background())
	entry := &transientRetryEntry{attempt: attempt, cancel: cancel, retryCtx: retryCtx}
	if previous, ok := s.transientRetries.Load(sessionID); ok {
		if previous, ok := previous.(*transientRetryEntry); ok {
			previous.mu.Lock()
			entry.started = previous.started
			entry.mode, entry.continuationPolicy, entry.continuation = previous.mode, previous.continuationPolicy, previous.continuation
			entry.providerID, entry.modelID = previous.providerID, previous.modelID
			previous.mu.Unlock()
		}
	}
	if initialize != nil {
		initialize(entry)
	}
	state.owned.Store(true)
	s.transientRetries.Store(sessionID, entry)
	return entry
}

func (s *Service) armTransientRetryEntryLocked(
	taskID, sessionID, execID string,
	entry *transientRetryEntry,
	delay time.Duration,
) {
	if entry == nil || !entry.arm() {
		return
	}
	go s.runTransientRetry(entry.retryCtx, taskID, sessionID, execID, entry, delay)
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
	var retryEntry *transientRetryEntry
	if value, ok := s.transientRetries.Load(sessionID); ok {
		if entry, ok := value.(*transientRetryEntry); ok {
			retryEntry = entry
			if entry.mode == recoveryModeContinue {
				s.retryInterruptedContinuation(ctx, taskID, sessionID, execID, entry)
				return
			}
		}
	}
	v, ok := s.lastTurnPrompt.Load(sessionID)
	if !ok {
		if ctx.Err() != nil {
			return
		}
		if retryEntry != nil && retryEntry.retainedRuntime != nil {
			s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, retryEntry, "refused")
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
	if retryEntry != nil && retryEntry.retainedRuntime != nil {
		switch s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, retryEntry) {
		case retainedRuntimeRetryUsable:
			s.retryRetainedRuntimePrompt(ctx, taskID, sessionID, retryEntry, cp)
			return
		case retainedRuntimeRetryBlocked:
			s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, retryEntry, "refused")
			return
		}
	}
	if retryEntry != nil {
		retryEntry.mu.Lock()
		retryEntry.started++
		retryEntry.mu.Unlock()
	}
	initialCreatePromptPassthrough := false
	if session, sessionErr := s.repo.GetTaskSession(ctx, sessionID); sessionErr == nil && session != nil {
		_, initialCreatePromptPassthrough = s.hydrateInitialCreatePromptPassthrough(session)
	}

	if execID != "" {
		if !s.claimForcedExecutionCleanup(sessionID, execID) {
			s.logger.Debug("skipping transient retry because execution teardown is already owned",
				zap.String("session_id", sessionID),
				zap.String("execution_id", execID))
			s.resetTransientRetry(sessionID)
			return
		}
		claim, claimed := s.executionTeardownClaimFor(sessionID, execID)
		if err := s.stopTransientRetryExecution(ctx, execID); err != nil && !agentruntime.IsNotFound(err) {
			s.logger.Debug("failed to stop failed execution before transient retry",
				zap.String("session_id", sessionID),
				zap.String("execution_id", execID),
				zap.Error(err))
		} else if claimed {
			s.completeExecutionTeardownClaim(sessionID, execID, claim)
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

	if _, err := s.promptTask(ctx, taskID, sessionID, cp.text, cp.model, cp.planMode, cp.attachments, false, launchOriginAutomatic, promptTaskOptions{
		onAccepted:                     cp.onAccepted,
		initialCreatePromptPassthrough: initialCreatePromptPassthrough,
	}); err != nil {
		if ctx.Err() != nil {
			return
		}
		var retainedFailure *agentruntime.RetainedPromptFailureError
		if errors.As(err, &retainedFailure) {
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
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForExecution(executionID)
	}
	return s.executor.StopExecution(stopCtx, executionID, "transient retry: relaunching agent", true)
}

// createTransientRetryStatusMessage emits the calm yellow "retrying" status
// (variant=warning) with a Cancel action, driving the frontend's
// AgentWarningStatus instead of the red AgentErrorStatus.
