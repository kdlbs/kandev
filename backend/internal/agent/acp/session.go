// Package acp provides ACP (Agent Client Protocol) session management
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/pkg/acp/jsonrpc"
	"go.uber.org/zap"
)

// Session represents an active ACP session with an agent
type Session struct {
	InstanceID  string
	TaskID      string
	SessionID   string // ACP session ID (from agent)
	Client      *jsonrpc.Client
	Stdin       io.WriteCloser
	Stdout      io.Reader
	CreatedAt   time.Time
	Status      string // initializing, ready, prompting, complete, error
	mu          sync.RWMutex
}

// UpdateHandler is called when the agent sends session updates
type UpdateHandler func(instanceID, taskID string, updateType string, data json.RawMessage)

// SessionManager manages ACP sessions for multiple agents
type SessionManager struct {
	sessions map[string]*Session // by instance ID
	mu       sync.RWMutex
	eventBus bus.EventBus
	logger   *logger.Logger
	
	// Handler for session updates
	updateHandler UpdateHandler
}

// NewSessionManager creates a new session manager
func NewSessionManager(eventBus bus.EventBus, log *logger.Logger) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "acp-session-manager")),
	}
}

// SetUpdateHandler sets the handler for session updates
func (m *SessionManager) SetUpdateHandler(handler UpdateHandler) {
	m.updateHandler = handler
}

// CreateSession creates a new ACP session for an agent instance
func (m *SessionManager) CreateSession(ctx context.Context, instanceID, taskID string, stdin io.WriteCloser, stdout io.Reader) error {
	m.logger.Info("Creating ACP session",
		zap.String("instance_id", instanceID),
		zap.String("task_id", taskID))

	// Create JSON-RPC client
	client := jsonrpc.NewClient(stdin, stdout, m.logger)

	session := &Session{
		InstanceID: instanceID,
		TaskID:     taskID,
		Client:     client,
		Stdin:      stdin,
		Stdout:     stdout,
		CreatedAt:  time.Now(),
		Status:     "initializing",
	}

	// Set notification handler
	client.SetNotificationHandler(func(method string, params json.RawMessage) {
		m.handleNotification(session, method, params)
	})

	// Start the client read loop
	client.Start(ctx)

	// Store session
	m.mu.Lock()
	m.sessions[instanceID] = session
	m.mu.Unlock()

	return nil
}

// Initialize performs the ACP initialize handshake
func (m *SessionManager) Initialize(ctx context.Context, instanceID string) error {
	session, err := m.getSession(instanceID)
	if err != nil {
		return err
	}

	m.logger.Info("Initializing ACP session", zap.String("instance_id", instanceID))

	params := jsonrpc.InitializeParams{
		ProtocolVersion: "1.0",
		ClientInfo: jsonrpc.ClientInfo{
			Name:    "kandev",
			Version: "0.1.0",
		},
		Capabilities: jsonrpc.ClientCapabilities{
			Streaming: true,
		},
	}

	resp, err := session.Client.Call(ctx, jsonrpc.MethodInitialize, params)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s (code %d)", resp.Error.Message, resp.Error.Code)
	}

	session.mu.Lock()
	session.Status = "ready"
	session.mu.Unlock()

	m.logger.Info("ACP session initialized", zap.String("instance_id", instanceID))
	return nil
}

// NewSession creates a new ACP session (session/new)
func (m *SessionManager) NewSession(ctx context.Context, instanceID string) (string, error) {
	session, err := m.getSession(instanceID)
	if err != nil {
		return "", err
	}

	m.logger.Info("Creating new ACP session", zap.String("instance_id", instanceID))

	resp, err := session.Client.Call(ctx, jsonrpc.MethodSessionNew, jsonrpc.SessionNewParams{})
	if err != nil {
		return "", fmt.Errorf("session/new failed: %w", err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("session/new error: %s", resp.Error.Message)
	}

	var result jsonrpc.SessionNewResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse session/new result: %w", err)
	}

	session.mu.Lock()
	session.SessionID = result.SessionID
	session.mu.Unlock()

	m.logger.Info("ACP session created",
		zap.String("instance_id", instanceID),
		zap.String("session_id", result.SessionID))

	return result.SessionID, nil
}

// LoadSession resumes an existing ACP session (session/load)
func (m *SessionManager) LoadSession(ctx context.Context, instanceID, sessionID string) error {
	session, err := m.getSession(instanceID)
	if err != nil {
		return err
	}

	m.logger.Info("Loading ACP session",
		zap.String("instance_id", instanceID),
		zap.String("session_id", sessionID))

	params := jsonrpc.SessionLoadParams{
		SessionID: sessionID,
	}

	resp, err := session.Client.Call(ctx, jsonrpc.MethodSessionLoad, params)
	if err != nil {
		return fmt.Errorf("session/load failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("session/load error: %s", resp.Error.Message)
	}

	session.mu.Lock()
	session.SessionID = sessionID
	session.mu.Unlock()

	m.logger.Info("ACP session loaded", zap.String("instance_id", instanceID))
	return nil
}

// Prompt sends a prompt to the agent (session/prompt)
func (m *SessionManager) Prompt(ctx context.Context, instanceID, message string) error {
	session, err := m.getSession(instanceID)
	if err != nil {
		return err
	}

	m.logger.Info("Sending prompt to agent",
		zap.String("instance_id", instanceID),
		zap.String("message_length", fmt.Sprintf("%d", len(message))))

	session.mu.Lock()
	session.Status = "prompting"
	session.mu.Unlock()

	params := jsonrpc.SessionPromptParams{
		Message: message,
	}

	resp, err := session.Client.Call(ctx, jsonrpc.MethodSessionPrompt, params)
	if err != nil {
		return fmt.Errorf("session/prompt failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("session/prompt error: %s", resp.Error.Message)
	}

	return nil
}

// Cancel cancels the current operation (session/cancel)
func (m *SessionManager) Cancel(ctx context.Context, instanceID, reason string) error {
	session, err := m.getSession(instanceID)
	if err != nil {
		return err
	}

	m.logger.Info("Cancelling agent operation",
		zap.String("instance_id", instanceID),
		zap.String("reason", reason))

	params := jsonrpc.SessionCancelParams{
		Reason: reason,
	}

	// Cancel is a notification, not a request
	return session.Client.Notify(jsonrpc.MethodSessionCancel, params)
}

// CloseSession closes an ACP session
func (m *SessionManager) CloseSession(instanceID string) error {
	m.mu.Lock()
	session, exists := m.sessions[instanceID]
	if exists {
		delete(m.sessions, instanceID)
	}
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("session not found: %s", instanceID)
	}

	m.logger.Info("Closing ACP session", zap.String("instance_id", instanceID))

	session.Client.Stop()
	if session.Stdin != nil {
		session.Stdin.Close()
	}

	return nil
}

// GetSession returns a session by instance ID
func (m *SessionManager) GetSession(instanceID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[instanceID]
	return session, exists
}

// getSession returns a session or error if not found
func (m *SessionManager) getSession(instanceID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[instanceID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", instanceID)
	}
	return session, nil
}

// handleNotification handles incoming notifications from the agent
func (m *SessionManager) handleNotification(session *Session, method string, params json.RawMessage) {
	m.logger.Debug("Received notification",
		zap.String("instance_id", session.InstanceID),
		zap.String("method", method))

	switch method {
	case jsonrpc.NotificationSessionUpdate:
		var update jsonrpc.SessionUpdate
		if err := json.Unmarshal(params, &update); err != nil {
			m.logger.Error("Failed to parse session update", zap.Error(err))
			return
		}

		// Handle completion
		if update.Type == "complete" {
			session.mu.Lock()
			session.Status = "complete"
			session.mu.Unlock()
		}

		// Forward to update handler
		if m.updateHandler != nil {
			m.updateHandler(session.InstanceID, session.TaskID, update.Type, update.Data)
		}

	default:
		m.logger.Warn("Unknown notification method", zap.String("method", method))
	}
}

// GetSessionID returns the ACP session ID for an instance
func (m *SessionManager) GetSessionID(instanceID string) (string, bool) {
	session, exists := m.GetSession(instanceID)
	if !exists {
		return "", false
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.SessionID, session.SessionID != ""
}

