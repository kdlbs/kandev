package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/settings/profileconfig"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/trace"
)

const (
	modelConfigOptionID                = "model"
	attachmentDeliveryPath             = "path"
	attachmentDeliveryPrompt           = "prompt"
	pendingDispatchedPromptWaitTimeout = 10 * time.Second
)

// PendingDispatchedPromptTimeoutError reports that a successor prompt could
// not observe completion of an earlier dispatch-only prompt within the
// bounded admission interval. The pending gate remains set so cancellation
// escalation remains the only path that can release the predecessor.
type PendingDispatchedPromptTimeoutError struct {
	ExecutionID string
	Timeout     time.Duration
}

func (e *PendingDispatchedPromptTimeoutError) Error() string {
	return fmt.Sprintf(
		"pending dispatched prompt completion timed out for execution %q after %s",
		e.ExecutionID,
		e.Timeout,
	)
}

// SessionManager handles ACP session initialization and management
type SessionManager struct {
	logger               *logger.Logger
	eventPublisher       *EventPublisher
	streamManager        *StreamManager
	executionStore       *ExecutionStore
	promptStarter        func(executionID string) (uint64, error)
	initialPromptFailure func(failure InitialPromptFailure)
	historyManager       *SessionHistoryManager
	attachmentReader     AttachmentReader
	stopCh               <-chan struct{} // For graceful shutdown coordination
	// beforeSteerDispatchHook, when set (tests only), runs inside tryDispatchSteer
	// after the active generation is selected but before the steer RPC, while
	// promptLifecycleMu is held — used to drive the completion-vs-dispatch race
	// deterministically.
	beforeSteerDispatchHook func()
	// beforePromptDispatchHook, when set (tests only), runs inside sendPrompt after
	// the generation is begun but before it is dispatched — used to pin a
	// predecessor in the admitted-but-undispatched window so a racing steer's
	// ordering can be observed deterministically.
	beforePromptDispatchHook func()
}

// InitialPromptFailure captures the immutable prompt identity at the point
// where initial delivery fails, before a successor can begin.
type InitialPromptFailure struct {
	ExecutionID      string
	SessionID        string
	PromptGeneration uint64
	TurnID           string
	Err              error
}

type sendPromptCallbacks struct {
	onDispatched func()
	onFailure    func(InitialPromptFailure)
}

// NewSessionManager creates a new SessionManager
func NewSessionManager(log *logger.Logger, stopCh <-chan struct{}) *SessionManager {
	return &SessionManager{
		logger: log,
		stopCh: stopCh,
	}
}

// SetDependencies sets the optional dependencies for full session orchestration.
// These are set after construction to avoid circular dependencies.
func (sm *SessionManager) SetDependencies(ep *EventPublisher, strm *StreamManager, store *ExecutionStore, history *SessionHistoryManager) {
	sm.eventPublisher = ep
	sm.streamManager = strm
	sm.executionStore = store
	sm.historyManager = history
}

// SetAttachmentReader wires the backend attachment reader used to materialize
// claimed file descriptors into the active agentctl session before prompts.
func (sm *SessionManager) SetAttachmentReader(reader AttachmentReader) {
	sm.attachmentReader = reader
}

func (sm *SessionManager) SetPromptStarter(starter func(executionID string) (uint64, error)) {
	sm.promptStarter = starter
}

func (sm *SessionManager) SetInitialPromptFailureHandler(handler func(failure InitialPromptFailure)) {
	sm.initialPromptFailure = handler
}

// InitializeResult contains the result of session initialization
type InitializeResult struct {
	AgentName    string
	AgentVersion string
	SessionID    string
}

// InitializeSession initializes an ACP session with the agent.
// It handles the initialize handshake and session creation/loading based on config.
//
// Session behavior:
//   - If agentConfig.Runtime().SessionConfig.NativeSessionResume is true AND existingSessionID is provided: use session/load
//   - If NativeSessionResume is false (CLI handles resume): always use session/new
//   - Otherwise: use session/new
func (sm *SessionManager) InitializeSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig agents.Agent,
	existingSessionID string,
	workspacePath string,
	mcpServers []agentctltypes.McpServer,
) (*InitializeResult, error) {
	rt := agentConfig.Runtime()
	sm.logger.Info("initializing ACP session",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("workspace_path", workspacePath),
		zap.Bool("native_session_resume", rt.SessionConfig.NativeSessionResume),
		zap.String("existing_session_id", existingSessionID))

	// Step 1: Send initialize request
	sm.logger.Info("sending ACP initialize request",
		zap.String("agent_type", agentConfig.ID()))

	agentInfo, err := client.Initialize(ctx, "kandev", "1.0.0")
	if err != nil {
		sm.logger.Error("ACP initialize failed",
			zap.String("agent_type", agentConfig.ID()),
			zap.Error(err))
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	result := &InitializeResult{
		AgentName:    "unknown",
		AgentVersion: "unknown",
	}
	if agentInfo != nil {
		result.AgentName = agentInfo.Name
		result.AgentVersion = agentInfo.Version
	}

	sm.logger.Info("ACP initialize response received",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("agent_name", result.AgentName),
		zap.String("agent_version", result.AgentVersion))

	// Step 2: Create or resume ACP session based on configuration
	sessionID, err := sm.createOrLoadSession(ctx, client, agentConfig, existingSessionID, workspacePath, mcpServers)
	if err != nil {
		return nil, err
	}

	result.SessionID = sessionID
	return result, nil
}

// createOrLoadSession creates a new session or loads an existing one based on agent config.
func (sm *SessionManager) createOrLoadSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig agents.Agent,
	existingSessionID string,
	workspacePath string,
	mcpServers []agentctltypes.McpServer,
) (string, error) {
	rt := agentConfig.Runtime()
	sm.logger.Debug("createOrLoadSession decision",
		zap.String("agent_type", agentConfig.ID()),
		zap.Bool("native_session_resume", rt.SessionConfig.NativeSessionResume),
		zap.String("existing_session_id", existingSessionID),
		zap.Bool("will_attempt_load", rt.SessionConfig.NativeSessionResume && existingSessionID != ""))
	if rt.SessionConfig.NativeSessionResume && existingSessionID != "" {
		sessionID, err := sm.loadSession(ctx, client, agentConfig, existingSessionID, mcpServers)
		if err == nil {
			return sessionID, nil
		}
		// If the underlying ACP connection is dead (peer disconnected, context
		// cancelled), session/new on the same client will return the same
		// transport error — falling back just emits a noisy duplicate failure
		// and delays the FAILED transition. Short-circuit so the caller can
		// rebuild the connection (next resume cycle gets a fresh agentctl
		// instance and a fresh ACP connection).
		if isTransportDeadErr(err) {
			sm.logger.Warn("session/load failed at transport layer, not retrying with session/new",
				zap.String("agent_type", agentConfig.ID()),
				zap.String("existing_session_id", existingSessionID),
				zap.String("reason", err.Error()))
			return "", err
		}
		// session/load can fail for reasons that don't justify aborting the
		// session: agent doesn't support the method (capability mismatch /
		// method not found), the upstream agent CLI no longer recognises the
		// stored token (expired / version drift / agent-side GC), etc. In all
		// those cases we want to start a fresh ACP session — the kandev-side
		// row identity is still preserved, only the agent CLI's conversation
		// memory is reset. The caller (executor) overwrites the stored token
		// when the new session ID flows back through the events pipeline.
		sm.logger.Warn("session/load failed, falling back to session/new",
			zap.String("agent_type", agentConfig.ID()),
			zap.String("existing_session_id", existingSessionID),
			zap.String("reason", err.Error()),
			zap.Bool("method_not_found", isMethodNotFoundErr(err)),
			zap.Bool("capability_mismatch", strings.Contains(err.Error(), "LoadSession capability is false")),
			zap.Bool("session_unknown", isSessionUnknownErr(err)))
		return sm.createNewSession(ctx, client, agentConfig, workspacePath, mcpServers)
	}
	return sm.createNewSession(ctx, client, agentConfig, workspacePath, mcpServers)
}

// shouldInjectResumeContext determines if we should inject resume context for this session.
// Returns true if:
// 1. The agent explicitly opts in via SessionConfig.HistoryContextInjection
// 2. There's existing history for this session
func (sm *SessionManager) shouldInjectResumeContext(agentConfig agents.Agent, taskSessionID string) bool {
	if sm.historyManager == nil {
		return false
	}

	rt := agentConfig.Runtime()

	// Only inject if the agent explicitly opts in to history-based context injection
	if !rt.SessionConfig.HistoryContextInjection {
		return false
	}

	// Check if we have history for this session
	return sm.historyManager.HasHistory(taskSessionID)
}

// getResumeContextPrompt generates a prompt with resume context if available.
// If there's no history or context injection is disabled, returns the original prompt.
func (sm *SessionManager) getResumeContextPrompt(agentConfig agents.Agent, taskSessionID, originalPrompt string) string {
	if !sm.shouldInjectResumeContext(agentConfig, taskSessionID) {
		return originalPrompt
	}

	resumePrompt, err := sm.historyManager.GenerateResumeContext(taskSessionID, originalPrompt)
	if err != nil {
		sm.logger.Warn("failed to generate resume context, using original prompt",
			zap.String("session_id", taskSessionID),
			zap.Error(err))
		return originalPrompt
	}

	return resumePrompt
}

// loadSession loads an existing session via ACP session/load
func (sm *SessionManager) loadSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig agents.Agent,
	sessionID string,
	mcpServers []agentctltypes.McpServer,
) (string, error) {
	sm.logger.Info("sending ACP session/load request",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("session_id", sessionID))

	if err := client.LoadSession(ctx, sessionID, mcpServers); err != nil {
		// context.Canceled is caller teardown (WS disconnect, session already
		// gone) rather than an agent or transport fault, so it does not warrant
		// an ERROR + stacktrace. DeadlineExceeded is a real startup/handshake
		// timeout and stays ERROR — matching the executor startup-deadline
		// classification. The caller still classifies canceled loads as
		// transport-dead and skips the session/new fallback.
		if errors.Is(err, context.Canceled) {
			sm.logger.Warn("ACP session/load aborted by context",
				zap.String("agent_type", agentConfig.ID()),
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			sm.logger.Error("ACP session/load failed",
				zap.String("agent_type", agentConfig.ID()),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return "", fmt.Errorf("session/load failed: %w", err)
	}

	sm.logger.Info("ACP session loaded successfully",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("session_id", sessionID))

	return sessionID, nil
}

// createNewSession creates a new session via ACP session/new
func (sm *SessionManager) createNewSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig agents.Agent,
	workspacePath string,
	mcpServers []agentctltypes.McpServer,
) (string, error) {
	sm.logger.Info("sending ACP session/new request",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("workspace_path", workspacePath))

	sessionID, err := client.NewSession(ctx, workspacePath, mcpServers)
	if err != nil {
		sm.logger.Error("ACP session/new failed",
			zap.String("agent_type", agentConfig.ID()),
			zap.String("workspace_path", workspacePath),
			zap.Error(err))
		return "", fmt.Errorf("session/new failed: %w", err)
	}

	sm.logger.Info("ACP session created successfully",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("session_id", sessionID))

	return sessionID, nil
}

// InitializeAndPrompt performs full ACP session initialization and sends the initial prompt.
// This offices:
// 1. Session initialization (initialize + session/new or session/load)
// 2. Publishing ACP session created event
// 3. Connecting WebSocket streams
// 4. Sending the initial task prompt (if provided)
// 5. Marking the execution as ready
//
// Returns the session ID on success.
func (sm *SessionManager) InitializeAndPrompt(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
	markReady func(executionID string) error,
	startModelPolicy StartModelPolicy,
	profileMode string,
	profileConfigOptions map[string]string,
) error {
	// Preserve the legacy call contract for direct callers: its supplied
	// configuration rides the profile layer (no separate runtime override).
	// The lifecycle manager uses the layered entry point so profile settings
	// can be snapshotted first.
	return sm.InitializeAndPromptWithLayers(
		ctx, execution, agentConfig, taskDescription, attachments, mcpServers,
		markReady,
		startModelPolicy.Model, profileMode, profileConfigOptions,
		"", "", nil,
		startModelPolicy,
	)
}

// InitializeAndPromptWithLayers performs ACP initialization with a distinct
// profile layer and final runtime layer. The original effective snapshot is
// published after the profile layer and before the runtime layer.
func (sm *SessionManager) InitializeAndPromptWithLayers(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
	markReady func(executionID string) error,
	profileModel string,
	profileMode string,
	profileConfigOptions map[string]string,
	runtimeModel string,
	runtimeMode string,
	runtimeConfigOptions map[string]string,
	startModelPolicy StartModelPolicy,
) error {
	ctx, result, err := sm.initializeACPConnection(ctx, execution, agentConfig, mcpServers)
	if err != nil {
		return err
	}

	execution.ACPSessionID = result.SessionID
	if !cacheFreshSessionModelState(execution) && (profileModel != "" || runtimeModel != "") {
		waitForFreshSessionModelState(ctx, sm.logger, execution)
	}
	providerDefaultConfig := execution.GetModelState()

	// Decide the effective model up front under the executor-authoritative
	// policy, then hand the decided model to the layers so it is applied once.
	// The gate is the effective model (which includes the persisted runtime
	// override), not only the profile's start model.
	effectiveModel := sm.applyStartModelPolicyToEffectiveModel(
		ctx, execution, result.SessionID, profileModel, runtimeModel, startModelPolicy,
	)
	if effectiveModel.err != nil {
		return effectiveModel.err
	}
	if effectiveModel.decision.Warning {
		sm.publishModelSelectionWarningEvent(execution, result.SessionID, effectiveModel.decision)
	}
	// The layers apply the policy-decided model; the runtime override is
	// neutralized because the decision already accounted for it. When the
	// policy ran it already applied the model via SetModel — the layers must
	// not apply it again (a second SetModel failure would be only logged,
	// and the original-config snapshot could disagree with reality).
	profileModel = effectiveModel.model
	runtimeModel = ""
	// Only mark the launch initialized once the start-model policy has
	// succeeded — a strict unavailable model fails here, and a failed launch
	// must not look initialized.
	execution.setSessionInitialized(true)

	finalConfigID, profileModelApplied, profileModeApplied, profileConfigOptionsApplied := sm.applyProfileSessionLayers(
		ctx, execution, result.SessionID, profileModel, profileMode, profileConfigOptions,
		effectiveModel.handled, effectiveModel.appliedModel,
	)

	// Capture the effective profile state before runtime overrides or workflow
	// launch settings are applied. This event is intentionally separate from
	// ConfigBaselineCandidate, which remains provider-default comparison data.
	sm.publishOriginalConfigOptionsWithProfile(execution, result.SessionID, profileModelApplied, profileConfigOptionsApplied)

	finalConfigID, runtimeFailures := sm.applyRuntimeSessionLayers(
		ctx, execution, result.SessionID, finalConfigID,
		profileModelApplied, profileModeApplied, runtimeModel, runtimeMode, runtimeConfigOptions,
	)
	sm.publishSettledConfigOptions(execution, result.SessionID, finalConfigID, providerDefaultConfig)
	sm.publishWorkflowSessionConfigFailures(execution, result.SessionID, runtimeFailures)

	// Publish session created event
	if sm.eventPublisher != nil {
		sm.eventPublisher.PublishACPSessionCreated(execution, result.SessionID)
	}

	// Send the task prompt if provided, or mark the execution as ready.
	sm.dispatchInitialPrompt(ctx, execution, agentConfig, taskDescription, attachments, markReady)

	return nil
}

// applyStartModelPolicyToEffectiveModel resolves the effective model (profile
// start model, overridden by the persisted runtime model) and applies the
// executor-authoritative policy. The caller neutralizes the runtime override
// afterwards because the decision already accounted for it.
func (sm *SessionManager) applyStartModelPolicyToEffectiveModel(
	ctx context.Context,
	execution *AgentExecution,
	acpSessionID string,
	profileModel, runtimeModel string,
	startModelPolicy StartModelPolicy,
) (effective struct {
	model        string
	appliedModel string
	handled      bool
	decision     ModelSelectionDecision
	err          error
}) {
	effective.model = profileModel
	if runtimeModel != "" {
		effective.model = runtimeModel
	}
	if effective.model == "" || execution.agentctl == nil {
		return effective
	}
	// The policy owns this model decision even when it deliberately continues
	// without an applied model (auto-fallback or method-not-found). Later
	// profile layers must not retry an outcome that was already handled.
	effective.handled = true
	startModelPolicy.Model = effective.model
	decision, policyErr := applyStartModelPolicy(
		ctx, sm.logger, execution.agentctl,
		execution.GetModelState(), startModelPolicy,
	)
	if policyErr != nil {
		sm.logger.Error("start model unavailable, failing session start",
			zap.String("execution_id", execution.ID),
			zap.Error(policyErr))
		effective.err = policyErr
		return effective
	}
	effective.decision = decision
	if decision.Outcome == ModelSelectionOutcomeApplied ||
		decision.Outcome == ModelSelectionOutcomeExplicitFallback {
		effective.appliedModel = decision.EffectiveModel
	}
	if decision.Outcome == ModelSelectionOutcomeExplicitFallback {
		effective.model = decision.EffectiveModel
	}
	return effective
}

func (sm *SessionManager) initializeACPConnection(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	mcpServers []agentctltypes.McpServer,
) (context.Context, *InitializeResult, error) {
	_, sessionSpan := tracing.TraceSessionStart(
		context.Background(), execution.TaskID, execution.SessionID, execution.ID,
	)
	execution.SetSessionSpan(sessionSpan)
	ctx = trace.ContextWithSpan(ctx, sessionSpan)
	if execution.agentctl != nil {
		execution.agentctl.SetTraceContext(execution.SessionTraceContext())
	}
	ctx, initSpan := tracing.TraceSessionInit(ctx, execution.TaskID, execution.SessionID, execution.ID)
	defer initSpan.End()

	rt := agentConfig.Runtime()
	sm.logger.Info("initializing ACP session",
		zap.String("execution_id", execution.ID), zap.String("agentctl_url", execution.agentctl.BaseURL()),
		zap.String("agent_type", agentConfig.ID()), zap.String("existing_acp_session_id", execution.ACPSessionID),
		zap.Bool("native_session_resume", rt.SessionConfig.NativeSessionResume))
	if sm.streamManager != nil {
		updatesReady := make(chan struct{})
		sm.streamManager.ConnectAll(execution, updatesReady)
		select {
		case <-updatesReady:
			sm.logger.Debug("updates stream ready")
		case <-time.After(10 * time.Second):
			return ctx, nil, fmt.Errorf("timeout waiting for agent stream to connect")
		}
	}
	result, err := sm.InitializeSession(ctx, execution.agentctl, agentConfig, execution.ACPSessionID, execution.WorkspacePath, mcpServers)
	if err != nil {
		// loadSession already logged the root cause. context.Canceled is
		// caller teardown, so keep this outer boundary at WARN too —
		// otherwise the intentional load-path downgrade is undone by a
		// second ERROR stacktrace here. DeadlineExceeded stays ERROR.
		if errors.Is(err, context.Canceled) {
			sm.logger.Warn("session initialization aborted by context",
				zap.String("execution_id", execution.ID), zap.Error(err))
		} else {
			sm.logger.Error("session initialization failed",
				zap.String("execution_id", execution.ID), zap.Error(err))
		}
		return ctx, nil, err
	}
	sm.logger.Info("ACP session initialized",
		zap.String("execution_id", execution.ID), zap.String("agent_name", result.AgentName),
		zap.String("agent_version", result.AgentVersion), zap.String("session_id", result.SessionID))
	return ctx, result, nil
}

func (sm *SessionManager) applyProfileSessionLayers(
	ctx context.Context,
	execution *AgentExecution,
	acpSessionID string,
	profileModel string,
	profileMode string,
	profileConfigOptions map[string]string,
	modelPolicyHandled bool,
	policyAppliedModel string,
) (string, string, string, map[string]string) {
	if execution.agentctl == nil {
		return "", "", "", nil
	}
	finalConfigID := ""
	profileModelApplied := ""
	profileModeApplied := ""
	profileConfigOptionsApplied := make(map[string]string)
	if profileModel != "" {
		if modelPolicyHandled {
			if policyAppliedModel == "" {
				sm.logger.Debug("start-model policy handled model without applying one",
					zap.String("execution_id", execution.ID), zap.String("model", profileModel))
			} else {
				// The start-model policy applied this model via SetModel before
				// the layers ran — record it as applied without repeating the
				// call, so a duplicate SetModel cannot fail silently behind the
				// policy's back.
				finalConfigID = modelConfigIDFromState(execution.GetModelState())
				profileModelApplied = policyAppliedModel
				sm.logger.Info("profile model applied by start-model policy",
					zap.String("execution_id", execution.ID), zap.String("model", policyAppliedModel))
			}
		} else if err := execution.agentctl.SetModel(ctx, profileModel); err != nil {
			sm.logger.Warn("failed to set profile model via ACP",
				zap.String("execution_id", execution.ID), zap.String("model", profileModel), zap.Error(err))
		} else {
			finalConfigID = modelConfigIDFromState(execution.GetModelState())
			profileModelApplied = profileModel
			sm.logger.Info("set profile model on ACP session",
				zap.String("execution_id", execution.ID), zap.String("model", profileModel))
		}
	}
	if profileMode != "" {
		if err := execution.agentctl.SetMode(ctx, acpSessionID, profileMode); err != nil {
			sm.logger.Warn("failed to set profile mode via ACP",
				zap.String("execution_id", execution.ID), zap.String("mode", profileMode), zap.Error(err))
		} else {
			profileModeApplied = profileMode
			sm.logger.Info("set profile mode on ACP session",
				zap.String("execution_id", execution.ID), zap.String("mode", profileMode))
		}
	}
	sanitizedOptions := profileconfig.SanitizeConfigOptions(profileConfigOptions)
	for _, configID := range sortedConfigOptionKeys(sanitizedOptions) {
		value := sanitizedOptions[configID]
		if err := execution.agentctl.SetConfigOption(ctx, configID, value); err != nil {
			sm.logger.Warn("failed to set profile config option via ACP",
				zap.String("execution_id", execution.ID), zap.String("config_id", configID),
				zap.String("value", value), zap.Error(err))
			continue
		}
		finalConfigID = configID
		profileConfigOptionsApplied[configID] = value
		sm.logger.Info("set profile config option on ACP session",
			zap.String("execution_id", execution.ID), zap.String("config_id", configID), zap.String("value", value))
	}
	return finalConfigID, profileModelApplied, profileModeApplied, profileConfigOptionsApplied
}

func (sm *SessionManager) applyRuntimeSessionLayers(
	ctx context.Context,
	execution *AgentExecution,
	acpSessionID string,
	finalConfigID string,
	profileModel string,
	profileMode string,
	runtimeModel string,
	runtimeMode string,
	runtimeConfigOptions map[string]string,
) (string, []string) {
	failed := make([]string, 0)
	if execution.agentctl == nil {
		return finalConfigID, failed
	}
	if runtimeModel != "" && runtimeModel != profileModel {
		if err := execution.agentctl.SetModel(ctx, runtimeModel); err != nil {
			failed = append(failed, "model")
			sm.logger.Warn("failed to set runtime model via ACP",
				zap.String("execution_id", execution.ID), zap.String("model", runtimeModel), zap.Error(err))
		} else {
			finalConfigID = modelConfigIDFromState(execution.GetModelState())
			// Logged like the profile layer above: without this line a trace
			// shows only the profile write, and a runtime layer that never ran
			// is indistinguishable from one that ran and was overwritten.
			sm.logger.Info("set runtime model on ACP session",
				zap.String("execution_id", execution.ID), zap.String("model", runtimeModel))
		}
	}
	if runtimeMode != "" && runtimeMode != profileMode {
		if err := execution.agentctl.SetMode(ctx, acpSessionID, runtimeMode); err != nil {
			failed = append(failed, "mode")
			sm.logger.Warn("failed to set runtime mode via ACP",
				zap.String("execution_id", execution.ID), zap.String("mode", runtimeMode), zap.Error(err))
		}
	}
	// Fail safe: when the current agent's option catalog is not yet known, we
	// cannot verify which persisted options it supports, so replay nothing
	// rather than sending a prior agent's keys (spec failure mode: unknown
	// catalog must not send unverified options). This covers flat-model-list
	// agents whose startup wait exits on the model list before any config
	// options settle.
	modelState := execution.GetModelState()
	var sanitizedOptions map[string]string
	if _, catalogKnown := capturedRuntimeConfigOptionCatalog(modelState); catalogKnown {
		sanitizedOptions = sanitizeRuntimeConfigOptionsWithCatalog(runtimeConfigOptions, modelState)
	}
	for _, configID := range sortedConfigOptionKeys(sanitizedOptions) {
		value := sanitizedOptions[configID]
		if err := execution.agentctl.SetConfigOption(ctx, configID, value); err != nil {
			failed = append(failed, configID)
			sm.logger.Warn("failed to set runtime config option via ACP",
				zap.String("execution_id", execution.ID), zap.String("config_id", configID),
				zap.String("value", value), zap.Error(err))
			continue
		}
		finalConfigID = configID
	}
	return finalConfigID, failed
}

func sortedConfigOptionKeys(options map[string]string) []string {
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (sm *SessionManager) publishSettledConfigOptions(
	execution *AgentExecution,
	acpSessionID string,
	finalConfigID string,
	providerDefaultConfig *CachedModelState,
) {
	if sm.eventPublisher == nil {
		return
	}
	baselineCandidate, live, ready := execution.SettleConfigOptions(finalConfigID, providerDefaultConfig)
	if !ready || baselineCandidate == nil || live == nil {
		return
	}
	sm.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
		Type:                    streams.EventTypeSessionModels,
		SessionID:               acpSessionID,
		CurrentModelID:          live.CurrentModelID,
		SessionModels:           live.Models,
		ConfigOptions:           live.ConfigOptions,
		ConfigBaselineCandidate: baselineCandidate.ConfigOptions,
		Data:                    map[string]any{"config_options_settled": true},
	})
}

// publishModelFallbackEvent broadcasts an explicit "using fallback model"
// signal so the UI can surface why the session is not on the configured
// start model. The event carries the fallback model id in Data.
func (sm *SessionManager) publishModelFallbackEvent(
	execution *AgentExecution,
	acpSessionID string,
	fallbackModel string,
) {
	if sm.eventPublisher == nil || fallbackModel == "" {
		return
	}
	sm.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
		Type:          streams.EventTypeSessionModelFallback,
		SessionID:     acpSessionID,
		FallbackModel: fallbackModel,
	})
}

// publishModelSelectionWarningEvent emits the provider-neutral decision and
// keeps the legacy fallback event as a compatibility projection for clients
// that still listen for session_model_fallback.
func (sm *SessionManager) publishModelSelectionWarningEvent(
	execution *AgentExecution,
	acpSessionID string,
	decision ModelSelectionDecision,
) {
	if sm.eventPublisher == nil || execution == nil || !decision.Warning {
		return
	}
	decisionIDBytes := sha256.Sum256([]byte(strings.Join([]string{
		execution.ID,
		acpSessionID,
		decision.RequestedModel,
		decision.Reason,
		decision.EffectiveModel,
	}, "|")))
	warning := &streams.ModelSelectionWarning{
		Kind:              "model_selection_warning",
		DecisionID:        hex.EncodeToString(decisionIDBytes[:]),
		Reason:            decision.Reason,
		RequestedModel:    decision.RequestedModel,
		EffectiveModel:    decision.EffectiveModel,
		AgentID:           execution.AgentID,
		ExecutorType:      string(execution.RuntimeName),
		ExecutorProfileID: execution.metadataString(MetadataKeyExecutorProfileID),
	}
	if decision.Outcome == ModelSelectionOutcomeExplicitFallback {
		warning.FallbackModel = decision.EffectiveModel
	}
	sm.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
		Type:                  streams.EventTypeSessionModelSelectionWarning,
		SessionID:             acpSessionID,
		CurrentModelID:        warning.EffectiveModel,
		ModelSelectionWarning: warning,
	})
	if warning.FallbackModel != "" {
		sm.publishModelFallbackEvent(execution, acpSessionID, warning.FallbackModel)
	}
}

func (sm *SessionManager) publishWorkflowSessionConfigFailures(
	execution *AgentExecution,
	acpSessionID string,
	failures []string,
) {
	if sm.eventPublisher == nil || len(failures) == 0 {
		return
	}
	sm.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
		Type:      streams.EventTypeSessionModels,
		SessionID: acpSessionID,
		Data: map[string]any{
			"workflow_session_config_failures": append([]string(nil), failures...),
		},
	})
}

// publishOriginalConfigOptions publishes the profile-settled model and option
// values before any runtime override layer is applied. The orchestrator uses
// the marker to write the task session's immutable original snapshot once.
func (sm *SessionManager) publishOriginalConfigOptions(execution *AgentExecution, acpSessionID string) {
	sm.publishOriginalConfigOptionsWithProfile(execution, acpSessionID, "", nil)
}

func (sm *SessionManager) publishOriginalConfigOptionsWithProfile(
	execution *AgentExecution,
	acpSessionID string,
	profileModel string,
	profileConfigOptions map[string]string,
) {
	if sm.eventPublisher == nil || execution == nil {
		return
	}
	state := execution.GetModelState()
	if state == nil {
		return
	}
	options := make([]streams.ConfigOption, len(state.ConfigOptions))
	copy(options, state.ConfigOptions)
	currentModel := state.CurrentModelID
	if profileModel != "" {
		currentModel = profileModel
	}
	if currentModel != "" {
		if !setConfigOptionValue(options, modelConfigOptionID, currentModel) {
			options = append(options, streams.ConfigOption{ID: modelConfigOptionID, Category: modelConfigOptionID, CurrentValue: currentModel})
		}
	}
	for configID, value := range profileconfig.SanitizeConfigOptions(profileConfigOptions) {
		if !setConfigOptionValue(options, configID, value) {
			options = append(options, streams.ConfigOption{ID: configID, CurrentValue: value})
		}
	}
	if currentModel == "" && len(options) == 0 {
		return
	}
	sm.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
		Type:                    streams.EventTypeSessionModels,
		SessionID:               acpSessionID,
		CurrentModelID:          currentModel,
		SessionModels:           state.Models,
		ConfigOptions:           options,
		OriginalConfigCandidate: options,
		Data:                    map[string]any{"original_config_settled": true},
	})
}

func setConfigOptionValue(options []streams.ConfigOption, configID, value string) bool {
	for index := range options {
		if options[index].ID == configID || (configID == modelConfigOptionID && options[index].Category == modelConfigOptionID) {
			options[index].CurrentValue = value
			return true
		}
	}
	return false
}

func modelConfigIDFromState(state *CachedModelState) string {
	if state == nil {
		return modelConfigOptionID
	}
	for _, option := range state.ConfigOptions {
		if option.ID == modelConfigOptionID || option.Category == modelConfigOptionID {
			return option.ID
		}
	}
	return modelConfigOptionID
}

// convertAttachments converts lifecycle.MessageAttachment to v1.MessageAttachment for ACP.
func convertAttachments(attachments []MessageAttachment) []v1.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]v1.MessageAttachment, 0, len(attachments))
	for _, att := range attachments {
		result = append(result, v1.MessageAttachment{
			AttachmentID: att.AttachmentID,
			Type:         att.Type,
			Data:         att.Data,
			MimeType:     att.MimeType,
			Name:         att.Name,
			SizeBytes:    att.SizeBytes,
			DeliveryMode: att.DeliveryMode,
		})
	}
	return result
}

// dispatchInitialPrompt sends the initial task prompt or marks the execution as ready.
// For sessions with a task description, sends the prompt asynchronously.
// For resumed sessions (fork_session pattern), defers context injection to the first user message.
// For all other cases, marks the execution ready immediately.
func (sm *SessionManager) dispatchInitialPrompt(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent, taskDescription string, attachments []MessageAttachment, markReady func(executionID string) error) {
	switch {
	case taskDescription != "" || len(attachments) > 0:
		// The orchestrator wrapped the prompt with the Kandev system block
		// before handing it to the runtime; do not re-wrap here.
		effectivePrompt := sm.getResumeContextPrompt(agentConfig, execution.SessionID, taskDescription)
		if effectivePrompt != taskDescription {
			sm.logger.Info("injecting resume context into initial prompt",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Int("original_length", len(taskDescription)),
				zap.Int("effective_length", len(effectivePrompt)))
		}
		acpAttachments := convertAttachments(attachments)
		go func() {
			promptCtx, cancel := appctx.Detached(ctx, sm.stopCh, 0)
			defer cancel()
			_, err := sm.sendPrompt(
				promptCtx,
				execution,
				effectivePrompt,
				false,
				acpAttachments,
				false,
				sendPromptCallbacks{onFailure: sm.initialPromptFailure},
				false,
			)
			if err != nil {
				// During graceful shutdown the agent subprocess is terminated,
				// so an in-flight initial prompt fails with a transport death
				// or context cancellation. That is an expected teardown race,
				// not a fault: log WARN without a stack trace so it does not
				// masquerade as a crash.
				if sm.stopChClosed() || isTransportDeadErr(err) {
					sm.logger.Warn("initial prompt aborted during shutdown",
						zap.String("execution_id", execution.ID),
						zap.String("error", err.Error()))
				} else {
					sm.logger.Error("initial prompt failed",
						zap.String("execution_id", execution.ID),
						zap.Error(err))
				}
			}
		}()
	case sm.shouldInjectResumeContext(agentConfig, execution.SessionID):
		execution.needsResumeContext = true
		sm.logger.Info("session has history for context injection, will inject on first user prompt",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID))
		if err := markReady(execution.ID); err != nil {
			sm.logger.Error("failed to mark execution as ready",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}
	default:
		sm.logger.Debug("no task description and no resume context needed, marking as ready",
			zap.String("execution_id", execution.ID))
		if err := markReady(execution.ID); err != nil {
			sm.logger.Error("failed to mark execution as ready",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}
	}
}

// buildEffectivePrompt applies resume context injection if needed, returning the effective prompt to send.
// The Kandev MCP system block is intentionally NOT re-injected here — it is
// only attached to the very first prompt of a task at the orchestrator layer.
// On resume, the agent CLI's restored conversation already contains it.
func (sm *SessionManager) buildEffectivePrompt(execution *AgentExecution, prompt string) string {
	if !execution.needsResumeContext || execution.resumeContextInjected {
		return prompt
	}
	effectivePrompt := prompt
	if sm.historyManager != nil {
		resumePrompt, err := sm.historyManager.GenerateResumeContext(execution.SessionID, prompt)
		switch {
		case err != nil:
			sm.logger.Warn("failed to generate resume context for follow-up prompt",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		case resumePrompt != prompt:
			effectivePrompt = resumePrompt
			sm.logger.Info("injecting resume context into follow-up prompt",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Int("original_length", len(prompt)),
				zap.Int("effective_length", len(effectivePrompt)))
			sm.logger.Info("resume context prompt content",
				zap.String("execution_id", execution.ID),
				zap.String("resume_prompt", effectivePrompt))
		}
	}
	execution.resumeContextInjected = true
	return effectivePrompt
}

// waitForPromptDone waits for the prompt to complete, checking for stalls periodically.
func (sm *SessionManager) waitForPromptDone(
	ctx context.Context,
	execution *AgentExecution,
	promptGeneration uint64,
) (*PromptResult, error) {
	stallTicker := time.NewTicker(time.Minute)
	defer stallTicker.Stop()
	stallReported := false
	startupGeneration := execution.startupAttemptSnapshot()

	for {
		select {
		case signal := <-execution.promptDoneCh:
			if signal.StartupGeneration != startupGeneration &&
				(signal.StartupGeneration != 0 || startupGeneration != 0) {
				sm.logger.Debug("ignoring completion signal for superseded startup generation",
					zap.String("execution_id", execution.ID),
					zap.Uint64("signal_startup_generation", signal.StartupGeneration),
					zap.Uint64("active_startup_generation", startupGeneration))
				continue
			}
			if signal.PromptGeneration != 0 &&
				promptGeneration != 0 &&
				signal.PromptGeneration != promptGeneration {
				sm.logger.Debug("ignoring completion signal for superseded prompt generation",
					zap.String("execution_id", execution.ID),
					zap.Uint64("signal_prompt_generation", signal.PromptGeneration),
					zap.Uint64("active_prompt_generation", promptGeneration))
				continue
			}
			if signal.IsError {
				// A transport death or cancel-release during shutdown is a
				// benign teardown race, not an agent fault. Log WARN (no stack
				// trace) for those; keep ERROR for genuine agent failures on an
				// active session.
				if isBenignPromptTeardown(signal.Error) || sm.stopChClosed() {
					sm.logger.Warn("prompt aborted during shutdown",
						zap.String("execution_id", execution.ID),
						zap.String("error", signal.Error))
				} else {
					sm.logger.Error("prompt completed with error",
						zap.String("execution_id", execution.ID),
						zap.String("error", signal.Error))
				}
				// Wrap cancel-release sentinels so PromptTask can identify them and
				// skip the REVIEW task-state transition — the user is cancelling, not
				// hitting a real agent failure.
				if isCancelReleaseError(signal.Error) {
					return nil, fmt.Errorf("%w: %s: %w", ErrAgentReported, signal.Error, ErrCancelEscalated)
				}
				return nil, fmt.Errorf("%w: %s", ErrAgentReported, signal.Error)
			}

			// Peek at buffer for return value
			execution.messageMu.Lock()
			agentMessage := execution.messageBuffer.String()
			execution.messageMu.Unlock()

			sm.logger.Info("prompt completed",
				zap.String("execution_id", execution.ID),
				zap.String("stop_reason", signal.StopReason),
				zap.Int("message_length", len(agentMessage)))

			// Note: markReady is NOT called here — handleAgentEvent(complete) already handles it.
			return &PromptResult{
				StopReason:   signal.StopReason,
				AgentMessage: agentMessage,
			}, nil

		case <-ctx.Done():
			return nil, ctx.Err()

		case <-stallTicker.C:
			lastActivity, agentEventSeen, activityEpoch := execution.promptActivitySnapshot()
			elapsed := time.Since(lastActivity)
			neverStarted := !agentEventSeen

			if elapsed >= 5*time.Minute && !stallReported {
				sm.logger.Warn("agent stall detected: no events received",
					zap.String("execution_id", execution.ID),
					zap.Duration("elapsed_since_last_event", elapsed),
					zap.Time("last_activity", lastActivity),
					zap.Bool("never_started", neverStarted))
				if sm.eventPublisher != nil {
					sm.eventPublisher.PublishAgentStalled(
						ctx,
						execution,
						promptGeneration,
						lastActivity,
						elapsed,
						activityEpoch,
						neverStarted,
					)
				}
				stallReported = true
			}
		}
	}
}

func isCancelReleaseError(msg string) bool {
	return strings.HasPrefix(msg, "cancel escalated") ||
		strings.Contains(msg, "prompt abandoned after cancel")
}

// stopChClosed reports whether graceful shutdown has already been signalled.
// A closed stopCh means an in-flight prompt failure is a teardown race rather
// than a fault. A nil stopCh (e.g. tests that don't wire shutdown) is treated
// as not shutting down.
func (sm *SessionManager) stopChClosed() bool {
	if sm.stopCh == nil {
		return false
	}
	select {
	case <-sm.stopCh:
		return true
	default:
		return false
	}
}

// isBenignPromptTeardown reports whether a prompt-completion error string is a
// benign teardown race: an ACP transport death (the agent subprocess was
// terminated mid-prompt during shutdown) or a cancel-release sentinel. The
// prompt-completion signal carries the error as a string, so this matches the
// same phrases isTransportDeadErr matches on an error value.
func isBenignPromptTeardown(msg string) bool {
	if msg == "" {
		return false
	}
	if isCancelReleaseError(msg) {
		return true
	}
	return strings.Contains(msg, "peer disconnected") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "notification queue overflow") ||
		strings.Contains(msg, context.Canceled.Error())
}

// SendPrompt sends a prompt to an agent execution and waits for completion.
// For initial prompts, pass validateStatus=false. For follow-up prompts, pass validateStatus=true.
// When dispatchOnly is true, returns once agentctl.Prompt has accepted the prompt
// instead of blocking on the agent's complete event — used by the MCP message_task
// path so the tool call doesn't hang for the duration of the target's turn.
// Attachments (images) are passed to the agent if provided.
// Returns the prompt result containing the stop reason and agent message.
// Note: MarkReady is handled by handleAgentEvent(complete), not by this method.
func (sm *SessionManager) SendPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
) (*PromptResult, error) {
	return sm.sendPrompt(ctx, execution, prompt, validateStatus, attachments, dispatchOnly, sendPromptCallbacks{}, false)
}

// SendPromptWithDispatchCallback reports the point at which agentctl accepted
// the prompt while preserving SendPrompt's completion semantics.
func (sm *SessionManager) SendPromptWithDispatchCallback(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*PromptResult, error) {
	return sm.sendPrompt(
		ctx,
		execution,
		prompt,
		validateStatus,
		attachments,
		dispatchOnly,
		sendPromptCallbacks{onDispatched: onDispatched},
		false,
	)
}

// activePromptGeneration returns the dispatched, in-flight prompt generation a
// steer must reuse, or 0 when there is none to steer into. The execution store
// is the authoritative, lock-guarded source; tests that build a SessionManager
// without one apply the same predicate directly.
func (sm *SessionManager) activePromptGeneration(execution *AgentExecution) uint64 {
	if sm.executionStore != nil {
		return sm.executionStore.ActivePromptGeneration(execution.ID)
	}
	gen := execution.promptGeneration
	if gen == 0 || execution.dispatchedPromptGeneration != gen || execution.promptCompletionGeneration == gen {
		return 0
	}
	return gen
}

// markPromptDispatched records that an ordinary prompt's generation reached
// agentctl, making it eligible for a steer to reuse. Mirrors the store predicate
// for the no-store test path.
func (sm *SessionManager) markPromptDispatched(execution *AgentExecution, generation uint64) {
	if sm.executionStore != nil {
		sm.executionStore.MarkPromptDispatched(execution.ID, generation)
		return
	}
	if generation != 0 && execution.promptGeneration == generation &&
		execution.promptCompletionGeneration != generation {
		execution.dispatchedPromptGeneration = generation
		execution.armPromptActivity()
	}
}

// SendPromptSteerWithDispatchCallback delivers a steer into a still-generating
// turn. It deliberately bypasses the serialized path in sendPrompt: taking
// execution.promptMu or waiting on the pending-dispatch barrier would hold the
// steer until the very turn it is meant to interrupt has ended, defeating the
// feature. Instead it issues the ACP PromptSteer against the *active* prompt
// generation, so agentctl folds the message into the running turn (or hands the
// turn off) while the predecessor's completion waiter and streaming buffers stay
// authoritative.
//
// Reusing the active generation is load-bearing: the single real completion the
// ACP adapter emits (the predecessor's early end_turn is suppressed there) then
// carries that generation and wakes the predecessor's waitForPromptDone. A fresh
// generation would be discarded as a mismatch and the predecessor would hang.
// This path therefore never begins a new generation, resets buffers, drains
// promptDoneCh, or opens a completion barrier — the ACP adapter's
// beginSteerHandoff owns the gate transfer and generation-keyed suppression.
//
// Delivery is opportunistic: with no live turn to fold into (idle, a turn that
// was admitted but not yet dispatched, one that already completed, or a
// disconnected stream) there is nothing to steer, so it falls back to an
// ordinary prompt delivered in submission order rather than dropping the message.
func (sm *SessionManager) SendPromptSteerWithDispatchCallback(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*PromptResult, error) {
	if execution.agentctl == nil {
		return nil, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	if validateStatus && execution.Status != v1.AgentStatusRunning && execution.Status != v1.AgentStatusReady {
		return nil, fmt.Errorf("execution %q is not ready for prompts (status: %s)", execution.ID, execution.Status)
	}

	steered, err := sm.tryDispatchSteer(ctx, execution, prompt, attachments)
	if err != nil {
		return nil, err
	}
	if !steered {
		return sm.sendPrompt(
			ctx,
			execution,
			prompt,
			validateStatus,
			attachments,
			dispatchOnly,
			sendPromptCallbacks{onDispatched: onDispatched},
			false,
		)
	}
	if onDispatched != nil {
		onDispatched()
	}
	// A steer never owns turn completion — the predecessor's waiter does — so it
	// always returns dispatched-and-continue regardless of the caller's
	// dispatchOnly hint.
	return &PromptResult{StopReason: PromptStopReasonDispatched}, nil
}

// steerDispatchTimeout bounds how long tryDispatchSteer holds promptLifecycleMu
// waiting for agentctl to acknowledge the steer. The healthy path is a single WS
// round-trip (agentctl acks before it runs the prompt), but sendStreamRequest has
// no deadline of its own, so a half-open connection could otherwise pin the lock
// — wedging this execution's completion claims and new prompts — until the stream
// tears down. A timeout lands on the error path, which is send-safe under the
// no-resend fallback. Exposed as a var so tests can shorten it.
var steerDispatchTimeout = 5 * time.Second

// tryDispatchSteer selects the active dispatched generation and issues the steer
// against it while holding promptLifecycleMu, so neither a completion
// (claimPromptCompletion) nor a new turn (beginExecutionPrompt) — both of which
// take the same lock — can invalidate the chosen generation between the read and
// the send. Without this the read-then-send gap is a TOCTOU: the turn could
// complete in the window and the steer would fire on a dead generation whose
// completion the manager then discards as already-claimed.
//
// Holding the lock across the RPC is deadlock-free: the ACP client resolves the
// prompt-ack on its read loop, while the event worker that takes this lock is a
// separate goroutine (see agent.go readUpdatesStream) — a completion event
// arriving first is enqueued, never inline, so it cannot backpressure the ack.
// The RPC is bounded by steerDispatchTimeout so the lock is never held on a
// stalled connection, and the history append is done after the lock is released.
//
// Returns (false, nil) when there is nothing to steer into — no live generation,
// or the stream is not connected — so the caller degrades to an ordinary prompt.
// It deliberately uses the single fast RPC, not dispatchPrompt's reconnect path:
// a disconnected stream means there is no live turn to fold into, and reconnect
// can block for seconds, which must not run under the lifecycle lock.
func (sm *SessionManager) tryDispatchSteer(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
) (bool, error) {
	if sessionSpan := trace.SpanFromContext(execution.SessionTraceContext()); sessionSpan.SpanContext().IsValid() {
		ctx = trace.ContextWithSpan(ctx, sessionSpan)
	}
	dispatched, err := sm.dispatchSteerLocked(ctx, execution, prompt, attachments)
	if err != nil {
		if isAgentStreamNotConnectedErr(err) {
			return false, nil
		}
		return false, err
	}
	if !dispatched {
		return false, nil
	}
	// History I/O is done outside promptLifecycleMu so a slow history store never
	// extends the lifecycle lock's hold.
	sm.recordSteerActivity(execution, prompt)
	return true, nil
}

// dispatchSteerLocked performs the lock-guarded critical section of a steer: read
// the live generation and issue the bounded RPC under promptLifecycleMu. Returns
// (false, nil) when there is no live generation to steer into.
func (sm *SessionManager) dispatchSteerLocked(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
) (bool, error) {
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()

	generation := sm.activePromptGeneration(execution)
	if generation == 0 {
		return false, nil
	}
	if sm.beforeSteerDispatchHook != nil {
		sm.beforeSteerDispatchHook()
	}
	effectivePrompt := sm.buildEffectivePrompt(execution, prompt)
	dispatchCtx, cancel := context.WithTimeout(ctx, steerDispatchTimeout)
	defer cancel()
	if err := sm.callAgentctlPrompt(dispatchCtx, execution, effectivePrompt, attachments, generation, true); err != nil {
		return false, err
	}
	return true, nil
}

// recordSteerActivity mirrors the ordinary prompt path's history append and
// activity-timestamp bump for a steer that actually dispatched. Kept off the
// fallback path so a steer that degrades to an ordinary prompt is not recorded
// twice.
//
// Deliberately does not touch agentEventSincePrompt: a steer is user input
// injected into an already-running turn, not agent output, so it must not
// mask a stalled agent as having produced something.
func (sm *SessionManager) recordSteerActivity(execution *AgentExecution, prompt string) {
	if sm.historyManager != nil && execution.historyEnabled && execution.SessionID != "" {
		if err := sm.historyManager.AppendUserMessage(execution.SessionID, prompt); err != nil {
			sm.logger.Warn("failed to store steer user message to history", zap.Error(err))
		}
	}
	execution.lastActivityAtMu.Lock()
	execution.lastActivityAt = time.Now()
	execution.lastActivityAtMu.Unlock()
}

func (sm *SessionManager) sendPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	callbacks sendPromptCallbacks,
	steer bool,
) (*PromptResult, error) {
	if execution.agentctl == nil {
		err := fmt.Errorf("execution %q has no agentctl client", execution.ID)
		sm.reportPromptFailure(execution, 0, err, callbacks.onFailure)
		return nil, err
	}

	// Agentctl acknowledges prompt dispatch before the adapter's session/prompt
	// RPC completes. Serialize here as well as in the ACP adapter so two callers
	// cannot reset the same buffers and race on the shared completion channel.
	execution.promptMu.Lock()
	defer execution.promptMu.Unlock()

	// Dispatch-only callers return before the agent completes, but the next prompt
	// still must not reset shared buffers or consume that earlier completion as its
	// own. Keep the pending completion as an execution-level barrier.
	if err := waitForPendingDispatchedPrompt(ctx, execution); err != nil {
		return nil, err
	}

	// Drain any stale signal left in the channel by a prior dispatch-only prompt
	// whose completion arrived after SendPrompt returned. Without this, the next
	// waitForPromptDone would consume the stale signal and report an immediate
	// (wrong) completion for the new prompt.
	select {
	case <-execution.promptDoneCh:
	default:
	}

	// Signal when this prompt completes so CancelAgent can wait for it.
	// In dispatch-only mode there is no in-flight wait to coordinate with, so the
	// barrier is unused — skip it to avoid closing the channel before the agent's
	// async processing actually finishes.
	if !dispatchOnly {
		defer beginPromptBarrier(execution)()
	}

	preparedCtx, effectivePrompt, promptGeneration, err := sm.preparePrompt(ctx, execution, prompt, validateStatus, attachments)
	if err != nil {
		sm.reportPromptFailure(execution, promptGeneration, err, callbacks.onFailure)
		return nil, err
	}
	materializedAttachments, err := sm.materializeAttachments(preparedCtx, execution, attachments)
	if err != nil {
		sm.reportPromptFailure(execution, promptGeneration, err, callbacks.onFailure)
		return nil, err
	}
	if sm.beforePromptDispatchHook != nil {
		sm.beforePromptDispatchHook()
	}
	if err := sm.triggerPrompt(preparedCtx, execution, effectivePrompt, materializedAttachments, promptGeneration, steer); err != nil {
		sm.reportPromptFailure(execution, promptGeneration, err, callbacks.onFailure)
		return nil, err
	}
	// The generation is now accepted by agentctl and in flight, so a concurrent
	// steer may reuse it. (The steer path never marks — it reuses, not owns.)
	sm.markPromptDispatched(execution, promptGeneration)
	result, err := sm.finishAcceptedPrompt(
		preparedCtx,
		execution,
		dispatchOnly,
		callbacks.onDispatched,
		promptGeneration,
	)
	if err != nil {
		sm.reportPromptFailure(execution, promptGeneration, err, callbacks.onFailure)
	}
	return result, err
}

func (sm *SessionManager) reportPromptFailure(
	execution *AgentExecution,
	promptGeneration uint64,
	err error,
	handler func(InitialPromptFailure),
) {
	if handler == nil {
		return
	}
	handler(InitialPromptFailure{
		ExecutionID:      execution.ID,
		SessionID:        execution.SessionID,
		PromptGeneration: promptGeneration,
		TurnID:           execution.promptTurnIDSnapshot(),
		Err:              err,
	})
}

// materializeAttachments resolves claimed backend descriptors into the active
// agentctl session. The prompt and queue protocols carry only bounded
// descriptors; bytes are streamed directly from backend storage to agentctl.
// Materialized descriptors preserve the requested delivery mode. Path-mode
// files stay on disk; prompt-mode images are converted to native ACP content
// by agentctl from the materialized file. Legacy inline attachments are left
// intact.
func (sm *SessionManager) materializeAttachments(
	ctx context.Context,
	execution *AgentExecution,
	attachments []v1.MessageAttachment,
) ([]v1.MessageAttachment, error) {
	if len(attachments) == 0 {
		return attachments, nil
	}
	result := append([]v1.MessageAttachment(nil), attachments...)
	for i, attachment := range result {
		if attachment.AttachmentID == "" {
			continue
		}
		if sm.attachmentReader == nil {
			return nil, fmt.Errorf("attachment %q cannot be delivered: attachment storage is unavailable", attachment.AttachmentID)
		}
		if execution.ACPSessionID == "" {
			return nil, fmt.Errorf("attachment %q cannot be delivered: ACP session is not ready", attachment.AttachmentID)
		}
		reader, name, mimeType, sizeBytes, err := sm.attachmentReader.OpenClaimed(
			ctx, attachment.AttachmentID, execution.TaskID, execution.SessionID,
		)
		if err != nil {
			return nil, fmt.Errorf("open attachment %q: %w", attachment.AttachmentID, err)
		}
		materialized, materializeErr := execution.agentctl.MaterializeAttachment(
			ctx,
			execution.ACPSessionID,
			attachment.AttachmentID,
			name,
			mimeType,
			sizeBytes,
			reader,
		)
		closeErr := reader.Close()
		if materializeErr != nil {
			return nil, fmt.Errorf("materialize attachment %q: %w", attachment.AttachmentID, materializeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close attachment %q: %w", attachment.AttachmentID, closeErr)
		}
		if materialized == nil || strings.TrimSpace(materialized.Name) == "" {
			return nil, fmt.Errorf("materialize attachment %q: agentctl returned no filename", attachment.AttachmentID)
		}
		result[i].Name = materialized.Name
		result[i].MimeType = mimeType
		result[i].SizeBytes = sizeBytes
		result[i].Data = ""
		if result[i].DeliveryMode == "" {
			result[i].DeliveryMode = attachmentDeliveryPrompt
		}
		if result[i].DeliveryMode != attachmentDeliveryPath {
			sm.logger.Debug("preserving native delivery for staged attachment",
				zap.String("attachment_id", attachment.AttachmentID),
				zap.String("requested_delivery_mode", attachment.DeliveryMode),
				zap.Int64("size_bytes", sizeBytes))
		}
	}
	return result, nil
}

func (sm *SessionManager) preparePrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	attachments []v1.MessageAttachment,
) (context.Context, string, uint64, error) {
	if sessionSpan := trace.SpanFromContext(execution.SessionTraceContext()); sessionSpan.SpanContext().IsValid() {
		ctx = trace.ContextWithSpan(ctx, sessionSpan)
	}
	// For follow-up prompts, validate status before claiming a new generation.
	if validateStatus {
		if execution.Status != v1.AgentStatusRunning && execution.Status != v1.AgentStatusReady {
			return ctx, "", 0, fmt.Errorf("execution %q is not ready for prompts (status: %s)", execution.ID, execution.Status)
		}
	}

	// Every dispatch attempt gets a distinct identity, including initial prompts
	// and replacements accepted while the execution is already running.
	var promptGeneration uint64
	switch {
	case sm.promptStarter != nil:
		var err error
		promptGeneration, err = sm.promptStarter(execution.ID)
		if err != nil {
			return ctx, "", 0, err
		}
	case sm.executionStore != nil:
		var err error
		promptGeneration, err = sm.executionStore.BeginPrompt(execution.ID)
		if err != nil {
			return ctx, "", 0, err
		}
	default:
		// Tests that construct SessionManager without lifecycle dependencies
		// still need a generation, but no concurrent owner can mutate it here.
		promptGeneration = beginExecutionPrompt(execution)
	}

	// A disconnect can release the prior SendPrompt before its disconnect
	// callback has persisted a partial assistant response. Drain that history
	// segment before resetting the streaming state; the callback uses the same
	// locked drain, so the segment is persisted at most once.
	resetStreamingStateWithHistory(execution, sm.historyManager, sm.logger)

	effectivePrompt := sm.buildEffectivePrompt(execution, prompt)
	sm.logger.Info("sending prompt to agent",
		zap.String("execution_id", execution.ID),
		zap.Int("prompt_length", len(effectivePrompt)),
		zap.Int("attachments_count", len(attachments)))
	if sm.historyManager != nil && execution.historyEnabled && execution.SessionID != "" {
		if err := sm.historyManager.AppendUserMessage(execution.SessionID, prompt); err != nil {
			sm.logger.Warn("failed to store user message to history", zap.Error(err))
		}
	}
	return ctx, effectivePrompt, promptGeneration, nil
}

func (sm *SessionManager) triggerPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
	steer bool,
) error {
	err := sm.dispatchPrompt(ctx, execution, prompt, attachments, promptGeneration, steer)
	if err == nil {
		return nil
	}
	if isCancelReleaseError(err.Error()) {
		sm.logger.Info("prompt trigger abandoned after cancel; requeueing",
			zap.String("execution_id", execution.ID), zap.Error(err))
		return fmt.Errorf("failed to trigger prompt: %w: %w", err, ErrCancelEscalated)
	}
	sm.logger.Error("failed to trigger prompt",
		zap.String("execution_id", execution.ID), zap.Error(err))
	return fmt.Errorf("failed to trigger prompt: %w", err)
}

func waitForPendingDispatchedPrompt(ctx context.Context, execution *AgentExecution) error {
	if !execution.dispatchedPromptPending.Load() {
		return nil
	}
	timer := time.NewTimer(pendingDispatchedPromptWaitTimeout)
	defer timer.Stop()
	select {
	case <-execution.promptDoneCh:
		execution.dispatchedPromptPending.Store(false)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		// Prefer a completion that arrived at the timeout boundary. If no
		// signal is available, retain the gate for cancellation escalation.
		select {
		case <-execution.promptDoneCh:
			execution.dispatchedPromptPending.Store(false)
			return nil
		default:
			return &PendingDispatchedPromptTimeoutError{
				ExecutionID: execution.ID,
				Timeout:     pendingDispatchedPromptWaitTimeout,
			}
		}
	}
}

func (sm *SessionManager) finishAcceptedPrompt(
	ctx context.Context,
	execution *AgentExecution,
	dispatchOnly bool,
	onDispatched func(),
	promptGeneration uint64,
) (*PromptResult, error) {
	if dispatchOnly {
		execution.dispatchedPromptPending.Store(true)
	}
	if onDispatched != nil {
		onDispatched()
	}
	if dispatchOnly {
		return &PromptResult{StopReason: PromptStopReasonDispatched}, nil
	}
	// Wait for completion signal from handleAgentEvent(complete) or stream disconnect.
	return sm.waitForPromptDone(ctx, execution, promptGeneration)
}

// callAgentctlPrompt selects the steer or ordinary prompt RPC. Kept in one place
// so the initial dispatch and the post-reconnect retry cannot diverge on which
// one they send.
func (sm *SessionManager) callAgentctlPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
	steer bool,
) error {
	if steer {
		return execution.agentctl.PromptSteer(ctx, prompt, attachments, promptGeneration)
	}
	return execution.agentctl.Prompt(ctx, prompt, attachments, promptGeneration)
}

func (sm *SessionManager) dispatchPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
	steer bool,
) error {
	err := sm.callAgentctlPrompt(ctx, execution, prompt, attachments, promptGeneration, steer)
	if err == nil || !isAgentStreamNotConnectedErr(err) || sm.streamManager == nil {
		return err
	}

	sm.logger.Warn("agent stream not connected, reconnecting and retrying prompt once",
		zap.String("execution_id", execution.ID))
	retryErr := sm.retryPromptAfterReconnect(ctx, execution, prompt, attachments, promptGeneration, steer)
	if retryErr == nil {
		return nil
	}
	sm.logger.Warn("prompt retry after stream reconnect failed",
		zap.String("execution_id", execution.ID),
		zap.Error(retryErr))
	return retryErr
}

// beginPromptBarrier sets up a completion signal on the execution so CancelAgent
// can wait for the in-flight SendPrompt to finish before the caller retries.
// Returns a cleanup function that must be deferred: defer beginPromptBarrier(exec)()
func beginPromptBarrier(execution *AgentExecution) func() {
	ch := make(chan struct{})
	execution.promptFinishedMu.Lock()
	execution.promptFinished = ch
	execution.promptFinishedMu.Unlock()
	return func() { close(ch) }
}

func (sm *SessionManager) retryPromptAfterReconnect(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
	steer bool,
) error {
	reconnectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var lastErr error
	for {
		if !execution.agentctl.HasAgentStream() {
			ready := make(chan struct{})
			sm.streamManager.connectUpdatesStreamAsync(execution, ready)

			select {
			case <-ready:
			case <-reconnectCtx.Done():
				if lastErr != nil {
					return fmt.Errorf("timed out waiting for updates stream reconnect: %w", lastErr)
				}
				return reconnectCtx.Err()
			}
		}

		if execution.agentctl.HasAgentStream() {
			if err := sm.callAgentctlPrompt(
				reconnectCtx, execution, prompt, attachments, promptGeneration, steer,
			); err == nil {
				return nil
			} else if !isAgentStreamNotConnectedErr(err) {
				return err
			} else {
				lastErr = err
			}
		} else {
			lastErr = fmt.Errorf("agent stream not connected")
		}

		select {
		case <-reconnectCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for updates stream reconnect: %w", lastErr)
			}
			return reconnectCtx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// jsonRPCMethodNotFound is the JSON-RPC 2.0 error code for "Method not found".
const jsonRPCMethodNotFound = -32601

// jsonRPCResourceNotFound is the ACP-specific error code that agents return when
// asked to load a session ID they don't know about (e.g. claude-agent-acp after
// its in-memory session map was cleared by a restart).
const jsonRPCResourceNotFound = -32002

// isMethodNotFoundErr checks if an error wraps a JSON-RPC "Method not found" error.
func isMethodNotFoundErr(err error) bool {
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Code == jsonRPCMethodNotFound
	}
	return false
}

// isSessionUnknownErr reports whether an error from session/load means the
// agent doesn't know about the requested session ID — typically because the
// agent process restarted and lost its in-memory session map. The caller
// should fall back to creating a fresh session rather than aborting the launch.
func isSessionUnknownErr(err error) bool {
	if err == nil {
		return false
	}
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) && reqErr.Code == jsonRPCResourceNotFound {
		return true
	}
	// Some agents return the error in the wrapped message string instead of a
	// structured RequestError. Match the canonical phrase as a safety net.
	return strings.Contains(err.Error(), "Resource not found")
}

func isAgentStreamNotConnectedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "agent stream not connected")
}

// isTransportDeadErr reports whether a session/load failure is caused by the
// underlying ACP connection being gone rather than an agent-side error. The
// coder/acp-go-sdk surfaces this as a JSON-RPC internal-error whose data map
// carries the canonical phrase "peer disconnected before response" (also
// emitted while waiting for pre-response notifications). The error reaches us
// as a string through the agentctl WS layer, so we match the phrase.
// "connection closed" is the SDK's own cause string emitted from
// shutdownReceive — pulling double duty as a fallback for paths where the
// peer-disconnected wrapping isn't applied. Canonical context cancellation
// errors short-circuit too: the caller's ctx going down means session/new
// retry will fail for the same reason, so treat it as transport-dead.
func isTransportDeadErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "peer disconnected") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "notification queue overflow")
}
