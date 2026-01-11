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

// PendingPermission represents a permission request waiting for user response
type PendingPermission struct {
	ID          string                      // Unique ID for this pending request
	RPCID       interface{}                 // JSON-RPC request ID from agent
	InstanceID  string                      // Agent instance
	TaskID      string                      // Task being worked on
	SessionID   string                      // ACP session ID
	ToolCallID  string                      // Tool call requesting permission
	Title       string                      // Human-readable title
	Options     []jsonrpc.PermissionOption  // Available choices
	ResponseCh  chan *PermissionResponseMsg // Channel to receive user response
	CreatedAt   time.Time
}

// PermissionResponseMsg is the message sent by the user in response to a permission request
type PermissionResponseMsg struct {
	OptionID  string
	Cancelled bool
}

// PermissionRequestHandler is called when an agent requests permission from the user
type PermissionRequestHandler func(req *PendingPermission)

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

	// Handler for permission requests
	permissionHandler PermissionRequestHandler

	// Pending permission requests waiting for user response
	pendingPermissions map[string]*PendingPermission
	permissionMu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager(eventBus bus.EventBus, log *logger.Logger) *SessionManager {
	return &SessionManager{
		sessions:           make(map[string]*Session),
		pendingPermissions: make(map[string]*PendingPermission),
		eventBus:           eventBus,
		logger:             log.WithFields(zap.String("component", "acp-session-manager")),
	}
}

// SetUpdateHandler sets the handler for session updates
func (m *SessionManager) SetUpdateHandler(handler UpdateHandler) {
	m.updateHandler = handler
}

// SetPermissionHandler sets the handler for permission requests from agents
func (m *SessionManager) SetPermissionHandler(handler PermissionRequestHandler) {
	m.permissionHandler = handler
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

	// Set request handler for agent-to-client requests (like session/request_permission)
	client.SetRequestHandler(func(id interface{}, method string, params json.RawMessage) {
		m.handleRequest(session, id, method, params)
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
		ProtocolVersion: 1,
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
func (m *SessionManager) NewSession(ctx context.Context, instanceID, cwd string) (string, error) {
	session, err := m.getSession(instanceID)
	if err != nil {
		return "", err
	}

	m.logger.Info("Creating new ACP session",
		zap.String("instance_id", instanceID),
		zap.String("cwd", cwd))

	params := jsonrpc.SessionNewParams{
		Cwd:        cwd,
		McpServers: []jsonrpc.McpServer{}, // Empty array, no MCP servers for now
	}

	resp, err := session.Client.Call(ctx, jsonrpc.MethodSessionNew, params)
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

	session.mu.RLock()
	sessionID := session.SessionID
	session.mu.RUnlock()

	if sessionID == "" {
		return fmt.Errorf("no session ID available, call NewSession first")
	}

	m.logger.Info("Sending prompt to agent",
		zap.String("instance_id", instanceID),
		zap.String("session_id", sessionID),
		zap.Int("message_length", len(message)))

	session.mu.Lock()
	session.Status = "prompting"
	session.mu.Unlock()

	// ACP protocol requires prompt to be an array of ContentBlock
	params := jsonrpc.SessionPromptParams{
		SessionID: sessionID,
		Prompt: []jsonrpc.ContentBlock{
			{
				Type: "text",
				Text: message,
			},
		},
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

// handleRequest handles incoming requests from the agent (e.g., session/request_permission)
func (m *SessionManager) handleRequest(session *Session, id interface{}, method string, params json.RawMessage) {
	m.logger.Info("Received request from agent",
		zap.String("instance_id", session.InstanceID),
		zap.String("method", method),
		zap.Any("id", id))

	switch method {
	case jsonrpc.MethodRequestPermission:
		m.handleRequestPermission(session, id, params)
	default:
		m.logger.Warn("Unknown request method from agent", zap.String("method", method))
		// Send error response for unknown methods
		session.Client.SendResponse(id, nil, &jsonrpc.Error{
			Code:    jsonrpc.MethodNotFound,
			Message: fmt.Sprintf("Unknown method: %s", method),
		})
	}
}

// handleRequestPermission handles session/request_permission requests from the agent
func (m *SessionManager) handleRequestPermission(session *Session, id interface{}, params json.RawMessage) {
	var reqParams jsonrpc.RequestPermissionParams
	if err := json.Unmarshal(params, &reqParams); err != nil {
		m.logger.Error("Failed to parse request_permission params", zap.Error(err))
		session.Client.SendResponse(id, nil, &jsonrpc.Error{
			Code:    jsonrpc.InvalidParams,
			Message: "Invalid params",
		})
		return
	}

	m.logger.Info("Agent requesting permission",
		zap.String("instance_id", session.InstanceID),
		zap.String("session_id", reqParams.SessionID),
		zap.String("tool_call_id", reqParams.ToolCall.ToolCallID),
		zap.String("title", reqParams.ToolCall.Title),
		zap.Int("num_options", len(reqParams.Options)))

	// If no permission handler is set, fall back to auto-approve
	if m.permissionHandler == nil {
		m.autoApprovePermission(session, id, &reqParams)
		return
	}

	// Create a pending permission request
	pendingID := fmt.Sprintf("%s-%d", session.InstanceID, time.Now().UnixNano())
	pending := &PendingPermission{
		ID:         pendingID,
		RPCID:      id,
		InstanceID: session.InstanceID,
		TaskID:     session.TaskID,
		SessionID:  reqParams.SessionID,
		ToolCallID: reqParams.ToolCall.ToolCallID,
		Title:      reqParams.ToolCall.Title,
		Options:    reqParams.Options,
		ResponseCh: make(chan *PermissionResponseMsg, 1),
		CreatedAt:  time.Now(),
	}

	// Store the pending request
	m.permissionMu.Lock()
	m.pendingPermissions[pendingID] = pending
	m.permissionMu.Unlock()

	// Notify the permission handler (which will send WebSocket notification to user)
	m.permissionHandler(pending)

	// Wait for user response in a goroutine to not block the read loop
	go m.waitForPermissionResponse(session, pending)
}

// waitForPermissionResponse waits for user response and sends it to the agent
func (m *SessionManager) waitForPermissionResponse(session *Session, pending *PendingPermission) {
	// Wait for response with a 5-minute timeout
	timeout := time.After(5 * time.Minute)

	select {
	case resp := <-pending.ResponseCh:
		m.logger.Info("Received permission response from user",
			zap.String("pending_id", pending.ID),
			zap.String("option_id", resp.OptionID),
			zap.Bool("cancelled", resp.Cancelled))

		var result jsonrpc.RequestPermissionResult
		if resp.Cancelled {
			result = jsonrpc.RequestPermissionResult{
				Outcome: jsonrpc.PermissionOutcome{
					Outcome: "cancelled",
				},
			}
		} else {
			result = jsonrpc.RequestPermissionResult{
				Outcome: jsonrpc.PermissionOutcome{
					Outcome:  "selected",
					OptionID: resp.OptionID,
				},
			}
		}

		if err := session.Client.SendResponse(pending.RPCID, result, nil); err != nil {
			m.logger.Error("Failed to send permission response to agent", zap.Error(err))
		}

	case <-timeout:
		m.logger.Warn("Permission request timed out, cancelling",
			zap.String("pending_id", pending.ID))

		result := jsonrpc.RequestPermissionResult{
			Outcome: jsonrpc.PermissionOutcome{
				Outcome: "cancelled",
			},
		}
		session.Client.SendResponse(pending.RPCID, result, nil)
	}

	// Clean up
	m.permissionMu.Lock()
	delete(m.pendingPermissions, pending.ID)
	m.permissionMu.Unlock()
}

// autoApprovePermission auto-approves a permission request (fallback behavior)
func (m *SessionManager) autoApprovePermission(session *Session, id interface{}, reqParams *jsonrpc.RequestPermissionParams) {
	var selectedOptionID string
	for _, opt := range reqParams.Options {
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			selectedOptionID = opt.OptionID
			m.logger.Info("Auto-approving permission request",
				zap.String("option_id", selectedOptionID),
				zap.String("option_name", opt.Name),
				zap.String("kind", opt.Kind))
			break
		}
	}

	// If no allow option found, use the first option
	if selectedOptionID == "" && len(reqParams.Options) > 0 {
		selectedOptionID = reqParams.Options[0].OptionID
		m.logger.Info("No allow option found, using first option",
			zap.String("option_id", selectedOptionID))
	}

	result := jsonrpc.RequestPermissionResult{
		Outcome: jsonrpc.PermissionOutcome{
			Outcome:  "selected",
			OptionID: selectedOptionID,
		},
	}

	if err := session.Client.SendResponse(id, result, nil); err != nil {
		m.logger.Error("Failed to send permission response", zap.Error(err))
	} else {
		m.logger.Info("Sent auto-approved permission response",
			zap.String("instance_id", session.InstanceID),
			zap.String("selected_option", selectedOptionID))
	}
}

// RespondToPermission sends a user's response to a pending permission request
func (m *SessionManager) RespondToPermission(pendingID string, optionID string, cancelled bool) error {
	m.permissionMu.RLock()
	pending, exists := m.pendingPermissions[pendingID]
	m.permissionMu.RUnlock()

	if !exists {
		return fmt.Errorf("pending permission request not found: %s", pendingID)
	}

	// Send response through channel (non-blocking)
	select {
	case pending.ResponseCh <- &PermissionResponseMsg{
		OptionID:  optionID,
		Cancelled: cancelled,
	}:
		m.logger.Info("Sent permission response to waiting goroutine",
			zap.String("pending_id", pendingID),
			zap.String("option_id", optionID))
		return nil
	default:
		return fmt.Errorf("permission request already responded to or timed out")
	}
}

// GetPendingPermission returns a pending permission request by ID
func (m *SessionManager) GetPendingPermission(pendingID string) (*PendingPermission, bool) {
	m.permissionMu.RLock()
	defer m.permissionMu.RUnlock()
	pending, exists := m.pendingPermissions[pendingID]
	return pending, exists
}

// GetPendingPermissionsForTask returns all pending permissions for a task
func (m *SessionManager) GetPendingPermissionsForTask(taskID string) []*PendingPermission {
	m.permissionMu.RLock()
	defer m.permissionMu.RUnlock()

	var result []*PendingPermission
	for _, pending := range m.pendingPermissions {
		if pending.TaskID == taskID {
			result = append(result, pending)
		}
	}
	return result
}
