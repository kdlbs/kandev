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
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// SessionManager handles ACP session initialization and management
type SessionManager struct {
	logger         *logger.Logger
	eventPublisher *EventPublisher
	streamManager  *StreamManager
	executionStore *ExecutionStore
}

// NewSessionManager creates a new SessionManager
func NewSessionManager(log *logger.Logger) *SessionManager {
	return &SessionManager{
		logger: log,
	}
}

// SetDependencies sets the optional dependencies for full session orchestration.
// These are set after construction to avoid circular dependencies.
func (sm *SessionManager) SetDependencies(ep *EventPublisher, strm *StreamManager, store *ExecutionStore) {
	sm.eventPublisher = ep
	sm.streamManager = strm
	sm.executionStore = store
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
		return sm.loadSession(ctx, client, agentConfig, existingSessionID)
	}
	return sm.createNewSession(ctx, client, agentConfig, workspacePath, mcpServers)
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
	if taskDescription != "" {
		go func() {
			// Use a long timeout for initial prompts
			promptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			_, err := sm.SendPrompt(promptCtx, execution, taskDescription, false, markReady)
			if err != nil {
				sm.logger.Error("initial prompt failed",
					zap.String("execution_id", execution.ID),
					zap.Error(err))
			}
		}()
	} else {
		sm.logger.Warn("no task description provided, marking as ready",
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

	// Clear buffers before starting prompt
	execution.messageMu.Lock()
	execution.messageBuffer.Reset()
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	execution.messageMu.Unlock()

	sm.logger.Info("sending prompt to agent",
		zap.String("execution_id", execution.ID),
		zap.Int("prompt_length", len(prompt)))

	// Prompt is synchronous - blocks until agent completes
	resp, err := execution.agentctl.Prompt(ctx, prompt)
	if err != nil {
		sm.logger.Error("ACP prompt failed",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	// Extract accumulated content from buffers
	execution.messageMu.Lock()
	agentMessage := execution.messageBuffer.String()
	execution.messageBuffer.Reset()
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	execution.messageMu.Unlock()

	stopReason := ""
	if resp != nil {
		stopReason = string(resp.StopReason)
	}

	sm.logger.Info("ACP prompt completed",
		zap.String("execution_id", execution.ID),
		zap.String("stop_reason", stopReason),
		zap.Int("message_length", len(agentMessage)))

	// Publish complete event with accumulated agent message
	// This is needed for ACP where Prompt() is synchronous and there's no
	// separate "complete" notification from the agent
	if sm.eventPublisher != nil {
		completeEvent := streams.AgentEvent{
			Type:      streams.EventTypeComplete,
			SessionID: execution.ACPSessionID,
			Text:      strings.TrimSpace(agentMessage),
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
