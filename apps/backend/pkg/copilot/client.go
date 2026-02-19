// Package copilot provides integration with the GitHub Copilot SDK.
// This is a thin wrapper around github.com/github/copilot-sdk/go that
// provides a consistent interface for the kandev agent system.
//
// When CLIUrl is configured, the SDK connects to an externally managed
// Copilot CLI server via TCP (JSON-RPC). Otherwise, the SDK spawns and
// manages the CLI process internally via stdio.
package copilot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/github/copilot-sdk/go"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Re-export SDK types for convenience
type (
	SessionEvent     = copilot.SessionEvent
	SessionEventType = copilot.SessionEventType
	SessionConfig    = copilot.SessionConfig
	MessageOptions   = copilot.MessageOptions
	Data             = copilot.Data
	// Permission types
	PermissionHandler       = copilot.PermissionHandler
	PermissionRequest       = copilot.PermissionRequest
	PermissionInvocation    = copilot.PermissionInvocation
	PermissionRequestResult = copilot.PermissionRequestResult
	// MCP types
	MCPServerConfig = copilot.MCPServerConfig
)

// Re-export event type constants
const (
	EventTypeSessionStart            = copilot.SessionStart
	EventTypeSessionResume           = copilot.SessionResume
	EventTypeSessionIdle             = copilot.SessionIdle
	EventTypeSessionError            = copilot.SessionError
	EventTypeSessionUsageInfo        = copilot.SessionUsageInfo
	EventTypeAssistantMessage        = copilot.AssistantMessage
	EventTypeAssistantMessageDelta   = copilot.AssistantMessageDelta
	EventTypeAssistantReasoning      = copilot.AssistantReasoning
	EventTypeAssistantReasoningDelta = copilot.AssistantReasoningDelta
	EventTypeAssistantTurnStart      = copilot.AssistantTurnStart
	EventTypeAssistantTurnEnd        = copilot.AssistantTurnEnd
	EventTypeAssistantUsage          = copilot.AssistantUsage
	EventTypeToolStart               = copilot.ToolExecutionStart
	EventTypeToolComplete            = copilot.ToolExecutionComplete
	EventTypeToolProgress            = copilot.ToolExecutionProgress
	EventTypeAbort                   = copilot.Abort
)

// Client wraps the Copilot SDK client with additional functionality.
type Client struct {
	sdkClient *copilot.Client
	session   *copilot.Session
	logger    *logger.Logger

	// Configuration
	cliURL string
	model  string

	// Event handler
	eventHandler func(SessionEvent)
	unsubscribe  func()
	handlerMu    sync.RWMutex

	// Permission handler
	permissionHandler PermissionHandler
	permissionMu      sync.RWMutex

	// State
	sessionID string
	mu        sync.RWMutex
	started   bool
}

// ClientConfig holds configuration for creating a Client.
type ClientConfig struct {
	// CLIUrl is the address of an externally managed Copilot CLI server (e.g. "localhost:12345").
	// When set, the SDK connects via TCP instead of spawning its own process.
	CLIUrl string
	Model  string
}

// NewClient creates a new Copilot client wrapper.
func NewClient(cfg ClientConfig, log *logger.Logger) *Client {
	if cfg.Model == "" {
		cfg.Model = "gpt-4.1"
	}

	return &Client{
		cliURL: cfg.CLIUrl,
		model:  cfg.Model,
		logger: log.WithFields(zap.String("component", "copilot-sdk-client")),
	}
}

// SetEventHandler sets the handler for session events.
func (c *Client) SetEventHandler(handler func(SessionEvent)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.eventHandler = handler
}

// SetPermissionHandler sets the handler for permission requests.
func (c *Client) SetPermissionHandler(handler PermissionHandler) {
	c.permissionMu.Lock()
	defer c.permissionMu.Unlock()
	c.permissionHandler = handler
}

// Start initializes the Copilot SDK client.
// When CLIUrl is configured, the SDK connects to an external CLI server via TCP.
// Otherwise, the SDK spawns and manages the CLI process internally via stdio.
// The actual connection is deferred to the first CreateSession call via AutoStart.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("client already started")
	}

	c.logger.Info("starting Copilot SDK client",
		zap.String("model", c.model),
		zap.String("cli_url", c.cliURL))

	if c.cliURL != "" {
		// Connect to externally managed CLI server via TCP
		c.sdkClient = copilot.NewClient(&copilot.ClientOptions{
			CLIUrl:   c.cliURL,
			LogLevel: "error",
		})
	} else {
		// SDK spawns and manages the CLI process internally (stdio)
		c.sdkClient = copilot.NewClient(nil)
	}

	// SDK AutoStart (default: true) defers the actual connection
	// to the first CreateSession call, so no explicit Start() needed.

	c.started = true
	c.logger.Info("Copilot SDK client initialized")

	return nil
}

// Stop shuts down the Copilot SDK client.
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.logger.Info("stopping Copilot SDK client")

	// Unsubscribe from events
	c.handlerMu.Lock()
	if c.unsubscribe != nil {
		c.unsubscribe()
		c.unsubscribe = nil
	}
	c.handlerMu.Unlock()

	// Destroy session if active
	if c.session != nil {
		if err := c.session.Destroy(); err != nil {
			c.logger.Warn("error destroying session", zap.Error(err))
		}
		c.session = nil
	}

	// Stop the SDK client
	if c.sdkClient != nil {
		errs := c.sdkClient.Stop()
		for _, err := range errs {
			c.logger.Warn("error stopping SDK client", zap.Error(err))
		}
		c.sdkClient = nil
	}

	c.started = false
	return nil
}

// CreateSession creates a new Copilot session.
// mcpServers configures MCP servers for the session (nil if none).
func (c *Client) CreateSession(ctx context.Context, mcpServers map[string]MCPServerConfig) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return "", fmt.Errorf("client not started")
	}

	// Unsubscribe from previous session events
	c.handlerMu.Lock()
	if c.unsubscribe != nil {
		c.unsubscribe()
		c.unsubscribe = nil
	}
	c.handlerMu.Unlock()

	// Destroy existing session if any
	if c.session != nil {
		if err := c.session.Destroy(); err != nil {
			c.logger.Warn("error destroying previous session", zap.Error(err))
		}
		c.session = nil
	}

	c.logger.Info("creating new session",
		zap.String("model", c.model),
		zap.Int("mcp_servers", len(mcpServers)))

	// Get permission handler
	c.permissionMu.RLock()
	permHandler := c.permissionHandler
	c.permissionMu.RUnlock()

	// Create session with configuration
	session, err := c.sdkClient.CreateSession(&copilot.SessionConfig{
		Model:               c.model,
		Streaming:           true,
		OnPermissionRequest: permHandler,
		MCPServers:          mcpServers,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Register event handler BEFORE storing the session so no events are lost
	// between session creation and handler registration.
	c.handlerMu.Lock()
	if c.eventHandler != nil {
		c.unsubscribe = session.On(c.eventHandler)
	}
	c.handlerMu.Unlock()

	c.session = session
	c.sessionID = session.SessionID

	c.logger.Info("session created", zap.String("session_id", c.sessionID))

	return c.sessionID, nil
}

// ResumeSession resumes an existing session with streaming enabled.
// mcpServers configures MCP servers for the resumed session (nil if none).
func (c *Client) ResumeSession(ctx context.Context, sessionID string, mcpServers map[string]MCPServerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return fmt.Errorf("client not started")
	}

	// Unsubscribe from previous session events
	c.handlerMu.Lock()
	if c.unsubscribe != nil {
		c.unsubscribe()
		c.unsubscribe = nil
	}
	c.handlerMu.Unlock()

	// Destroy existing session if any
	if c.session != nil {
		if err := c.session.Destroy(); err != nil {
			c.logger.Warn("error destroying previous session", zap.Error(err))
		}
		c.session = nil
	}

	c.logger.Info("resuming session",
		zap.String("session_id", sessionID),
		zap.Int("mcp_servers", len(mcpServers)))

	// Get permission handler
	c.permissionMu.RLock()
	permHandler := c.permissionHandler
	c.permissionMu.RUnlock()

	// Use ResumeSessionWithOptions to enable streaming on resumed sessions
	session, err := c.sdkClient.ResumeSessionWithOptions(sessionID, &copilot.ResumeSessionConfig{
		Streaming:           true,
		OnPermissionRequest: permHandler,
		MCPServers:          mcpServers,
	})
	if err != nil {
		return fmt.Errorf("failed to resume session: %w", err)
	}

	// Register event handler BEFORE storing the session so no events are lost.
	c.handlerMu.Lock()
	if c.eventHandler != nil {
		c.unsubscribe = session.On(c.eventHandler)
	}
	c.handlerMu.Unlock()

	c.session = session
	c.sessionID = sessionID

	c.logger.Info("session resumed", zap.String("session_id", sessionID))

	return nil
}

// Send sends a message to the current session (non-blocking).
func (c *Client) Send(ctx context.Context, message string) (string, error) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		return "", fmt.Errorf("no active session")
	}

	c.logger.Info("sending message to session")

	messageID, err := session.Send(copilot.MessageOptions{
		Prompt: message,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return messageID, nil
}

// SendAndWait sends a message and waits for completion.
func (c *Client) SendAndWait(ctx context.Context, message string, timeout time.Duration) (*SessionEvent, error) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	c.logger.Info("sending message and waiting for completion")

	result, err := session.SendAndWait(copilot.MessageOptions{
		Prompt: message,
	}, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	return result, nil
}

// Abort cancels the current operation.
func (c *Client) Abort(ctx context.Context) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		return nil
	}

	c.logger.Info("aborting current operation")
	return session.Abort()
}

// GetSessionID returns the current session ID.
func (c *Client) GetSessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// IsStarted returns whether the client has been started.
func (c *Client) IsStarted() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.started
}
