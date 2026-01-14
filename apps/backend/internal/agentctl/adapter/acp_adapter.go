package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/acp"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ACPAdapter implements AgentAdapter for agents using the ACP protocol.
// ACP (Agent Communication Protocol) uses JSON-RPC 2.0 over stdin/stdout.
// The subprocess is managed externally (by process.Manager) and stdin/stdout
// are passed to the adapter.
type ACPAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Subprocess stdin/stdout (managed externally)
	stdin  io.Writer
	stdout io.Reader

	// ACP SDK connection
	acpClient *acpclient.Client
	acpConn   *acp.ClientSideConnection
	sessionID string

	// Update channel
	updatesCh chan SessionUpdate

	// Permission handler
	permissionHandler PermissionHandler

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// NewACPAdapter creates a new ACP protocol adapter.
// stdin and stdout are the subprocess's stdin/stdout pipes (managed by process.Manager).
func NewACPAdapter(stdin io.Writer, stdout io.Reader, cfg *Config, log *logger.Logger) *ACPAdapter {
	return &ACPAdapter{
		cfg:       cfg,
		logger:    log.WithFields(zap.String("adapter", "acp")),
		stdin:     stdin,
		stdout:    stdout,
		updatesCh: make(chan SessionUpdate, 100),
	}
}

// Initialize establishes the ACP connection with the agent subprocess.
// The subprocess should already be running (started by process.Manager).
func (a *ACPAdapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing ACP adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create ACP client with update handler that converts to SessionUpdate
	a.acpClient = acpclient.NewClient(
		acpclient.WithLogger(a.logger.Zap()),
		acpclient.WithWorkspaceRoot(a.cfg.WorkDir),
		acpclient.WithUpdateHandler(a.handleACPUpdate),
		acpclient.WithPermissionHandler(a.handlePermissionRequest),
	)

	// Create ACP SDK connection
	a.acpConn = acp.NewClientSideConnection(a.acpClient, a.stdin, a.stdout)
	a.acpConn.SetLogger(slog.Default().With("component", "acp-conn"))

	a.logger.Info("ACP adapter initialized")

	return nil
}

// NewSession creates a new agent session.
func (a *ACPAdapter) NewSession(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.acpConn == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	resp, err := a.acpConn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        a.cfg.WorkDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	a.sessionID = string(resp.SessionId)
	a.logger.Info("created new session", zap.String("session_id", a.sessionID))

	return a.sessionID, nil
}

// LoadSession resumes an existing session.
func (a *ACPAdapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.acpConn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	_, err := a.acpConn.LoadSession(ctx, acp.LoadSessionRequest{
		SessionId: acp.SessionId(sessionID),
	})
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	a.sessionID = sessionID
	a.logger.Info("loaded session", zap.String("session_id", a.sessionID))

	return nil
}

// Prompt sends a prompt to the agent.
func (a *ACPAdapter) Prompt(ctx context.Context, message string) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt", zap.String("session_id", sessionID))

	_, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    []acp.ContentBlock{acp.TextBlock(message)},
	})
	return err
}

// Cancel cancels the current operation.
func (a *ACPAdapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling session", zap.String("session_id", sessionID))

	return conn.Cancel(ctx, acp.CancelNotification{
		SessionId: acp.SessionId(sessionID),
	})
}

// Updates returns the channel for session updates.
func (a *ACPAdapter) Updates() <-chan SessionUpdate {
	return a.updatesCh
}

// GetSessionID returns the current session ID.
func (a *ACPAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *ACPAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *ACPAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing ACP adapter")

	// Close update channel
	close(a.updatesCh)

	// Note: We don't close stdin or manage the subprocess here.
	// That's handled by process.Manager which owns the subprocess.

	return nil
}

// GetACPConnection returns the underlying ACP connection for advanced usage.
func (a *ACPAdapter) GetACPConnection() *acp.ClientSideConnection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.acpConn
}

// handleACPUpdate converts ACP SessionNotification to protocol-agnostic SessionUpdate.
func (a *ACPAdapter) handleACPUpdate(n acp.SessionNotification) {
	update := a.convertNotification(n)
	if update != nil {
		select {
		case a.updatesCh <- *update:
		default:
			a.logger.Warn("updates channel full, dropping notification")
		}
	}
}

// convertNotification converts an ACP SessionNotification to a SessionUpdate.
func (a *ACPAdapter) convertNotification(n acp.SessionNotification) *SessionUpdate {
	u := n.Update

	switch {
	case u.AgentMessageChunk != nil:
		if u.AgentMessageChunk.Content.Text != nil {
			return &SessionUpdate{
				Type:      "message_chunk",
				SessionID: string(n.SessionId),
				Text:      u.AgentMessageChunk.Content.Text.Text,
			}
		}

	case u.ToolCall != nil:
		return &SessionUpdate{
			Type:       "tool_call",
			SessionID:  string(n.SessionId),
			ToolCallID: string(u.ToolCall.ToolCallId),
			ToolTitle:  u.ToolCall.Title,
			ToolStatus: string(u.ToolCall.Status),
		}

	case u.ToolCallUpdate != nil:
		status := ""
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		return &SessionUpdate{
			Type:       "tool_update",
			SessionID:  string(n.SessionId),
			ToolCallID: string(u.ToolCallUpdate.ToolCallId),
			ToolStatus: status,
		}

	case u.Plan != nil:
		entries := make([]PlanEntry, len(u.Plan.Entries))
		for i, e := range u.Plan.Entries {
			entries[i] = PlanEntry{
				Description: e.Content,
				Status:      string(e.Status),
				Priority:    string(e.Priority),
			}
		}
		return &SessionUpdate{
			Type:        "plan",
			SessionID:   string(n.SessionId),
			PlanEntries: entries,
		}
	}

	return nil
}

// handlePermissionRequest converts permission request to adapter format.
func (a *ACPAdapter) handlePermissionRequest(ctx context.Context, req *acpclient.PermissionRequest) (*acpclient.PermissionResponse, error) {
	a.mu.RLock()
	handler := a.permissionHandler
	a.mu.RUnlock()

	if handler == nil {
		// Auto-approve if no handler
		if len(req.Options) > 0 {
			return &acpclient.PermissionResponse{OptionID: req.Options[0].OptionID}, nil
		}
		return &acpclient.PermissionResponse{Cancelled: true}, nil
	}

	// Convert to adapter types
	options := make([]PermissionOption, len(req.Options))
	for i, opt := range req.Options {
		options[i] = PermissionOption{
			OptionID: opt.OptionID,
			Name:     opt.Name,
			Kind:     opt.Kind,
		}
	}

	adapterReq := &PermissionRequest{
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		Title:      req.Title,
		Options:    options,
	}

	resp, err := handler(ctx, adapterReq)
	if err != nil {
		return nil, err
	}

	return &acpclient.PermissionResponse{
		OptionID:  resp.OptionID,
		Cancelled: resp.Cancelled,
	}, nil
}

// Verify interface implementation
var _ AgentAdapter = (*ACPAdapter)(nil)

