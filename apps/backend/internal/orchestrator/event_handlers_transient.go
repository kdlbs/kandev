package orchestrator

import (
	"context"
	"fmt"
	"sort"
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
	attempt  int
	cancel   func()
	retryCtx context.Context
	mu       sync.Mutex
	claimed  bool
	armed    bool
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

func (e *transientRetryEntry) arm() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.armed {
		return false
	}
	e.armed = true
	return true
}

func (s *Service) transientRetryNoticeFenceDuration() time.Duration {
	if s.transientRetryNoticeFenceTTL > 0 {
		return s.transientRetryNoticeFenceTTL
	}
	return defaultTransientRetryNoticeFenceTTL
}

// acquireTransientRetryNoticeState returns the one state object for a
// session. The release function keeps the map entry alive until no caller can
// still hold its mutex, even when a lifecycle operation is waiting behind
// another caller.
func (s *Service) acquireTransientRetryNoticeState(sessionID string) (*transientRetryNoticeState, func()) {
	if sessionID == "" {
		return nil, func() {}
	}

	s.transientRetryNoticeStatesMu.Lock()
	if s.transientRetryNoticeStates == nil {
		s.transientRetryNoticeStates = make(map[string]*transientRetryNoticeState)
	}
	state := s.transientRetryNoticeStates[sessionID]
	if state == nil {
		state = &transientRetryNoticeState{}
		s.transientRetryNoticeStates[sessionID] = state
	}
	state.refs++
	s.transientRetryNoticeStatesMu.Unlock()

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			s.releaseTransientRetryNoticeState(sessionID, state)
		})
	}
	return state, release
}

func (s *Service) releaseTransientRetryNoticeState(sessionID string, state *transientRetryNoticeState) {
	s.transientRetryNoticeStatesMu.Lock()
	if state.refs > 0 {
		state.refs--
	}
	s.reclaimTransientRetryNoticeStateLocked(sessionID, state)
	s.transientRetryNoticeStatesMu.Unlock()
}

func (s *Service) reclaimTransientRetryNoticeStateLocked(sessionID string, state *transientRetryNoticeState) {
	if state.refs != 0 || state.owned.Load() {
		return
	}
	if state.retired.Load() && state.retiredUntil.Load() > time.Now().UnixNano() {
		return
	}
	if current, ok := s.transientRetryNoticeStates[sessionID]; !ok || current != state {
		return
	}
	if state.fenceTimer != nil {
		state.fenceTimer.Stop()
		state.fenceTimer = nil
	}
	delete(s.transientRetryNoticeStates, sessionID)
}

func (s *Service) retireTransientRetryNoticeLocked(sessionID string, state *transientRetryNoticeState) {
	// Callers hold state.mu. Keep this order, state.mu -> statesMu, everywhere
	// that touches the lifecycle map so a waiter cannot observe a replacement
	// state while another caller still owns this state's mutex.
	state.retired.Store(true)
	ttl := s.transientRetryNoticeFenceDuration()
	state.retiredUntil.Store(time.Now().Add(ttl).UnixNano())

	s.transientRetryNoticeStatesMu.Lock()
	if current, ok := s.transientRetryNoticeStates[sessionID]; ok && current == state {
		if state.fenceTimer != nil {
			state.fenceTimer.Stop()
		}
		state.fenceTimer = time.AfterFunc(ttl, func() {
			s.expireTransientRetryNoticeFence(sessionID, state)
		})
	}
	s.transientRetryNoticeStatesMu.Unlock()
}

func (s *Service) expireTransientRetryNoticeFence(sessionID string, state *transientRetryNoticeState) {
	s.transientRetryNoticeStatesMu.Lock()
	if current, ok := s.transientRetryNoticeStates[sessionID]; !ok || current != state {
		s.transientRetryNoticeStatesMu.Unlock()
		return
	}
	state.fenceTimer = nil
	if state.retired.Load() {
		if remaining := time.Until(time.Unix(0, state.retiredUntil.Load())); remaining > 0 {
			state.fenceTimer = time.AfterFunc(remaining, func() {
				s.expireTransientRetryNoticeFence(sessionID, state)
			})
		} else {
			s.reclaimTransientRetryNoticeStateLocked(sessionID, state)
		}
	}
	s.transientRetryNoticeStatesMu.Unlock()
}

func (s *Service) clearTransientRetryNoticeFenceLocked(sessionID string, state *transientRetryNoticeState) {
	state.retired.Store(false)
	state.retiredUntil.Store(0)
	s.transientRetryNoticeStatesMu.Lock()
	if current, ok := s.transientRetryNoticeStates[sessionID]; ok && current == state && state.fenceTimer != nil {
		state.fenceTimer.Stop()
		state.fenceTimer = nil
	}
	s.transientRetryNoticeStatesMu.Unlock()
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
	if data.DynamicRouteAttempt || !s.promptAttemptPreResultSafe(data) {
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

	// Emit the yellow status (against the failed turn) before completing it.
	s.createTransientRetryStatusMessageLocked(noticeState, ctx, data, classified, attempt, delay, retryAt)
	// Reserve the next timer while the notice lifecycle is still serialized. A
	// concurrent failure can replace this reservation, but cannot arm it until
	// its own failed turn has been parked.
	entry := s.reserveTransientRetryWithMetadataLocked(
		noticeState,
		data.TaskID,
		data.SessionID,
		data.AgentExecutionID,
		attempt,
		delay,
		retryAt,
		classified,
	)
	noticeState.mu.Unlock()
	releaseNoticeState()

	s.reconcileCIAutoFixTurnBeforeCompletion(ctx, data.TaskID, data.SessionID, "")
	s.completeTurnForSession(ctx, data.SessionID)

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
	entry := s.reserveTransientRetryWithMetadataLocked(state, taskID, sessionID, execID, attempt, delay, retryAt, classified)
	if entry != nil {
		s.armTransientRetryEntryLocked(taskID, sessionID, execID, entry, delay)
	}
}

func (s *Service) reserveTransientRetryWithMetadataLocked(
	state *transientRetryNoticeState,
	taskID, sessionID, execID string,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
	classified *routingerr.Error,
) *transientRetryEntry {
	if state.retired.Load() {
		return nil
	}
	retryCtx, cancel := context.WithCancel(context.Background())
	entry := &transientRetryEntry{attempt: attempt, cancel: cancel, retryCtx: retryCtx}
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
	state, release := s.acquireTransientRetryNoticeState(data.SessionID)
	if state == nil {
		return
	}
	state.mu.Lock()
	s.createTransientRetryStatusMessageLocked(state, ctx, data, classified, attempt, delay, retryAt)
	state.mu.Unlock()
	release()
}

func (s *Service) createTransientRetryStatusMessageLocked(
	state *transientRetryNoticeState,
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
) {
	if s.messageCreator == nil || state.retired.Load() {
		return
	}
	content, meta := transientRetryStatusMessage(data, classified, attempt, delay, retryAt)
	if s.transientRetryMessages != nil {
		s.updateTransientRetryStatusMessageLocked(ctx, data, content, meta)
		return
	}
	s.createTransientRetryStatusMessageRecord(ctx, data, content, meta)
}

func transientRetryStatusMessage(
	data watcher.AgentEventData,
	classified *routingerr.Error,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
) (string, map[string]interface{}) {
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
	return content, meta
}

func (s *Service) createTransientRetryStatusMessageRecord(
	ctx context.Context,
	data watcher.AgentEventData,
	content string,
	meta map[string]interface{},
) {
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

func (s *Service) updateTransientRetryStatusMessageLocked(
	ctx context.Context,
	data watcher.AgentEventData,
	content string,
	metadata map[string]interface{},
) {
	// state.mu intentionally covers this DB I/O. Retirement and notice writes
	// must serialize so cancellation cannot delete a row that this update then
	// resurrects.
	messages, err := s.transientRetryMessages.ListMessages(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to list transient retry status messages before write",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	notices := transientRetryNotices(messages, data.TaskID, data.SessionID)
	if len(notices) == 0 {
		s.createTransientRetryStatusMessageRecord(ctx, data, content, metadata)
		return
	}

	current := notices[0]
	current.Content = content
	current.Type = models.MessageTypeStatus
	current.Metadata = metadata
	current.RequestsInput = false
	if err := s.transientRetryMessages.UpdateMessage(ctx, current); err != nil {
		s.logger.Warn("failed to update transient retry status message",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("message_id", current.ID),
			zap.Error(err))
		return
	}

	for _, duplicate := range notices[1:] {
		if err := s.transientRetryMessages.DeleteMessage(ctx, duplicate.ID); err != nil {
			s.logger.Warn("failed to delete duplicate transient retry status message",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.String("message_id", duplicate.ID),
				zap.Error(err))
		}
	}
}

func transientRetryNotices(messages []*models.Message, taskID, sessionID string) []*models.Message {
	notices := make([]*models.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil || message.TaskID != taskID || message.TaskSessionID != sessionID ||
			message.Type != models.MessageTypeStatus || message.Metadata == nil {
			continue
		}
		if retrying, ok := message.Metadata["retrying"].(bool); !ok || !retrying {
			continue
		}
		notices = append(notices, message)
	}
	sort.SliceStable(notices, func(i, j int) bool {
		if notices[i].CreatedAt.Equal(notices[j].CreatedAt) {
			return notices[i].ID > notices[j].ID
		}
		return notices[i].CreatedAt.After(notices[j].CreatedAt)
	})
	return notices
}

func classifyKanbanFailure(data watcher.AgentEventData) *routingerr.Error {
	providerID := data.AgentID
	message := data.ErrorMessage
	var resetHint *time.Time
	if providerError := data.ProviderError; providerError != nil {
		// Provider rules are keyed by agent ID. OpenCode diagnostics carry the
		// model-provider ID instead ("opencode-go"), which has no rules; keeping
		// the agent ID there lets the OpenCode usage-limit rule classify the
		// failure so dynamic routing can advance to the next candidate.
		if id := providerError.ProviderID; id != "" &&
			!routingerr.HasProviderRules(providerID) && routingerr.HasProviderRules(id) {
			providerID = id
		}
		if providerError.Message != "" {
			message = providerError.Message
		}
		resetHint = providerError.ResetAt
	}
	phase := routingerr.PhasePromptSend
	if data.DynamicRouteAttempt {
		switch {
		case data.EffectObserved:
			phase = routingerr.PhaseToolExecution
		case data.OutputObserved:
			phase = routingerr.PhaseStreaming
		case !data.EvidenceKnown:
			// Unknown attempt state is deliberately classified outside the
			// pre-result phases. The dynamic route gate also requires explicit
			// evidence, so this remains a defensive second fence.
			phase = routingerr.PhaseStreaming
		}
	}
	return routingerr.Classify(routingerr.Input{
		Phase:      phase,
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
	case routingerr.CodeAgentTransportLost:
		return "Agent connection lost"
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
		case routingerr.CodeAgentTransportLost:
			condition = "The agent connection kept dropping"
		}
	}
	return condition + " after several retries. Resume to try again, or start a fresh session."
}

// clearTransientRetryState clears a session's retry entry, cancels its timer,
// and drops the cached prompt (which may hold large/sensitive attachment data).
func (s *Service) clearTransientRetryState(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	active := s.clearTransientRetryStateLocked(sessionID, state)
	state.mu.Unlock()
	release()
	return active
}

func (s *Service) clearTransientRetryStateLocked(sessionID string, state *transientRetryNoticeState) bool {
	s.lastTurnPrompt.Delete(sessionID)
	v, ok := s.transientRetries.LoadAndDelete(sessionID)
	if ok {
		if entry, ok := v.(*transientRetryEntry); ok && entry.cancel != nil {
			entry.cancel()
		}
	}
	state.owned.Store(false)
	return ok
}

// resetTransientRetry clears in-memory retry state and retires the persisted
// retry notice(s). The detached context keeps durable cleanup best effort even
// when the event that ended the retry was cancelled by its caller.
func (s *Service) resetTransientRetry(sessionID string) {
	s.resetTransientRetryWithContext(context.Background(), sessionID, false)
}

// forceResolve is used by explicit stop/cancel and terminal paths where a
// persisted notice can outlive the in-memory retry entry. Normal successful
// turns skip the transcript scan when no retry loop was owned.
func (s *Service) resetTransientRetryWithContext(ctx context.Context, sessionID string, forceResolve bool) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.resetTransientRetryWithContextLocked(state, ctx, sessionID, forceResolve)
	state.mu.Unlock()
	release()
}

func (s *Service) resetTransientRetryWithContextLocked(
	state *transientRetryNoticeState,
	ctx context.Context,
	sessionID string,
	forceResolve bool,
) {
	if !s.clearTransientRetryStateLocked(sessionID, state) && !forceResolve {
		return
	}
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.resolveTransientRetryMessagesLocked(context.WithoutCancel(ctx), sessionID)
}

// resolveTransientRetryMessages removes every persisted retry status message
// for a session. The task service owns the durable write and MessageDeleted
// publication. Cleanup is intentionally non-fatal to the transition that
// ended the retry loop.
func (s *Service) resolveTransientRetryMessages(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.resolveTransientRetryMessagesLocked(ctx, sessionID)
	state.mu.Unlock()
	release()
}

func (s *Service) resolveTransientRetryMessagesLocked(ctx context.Context, sessionID string) {
	if s.transientRetryMessages == nil || sessionID == "" {
		return
	}
	messages, err := s.transientRetryMessages.ListMessages(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to list transient retry status messages",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	for _, message := range messages {
		if message == nil || message.Metadata == nil {
			continue
		}
		retrying, ok := message.Metadata["retrying"].(bool)
		if !ok || !retrying {
			continue
		}
		if err := s.transientRetryMessages.DeleteMessage(ctx, message.ID); err != nil {
			s.logger.Warn("failed to delete transient retry status message",
				zap.String("session_id", sessionID),
				zap.String("message_id", message.ID),
				zap.Error(err))
		}
	}
}

// retireAndClearTransientRetryState closes the in-memory lifecycle under the
// notice mutex. Callers use it when they already hold the task runtime mutex;
// durable cleanup remains outside that broader runtime critical section.
func (s *Service) retireAndClearTransientRetryState(sessionID string) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.clearTransientRetryStateLocked(sessionID, state)
	state.mu.Unlock()
	release()
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
	noticeState, releaseNoticeState := s.acquireTransientRetryNoticeState(sessionID)
	if noticeState == nil {
		return false
	}
	noticeState.mu.Lock()
	_, active := s.transientRetries.Load(sessionID)
	s.resetTransientRetryWithContextLocked(noticeState, ctx, sessionID, true)
	noticeState.mu.Unlock()
	releaseNoticeState()
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
		UserInitiated:    true,
	})
	return true
}
