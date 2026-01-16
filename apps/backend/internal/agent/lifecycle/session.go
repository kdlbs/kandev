package lifecycle

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agentctl"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

// SessionManager handles ACP session initialization and management
type SessionManager struct {
	logger *logger.Logger
}

// NewSessionManager creates a new SessionManager
func NewSessionManager(log *logger.Logger) *SessionManager {
	return &SessionManager{
		logger: log,
	}
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
	sessionID, err := sm.createOrLoadSession(ctx, client, agentConfig, existingSessionID, workspacePath)
	if err != nil {
		return nil, err
	}

	result.SessionID = sessionID
	return result, nil
}

// createOrLoadSession determines whether to create a new session or load an existing one
// based on the agent configuration and whether an existing session ID is provided.
func (sm *SessionManager) createOrLoadSession(
	ctx context.Context,
	client *agentctl.Client,
	agentConfig *registry.AgentTypeConfig,
	existingSessionID string,
	workspacePath string,
) (string, error) {
	// Use session/load only when:
	// 1. ResumeViaACP is true (agent supports ACP-based resume)
	// 2. An existing session ID is provided
	shouldLoadSession := agentConfig.SessionConfig.ResumeViaACP && existingSessionID != ""

	if shouldLoadSession {
		return sm.loadSession(ctx, client, agentConfig, existingSessionID)
	}

	// Log why we're creating a new session when an existing session ID was provided
	if existingSessionID != "" && !agentConfig.SessionConfig.ResumeViaACP {
		sm.logger.Info("skipping ACP session/load (resume handled via CLI)",
			zap.String("agent_type", agentConfig.ID),
			zap.String("session_id", existingSessionID),
			zap.String("resume_flag", agentConfig.SessionConfig.ResumeFlag))
	}

	return sm.createNewSession(ctx, client, agentConfig, workspacePath)
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
) (string, error) {
	sm.logger.Info("sending ACP session/new request",
		zap.String("agent_type", agentConfig.ID),
		zap.String("workspace_path", workspacePath))

	sessionID, err := client.NewSession(ctx, workspacePath)
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
