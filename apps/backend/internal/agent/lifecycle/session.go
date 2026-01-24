package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/agent"
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
//   - If agentConfig.SessionConfig.ResumeViaACP is true AND existingSessionID is provided: use session/load
//   - If agentConfig.SessionConfig.ResumeViaACP is false (CLI handles resume): always use session/new
//   - Otherwise: use session/new
func (sm *SessionManager) InitializeSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig *registry.AgentTypeConfig,
	existingSessionID string,
	workspacePath string,
	mcpServers []agentctltypes.McpServer,
) (*InitializeResult, error) {
	sm.logger.Info("initializing ACP session",
		zap.String("agent_type", agentConfig.ID),
		zap.String("workspace_path", workspacePath),
		zap.Bool("resume_via_acp", agentConfig.SessionConfig.ResumeViaACP),
		zap.String("existing_session_id", existingSessionID))

	// Step 1: Send initialize request
	sm.logger.Info("sending ACP initialize request",
		zap.String("agent_type", agentConfig.ID))

	agentInfo, err := client.Initialize(ctx, "kandev", "1.0.0")
	if err != nil {
		sm.logger.Error("ACP initialize failed",
			zap.String("agent_type", agentConfig.ID),
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
		zap.String("agent_type", agentConfig.ID),
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
	agentConfig *registry.AgentTypeConfig,
	existingSessionID string,
	workspacePath string,
	mcpServers []agentctltypes.McpServer,
) (string, error) {
	if agentConfig.SessionConfig.ResumeViaACP && existingSessionID != "" {
		sessionID, err := sm.loadSession(ctx, client, agentConfig, existingSessionID)
		if err != nil {
			// If session/load fails with "Method not found" or "LoadSession capability is false",
			// fall back to creating a new session. This handles agents that don't support session/load.
			if strings.Contains(err.Error(), "Method not found") ||
				strings.Contains(err.Error(), "LoadSession capability is false") {
				sm.logger.Warn("agent does not support session loading, falling back to session/new",
					zap.String("agent_type", agentConfig.ID),
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
// 1. The agent uses ACP protocol
// 2. The agent doesn't support session/load (ResumeViaACP is false)
// 3. We have a task session ID (for history lookup)
// 4. There's existing history for this session
func (sm *SessionManager) shouldInjectResumeContext(agentConfig *registry.AgentTypeConfig, taskSessionID string) bool {
	if sm.historyManager == nil {
		return false
	}

	// Only inject for ACP agents that don't support session/load
	if agentConfig.Protocol != agent.ProtocolACP {
		return false
	}

	// If agent supports session/load via ACP, don't inject (it will restore context natively)
	if agentConfig.SessionConfig.ResumeViaACP {
		return false
	}

	// Check if we have history for this session
	return sm.historyManager.HasHistory(taskSessionID)
}

// getResumeContextPrompt generates a prompt with resume context if available.
// If there's no history or context injection is disabled, returns the original prompt.
func (sm *SessionManager) getResumeContextPrompt(agentConfig *registry.AgentTypeConfig, taskSessionID, originalPrompt string) string {
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
	agentConfig *registry.AgentTypeConfig,
	sessionID string,
) (string, error) {
	sm.logger.Info("sending ACP session/load request",
		zap.String("agent_type", agentConfig.ID),
		zap.String("session_id", sessionID))

	if err := client.LoadSession(ctx, sessionID); err != nil {
		sm.logger.Error("ACP session/load failed",
			zap.String("agent_type", agentConfig.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return "", fmt.Errorf("session/load failed: %w", err)
	}

	sm.logger.Info("ACP session loaded successfully",
		zap.String("agent_type", agentConfig.ID),
		zap.String("session_id", sessionID))

	return sessionID, nil
}

// createNewSession creates a new session via ACP session/new
func (sm *SessionManager) createNewSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig *registry.AgentTypeConfig,
	workspacePath string,
	mcpServers []agentctltypes.McpServer,
) (string, error) {
	sm.logger.Info("sending ACP session/new request",
		zap.String("agent_type", agentConfig.ID),
		zap.String("workspace_path", workspacePath))

	sessionID, err := client.NewSession(ctx, workspacePath, mcpServers)
	if err != nil {
		sm.logger.Error("ACP session/new failed",
			zap.String("agent_type", agentConfig.ID),
			zap.String("workspace_path", workspacePath),
			zap.Error(err))
		return "", fmt.Errorf("session/new failed: %w", err)
	}

	sm.logger.Info("ACP session created successfully",
		zap.String("agent_type", agentConfig.ID),
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
	agentConfig *registry.AgentTypeConfig,
	taskDescription string,
	mcpServers []agentctltypes.McpServer,
	markReady func(executionID string) error,
) error {
	sm.logger.Info("initializing ACP session",
		zap.String("execution_id", execution.ID),
		zap.String("agentctl_url", execution.agentctl.BaseURL()),
		zap.String("agent_type", agentConfig.ID),
		zap.String("existing_acp_session_id", execution.ACPSessionID),
		zap.Bool("resume_via_acp", agentConfig.SessionConfig.ResumeViaACP))

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

	// Set up WebSocket streams
	if sm.streamManager != nil {
		updatesReady := make(chan struct{})
		sm.streamManager.ConnectAll(execution, updatesReady)

		// Wait for the updates stream to connect before sending prompt
		select {
		case <-updatesReady:
			sm.logger.Debug("updates stream ready")
		case <-time.After(5 * time.Second):
			sm.logger.Warn("timeout waiting for updates stream to connect, proceeding anyway")
		}
	}

	// Send the task prompt if provided - run asynchronously so orchestrator.start returns quickly.
	// The agent will process the prompt and emit events via the WebSocket stream.
	//
	// For resumed sessions (fork_session pattern): If we have session history but no new task description,
	// we still need to send a resume context prompt so the agent knows about the prior conversation.
	if taskDescription != "" {
		// Check if we need to inject resume context (fork_session pattern for ACP agents)
		effectivePrompt := sm.getResumeContextPrompt(agentConfig, execution.SessionID, taskDescription)
		if effectivePrompt != taskDescription {
			sm.logger.Info("injecting resume context into initial prompt",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Int("original_length", len(taskDescription)),
				zap.Int("effective_length", len(effectivePrompt)))
		}

		go func() {
			// Use detached context that respects stopCh for graceful shutdown
			promptCtx, cancel := appctx.Detached(ctx, sm.stopCh, 10*time.Minute)
			defer cancel()

			_, err := sm.SendPrompt(promptCtx, execution, effectivePrompt, false, markReady)
			if err != nil {
				sm.logger.Error("initial prompt failed",
					zap.String("execution_id", execution.ID),
					zap.Error(err))
			}
		}()
	} else if sm.shouldInjectResumeContext(agentConfig, execution.SessionID) {
		// No task description, but we have session history (resumed session)
		// Don't send a prompt now - we'll inject context when the user sends their first message.
		// This avoids triggering IN_PROGRESS state until the user actually interacts.
		execution.needsResumeContext = true
		sm.logger.Info("session has history for context injection, will inject on first user prompt",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID))
		if err := markReady(execution.ID); err != nil {
			sm.logger.Error("failed to mark execution as ready",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}
	} else {
		sm.logger.Debug("no task description and no resume context needed, marking as ready",
			zap.String("execution_id", execution.ID))
		if err := markReady(execution.ID); err != nil {
			sm.logger.Error("failed to mark execution as ready",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}
	}

	return nil
}

// SendPrompt sends a prompt to an agent execution and waits for completion.
// For initial prompts, pass validateStatus=false. For follow-up prompts, pass validateStatus=true.
// Returns the prompt result containing the stop reason and agent message.
func (sm *SessionManager) SendPrompt(
	ctx context.Context,
	execution *AgentExecution,
	prompt string,
	validateStatus bool,
	markReady func(executionID string) error,
) (*PromptResult, error) {
	if execution.agentctl == nil {
		return nil, fmt.Errorf("execution %q has no agentctl client", execution.ID)
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
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	execution.currentMessageID = "" // Clear streaming message ID for new turn
	execution.messageMu.Unlock()

	// Check if we need to inject resume context (fork_session pattern)
	// This happens on the first user prompt after resuming a session
	effectivePrompt := prompt
	if execution.needsResumeContext && !execution.resumeContextInjected {
		if sm.historyManager != nil {
			resumePrompt, err := sm.historyManager.GenerateResumeContext(execution.SessionID, prompt)
			if err != nil {
				sm.logger.Warn("failed to generate resume context for follow-up prompt",
					zap.String("execution_id", execution.ID),
					zap.Error(err))
			} else if resumePrompt != prompt {
				effectivePrompt = resumePrompt
				execution.resumeContextInjected = true
				sm.logger.Info("injecting resume context into follow-up prompt",
					zap.String("execution_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.Int("original_length", len(prompt)),
					zap.Int("effective_length", len(effectivePrompt)))
				// Log the full resume context for debugging (use Info for now so user can inspect)
				sm.logger.Info("resume context prompt content",
					zap.String("execution_id", execution.ID),
					zap.String("resume_prompt", effectivePrompt))
			}
		}
	}

	sm.logger.Info("sending prompt to agent",
		zap.String("execution_id", execution.ID),
		zap.Int("prompt_length", len(effectivePrompt)))

	// Store user prompt to session history for context injection (store original, not with injected context)
	if sm.historyManager != nil && execution.SessionID != "" {
		if err := sm.historyManager.AppendUserMessage(execution.SessionID, prompt); err != nil {
			sm.logger.Warn("failed to store user message to history", zap.Error(err))
		}
	}

	// Prompt is synchronous - blocks until agent completes
	resp, err := execution.agentctl.Prompt(ctx, effectivePrompt)
	if err != nil {
		sm.logger.Error("ACP prompt failed",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	// Extract accumulated content from buffers and handle streaming message completion
	execution.messageMu.Lock()
	agentMessage := execution.messageBuffer.String()
	execution.messageBuffer.Reset()
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	currentMsgID := execution.currentMessageID
	execution.currentMessageID = "" // Clear for next turn
	execution.messageMu.Unlock()

	stopReason := ""
	if resp != nil {
		stopReason = string(resp.StopReason)
	}

	trimmedMessage := strings.TrimSpace(agentMessage)

	sm.logger.Info("ACP prompt completed",
		zap.String("execution_id", execution.ID),
		zap.String("stop_reason", stopReason),
		zap.Int("message_length", len(trimmedMessage)),
		zap.Bool("has_streaming_msg", currentMsgID != ""))

	// Handle remaining content from the buffer
	if sm.eventPublisher != nil {
		// If there's remaining content and an active streaming message,
		// append it to the streaming message instead of creating a new one
		if trimmedMessage != "" && currentMsgID != "" {
			// Publish streaming append with the remaining content
			sm.eventPublisher.PublishAgentStreamEventPayload(&AgentStreamEventPayload{
				Type:      "agent/event",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				AgentID:   execution.AgentProfileID,
				TaskID:    execution.TaskID,
				SessionID: execution.SessionID,
				Data: &AgentStreamEventData{
					Type:      "message_streaming",
					MessageID: currentMsgID,
					IsAppend:  true,
					Text:      trimmedMessage,
				},
			})
			// Content was handled via streaming, publish complete with empty text
			trimmedMessage = ""
		}

		// Publish complete event (with remaining text only if no streaming message was active)
		completeEvent := streams.AgentEvent{
			Type:      streams.EventTypeComplete,
			SessionID: execution.ACPSessionID,
			Text:      trimmedMessage,
		}
		sm.eventPublisher.PublishAgentStreamEvent(execution, completeEvent)
	}

	// Mark as READY for next prompt
	if err := markReady(execution.ID); err != nil {
		sm.logger.Error("failed to mark execution as ready after prompt",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
	}

	return &PromptResult{
		StopReason:   stopReason,
		AgentMessage: agentMessage,
	}, nil
}
