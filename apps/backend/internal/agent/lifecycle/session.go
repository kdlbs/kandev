package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/sysprompt"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/trace"
)

// SessionManager handles ACP session initialization and management
type SessionManager struct {
	logger         *logger.Logger
	eventPublisher *EventPublisher
	streamManager  *StreamManager
	executionStore *ExecutionStore
	historyManager *SessionHistoryManager
	stopCh         <-chan struct{} // For graceful shutdown coordination
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
	if rt.SessionConfig.NativeSessionResume && existingSessionID != "" {
		sessionID, err := sm.loadSession(ctx, client, agentConfig, existingSessionID)
		if err != nil {
			// If session/load fails because the agent doesn't support it, fall back to session/new.
			// Check for: JSON-RPC -32601 (Method not found) or our own capability check string.
			if isMethodNotFoundErr(err) ||
				strings.Contains(err.Error(), "LoadSession capability is false") {
				sm.logger.Warn("agent does not support session loading, falling back to session/new",
					zap.String("agent_type", agentConfig.ID()),
					zap.String("existing_session_id", existingSessionID),
					zap.String("reason", err.Error()))
				return sm.createNewSession(ctx, client, agentConfig, workspacePath, mcpServers)
			}
			return "", err
		}
		return sessionID, nil
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

// injectKandevContext prepends the Kandev system prompt and session context to the user's prompt.
// This provides consistent guidance to the agent about using Kandev tools and the current session IDs.
func (sm *SessionManager) injectKandevContext(taskID, sessionID, prompt string) string {
	return sysprompt.InjectKandevContext(taskID, sessionID, prompt)
}

// loadSession loads an existing session via ACP session/load
func (sm *SessionManager) loadSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig agents.Agent,
	sessionID string,
) (string, error) {
	sm.logger.Info("sending ACP session/load request",
		zap.String("agent_type", agentConfig.ID()),
		zap.String("session_id", sessionID))

	if err := client.LoadSession(ctx, sessionID); err != nil {
		sm.logger.Error("ACP session/load failed",
			zap.String("agent_type", agentConfig.ID()),
			zap.String("session_id", sessionID),
			zap.Error(err))
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
// This orchestrates:
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
	mcpServers []agentctltypes.McpServer,
	markReady func(executionID string) error,
) error {
	// Create session-level trace span to group all operations under one trace
	_, sessionSpan := tracing.TraceSessionStart(
		context.Background(), execution.TaskID, execution.SessionID, execution.ID,
	)
	execution.SetSessionSpan(sessionSpan)
	ctx = trace.ContextWithSpan(ctx, sessionSpan)
	if execution.agentctl != nil {
		execution.agentctl.SetTraceContext(execution.SessionTraceContext())
	}

	// Create short-lived init span so the init phase is visible in trace backends
	// (the parent session span won't be exported until the session ends)
	ctx, initSpan := tracing.TraceSessionInit(ctx, execution.TaskID, execution.SessionID, execution.ID)
	defer initSpan.End()

	rt := agentConfig.Runtime()
	sm.logger.Info("initializing ACP session",
		zap.String("execution_id", execution.ID),
		zap.String("agentctl_url", execution.agentctl.BaseURL()),
		zap.String("agent_type", agentConfig.ID()),
		zap.String("existing_acp_session_id", execution.ACPSessionID),
		zap.Bool("native_session_resume", rt.SessionConfig.NativeSessionResume))

	// Connect WebSocket streams FIRST — agent operations now go over the stream
	if sm.streamManager != nil {
		updatesReady := make(chan struct{})
		sm.streamManager.ConnectAll(execution, updatesReady)

		// Wait for the updates stream to connect — required for agent operations
		select {
		case <-updatesReady:
			sm.logger.Debug("updates stream ready")
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for agent stream to connect")
		}
	}

	// Use InitializeSession for configuration-driven session initialization
	result, err := sm.InitializeSession(
		ctx,
		execution.agentctl,
		agentConfig,
		execution.ACPSessionID,
		execution.WorkspacePath,
		mcpServers,
	)
	if err != nil {
		sm.logger.Error("session initialization failed",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		return err
	}

	sm.logger.Info("ACP session initialized",
		zap.String("execution_id", execution.ID),
		zap.String("agent_name", result.AgentName),
		zap.String("agent_version", result.AgentVersion),
		zap.String("session_id", result.SessionID))

	execution.ACPSessionID = result.SessionID

	// Publish session created event
	if sm.eventPublisher != nil {
		sm.eventPublisher.PublishACPSessionCreated(execution, result.SessionID)
	}

	// Send the task prompt if provided, or mark the execution as ready.
	sm.dispatchInitialPrompt(ctx, execution, agentConfig, taskDescription, markReady)

	return nil
}

// dispatchInitialPrompt sends the initial task prompt or marks the execution as ready.
// For sessions with a task description, sends the prompt asynchronously.
// For resumed sessions (fork_session pattern), defers context injection to the first user message.
// For all other cases, marks the execution ready immediately.
func (sm *SessionManager) dispatchInitialPrompt(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent, taskDescription string, markReady func(executionID string) error) {
	switch {
	case taskDescription != "":
		promptWithContext := sm.injectKandevContext(execution.TaskID, execution.SessionID, taskDescription)
		effectivePrompt := sm.getResumeContextPrompt(agentConfig, execution.SessionID, promptWithContext)
		if effectivePrompt != taskDescription {
			sm.logger.Info("injecting resume context into initial prompt",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Int("original_length", len(taskDescription)),
				zap.Int("effective_length", len(effectivePrompt)))
		}
		go func() {
			promptCtx, cancel := appctx.Detached(ctx, sm.stopCh, constants.PromptTimeout)
			defer cancel()
			_, err := sm.SendPrompt(promptCtx, execution, effectivePrompt, false, nil)
			if err != nil {
				sm.logger.Error("initial prompt failed",
					zap.String("execution_id", execution.ID),
					zap.Error(err))
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
func (sm *SessionManager) buildEffectivePrompt(execution *AgentExecution, prompt string) string {
	if !execution.needsResumeContext || execution.resumeContextInjected {
		return prompt
	}
	// Inject Kandev context for resumed sessions (first prompt in this execution)
	promptWithContext := sm.injectKandevContext(execution.TaskID, execution.SessionID, prompt)
	effectivePrompt := promptWithContext
	if sm.historyManager != nil {
		resumePrompt, err := sm.historyManager.GenerateResumeContext(execution.SessionID, promptWithContext)
		switch {
		case err != nil:
			sm.logger.Warn("failed to generate resume context for follow-up prompt",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		case resumePrompt != promptWithContext:
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
func (sm *SessionManager) waitForPromptDone(ctx context.Context, execution *AgentExecution) (*PromptResult, error) {
	stallTicker := time.NewTicker(30 * time.Second)
	defer stallTicker.Stop()

	for {
		select {
		case signal := <-execution.promptDoneCh:
			if signal.IsError {
				sm.logger.Error("prompt completed with error",
					zap.String("execution_id", execution.ID),
					zap.String("error", signal.Error))
				return nil, fmt.Errorf("agent error: %s", signal.Error)
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
			execution.lastActivityAtMu.Lock()
			elapsed := time.Since(execution.lastActivityAt)
			lastActivity := execution.lastActivityAt
			execution.lastActivityAtMu.Unlock()

			if elapsed > 5*time.Minute {
				sm.logger.Warn("agent stall detected: no events received",
					zap.String("execution_id", execution.ID),
					zap.Duration("elapsed_since_last_event", elapsed),
					zap.Time("last_activity", lastActivity))
			}
		}
	}
}

// SendPrompt sends a prompt to an agent execution and waits for completion.
// For initial prompts, pass validateStatus=false. For follow-up prompts, pass validateStatus=true.
// Attachments (images) are passed to the agent if provided.
// Returns the prompt result containing the stop reason and agent message.
// Note: MarkReady is handled by handleAgentEvent(complete), not by this method.
func (sm *SessionManager) SendPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	attachments []v1.MessageAttachment,
) (*PromptResult, error) {
	if execution.agentctl == nil {
		return nil, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}

	// Inject session trace context so prompt spans become children of the session span
	if sessionSpan := trace.SpanFromContext(execution.SessionTraceContext()); sessionSpan.SpanContext().IsValid() {
		ctx = trace.ContextWithSpan(ctx, sessionSpan)
	}

	// For follow-up prompts, validate status and update to RUNNING
	if validateStatus {
		if execution.Status != v1.AgentStatusRunning && execution.Status != v1.AgentStatusReady {
			return nil, fmt.Errorf("execution %q is not ready for prompts (status: %s)", execution.ID, execution.Status)
		}
		if sm.executionStore != nil {
			sm.executionStore.UpdateStatus(execution.ID, v1.AgentStatusRunning)
		}
	}

	// Clear buffers and streaming state before starting prompt
	// This ensures each prompt starts fresh and doesn't append to previous message
	execution.messageMu.Lock()
	execution.messageBuffer.Reset()
	execution.thinkingBuffer.Reset()
	execution.currentMessageID = ""  // Clear streaming message ID for new turn
	execution.currentThinkingID = "" // Clear streaming thinking ID for new turn
	execution.messageMu.Unlock()

	// Apply resume context injection if needed (first prompt after resume)
	effectivePrompt := sm.buildEffectivePrompt(execution, prompt)

	sm.logger.Info("sending prompt to agent",
		zap.String("execution_id", execution.ID),
		zap.Int("prompt_length", len(effectivePrompt)),
		zap.Int("attachments_count", len(attachments)))

	// Store user prompt to session history for context injection (store original, not with injected context)
	if sm.historyManager != nil && execution.historyEnabled && execution.SessionID != "" {
		if err := sm.historyManager.AppendUserMessage(execution.SessionID, prompt); err != nil {
			sm.logger.Warn("failed to store user message to history", zap.Error(err))
		}
	}

	// Initialize activity timestamp for stall detection
	execution.lastActivityAtMu.Lock()
	execution.lastActivityAt = time.Now()
	execution.lastActivityAtMu.Unlock()

	// Drain any stale signal from a previous prompt cycle
	select {
	case <-execution.promptDoneCh:
	default:
	}

	// Fire the prompt (returns immediately now — completion comes via WebSocket complete event)
	if err := execution.agentctl.Prompt(ctx, effectivePrompt, attachments); err != nil {
		if isAgentStreamNotConnectedErr(err) && sm.streamManager != nil {
			sm.logger.Warn("agent stream not connected, reconnecting and retrying prompt once",
				zap.String("execution_id", execution.ID))
			retryErr := sm.retryPromptAfterReconnect(ctx, execution, effectivePrompt, attachments)
			if retryErr == nil {
				return sm.waitForPromptDone(ctx, execution)
			}
			sm.logger.Warn("prompt retry after stream reconnect failed",
				zap.String("execution_id", execution.ID),
				zap.Error(retryErr))
		}
		sm.logger.Error("failed to trigger prompt",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to trigger prompt: %w", err)
	}

	// Wait for completion signal from handleAgentEvent(complete) or stream disconnect.
	return sm.waitForPromptDone(ctx, execution)
}

func (sm *SessionManager) retryPromptAfterReconnect(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	attachments []v1.MessageAttachment,
) error {
	ready := make(chan struct{})
	go sm.streamManager.connectUpdatesStream(execution, ready)

	select {
	case <-ready:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for updates stream reconnect")
	}

	return execution.agentctl.Prompt(ctx, prompt, attachments)
}

// jsonRPCMethodNotFound is the JSON-RPC 2.0 error code for "Method not found".
const jsonRPCMethodNotFound = -32601

// isMethodNotFoundErr checks if an error wraps a JSON-RPC "Method not found" error.
func isMethodNotFoundErr(err error) bool {
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Code == jsonRPCMethodNotFound
	}
	return false
}

func isAgentStreamNotConnectedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "agent stream not connected")
}
