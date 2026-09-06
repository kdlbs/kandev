package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	freshSessionModelStateWait = 2 * time.Second
	freshSessionModelStatePoll = 10 * time.Millisecond
)

// WasSessionInitialized reports whether the execution completed ACP session setup.
// Returns false if the execution is not found or init hasn't completed.
func (m *Manager) WasSessionInitialized(executionID string) bool {
	exec, exists := m.executionStore.Get(executionID)
	if !exists {
		return false
	}
	return exec.isSessionInitialized()
}

// GetSessionAuthMethods returns auth methods for a session's execution.
// It first checks cached methods from agent_capabilities events. When the agent
// failed before reporting capabilities (e.g., immediate auth error), it falls
// back to static auth methods derived from the agent type.
func (m *Manager) GetSessionAuthMethods(sessionID string) []streams.AuthMethodInfo {
	exec, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	if methods := exec.GetAuthMethods(); len(methods) > 0 {
		return methods
	}
	return fallbackAuthMethods(exec.AgentID)
}

// fallbackAuthMethods returns static auth methods for known agent types.
// Used when the agent failed before sending agent_capabilities (e.g., auth error
// on first prompt before the agent could report its own auth methods).
func fallbackAuthMethods(agentID string) []streams.AuthMethodInfo {
	switch agentID {
	case "claude-acp":
		return []streams.AuthMethodInfo{
			{
				ID:          "claude-auth-login",
				Name:        "Anthropic Authentication",
				Description: "Log in to your Anthropic account",
				TerminalAuth: &streams.TerminalAuth{
					Command: "claude",
					Args:    []string{"auth", "login"},
					Label:   "Log in with Claude CLI",
				},
			},
		}
	case "auggie":
		return []streams.AuthMethodInfo{
			{
				ID:          "auggie-login",
				Name:        "Auggie Authentication",
				Description: "Log in to your Auggie account",
				TerminalAuth: &streams.TerminalAuth{
					Command: "auggie",
					Args:    []string{"login"},
					Label:   "Log in with Auggie CLI",
				},
			},
		}
	default:
		return nil
	}
}

// PromptAgent sends a follow-up prompt to a running agent.
// Attachments (images) are passed to the agent if provided.
// When dispatchOnly is true, returns once the prompt is accepted instead of
// waiting for the agent's turn to complete.
func (m *Manager) PromptAgent(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool) (*PromptResult, error) {
	return m.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, nil)
}

// PromptAgentWithDispatchCallback exposes agentctl acceptance to callers that
// must keep admission serialized until the queued prompt is actually dispatched.
func (m *Manager) PromptAgentWithDispatchCallback(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*PromptResult, error) {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return nil, fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	lease, err := m.acquireActivity(ctx, activity.KindExecutionRunning)
	if err != nil {
		return nil, err
	}
	key := executionActivityKey(executionID)
	m.trackActivity(key, lease)
	m.setRuntimeInterest(execution.SessionID, true)
	result, err := m.sessionManager.SendPromptWithDispatchCallback(ctx, execution, prompt, true, attachments, dispatchOnly, onDispatched)
	if err != nil || !dispatchOnly {
		m.releaseActivity(key)
		if err != nil {
			m.setRuntimeInterest(execution.SessionID, false)
		}
	}
	return result, err
}

// SteerAgentWithDispatchCallback delivers a steer: it hands the prompt into a
// turn that is still generating rather than serializing behind it. It mirrors
// PromptAgentWithDispatchCallback's activity accounting; only the underlying
// session-manager call differs.
func (m *Manager) SteerAgentWithDispatchCallback(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*PromptResult, error) {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return nil, fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	lease, err := m.acquireActivity(ctx, activity.KindExecutionRunning)
	if err != nil {
		return nil, err
	}
	key := executionActivityKey(executionID)
	m.trackActivity(key, lease)
	m.setRuntimeInterest(execution.SessionID, true)
	result, err := m.sessionManager.SendPromptSteerWithDispatchCallback(ctx, execution, prompt, true, attachments, dispatchOnly, onDispatched)
	if err != nil || !dispatchOnly {
		m.releaseActivity(key)
		if err != nil {
			m.setRuntimeInterest(execution.SessionID, false)
		}
	}
	return result, err
}

// cancelWaitTimeout bounds how long CancelAgent waits for the in-flight SendPrompt
// to exit after the in-flight session/prompt RPC has ended (cancel acknowledged).
// Exposed as a var (not const) so tests can shorten it without fake clocks.
var cancelWaitTimeout = 10 * time.Second

// cancelEscalationTimeout bounds the post-escalation wait after we inject a synthetic
// error onto promptDoneCh to unblock a stuck SendPrompt.
var cancelEscalationTimeout = 2 * time.Second

// CancelAgent interrupts the current agent turn without terminating the process,
// allowing the user to send a new prompt.
//
// When the agent subprocess accepts the ACP cancel but never publishes a `complete`
// event (and the update stream stays open), the in-flight SendPrompt would otherwise
// block forever. After cancelWaitTimeout we escalate by signalling promptDoneCh with
// a synthetic error so SendPrompt returns and its deferred cleanup closes
// promptFinished normally. Callers receive ErrCancelEscalated to signal that local
// lifecycle state was reconciled but the agent never confirmed the cancel, and
// higher layers (Service.CancelAgent) must still reconcile DB state.
func (m *Manager) CancelAgent(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	return m.cancelAgentExecution(ctx, execution)
}

// CancelAgentForPrompt cancels the execution that owns a previously captured
// prompt activity snapshot. It validates the complete identity while holding
// the execution-store lock, then operates on that exact execution pointer so
// a later session lookup cannot redirect cancellation to a successor.
func (m *Manager) CancelAgentForPrompt(
	ctx context.Context,
	sessionID, executionID string,
	generation, activityEpoch uint64,
) error {
	execution, err := m.executionStore.ClaimPromptActivity(sessionID, executionID, generation, activityEpoch)
	if err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return fmt.Errorf("session %q: %w", sessionID, ErrNoExecutionForSession)
		}
		return fmt.Errorf("session %q: %w", sessionID, err)
	}
	return m.cancelAgentExecution(ctx, execution)
}

func (m *Manager) cancelAgentExecution(ctx context.Context, execution *AgentExecution) error {
	executionID := execution.ID

	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	m.logger.Info("cancelling agent turn",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))

	cancelErr := client.Cancel(ctx)
	streamDisconnected := errors.Is(cancelErr, agentctlclient.ErrAgentStreamNotConnected)
	if cancelErr != nil &&
		!errors.Is(cancelErr, agentctlclient.ErrTurnCancelNotAcknowledged) &&
		!streamDisconnected {
		m.logger.Error("failed to cancel agent turn",
			zap.String("execution_id", executionID),
			zap.Error(cancelErr))
		return fmt.Errorf("failed to cancel agent: %w", cancelErr)
	}

	// Don't clear buffers or mark ready here.
	// The agent will respond to the original prompt with StopReason=cancelled,
	// which triggers handleCompleteEvent() to properly flush buffers and mark state.
	// Clearing here would race with in-flight notifications and lose content.

	execution.promptFinishedMu.Lock()
	ch := execution.promptFinished
	execution.promptFinishedMu.Unlock()

	if ch == nil {
		if execution.dispatchedPromptPending.Load() {
			return m.escalateStuckCancel(ctx, execution, nil)
		}
		if streamDisconnected {
			m.logger.Info("agent stream already disconnected; cancel is complete",
				zap.String("execution_id", executionID))
		}
		return nil
	}

	// Teardown may close the stream immediately before an explicit cancel (for
	// example, archive followed by cancel). Treat it like an unacknowledged
	// cancel: locally release any in-flight prompt and let orchestration perform
	// its idempotent DB reconciliation.
	if streamDisconnected {
		m.logger.Warn("agent stream disconnected before cancel; escalating locally",
			zap.String("execution_id", executionID),
			zap.Error(cancelErr))
		return m.escalateStuckCancel(ctx, execution, ch)
	}

	// The agent did not end the in-flight session/prompt RPC after cancel (e.g. it
	// does not implement session/cancel). Escalate immediately so SendPrompt and the
	// prompt gate are reconciled instead of waiting the full cancelWaitTimeout.
	if errors.Is(cancelErr, agentctlclient.ErrTurnCancelNotAcknowledged) {
		m.logger.Warn("agent cancel not acknowledged; escalating immediately",
			zap.String("execution_id", executionID),
			zap.Error(cancelErr))
		return m.escalateStuckCancel(ctx, execution, ch)
	}

	m.logger.Info("agent cancel sent, waiting for turn completion",
		zap.String("execution_id", executionID))

	// Wait for the in-flight SendPrompt to finish processing the cancel completion.
	// Without this, a follow-up PromptAgent races on promptDoneCh with two readers.
	select {
	case <-ch:
		if execution.dispatchedPromptPending.Load() {
			return m.escalateStuckCancel(ctx, execution, ch)
		}
		m.logger.Debug("in-flight prompt finished after cancel",
			zap.String("execution_id", executionID))
		return nil
	case <-time.After(cancelWaitTimeout):
		return m.escalateStuckCancel(ctx, execution, ch)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// escalateStuckCancel unblocks a SendPrompt that is stuck waiting for a completion
// event the agent will never emit. It injects a synthetic error onto promptDoneCh
// (non-blocking; the channel is buffered size 1), waits briefly for SendPrompt to
// exit and close promptFinished, and marks the execution ready so the workflow
// leaves the running state. Returns ErrCancelEscalated so callers know the ACP
// cancel did not get a clean acknowledgement.
//
// The ready-event publish below is dispatched asynchronously (asyncPublish=true
// on markReadyEventWithContext), not inline: every caller that can reach this
// escalation path (Service.CancelAgent, and cancelAgentSilent's callers —
// QueueAndInterruptForPeerMessage, retryClarificationAfterCancel,
// PauseForClarificationInput) does so while still holding sessionID's
// per-session cancelInFlightGuard, and the in-memory event bus delivers to
// the orchestrator's handleAgentReady (a queue subscriber for this event
// type) *synchronously*, on this same goroutine. An inline publish would
// have handleAgentReady try to re-acquire that same guard reentrantly and
// deadlock forever on the non-reentrant sync.Mutex. See
// markReadyEventWithContext's doc comment for the full explanation.
func (m *Manager) escalateStuckCancel(ctx context.Context, execution *AgentExecution, ch <-chan struct{}) error {
	m.logger.Warn("timed out waiting for in-flight prompt to finish after cancel; escalating",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID))

	// Clear the cancelled dispatch-only prompt before releasing its waiter. If
	// the waiter immediately admits a successor, a later cleanup must not clear
	// the successor's gate.
	execution.dispatchedPromptPending.Store(false)
	execution.signalPromptCompletionForStartupGeneration(
		execution.startupAttemptSnapshot(),
		PromptCompletionSignal{
			IsError:          true,
			Error:            "cancel escalated: agent did not complete turn within timeout",
			PromptGeneration: execution.promptGenerationSnapshot(),
		},
	)

	if ch != nil {
		select {
		case <-ch:
			m.logger.Info("in-flight prompt released after cancel escalation",
				zap.String("execution_id", execution.ID))
		case <-time.After(cancelEscalationTimeout):
			m.logger.Warn("in-flight prompt did not release after cancel escalation",
				zap.String("execution_id", execution.ID))
		case <-ctx.Done():
			// Fall through to MarkReady/drain below — once the synthetic signal is
			// queued, the cleanup must survive the caller's context cancellation
			// or the execution leaks in the Running state and the stale signal
			// breaks the next PromptAgent call.
		}
	}

	if err := m.markReadyEventWithContext(context.Background(), execution.ID, events.AgentReady, true); err != nil {
		m.logger.Warn("failed to mark execution ready after cancel escalation",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
	}

	// Drain any stale signal that may have been left if the agent completed
	// concurrently with the escalation timeout (Go's select is non-deterministic
	// when both promptFinished and time.After are ready, so our send above may
	// have landed on a channel that SendPrompt had just drained).
	select {
	case <-execution.promptDoneCh:
	default:
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrCancelEscalated
}

// SetSessionMode changes the session mode for a running agent.
// Always uses the execution's current ACPSessionID rather than the caller's copy,
// which may be stale during startup/restart when the session is being re-initialized.
func (m *Manager) SetSessionMode(ctx context.Context, executionID, _ string, modeID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	if !execution.isSessionInitialized() || execution.ACPSessionID == "" {
		return fmt.Errorf("execution %q ACP session is not ready", executionID)
	}
	return client.SetMode(ctx, execution.ACPSessionID, modeID)
}

// SetSessionModeBySessionID changes the session mode for a running agent by session ID.
func (m *Manager) SetSessionModeBySessionID(ctx context.Context, sessionID, modeID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	return m.SetSessionMode(ctx, execution.ID, execution.ACPSessionID, modeID)
}

// SetSessionModel changes the session model for a running agent. ACP agents
// swap the model in-place via their advertised model-selection mechanism.
// Passthrough (TUI) agents have no protocol channel — the model is a CLI flag
// baked into the launch — so the override is persisted on the execution and
// the PTY is relaunched so the next process picks up the new --model.
func (m *Manager) SetSessionModel(ctx context.Context, executionID, modelID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	if execution.PassthroughProcessID != "" {
		if err := m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
			exec.setMetadataValue(MetadataKeyModelOverride, modelID)
		}); err != nil {
			return fmt.Errorf("failed to persist model override for execution %q: %w", executionID, err)
		}
		return m.RestartAgentProcess(ctx, executionID)
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	return client.SetModel(ctx, modelID)
}

// SetSessionModelBySessionID changes the session model for a running agent by session ID.
func (m *Manager) SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	return m.SetSessionModel(ctx, execution.ID, modelID)
}

// SetSessionConfigOption changes an ACP session config option for a running agent.
func (m *Manager) SetSessionConfigOption(ctx context.Context, executionID, configID, value string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	if !execution.isSessionInitialized() || execution.ACPSessionID == "" {
		return fmt.Errorf("execution %q ACP session is not ready", executionID)
	}
	return client.SetConfigOption(ctx, configID, value)
}

// SetSessionConfigOptionBySessionID changes an ACP session config option by task session ID.
func (m *Manager) SetSessionConfigOptionBySessionID(ctx context.Context, sessionID, configID, value string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	return m.SetSessionConfigOption(ctx, execution.ID, configID, value)
}

// AuthenticateBySessionID triggers authentication for a given auth method on the agent.
func (m *Manager) AuthenticateBySessionID(ctx context.Context, sessionID, methodID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	return client.Authenticate(ctx, methodID)
}

// reapplySessionModeAfterReset re-applies the active session permission mode
// (e.g. auto / accept-edits) to a freshly (re)initialized ACP session so the
// user's choice survives a context reset instead of silently reverting to the
// agent's default.
//
// The mode to restore is resolved from the persisted session_mode in the DB
// (the authoritative, synchronously-written source — see persistSessionMode and
// the set_session_mode action), falling back to prev (the in-memory mode state
// captured before the reset) only when no provider is wired or nothing is
// persisted. Preferring the DB avoids re-applying a stale in-memory mode when a
// set_session_mode action persisted a newer mode in the same on_enter batch
// before its agent mode event updated modeState. A nil/empty resolved mode is a
// no-op. Addresses issue #1183.
func (m *Manager) reapplySessionModeAfterReset(ctx context.Context, execution *AgentExecution, newSessionID string, prev *CachedModeState) {
	fallback := ""
	if prev != nil {
		fallback = prev.CurrentModeID
	}
	mode := m.effectiveSessionMode(ctx, execution, fallback)
	if err := m.applySessionModeAfterReset(ctx, execution, newSessionID, mode); err != nil {
		m.logger.Warn("failed to re-apply session mode after context reset",
			zap.String("execution_id", execution.ID),
			zap.String("mode", mode),
			zap.Error(err))
	}
}

func (m *Manager) applySessionModeAfterReset(
	ctx context.Context,
	execution *AgentExecution,
	newSessionID, mode string,
) error {
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil || mode == "" {
		return nil
	}
	if err := client.SetMode(ctx, newSessionID, mode); err != nil {
		return fmt.Errorf("failed to restore session mode %q: %w", mode, err)
	}
	availableModes := []streams.SessionModeInfo(nil)
	if current := execution.GetModeState(); current != nil {
		availableModes = current.AvailableModes
	}
	// Restore the cache too: the fresh session would otherwise report the agent's
	// default mode, leaving modeState stale relative to what we just re-applied.
	execution.SetModeState(&CachedModeState{
		CurrentModeID:  mode,
		AvailableModes: availableModes,
	})
	m.logger.Info("re-applied session mode after context reset",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("mode", mode))
	return nil
}

// reapplySessionModelAfterReset applies the executor-authoritative model
// decision to a freshly initialized ACP session.
func (m *Manager) reapplySessionModelAfterReset(
	ctx context.Context,
	execution *AgentExecution,
	newSessionID, modelID string,
) error {
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil || modelID == "" {
		return nil
	}
	policy := m.resolveStartModelPolicy(ctx, execution.AgentProfileID)
	policy.Model = modelID
	decision, err := applyStartModelPolicy(
		ctx, m.logger, client, execution.GetModelState(), policy,
	)
	if err != nil {
		m.logger.Warn("failed to re-apply session model after context reset",
			zap.String("execution_id", execution.ID),
			zap.String("model", modelID),
			zap.Error(err))
		return err
	}
	if decision.Warning && m.sessionManager != nil {
		m.sessionManager.publishModelSelectionWarningEvent(execution, newSessionID, decision)
	}
	if decision.EffectiveModel != "" &&
		(decision.Outcome == ModelSelectionOutcomeApplied || decision.Outcome == ModelSelectionOutcomeExplicitFallback) {
		m.logger.Info("re-applied session model after context reset",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID),
			zap.String("new_acp_session_id", newSessionID),
			zap.String("model", decision.EffectiveModel),
			zap.Bool("using_fallback", decision.Outcome == ModelSelectionOutcomeExplicitFallback))
	}
	return nil
}

func cacheFreshSessionModelState(execution *AgentExecution) bool {
	if execution == nil {
		return false
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return false
	}
	state := client.GetLastSessionModelState()
	if state == nil {
		return false
	}
	execution.SetModelState(&CachedModelState{
		CurrentModelID: state.CurrentModelID,
		Models:         state.Models,
		ConfigOptions:  state.ConfigOptions,
	})
	return true
}

// waitForFreshSessionModelState gives the agent stream a chance to deliver the
// new session's model catalog before the reset/restart policy runs. ACP
// session/new returns before its session_models notification is dispatched to
// the lifecycle manager. This is the fallback for transports that do not
// include a synchronous snapshot in their session response. Reading the old
// nil/empty cache at that boundary would incorrectly treat an advertised model
// as unavailable and leave the agent on its provider default. A missing
// notification still follows the executor-authoritative default path after the
// bounded wait.
func waitForFreshSessionModelState(ctx context.Context, log *logger.Logger, execution *AgentExecution) bool {
	if execution == nil {
		return false
	}
	if freshSessionModelCatalogReady(execution.GetModelState()) {
		return true
	}

	waitCtx, cancel := context.WithTimeout(ctx, freshSessionModelStateWait)
	defer cancel()
	ticker := time.NewTicker(freshSessionModelStatePoll)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			if log != nil {
				log.Debug("fresh session model catalog was not reported before policy evaluation",
					zap.String("execution_id", execution.ID),
					zap.Error(waitCtx.Err()))
			}
			return false
		case <-ticker.C:
			if freshSessionModelCatalogReady(execution.GetModelState()) {
				return true
			}
		}
	}
}

func freshSessionModelCatalogReady(state *CachedModelState) bool {
	return state != nil && (len(state.Models) > 0 || len(state.ConfigOptions) > 0 || state.ConfigOptionsSettled)
}

func (m *Manager) effectiveSessionModelForReset(ctx context.Context, execution *AgentExecution) string {
	return m.captureSessionRuntimeConfigForReset(ctx, execution).Model
}

func (m *Manager) captureSessionRuntimeConfigForReset(
	ctx context.Context,
	execution *AgentExecution,
) models.SessionRuntimeConfig {
	profileModel, profileMode, profileOptions := m.resolveProfileSessionConfig(ctx, execution.AgentProfileID)
	modelState := execution.GetModelState()
	liveModel := profileModel
	if modelState != nil && strings.TrimSpace(modelState.CurrentModelID) != "" {
		liveModel = modelState.CurrentModelID
	}
	liveMode := profileMode
	if modeState := execution.GetModeState(); modeState != nil && strings.TrimSpace(modeState.CurrentModeID) != "" {
		liveMode = modeState.CurrentModeID
	}
	liveOptions := maps.Clone(profileOptions)
	if liveStateOptions, liveOptionsKnown := liveSessionRuntimeConfigOptions(modelState); liveOptionsKnown {
		// A settled provider snapshot is the live layer, including an empty
		// snapshot. Keep an allocated empty map so the effective-config helper
		// preserves that presence while persisted state can still overlay it.
		liveOptions = make(map[string]string, len(liveStateOptions))
		for id, value := range liveStateOptions {
			liveOptions[id] = value
		}
	}
	model, mode, options, optionsSet := m.effectiveSessionRuntimeConfigWithPresence(
		ctx, execution, liveModel, liveMode, liveOptions,
	)
	if !optionsSet {
		options = selectedRuntimeConfigOptions(modelState)
	}
	return models.SessionRuntimeConfig{
		Model:         model,
		Mode:          mode,
		ConfigOptions: sanitizeRuntimeConfigOptionsWithCatalog(options, modelState),
	}
}

func liveSessionRuntimeConfigOptions(state *CachedModelState) (map[string]string, bool) {
	if state == nil || (!state.ConfigOptionsSettled && len(state.ConfigOptions) == 0) {
		return nil, false
	}
	return selectedRuntimeConfigOptions(state), true
}

func selectedRuntimeConfigOptions(state *CachedModelState) map[string]string {
	if state == nil {
		return nil
	}
	options := make(map[string]string, len(state.ConfigOptions))
	for _, option := range state.ConfigOptions {
		id := strings.TrimSpace(option.ID)
		value := strings.TrimSpace(option.CurrentValue)
		if isRestorableRuntimeConfigOption(id, option.Category, value) {
			options[id] = value
		}
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

func sanitizeRuntimeConfigOptions(options map[string]string) map[string]string {
	return sanitizeRuntimeConfigOptionsWithCatalog(options, nil)
}

func sanitizeRuntimeConfigOptionsWithCatalog(options map[string]string, state *CachedModelState) map[string]string {
	if options == nil {
		return nil
	}
	catalog, catalogKnown := capturedRuntimeConfigOptionCatalog(state)
	cleaned := make(map[string]string, len(options))
	for id, value := range options {
		trimmedID := strings.TrimSpace(id)
		category := ""
		if catalogKnown {
			var ok bool
			category, ok = catalog[trimmedID]
			if !ok {
				continue
			}
		}
		if isRestorableRuntimeConfigOption(trimmedID, category, value) {
			cleaned[trimmedID] = strings.TrimSpace(value)
		}
	}
	return cleaned
}

func capturedRuntimeConfigOptionCatalog(state *CachedModelState) (map[string]string, bool) {
	if state == nil || (!state.ConfigOptionsSettled && len(state.ConfigOptions) == 0) {
		return nil, false
	}
	catalog := make(map[string]string, len(state.ConfigOptions))
	for _, option := range state.ConfigOptions {
		id := strings.TrimSpace(option.ID)
		if id == "" {
			continue
		}
		category := strings.TrimSpace(option.Category)
		if previous, exists := catalog[id]; exists && previous != "" {
			continue
		}
		catalog[id] = category
	}
	return catalog, true
}

func isRestorableRuntimeConfigOption(id, category, value string) bool {
	id = strings.TrimSpace(id)
	value = strings.TrimSpace(value)
	if id == "" || value == "" {
		return false
	}
	if strings.EqualFold(id, "model") || strings.EqualFold(id, "mode") {
		return false
	}
	return !strings.EqualFold(category, "model") && !strings.EqualFold(category, "mode")
}

func (m *Manager) restoreSessionRuntimeConfig(
	ctx context.Context,
	execution *AgentExecution,
	sessionID string,
	config models.SessionRuntimeConfig,
) error {
	if execution == nil {
		return fmt.Errorf("cannot restore session runtime configuration without an agentctl client")
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	releaseClient()
	if client == nil {
		return fmt.Errorf("cannot restore session runtime configuration without an agentctl client")
	}
	if err := m.reapplySessionModelAfterReset(ctx, execution, sessionID, config.Model); err != nil {
		return fmt.Errorf("restore session runtime model: %w", err)
	}
	if err := m.applySessionModeAfterReset(ctx, execution, sessionID, config.Mode); err != nil {
		return fmt.Errorf("restore session runtime mode: %w", err)
	}
	options := sanitizeRuntimeConfigOptionsWithCatalog(config.ConfigOptions, execution.GetModelState())
	optionIDs := make([]string, 0, len(options))
	for id := range options {
		optionIDs = append(optionIDs, id)
	}
	sort.Strings(optionIDs)
	for _, id := range optionIDs {
		value := options[id]
		client, releaseClient := execution.AcquireAgentCtlClient()
		if client == nil {
			return fmt.Errorf("restore session runtime option %q: agentctl client is unavailable", id)
		}
		err := client.SetConfigOption(ctx, id, value)
		releaseClient()
		if err != nil {
			return fmt.Errorf("restore session runtime option %q: %w", id, err)
		}
	}
	return nil
}

// ResetAgentContext resets the agent's conversation context. For ACP agents that support
// session reset, this creates a new session on the existing connection (fast, no process restart).
// For all other agents, this falls back to RestartAgentProcess (full subprocess restart).
func (m *Manager) ResetAgentContext(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}

	// Passthrough agents always need full restart
	if execution.PassthroughProcessID != "" {
		return m.RestartAgentProcess(ctx, executionID)
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if current, currentExists := m.executionStore.Get(executionID); !currentExists || current != execution {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	// Capture the complete effective session configuration before the reset. The
	// fresh ACP session starts at provider defaults, so asynchronous events from
	// it must not replace the task's pre-reset restoration intent.
	runtimeConfig := m.captureSessionRuntimeConfigForReset(ctx, execution)

	// Clear the old catalog before creating the fresh session. The new
	// session_models event is delivered asynchronously and must be the only
	// catalog consulted by the reset decision.
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.SetModelState(nil)
	})

	// Resolve agent config and MCP servers for session reset
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		releaseClient()
		m.logger.Info("cannot resolve agent config for session reset, falling back to process restart",
			zap.String("execution_id", executionID), zap.Error(err))
		return m.restartAgentProcess(ctx, executionID, &runtimeConfig)
	}

	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		releaseClient()
		m.logger.Warn("cannot resolve MCP servers for session reset, falling back to process restart",
			zap.String("execution_id", executionID), zap.Error(err))
		return m.restartAgentProcess(ctx, executionID, &runtimeConfig)
	}

	// Try session-level reset (only ACP adapters support this)
	newSessionID, err := client.ResetSession(ctx, execution.WorkspacePath, mcpServers)
	releaseClient()
	if err != nil {
		m.logger.Info("session reset not supported, falling back to process restart",
			zap.String("execution_id", executionID), zap.Error(err))
		return m.restartAgentProcess(ctx, executionID, &runtimeConfig)
	}

	// Success — update execution state without restarting process
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.ACPSessionID = newSessionID
		exec.needsResumeContext = false
		exec.resumeContextInjected = false

		m.resetStreamingStateWithHistory(exec)

		// Drain any stale prompt completion signal
		select {
		case <-exec.promptDoneCh:
		default:
		}
		exec.dispatchedPromptPending.Store(false)
	})

	// Restore the complete captured configuration. A strict-mode model that is
	// rejected by the fresh session fails the reset explicitly instead of
	// silently dropping to the provider default.
	if !cacheFreshSessionModelState(execution) &&
		(runtimeConfig.Model != "" || len(runtimeConfig.ConfigOptions) > 0) {
		waitForFreshSessionModelState(ctx, m.logger, execution)
	}
	if err := m.restoreSessionRuntimeConfig(ctx, execution, newSessionID, runtimeConfig); err != nil {
		restoreErr := fmt.Errorf("failed to restore session runtime configuration after reset: %w", err)
		m.updateExecutionError(executionID, restoreErr.Error())
		m.persistExecutorRunning(context.WithoutCancel(ctx), execution)
		return restoreErr
	}
	if err := m.updateStatusAndPersist(ctx, executionID, v1.AgentStatusReady); err != nil {
		m.updateExecutionError(executionID, "failed to mark reset agent ready: "+err.Error())
		m.persistExecutorRunning(context.WithoutCancel(ctx), execution)
		return fmt.Errorf("failed to mark reset agent ready: %w", err)
	}

	m.logger.Info("agent context reset via session (no process restart)",
		zap.String("execution_id", executionID),
		zap.String("session_id", execution.SessionID),
		zap.String("new_acp_session_id", newSessionID))

	m.eventPublisher.PublishAgentEvent(ctx, events.AgentContextReset, execution)
	// Boot-equivalent signal: a fresh ACP session is alive but no turn has run yet.
	// Use AgentBootReady so the orchestrator routes this to handleAgentBootReady
	// (idle/WAITING transition) rather than handleAgentReady (turn-end transition,
	// which would fire on_turn_complete against the current step — the original
	// boot-vs-turn ambiguity bug).
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentBootReady, execution)
	return nil
}

// ErrNoExecutionForSession is returned when no live execution is tracked for a session,
// typically because the agent process has crashed, exited, or the session state is stuck
// (for example after a backend restart that did not re-register the execution).
//
// Callers should treat this as a "there is nothing to cancel/stop" signal and still reconcile
// the session's state in the database, otherwise the session will appear stuck as RUNNING
// with no agent to drive it.
var ErrNoExecutionForSession = errors.New("no execution for session")

// ErrCancelEscalated is returned by CancelAgent when the agent subprocess accepted
// the ACP cancel but did not publish a completion event before the timeout. The
// lifecycle manager has locally unblocked the in-flight SendPrompt and marked the
// execution ready, but the agent never acknowledged the cancel. Callers should
// still reconcile session-level state (e.g. transition the task session to
// WAITING_FOR_INPUT) — the user's intent was unambiguous.
var ErrCancelEscalated = errors.New("cancel escalated: agent did not acknowledge within timeout")

// CancelAgentBySessionID cancels the current agent turn for a specific session.
// Returns ErrNoExecutionForSession (wrapped) when no execution is tracked for the session.
//
// Passthrough sessions don't speak ACP — Ctrl-C (0x03) written to PTY stdin is
// the only stop signal a TUI CLI understands. A write failure is non-fatal:
// the caller still reconciles DB state so the UI unsticks even if the PTY is
// already gone.
func (m *Manager) CancelAgentBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("session %q: %w", sessionID, ErrNoExecutionForSession)
	}

	if execution.PassthroughProcessID != "" {
		if err := m.WritePassthroughStdin(ctx, sessionID, "\x03"); err != nil {
			m.logger.Warn("failed to write Ctrl-C to passthrough stdin",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return nil
	}

	return m.CancelAgent(ctx, execution.ID)
}

// StopAgent stops an agent execution
func (m *Manager) StopAgent(ctx context.Context, executionID string, force bool) error {
	return m.StopAgentWithReason(ctx, executionID, "", force)
}

// StopAgentWithReason stops an agent execution and passes a semantic reason to runtime teardown.
func (m *Manager) StopAgentWithReason(ctx context.Context, executionID string, reason string, force bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		handled, err := m.stopPersistedKubernetesExecution(ctx, executionID, reason, force)
		if handled {
			return err
		}
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if current, currentExists := m.executionStore.Get(executionID); !currentExists || current != execution {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStopping)
	if err != nil {
		return err
	}
	defer activityLease.Release()
	// Keep the execution's running activity lease until backend teardown
	// succeeds. A failed stop remains retryable, so maintenance must not treat
	// a potentially live runtime as idle. RemoveExecution releases the lease on
	// the successful path.

	m.logger.Info("stopping agent",
		zap.String("execution_id", executionID),
		zap.String("reason", reason),
		zap.Bool("force", force),
		zap.Stringer("runtime", execution.RuntimeName))

	// Try to gracefully stop via agentctl first, then always close connections.
	agentStopFailed := m.stopExecutionAgentctl(ctx, executionID, execution, force)

	// Stop the agent execution via the runtime that created it. A failed stop
	// must remain tracked: removing it here would turn a retryable cleanup into
	// an unobservable orphan process.
	if err := m.stopAgentViaBackend(ctx, executionID, execution, reason, force, agentStopFailed); err != nil {
		return fmt.Errorf("stop runtime for execution %q: %w", executionID, err)
	}
	if execution.RuntimeName == executor.NameKubernetes && (force || shouldRunExecutorCleanup(reason)) {
		cleanupCtx, cancelCleanup := kubernetesDurableContext(ctx)
		err := m.deleteKubernetesRuntimeSecrets(cleanupCtx, execution.MetadataSnapshot())
		cancelCleanup()
		if err != nil {
			return fmt.Errorf("delete runtime secrets for execution %s: %w", executionID, err)
		}
	}

	// The runtime is now terminal. Detach agentctl under its own short critical
	// section before any cleanup or event snapshot can acquire prompt state.
	// Failed runtime stops intentionally retain the closed-but-reusable client so
	// a later retry can issue the agentctl stop request again.
	execution.agentctlLifecycleMu.Lock()
	execution.detachAgentctlClient()
	execution.agentctlLifecycleMu.Unlock()

	// Update execution status and remove from tracking
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.Status = v1.AgentStatusStopped
		now := time.Now()
		exec.FinishedAt = &now
	})

	// End session trace span
	execution.EndSessionSpan()

	m.RemoveExecution(executionID)
	m.clearRemoteStatus(execution.SessionID)

	m.logger.Info("agent stopped and removed from tracking",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID))

	// Publish stopped event
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentStopped, execution)

	return nil
}

func (m *Manager) stopExecutionAgentctl(
	ctx context.Context,
	executionID string,
	execution *AgentExecution,
	force bool,
) bool {
	execution.agentctlLifecycleMu.Lock()
	defer execution.agentctlLifecycleMu.Unlock()
	client := execution.currentAgentCtlClient() // protected by agentctlLifecycleMu
	if client == nil {
		return false
	}
	defer client.Close()
	if force {
		return false
	}
	if err := client.Stop(ctx); err != nil {
		// During shutdown the instance may already be stopping through another
		// lifecycle path, so a failed HTTP call is expected.
		if m.IsShuttingDown() {
			m.logger.Debug("failed to stop agent via agentctl",
				zap.String("execution_id", executionID), zap.Error(err))
		} else {
			m.logger.Warn("failed to stop agent via agentctl",
				zap.String("execution_id", executionID), zap.Error(err))
		}
		return true
	}
	return false
}

// StopBySessionID stops the agent for a specific session
func (m *Manager) StopBySessionID(ctx context.Context, sessionID string, force bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}

	return m.StopAgent(ctx, execution.ID, force)
}

// RestartAgentProcess stops the agent subprocess and starts a fresh one, clearing the agent's
// conversation context. For ACP agents this restarts via agentctl with a new ACP session.
// For passthrough (TUI) agents this kills the PTY process and relaunches without --resume.
func (m *Manager) RestartAgentProcess(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if current, currentExists := m.executionStore.Get(executionID); !currentExists || current != execution {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	return m.restartAgentProcess(ctx, executionID, nil)
}

// restartAgentProcess restarts an ACP process. When runtimeConfigOverride is
// supplied, it is the immutable pre-reset snapshot and must be used instead of
// reading state again after ResetAgentContext has cleared the old catalog.
func (m *Manager) restartAgentProcess(
	ctx context.Context,
	executionID string,
	runtimeConfigOverride *models.SessionRuntimeConfig,
) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}

	// Passthrough agents: kill PTY and relaunch fresh (no --resume).
	if execution.PassthroughProcessID != "" {
		return m.restartPassthroughProcess(ctx, execution)
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	releaseClient()

	preparation, err := m.prepareAgentRestart(ctx, execution, runtimeConfigOverride)
	if err != nil {
		return err
	}

	// 1. Close WebSocket streams (updates + workspace). Use per-stream Close
	// methods rather than client.Close — the latter is a terminal drain
	// barrier that flips the client into a closed state and would block
	// every StreamUpdates/StreamWorkspace call that this same restart path
	// makes a few lines below.
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	client.CloseUpdatesStream()
	client.CloseWorkspaceStream()
	releaseClient()

	// 2. Stop the agent subprocess via agentctl (keeps agentctl server alive)
	m.stopAgentProcessForRestart(ctx, execution)

	// 3. Reset execution state after the replacement command has been validated.
	m.resetAgentRestartState(executionID, preparation.commands)

	// 4. Wait for agentctl to be ready (it should still be running)
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	err = client.WaitForReady(ctx, 30*time.Second)
	releaseClient()
	if err != nil {
		m.updateExecutionError(executionID, "agentctl not ready after restart: "+err.Error())
		return fmt.Errorf("agentctl not ready after restart: %w", err)
	}

	// 5. Reconfigure and start new agent subprocess
	approvalPolicy, _ := m.resolveApprovalPolicyAndDisplayName(ctx, execution)
	if _, err := m.configureAndStartAgent(ctx, execution, approvalPolicy); err != nil {
		m.updateExecutionError(executionID, "failed to restart agent: "+err.Error())
		return fmt.Errorf("failed to restart agent: %w", err)
	}

	// 6. Wait for agent process to initialize
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	err = client.WaitForReady(ctx, 10*time.Second)
	releaseClient()
	if err != nil {
		m.logger.Warn("agent process slow to initialize after restart, continuing",
			zap.String("execution_id", executionID),
			zap.Error(err))
	}

	mcpServers, err := m.resolveMcpServers(ctx, execution, preparation.agentConfig)
	if err != nil {
		return fmt.Errorf("failed to resolve MCP config for restart: %w", err)
	}

	if err := m.initializeACPSessionForRestart(ctx, execution, preparation.agentConfig, mcpServers); err != nil {
		m.updateExecutionError(executionID, "failed to initialize ACP session after restart: "+err.Error())
		return fmt.Errorf("failed to initialize ACP session after restart: %w", err)
	}

	if !cacheFreshSessionModelState(execution) &&
		(preparation.runtimeConfig.Model != "" || len(preparation.runtimeConfig.ConfigOptions) > 0) {
		waitForFreshSessionModelState(ctx, m.logger, execution)
	}
	if err := m.restoreSessionRuntimeConfig(ctx, execution, execution.ACPSessionID, preparation.runtimeConfig); err != nil {
		m.updateExecutionError(executionID, "failed to restore session runtime configuration after restart: "+err.Error())
		return fmt.Errorf("failed to restore session runtime configuration after restart: %w", err)
	}
	if err := m.updateStatusAndPersist(ctx, execution.ID, v1.AgentStatusReady); err != nil {
		m.updateExecutionError(executionID, "failed to mark restarted agent ready: "+err.Error())
		return fmt.Errorf("failed to mark restarted agent ready: %w", err)
	}
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentBootReady, execution)

	m.logger.Info("agent process restarted with fresh context",
		zap.String("execution_id", executionID),
		zap.String("session_id", execution.SessionID),
		zap.String("new_acp_session_id", execution.ACPSessionID))

	m.eventPublisher.PublishAgentEvent(ctx, events.AgentContextReset, execution)
	return nil
}

func (m *Manager) stopAgentProcessForRestart(ctx context.Context, execution *AgentExecution) {
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return
	}
	if err := client.Stop(ctx); err != nil {
		m.logger.Warn("failed to stop agent subprocess during restart",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		// Continue — the process may already be stopped
	}
}

type agentRestartPreparation struct {
	agentConfig   agents.Agent
	commands      agentCommands
	runtimeConfig models.SessionRuntimeConfig
}

// prepareAgentRestart builds and validates the replacement before touching the current process.
func (m *Manager) prepareAgentRestart(
	ctx context.Context,
	execution *AgentExecution,
	runtimeConfigOverride *models.SessionRuntimeConfig,
) (agentRestartPreparation, error) {
	m.logger.Info("restarting agent process for context reset",
		zap.String("execution_id", execution.ID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))

	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return agentRestartPreparation{}, fmt.Errorf("failed to get agent config for restart: %w", err)
	}
	commands, err := m.buildFreshAgentCommand(ctx, execution, agentConfig)
	if err != nil {
		return agentRestartPreparation{}, fmt.Errorf("failed to rebuild agent command for restart: %w", err)
	}
	var runtimeConfig models.SessionRuntimeConfig
	if runtimeConfigOverride == nil {
		runtimeConfig = m.captureSessionRuntimeConfigForReset(ctx, execution)
	} else {
		runtimeConfig = cloneSessionRuntimeConfig(*runtimeConfigOverride)
	}
	return agentRestartPreparation{
		agentConfig:   agentConfig,
		commands:      commands,
		runtimeConfig: runtimeConfig,
	}, nil
}

func cloneSessionRuntimeConfig(config models.SessionRuntimeConfig) models.SessionRuntimeConfig {
	return models.SessionRuntimeConfig{
		Model:         config.Model,
		Mode:          config.Mode,
		ConfigOptions: maps.Clone(config.ConfigOptions),
	}
}

func (m *Manager) resetAgentRestartState(executionID string, commands agentCommands) {
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.ACPSessionID = ""
		exec.Status = v1.AgentStatusStarting
		exec.ErrorMessage = ""
		exec.needsResumeContext = false
		exec.resumeContextInjected = false
		exec.setSessionInitialized(false)
		exec.SetModelState(nil)
		exec.AgentCommand = commands.initial
		exec.ContinueCommand = commands.continue_
		exec.AgentArgs = commands.args
		exec.ContinueArgs = commands.continueArgs
		m.resetStreamingStateWithHistory(exec)
		select {
		case <-exec.promptDoneCh:
		default:
		}
		exec.dispatchedPromptPending.Store(false)
	})
}

// initializeACPSessionForRestart connects streams and creates a new ACP session without
// sending an initial prompt. The caller (workflow processOnEnter) handles prompting separately.
func (m *Manager) initializeACPSessionForRestart(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	mcpServers []agentctltypes.McpServer,
) error {
	// Connect WebSocket streams
	if m.streamManager != nil {
		updatesReady := make(chan struct{})
		m.streamManager.ConnectAll(execution, updatesReady)

		select {
		case <-updatesReady:
			m.logger.Debug("updates stream ready after restart")
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for agent stream to connect after restart")
		}
	}

	// Initialize ACP session (always session/new since ACPSessionID was cleared)
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agentctl client is unavailable")
	}
	result, err := m.sessionManager.InitializeSession(
		ctx,
		client,
		agentConfig,
		"", // empty — force session/new
		execution.WorkspacePath,
		mcpServers,
	)
	if err != nil {
		return fmt.Errorf("ACP session initialization failed: %w", err)
	}

	execution.ACPSessionID = result.SessionID

	if m.sessionManager.eventPublisher != nil {
		m.sessionManager.eventPublisher.PublishACPSessionCreated(execution, result.SessionID)
	}

	return nil
}

// GetExecution returns an agent execution by ID.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// The returned execution pointer should not be modified directly - use the Manager's
// methods to update execution state (MarkReady, MarkCompleted, UpdateStatus).
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecution(executionID string) (*AgentExecution, bool) {
	return m.executionStore.Get(executionID)
}

// GetExecutionBySessionID returns the agent execution for a session from the in-memory store only.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// A session can have at most one active execution at a time. If a session exists
// but has no active execution, this returns (nil, false).
//
// For workspace-oriented operations that need restart recovery,
// use GetOrEnsureExecution instead.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecutionBySessionID(sessionID string) (*AgentExecution, bool) {
	return m.executionStore.GetBySessionID(sessionID)
}

// ResolveTaskEnvironmentID returns the task environment ID for a session.
// User shell resources must be environment-scoped; missing mappings are
// lifecycle errors and must not be converted into session-scoped shell state.
func (m *Manager) ResolveTaskEnvironmentID(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}
	if exec, ok := m.executionStore.GetBySessionID(sessionID); ok {
		if exec.TaskEnvironmentID != "" {
			return exec.TaskEnvironmentID, nil
		}
		return "", fmt.Errorf("session %s has no task environment ID", sessionID)
	}
	if m.workspaceInfoProvider == nil {
		return "", fmt.Errorf("workspace info provider not configured")
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve task environment for session %s: %w", sessionID, err)
	}
	if info == nil || info.TaskEnvironmentID == "" {
		return "", fmt.Errorf("session %s has no task environment ID", sessionID)
	}
	return info.TaskEnvironmentID, nil
}

// IsRemoteSession checks whether a session is associated with a remote executor
// (e.g., sprites). It first checks the in-memory execution store, then falls back
// to the database via WorkspaceInfoProvider. This is useful when the execution
// hasn't been recreated yet after a backend restart.
func (m *Manager) IsRemoteSession(ctx context.Context, sessionID string) bool {
	// Check in-memory execution first (fast path).
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		if execution.RuntimeName == executor.NameSprites {
			return true
		}
		return execution.metadataBool(MetadataKeyIsRemote)
	}

	// Fall back to database records (post-restart, execution not yet recreated).
	if m.workspaceInfoProvider == nil {
		return false
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil || info == nil {
		return false
	}
	if models.IsRemoteExecutorType(models.ExecutorType(info.ExecutorType)) {
		return true
	}
	// Backwards compatibility: old records may only have RuntimeName set.
	return info.RuntimeName == executor.NameSprites || info.RuntimeName == executor.NameRemoteDocker
}

// ShouldUseContainerShell checks whether a session's shell should run inside a container/sandbox
// (via agentctl) rather than on the host. This is true for Docker, Sprites, and remote executors.
// It first checks the in-memory execution store, then falls back to the database.
func (m *Manager) ShouldUseContainerShell(ctx context.Context, sessionID string) bool {
	// Check in-memory execution first (fast path).
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		// Docker and Sprites executors run shells inside the container/sandbox
		if execution.RuntimeName == executor.NameDocker ||
			execution.RuntimeName == executor.NameSprites {
			return true
		}
		return execution.metadataBool(MetadataKeyIsRemote)
	}

	// Fall back to database records (post-restart, execution not yet recreated).
	if m.workspaceInfoProvider == nil {
		return false
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil || info == nil {
		return false
	}
	if models.IsContainerizedExecutorType(models.ExecutorType(info.ExecutorType)) {
		return true
	}
	// Backwards compatibility: old records may only have RuntimeName set.
	return info.RuntimeName == executor.NameDocker ||
		info.RuntimeName == executor.NameSprites ||
		info.RuntimeName == executor.NameRemoteDocker
}

// GetAvailableCommandsForSession returns the available slash commands for a session.
// Returns nil if the session doesn't exist or has no commands stored.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetAvailableCommandsForSession(sessionID string) []streams.AvailableCommand {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetAvailableCommands()
}

// GetModeStateForSession returns the cached session mode state.
// Returns nil if the session has no execution or no mode state cached.
func (m *Manager) GetModeStateForSession(sessionID string) *CachedModeState {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetModeState()
}

// GetModelStateForSession returns the cached session model state.
// Returns nil if the session has no execution or no model state cached.
func (m *Manager) GetModelStateForSession(sessionID string) *CachedModelState {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetModelState()
}

// ListExecutions returns all currently tracked agent executions.
//
// Returns a snapshot of all executions in memory at the time of call. The returned slice
// contains pointers to execution objects that may be modified by other goroutines after
// this method returns. Do not modify execution state directly - use Manager methods instead.
//
// The list includes executions in all states:
//   - Starting (process launching, agentctl initializing)
//   - Running (actively processing prompts)
//   - Ready (waiting for user input)
//   - Completed/Failed (finished but not yet removed)
//
// Thread-safe: Can be called concurrently. Returns a new slice on each call.
//
// Typical usage: Status endpoints, debugging, cleanup loops.
func (m *Manager) ListExecutions() []*AgentExecution {
	return m.executionStore.List()
}

const agentStartupLivenessGrace = 75 * time.Second

// ProbeAgentRunningForSession checks if an agent process is running or
// starting for a session and reports probe failures to the caller.
//
// For passthrough sessions (direct PTY mode), it checks whether the PTY process is alive
// in the InteractiveRunner. For ACP sessions, it probes agentctl's status endpoint.
//
// A successful probe returns true if:
//   - Passthrough process is alive in the InteractiveRunner
//   - Agent status is "running" (actively processing prompts)
//   - Agent status is "starting" (process launched but not yet ready)
//
// A successful probe returns false if:
//   - No execution exists for this session
//   - Passthrough process ID is set but process is not alive
//   - Agent is in any other state (stopped, failed, etc.)
//
// An unavailable agentctl client, interactive runner, or status endpoint is
// returned as an error rather than as a false result.
func (m *Manager) ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error) {
	// First check if we have an execution tracked for this session
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return false, nil
	}

	// Launch registers the execution before agentctl is guaranteed to answer
	// status requests. During that bounded boot window, a failed status probe
	// means "not ready yet", not "stale". Treat the execution-store STARTING
	// state as live long enough to cover StartAgentProcess's 60-second agentctl
	// readiness deadline; after the grace expires, normal probing allows stale
	// starting entries to be reaped.
	if execution.Status == v1.AgentStatusStarting &&
		!execution.StartedAt.IsZero() &&
		time.Since(execution.StartedAt) <= agentStartupLivenessGrace {
		return true, nil
	}

	// Passthrough sessions run as direct PTY processes via InteractiveRunner,
	// bypassing agentctl's ACP protocol. Check the process directly.
	if execution.PassthroughProcessID != "" {
		if runner := m.GetInteractiveRunner(); runner != nil {
			return runner.IsProcessReadyOrPending(execution.PassthroughProcessID), nil
		}
		return false, fmt.Errorf("interactive runner is unavailable")
	}

	// Probe agentctl status to verify the agent process is running
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return false, fmt.Errorf("agentctl client is unavailable")
	}

	status, err := client.GetStatus(ctx)
	if err != nil {
		m.logger.Debug("failed to get agentctl status",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false, err
	}

	return status.IsAgentRunning(), nil
}

// IsAgentRunningForSession preserves the legacy boolean probe for callers
// that do not need to distinguish a dead process from an unavailable probe.
func (m *Manager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	running, _ := m.ProbeAgentRunningForSession(ctx, sessionID)
	return running
}

// IsAgentReadyForPrompt reports whether the session can accept an ACP prompt
// immediately. A process can be "running" while its update stream is still
// reconnecting after resume; PromptAgent needs the stream-backed request path.
func (m *Manager) IsAgentReadyForPrompt(ctx context.Context, sessionID string) bool {
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return false
	}

	if execution.PassthroughProcessID != "" || execution.IsPassthrough {
		return m.IsAgentRunningForSession(ctx, sessionID)
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if execution.Status != v1.AgentStatusReady || client == nil {
		return false
	}
	if !execution.isSessionInitialized() || execution.ACPSessionID == "" {
		return false
	}

	return client.HasAgentStream()
}

func (m *Manager) RecoverAgentPromptStream(ctx context.Context, sessionID string) error {
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("session %q has no execution: %w", sessionID, ErrExecutionNotFound)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if execution.PassthroughProcessID != "" || execution.IsPassthrough || client == nil {
		releaseClient()
		return nil
	}
	// InitializeAndPrompt owns the first updates stream. Starting a recovery
	// stream before ACP initialization finishes creates competing consumers and
	// can split one prompt's events across them.
	if !execution.isSessionInitialized() || execution.ACPSessionID == "" {
		releaseClient()
		return nil
	}
	if client.HasAgentStream() {
		releaseClient()
		return nil
	}
	releaseClient()
	if m.streamManager == nil {
		return fmt.Errorf("stream manager is not configured")
	}

	ready := make(chan struct{})
	// Prompt recovery is reached through the per-session prompt path. Avoid a
	// broader reconnect registry here; HasAgentStream above covers steady state.
	m.streamManager.connectUpdatesStreamAsync(execution, ready)
	select {
	case <-ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("agentctl client is unavailable")
	}
	hasAgentStream := client.HasAgentStream()
	releaseClient()
	if !hasAgentStream {
		return fmt.Errorf("agent stream not connected")
	}
	if execution.Status == v1.AgentStatusFailed && execution.isSessionInitialized() && execution.ACPSessionID != "" {
		return m.restoreRecoveredFailedExecution(ctx, execution)
	}
	return nil
}

func (m *Manager) restoreRecoveredFailedExecution(ctx context.Context, execution *AgentExecution) error {
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agentctl client is unavailable")
	}
	status, err := client.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify agent status after stream recovery: %w", err)
	}
	if !status.IsAgentRunning() {
		return fmt.Errorf("agent process is not running after stream recovery: %s", status.AgentStatus)
	}
	if err := m.markBootReadyFromFailed(ctx, execution.ID); err != nil {
		return err
	}
	return nil
}

// UpdateStatus updates the status of an execution
func (m *Manager) UpdateStatus(executionID string, status v1.AgentStatus) error {
	return m.updateStatusAndPersist(context.Background(), executionID, status)
}

func (m *Manager) updateStatusAndPersist(ctx context.Context, executionID string, status v1.AgentStatus) error {
	var updated *AgentExecution
	if err := m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.Status = status
		updated = execution
	}); err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return fmt.Errorf("execution %q not found", executionID)
		}
		return err
	}

	m.logger.Debug("updated execution status",
		zap.String("execution_id", executionID),
		zap.String("status", string(status)))

	if updated != nil {
		m.persistExecutorRunning(context.WithoutCancel(ctx), updated)
	}
	return nil
}

// BeginPrompt advances prompt ownership and marks the execution running before dispatch.
func (m *Manager) BeginPrompt(executionID string) (uint64, error) {
	generation, err := m.executionStore.BeginPrompt(executionID)
	if err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return 0, fmt.Errorf("execution %q not found", executionID)
		}
		return 0, err
	}

	if updated, exists := m.executionStore.Get(executionID); exists {
		m.persistExecutorRunning(context.Background(), updated)
	}
	return generation, nil
}

// OwnsPromptGeneration reports whether a ready event's immutable execution and
// prompt generation still identify the lifecycle prompt active for sessionID.
func (m *Manager) OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool {
	return m.executionStore.OwnsPromptGeneration(sessionID, executionID, generation)
}

// OwnsPromptActivity reports whether a stall snapshot still belongs to the
// active prompt and no genuine event arrived after the snapshot was captured.
func (m *Manager) OwnsPromptActivity(
	sessionID, executionID string,
	generation, activityEpoch uint64,
) bool {
	return m.executionStore.OwnsPromptActivity(sessionID, executionID, generation, activityEpoch)
}

// GetPromptGenerationForSession returns the generation currently owned by the
// active prompt for sessionID. Orchestrator cancellation uses this together
// with the execution and turn IDs to reject stale terminal events while the
// lifecycle manager waits for an acknowledged cancel to settle.
func (m *Manager) GetPromptGenerationForSession(_ context.Context, sessionID string) (uint64, error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return 0, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	return execution.promptGenerationSnapshot(), nil
}

// GetPromptActivityForSession returns the execution ID, prompt generation,
// activity epoch, and last-activity timestamp currently owned by sessionID's
// active prompt. Unlike OwnsPromptGeneration/OwnsPromptActivity (which check
// a value someone else already captured), this is the capture step itself —
// for watchdogs that scan for stuck sessions and must snapshot "is anything
// happening right now" on their own, rather than waiting for a stall event to
// carry the values in its payload. lastActivityAt in particular is what lets
// a caller gate on real elapsed inactivity (time.Since(lastActivityAt))
// instead of an epoch comparison across its own scan window, which only
// catches activity that lands during that window and sails through a live
// agent that is simply between events. Returns ErrNoExecutionForSession
// (wrapped) when no execution is tracked for the session.
func (m *Manager) GetPromptActivityForSession(_ context.Context, sessionID string) (executionID string, generation, activityEpoch uint64, lastActivityAt time.Time, err error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return "", 0, 0, time.Time{}, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	lastActivityAt, _, activityEpoch = execution.promptActivitySnapshot()
	return execution.ID, execution.promptGenerationSnapshot(), activityEpoch, lastActivityAt, nil
}

// MarkReady marks an execution as ready for follow-up prompts AFTER A TURN.
// Use MarkBootReady instead when the agent has just initialized and hasn't yet
// processed a turn — orchestrator subscribers rely on the distinction.
//
// This transitions the execution to the "ready" state, indicating the agent has finished
// processing the current prompt and is waiting for user input. Called when:
//   - Agent finishes processing a prompt (via stream completion event)
//   - User cancels an agent turn (to allow new prompts)
//
// State Machine Transitions:
//
//	Running -> Ready (after prompt completion)
//	Any     -> Ready (after cancel)
//
// Publishes events.AgentReady. Returns error if execution not found.
func (m *Manager) MarkReady(executionID string) error {
	return m.markReadyEvent(executionID, events.AgentReady)
}

// MarkBootReady marks a freshly-initialized execution as ready for its first
// prompt. Distinct from MarkReady so the orchestrator can disambiguate boot
// signals from turn-end signals without race-prone flag tracking. Use this
// from session/init paths and post-context-reset; use MarkReady from the
// turn-completion path.
//
// State Machine Transitions:
//
//	Starting -> Ready (after initialization)
//
// Publishes events.AgentBootReady. Returns error if execution not found.
func (m *Manager) MarkBootReady(executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if exists {
		m.finalWorkspaceRefresh(execution, "startup_grace")
	}
	err := m.markReadyEventWithContext(context.Background(), executionID, events.AgentBootReady, false)
	if err == nil {
		if exists {
			m.setRuntimeInterest(execution.SessionID, false)
		}
		m.releaseActivity(executionActivityKey(executionID))
	}
	return err
}

// markReadyEvent is the shared body of MarkReady / MarkBootReady — both flip
// the execution to the Ready status and publish their respective event type.
func (m *Manager) markReadyEvent(executionID, eventType string) error {
	return m.markReadyEventWithContext(context.Background(), executionID, eventType, false)
}

func (m *Manager) markBootReadyFromFailed(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	if execution.Status != v1.AgentStatusFailed {
		return nil
	}
	return m.markReadyEventWithContext(ctx, executionID, events.AgentBootReady, false)
}

// markReadyEventWithContext flips executionID to Ready and publishes
// eventType. When asyncPublish is false (MarkReady, MarkBootReady, and
// markBootReadyFromFailed all pass false — unchanged behavior), the publish
// happens inline before this returns, matching the event bus's synchronous,
// order-preserving delivery to registered subscribers.
//
// When asyncPublish is true (escalateStuckCancel only — see its own doc
// comment), the publish is dispatched on its own goroutine instead. That
// caller is reached from Service.CancelAgent (and cancelAgentSilent's other
// guard-holding callers) while still holding sessionID's per-session
// cancelInFlightGuard: the in-memory event bus delivers to a queue
// subscription's handler *synchronously*, on the publishing goroutine, and
// the orchestrator's handleAgentReady is registered on exactly such a queue
// subscription for this event type. An inline publish here would have
// handleAgentReady try to re-acquire that same guard from the very same
// goroutine that is already holding it — sync.Mutex is not reentrant, so
// that blocks forever. Deferring the publish lets the guard-holding caller
// finish (and release the guard) first. The detached publish receives the
// immutable payload captured while Ready was set, so handleAgentReady can
// reject it if another prompt generation starts before delivery.
func (m *Manager) markReadyEventWithContext(ctx context.Context, executionID, eventType string, asyncPublish bool) error {
	var payload AgentEventPayload
	var updated *AgentExecution
	var alreadyReady bool
	if err := m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		if execution.Status == v1.AgentStatusReady {
			alreadyReady = true
			return
		}
		execution.Status = v1.AgentStatusReady
		payload = newAgentEventPayload(execution)
		updated = execution
	}); err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return fmt.Errorf("execution %q not found", executionID)
		}
		return err
	}
	if alreadyReady {
		return nil
	}
	m.persistExecutorRunning(context.WithoutCancel(ctx), updated)

	m.logger.Info("execution ready",
		zap.String("execution_id", executionID),
		zap.String("event_type", eventType))

	if asyncPublish {
		go m.eventPublisher.publishAgentEventPayload(context.Background(), eventType, payload)
	} else {
		m.eventPublisher.publishAgentEventPayload(ctx, eventType, payload)
	}
	return nil
}

// MarkCompleted marks an execution as completed or failed.
//
// This is called when the agent process terminates, either successfully or with an error.
// The final status is determined by exit code and error message:
//
//   - exitCode == 0 && errorMessage == "" → AgentStatusCompleted (success)
//   - Otherwise                            → AgentStatusFailed (failure)
//
// Parameters:
//   - executionID: The execution to mark as completed
//   - exitCode: Process exit code (0 = success, non-zero = failure)
//   - errorMessage: Human-readable error description (empty string if no error)
//
// State Machine:
//
//	This is a terminal state transition - no further state changes are expected after this.
//	Typical flow: Starting -> Running -> Ready -> ... -> Completed/Failed
//
// Publishes either AgentCompleted or AgentFailed event depending on final status.
//
// Returns error if execution not found.
func (m *Manager) MarkCompleted(executionID string, exitCode int, errorMessage string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	return m.markCompletedWithTurnID(
		executionID,
		exitCode,
		errorMessage,
		execution.promptTurnIDSnapshot(),
		nil,
	)
}

func (m *Manager) markCompletedWithTurnID(
	executionID string,
	exitCode int,
	errorMessage, turnID string,
	failureEvidence *PromptAttemptEvidence,
) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	// Guard against duplicate completion (e.g. ACP prompt error + process exit error).
	// MarkCompleted is a terminal transition — once in Completed/Failed/Stopped, skip
	// re-publishing. Stopped is a terminal state too (shutdown-race abort, StopAgent),
	// so a second completion must not overwrite it or re-emit an event.
	if isTerminalStatus(execution.Status) {
		m.logger.Warn("ignoring duplicate MarkCompleted for already-terminal execution",
			zap.String("execution_id", executionID),
			zap.String("current_status", string(execution.Status)),
			zap.Int("exit_code", exitCode))
		return nil
	}

	// A turn aborted because backend graceful shutdown killed the agent
	// subprocess is not an agent failure. Treat the terminal error as a benign
	// stop so the session stays resumable and the UI shows no red error banner.
	if (exitCode != 0 || errorMessage != "") && m.IsShuttingDown() {
		return m.markStoppedDuringShutdown(execution, exitCode, errorMessage, turnID)
	}
	if (exitCode != 0 || errorMessage != "") && isUninitializedStartupExecution(execution) {
		m.logger.Debug("deferring uninitialized startup process exit to startup owner",
			zap.String("execution_id", execution.ID),
			zap.Int("exit_code", exitCode))
		return nil
	}
	if (exitCode != 0 || errorMessage != "") && failureEvidence == nil {
		evidence := execution.promptAttemptEvidenceSnapshot()
		failureEvidence = &evidence
	}

	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		now := time.Now()
		exec.FinishedAt = &now
		exec.ExitCode = &exitCode
		exec.ErrorMessage = errorMessage

		if exitCode == 0 && errorMessage == "" {
			exec.Status = v1.AgentStatusCompleted
		} else {
			exec.Status = v1.AgentStatusFailed
		}
	})

	// End session trace span
	execution.EndSessionSpan()

	// Persist the terminal status to executors_running. Unlike the Ready/Running
	// transitions (which flow through updateStatusAndPersist), MarkCompleted is
	// the process-exit/crash boundary and historically skipped persistence — so a
	// row kept claiming a `running`/`starting` process after it had exited
	// (#1597). Re-stamping here leaves the row truthful (terminal
	// status + fresh last_seen_at) the moment the process is gone.
	m.persistExecutorRunning(context.Background(), execution)
	m.releaseActivity(executionActivityKey(executionID))

	m.logger.Info("execution completed",
		zap.String("execution_id", executionID),
		zap.Int("exit_code", exitCode),
		zap.String("status", string(execution.Status)))

	// Publish completion event
	eventType := events.AgentCompleted
	if execution.Status == v1.AgentStatusFailed {
		eventType = events.AgentFailed
		m.classifyAndMaybeRemediate(execution, exitCode, errorMessage)
	}
	if eventType == events.AgentFailed {
		m.eventPublisher.publishAgentEventWithTurnIDAndEvidence(
			context.Background(), eventType, execution, turnID, failureEvidence,
		)
		return nil
	}
	m.eventPublisher.publishAgentEventWithTurnID(context.Background(), eventType, execution, turnID)

	return nil
}

// isTerminalStatus reports whether a status is a final execution state that
// MarkCompleted / markStoppedDuringShutdown must not overwrite or re-publish.
func isTerminalStatus(status v1.AgentStatus) bool {
	return status == v1.AgentStatusCompleted ||
		status == v1.AgentStatusFailed ||
		status == v1.AgentStatusStopped
}

// markStoppedDuringShutdown records a terminal error completion that arrived
// while backend graceful shutdown was in progress as a benign STOPPED outcome
// instead of a FAILED one. It stamps the terminal fields, runs the same teardown
// as the failed branch, and publishes events.AgentStopped — mirroring the
// StopReasonBackendShutdown teardown so the orchestrator treats it as resumable.
// It deliberately does not remove the execution or run classifyAndMaybeRemediate.
//
// Both terminal chokepoints (handleCompleteEventMarkState and MarkCompleted) can
// reach this during shutdown for the same execution (ACP error event + process
// exit). The transition is therefore guarded under the store lock: if the
// execution is already terminal, or was removed by StopAgentWithReason, this is a
// stale/duplicate callback — skip teardown and the AgentStopped publish so the
// orchestrator does not process the same stop twice.
func (m *Manager) markStoppedDuringShutdown(
	execution *AgentExecution, exitCode int, errorMessage string, turnIDs ...string,
) error {
	applied := false
	err := m.executionStore.WithLock(execution.ID, func(exec *AgentExecution) {
		if isTerminalStatus(exec.Status) {
			return
		}
		now := time.Now()
		exec.FinishedAt = &now
		exec.ExitCode = &exitCode
		exec.ErrorMessage = errorMessage
		exec.Status = v1.AgentStatusStopped
		applied = true
	})
	if err != nil || !applied {
		m.logger.Debug("ignoring stale/duplicate shutdown completion",
			zap.String("execution_id", execution.ID),
			zap.String("current_status", string(execution.Status)),
			zap.Error(err))
		return nil
	}

	m.logger.Warn("error completion during shutdown, treating as cancellation",
		zap.String("execution_id", execution.ID),
		zap.String("task_id", execution.TaskID),
		zap.Int("exit_code", exitCode),
		zap.String("error", errorMessage))

	execution.EndSessionSpan()
	m.persistExecutorRunning(context.Background(), execution)
	m.releaseActivity(executionActivityKey(execution.ID))

	turnID := ""
	if len(turnIDs) > 0 {
		turnID = turnIDs[0]
	}
	m.eventPublisher.publishAgentEventWithTurnID(context.Background(), events.AgentStopped, execution, turnID)

	return nil
}

// classifyAndMaybeRemediate runs routingerr.Classify at the failure
// boundary, logs the structured result, and - for codes that carry a
// safe remediation (currently CodeNpxCacheCorrupted) - fires a
// best-effort cleanup goroutine so the next launch attempt (Office
// scheduler retry or Kanban "Resume") finds a clean environment.
//
// The remediation hook is injectable via Manager.remediateNpxCache;
// production wiring points it at routingerr.RemediateNpxCache. Tests
// stub it to avoid touching the real filesystem.
func (m *Manager) classifyAndMaybeRemediate(execution *AgentExecution, exitCode int, errorMessage string) {
	phase := routingerr.PhaseSessionInit
	if execution.isSessionInitialized() {
		phase = routingerr.PhasePromptSend
	}
	var exitPtr *int
	if exitCode != 0 {
		ec := exitCode
		exitPtr = &ec
	}
	e := routingerr.Classify(routingerr.Input{
		Phase:      phase,
		ProviderID: execution.AgentID,
		ExitCode:   exitPtr,
		Stderr:     errorMessage,
	})
	if e == nil {
		return
	}
	m.logger.Info("agent failure classified for provider routing",
		zap.String("execution_id", execution.ID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID),
		zap.String("provider_id", execution.AgentID),
		zap.String("phase", string(e.Phase)),
		zap.String("routing_code", string(e.Code)),
		zap.String("confidence", string(e.Confidence)),
		zap.String("classifier_rule", e.ClassifierRule),
		zap.Bool("fallback_allowed", e.FallbackAllowed),
		zap.Bool("auto_retryable", e.AutoRetryable),
		zap.Bool("user_action", e.UserAction),
		zap.String("remediation_path", e.RemediationPath))

	if e.Code != routingerr.CodeNpxCacheCorrupted || e.RemediationPath == "" {
		return
	}
	remediate := m.remediateNpxCache
	path := e.RemediationPath
	execID := execution.ID
	zapLog := m.logger.Zap().With(
		zap.String("execution_id", execID),
		zap.String("path", path),
	)
	go func() {
		if err := remediate(path, zapLog); err != nil {
			m.logger.Warn("npx cache remediation failed",
				zap.String("execution_id", execID),
				zap.String("path", path),
				zap.Error(err))
		}
	}()
}

// RespondToPermission sends a response to an agent's permission request.
//
// When an agent requests permission (e.g., to run a bash command, modify files, etc.),
// it pauses execution and waits for user approval. This method sends the user's response.
//
// Parameters:
//   - executionID: The agent execution waiting for permission
//   - pendingID: Unique ID of the permission request (from permission request event)
//   - optionID: The user-selected option ID (from the permission request's options array)
//   - cancelled: If true, indicates user cancelled/rejected the permission request.
//     When cancelled=true, optionID is ignored.
//
// Response Semantics:
//   - cancelled=false, optionID="approve" → User approved the action
//   - cancelled=false, optionID="deny"    → User explicitly denied the action
//   - cancelled=true, optionID=""         → User cancelled/closed the dialog
//
// After receiving the response, the agent will either:
//   - Continue executing (if approved)
//   - Skip the action and report failure (if denied/cancelled)
//
// Timeout: 30 seconds for agentctl to acknowledge the response.
//
// Returns error if execution not found, agentctl unavailable, or communication fails.
func (m *Manager) RespondToPermission(executionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("agent execution not found: %s", executionID)
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agent execution has no agentctl client: %s", executionID)
	}

	m.logger.Info("responding to permission request",
		zap.String("execution_id", executionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.RespondToPermission(ctx, pendingID, optionID, cancelled)
}

// RespondToPermissionBySessionID sends a response to a permission request using session ID.
//
// Convenience method that looks up the execution by session ID and delegates to RespondToPermission.
// See RespondToPermission for parameter semantics and behavior.
func (m *Manager) RespondToPermissionBySessionID(sessionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	return m.RespondToPermission(execution.ID, pendingID, optionID, cancelled)
}

// ListPendingPermissionsBySessionID reads safe live snapshots from the current
// execution for a task session. It never starts or resumes an execution.
func (m *Manager) ListPendingPermissionsBySessionID(ctx context.Context, sessionID string) ([]streams.PendingAgentPermission, error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agent execution has no agentctl client: %s", execution.ID)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return client.ListPendingPermissions(requestCtx)
}

// ResolvePermissionBySessionID selects one exact option on one exact request
// generation in the current execution.
func (m *Manager) ResolvePermissionBySessionID(ctx context.Context, sessionID, requestID, pendingID, optionID string) (*streams.PermissionResolveResponse, error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agent execution has no agentctl client: %s", execution.ID)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return client.ResolvePermission(requestCtx, requestID, pendingID, optionID)
}

func (m *Manager) CancelPermissionBySessionID(ctx context.Context, sessionID, requestID, pendingID string) (*streams.PermissionCancelResponse, error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agent execution has no agentctl client: %s", execution.ID)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return client.CancelPermission(requestCtx, requestID, pendingID)
}

// stopAgentViaBackend stops the agent execution via the runtime that created it.
func (m *Manager) stopAgentViaBackend(ctx context.Context, executionID string, execution *AgentExecution, reason string, force bool, agentStopFailed bool) error {
	if execution.RuntimeName == "" || m.executorRegistry == nil {
		return nil
	}
	rt, err := m.executorRegistry.GetBackend(execution.RuntimeName)
	if err != nil {
		m.logger.Warn("failed to get runtime for stopping execution",
			zap.String("execution_id", executionID),
			zap.Stringer("runtime", execution.RuntimeName),
			zap.Error(err))
		return fmt.Errorf("get runtime %s: %w", execution.RuntimeName, err)
	}
	m.stopPassthroughProcess(ctx, executionID, execution, rt)
	runtimeInstance := &ExecutorInstance{
		InstanceID:           execution.ID,
		TaskID:               execution.TaskID,
		SessionID:            execution.SessionID,
		ContainerID:          execution.ContainerID,
		StandaloneInstanceID: execution.standaloneInstanceID,
		StandalonePort:       execution.standalonePort,
		Metadata:             execution.MetadataSnapshot(),
		StopReason:           reason,
		AgentStopFailed:      agentStopFailed,
	}
	if execution.RuntimeName == executor.NameKubernetes {
		metadata, resolveErr := m.currentKubernetesConnectionMetadata(ctx, runtimeInstance.Metadata)
		if resolveErr != nil {
			return fmt.Errorf("resolve current Kubernetes cleanup connection: %w", resolveErr)
		}
		runtimeInstance.Metadata = metadata
	}
	if err := rt.StopInstance(ctx, runtimeInstance, force); err != nil {
		// During shutdown the runtime instance may already be stopping or
		// absent. Only surface this at WARN outside shutdown.
		if m.IsShuttingDown() {
			m.logger.Debug("failed to stop runtime instance",
				zap.String("execution_id", executionID),
				zap.Error(err))
		} else {
			m.logger.Warn("failed to stop runtime instance",
				zap.String("execution_id", executionID),
				zap.Error(err))
		}
		return err
	}
	return nil
}

// stopPassthroughProcess stops the passthrough interactive process if one is running.
func (m *Manager) stopPassthroughProcess(ctx context.Context, executionID string, execution *AgentExecution, rt ExecutorBackend) {
	if execution.PassthroughProcessID == "" {
		return
	}
	interactiveRunner := rt.GetInteractiveRunner()
	if interactiveRunner == nil {
		return
	}
	if err := interactiveRunner.Stop(ctx, execution.PassthroughProcessID); err != nil {
		m.logger.Warn("failed to stop passthrough process",
			zap.String("execution_id", executionID),
			zap.String("process_id", execution.PassthroughProcessID),
			zap.Error(err))
		return
	}
	m.logger.Info("passthrough process stopped",
		zap.String("execution_id", executionID),
		zap.String("process_id", execution.PassthroughProcessID))
}

// buildFreshAgentCommand rebuilds the agent command without resume flags by going through
// the standard command-building pipeline with an empty SessionID. This works for all
// agent types because each agent's BuildCommand respects SessionID="" to skip resume flags.
//
// It fails closed: if the profile cannot be resolved, or a configured
// command_prefix cannot be tokenised, it returns an error so a context reset
// never relaunches a configured agent without its wrapper.
func (m *Manager) buildFreshAgentCommand(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent) (agentCommands, error) {
	var profileInfo *AgentProfileInfo
	if execution.AgentProfileID != "" && m.profileResolver != nil {
		pi, resolveErr := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
		if resolveErr != nil {
			// A profile was expected but could not be resolved. We cannot tell
			// whether it carried a sandbox prefix, so refuse to relaunch rather
			// than risk running unwrapped.
			return agentCommands{}, fmt.Errorf("resolve profile %s for restart: %w", execution.AgentProfileID, resolveErr)
		}
		profileInfo = pi
	}

	model := ""
	autoApprove := false
	permissionValues := make(map[string]bool)
	if profileInfo != nil {
		model = profileInfo.Model
		autoApprove = profileInfo.AutoApprove
		permissionValues[agents.PermissionKeyAutoApprove] = profileInfo.AutoApprove
		permissionValues["allow_indexing"] = profileInfo.AllowIndexing
		permissionValues["dangerously_skip_permissions"] = profileInfo.DangerouslySkipPermissions
	}
	if override := execution.metadataString(MetadataKeyModelOverride); override != "" {
		model = override
	}

	// Preserve the profile's cli_flags and command_prefix across restarts. A
	// context reset that dropped the launcher prefix would relaunch a
	// configured agent unwrapped.
	cliFlagTokens, commandPrefixTokens, err := m.resolveProfileLaunchTokens(profileInfo)
	if err != nil {
		return agentCommands{}, err
	}
	managedRuntimeVersion, err := m.resolveManagedRuntimeVersion(ctx, execution.RuntimeName, agentConfig)
	if err != nil {
		return agentCommands{}, err
	}

	opts := agents.CommandOptions{
		Model:               model,
		SessionID:           "", // Fresh start — no resume flags
		AutoApprove:         autoApprove,
		PermissionValues:    permissionValues,
		CLIFlagTokens:       cliFlagTokens,
		CommandPrefixTokens: commandPrefixTokens,
		// Runtime is "standalone" / "docker" / "sprites" — MockAgent
		// reads this to pick a bare name (container PATH lookup) vs.
		// an absolute host path.
		Runtime:               execution.RuntimeName,
		ManagedRuntimeVersion: managedRuntimeVersion,
	}
	args := m.commandBuilder.BuildCommandArgs(agentConfig, opts)
	continueArgs := m.commandBuilder.BuildContinueCommandArgs(agentConfig, opts)
	if err := validateBuiltAgentCommands(args, continueArgs); err != nil {
		return agentCommands{}, err
	}
	return newAgentCommands(args, continueArgs), nil
}
