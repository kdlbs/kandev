// Package acp implements the ACP client interface for agentctl
package acp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// UpdateHandler is called when session updates are received from the agent
type UpdateHandler func(notification acp.SessionNotification)

// PermissionRequestHandler is called when the agent requests permission
// Returns the selected option ID, or empty string with cancelled=true to cancel
type PermissionRequestHandler func(ctx context.Context, req *types.PermissionRequest) (*types.PermissionResponse, error)

// Client implements acp.Client interface and handles all agent requests
type Client struct {
	logger        *zap.Logger
	workspaceRoot string

	mu                sync.RWMutex
	updateHandler     UpdateHandler
	permissionHandler PermissionRequestHandler
}

// ClientOption configures a Client
type ClientOption func(*Client)

// WithLogger sets the logger
func WithLogger(l *zap.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithWorkspaceRoot sets the workspace root for file operations
func WithWorkspaceRoot(root string) ClientOption {
	return func(c *Client) {
		c.workspaceRoot = root
	}
}

// WithUpdateHandler sets the handler for session updates
func WithUpdateHandler(h UpdateHandler) ClientOption {
	return func(c *Client) {
		c.updateHandler = h
	}
}

// WithPermissionHandler sets the handler for permission requests
func WithPermissionHandler(h PermissionRequestHandler) ClientOption {
	return func(c *Client) {
		c.permissionHandler = h
	}
}

// NewClient creates a new ACP client implementation
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		logger:        zap.NewNop(),
		workspaceRoot: "/workspace",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetUpdateHandler sets the update handler (thread-safe)
func (c *Client) SetUpdateHandler(h UpdateHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateHandler = h
}

// SetPermissionHandler sets the permission request handler (thread-safe)
func (c *Client) SetPermissionHandler(h PermissionRequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.permissionHandler = h
}

// RequestPermission handles permission requests from the agent
// If a permission handler is set, it forwards the request to the handler.
// Otherwise, auto-approves by selecting the first "allow" option.
func (c *Client) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	title := ""
	if p.ToolCall.Title != nil {
		title = *p.ToolCall.Title
	}
	c.logger.Info("received permission request",
		zap.String("session_id", string(p.SessionId)),
		zap.String("tool_call_id", string(p.ToolCall.ToolCallId)),
		zap.String("title", title),
		zap.Int("num_options", len(p.Options)))

	// No options - cancel
	if len(p.Options) == 0 {
		c.logger.Warn("no options available, cancelling permission request")
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}

	// Check if we have a permission handler
	c.mu.RLock()
	handler := c.permissionHandler
	c.mu.RUnlock()

	if handler != nil {
		// Forward to external handler (e.g., backend/user)
		return c.forwardPermissionRequest(ctx, handler, p)
	}

	// Fall back to auto-approve
	return c.autoApprovePermission(p)
}

// forwardPermissionRequest forwards the permission request to an external handler
func (c *Client) forwardPermissionRequest(ctx context.Context, handler PermissionRequestHandler, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	// Convert ACP types to shared types
	options := make([]types.PermissionOption, len(p.Options))
	for i, opt := range p.Options {
		options[i] = types.PermissionOption{
			OptionID: string(opt.OptionId),
			Name:     opt.Name,
			Kind:     string(opt.Kind),
		}
	}

	// Extract title - prefer Kind as the title, use Title as description
	title := ""
	description := ""
	if p.ToolCall.Title != nil {
		description = *p.ToolCall.Title
	}

	// Use Kind as the action type (e.g., "run_shell_command", "write_file")
	actionType := ""
	if p.ToolCall.Kind != nil {
		actionType = string(*p.ToolCall.Kind)
		title = actionType // Use kind as the title for cleaner display
	}

	// If no Kind, try to extract a short title from the verbose title
	// Gemini format: "pwd [current working directory /path] (Print the current working directory.)"
	if title == "" && description != "" {
		// Use the first word/command as the title
		if idx := strings.Index(description, " "); idx > 0 {
			title = description[:idx]
		} else {
			title = description
		}
	}

	// Build action details from raw input if available
	actionDetails := make(map[string]any)
	if p.ToolCall.RawInput != nil {
		actionDetails["raw_input"] = p.ToolCall.RawInput
	}
	if description != "" && description != title {
		actionDetails["description"] = description
	}

	req := &types.PermissionRequest{
		SessionID:     string(p.SessionId),
		ToolCallID:    string(p.ToolCall.ToolCallId),
		Title:         title,
		ActionType:    actionType,
		ActionDetails: actionDetails,
		Options:       options,
	}

	c.logger.Info("forwarding permission request to handler",
		zap.String("session_id", req.SessionID),
		zap.String("tool_call_id", req.ToolCallID))

	resp, err := handler(ctx, req)
	if err != nil {
		c.logger.Error("permission handler failed", zap.Error(err))
		// On error, cancel the permission request
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}

	if resp.Cancelled {
		c.logger.Info("permission request cancelled by user")
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}

	c.logger.Info("permission request approved by user",
		zap.String("option_id", resp.OptionID))
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{
				OptionId: acp.PermissionOptionId(resp.OptionID),
			},
		},
	}, nil
}

// autoApprovePermission auto-approves by selecting the first "allow" option
func (c *Client) autoApprovePermission(p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	// Find the first "allow" option
	var selectedOption *acp.PermissionOption
	for i := range p.Options {
		opt := &p.Options[i]
		if opt.Kind == acp.PermissionOptionKindAllowOnce || opt.Kind == acp.PermissionOptionKindAllowAlways {
			selectedOption = opt
			break
		}
	}

	// If no allow option, use the first option
	if selectedOption == nil {
		selectedOption = &p.Options[0]
	}

	c.logger.Info("auto-approving permission request",
		zap.String("option_id", string(selectedOption.OptionId)),
		zap.String("option_name", selectedOption.Name),
		zap.String("kind", string(selectedOption.Kind)))

	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{
				OptionId: selectedOption.OptionId,
			},
		},
	}, nil
}

// SessionUpdate handles session update notifications from the agent
func (c *Client) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	c.mu.RLock()
	handler := c.updateHandler
	c.mu.RUnlock()

	// Log the update type
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		if u.AgentMessageChunk.Content.Text != nil {
			c.logger.Debug("agent message chunk",
				zap.String("text", u.AgentMessageChunk.Content.Text.Text[:min(50, len(u.AgentMessageChunk.Content.Text.Text))]))
		}
	case u.ToolCall != nil:
		c.logger.Info("tool call",
			zap.String("tool_call_id", string(u.ToolCall.ToolCallId)),
			zap.String("title", u.ToolCall.Title),
			zap.String("status", string(u.ToolCall.Status)))
	case u.ToolCallUpdate != nil:
		c.logger.Debug("tool call update",
			zap.String("tool_call_id", string(u.ToolCallUpdate.ToolCallId)))
	case u.Plan != nil:
		c.logger.Info("plan update", zap.Int("entries", len(u.Plan.Entries)))
	}

	// Forward to handler if set
	if handler != nil {
		handler(n)
	}

	return nil
}

// ReadTextFile reads a text file from the workspace
func (c *Client) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	c.logger.Debug("reading file", zap.String("path", p.Path))

	// Validate path is absolute
	if !filepath.IsAbs(p.Path) {
		return acp.ReadTextFileResponse{}, fmt.Errorf("path must be absolute: %s", p.Path)
	}

	// Read the file
	b, err := os.ReadFile(p.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}

	content := string(b)

	// Handle line/limit parameters
	if p.Line != nil || p.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if p.Line != nil && *p.Line > 0 {
			start = *p.Line - 1
			if start > len(lines) {
				start = len(lines)
			}
		}
		end := len(lines)
		if p.Limit != nil && *p.Limit > 0 && start+*p.Limit < end {
			end = start + *p.Limit
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return acp.ReadTextFileResponse{Content: content}, nil
}

// WriteTextFile writes a text file to the workspace
func (c *Client) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	c.logger.Debug("writing file", zap.String("path", p.Path))

	// Validate path is absolute
	if !filepath.IsAbs(p.Path) {
		return acp.WriteTextFileResponse{}, fmt.Errorf("path must be absolute: %s", p.Path)
	}

	// Create directory if needed
	if dir := filepath.Dir(p.Path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return acp.WriteTextFileResponse{}, err
		}
	}

	return acp.WriteTextFileResponse{}, os.WriteFile(p.Path, []byte(p.Content), 0o644)
}

// CreateTerminal creates a new terminal
func (c *Client) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	c.logger.Debug("create terminal request", zap.String("command", p.Command))
	// For now, return a dummy terminal ID - we can implement real terminal support later
	return acp.CreateTerminalResponse{TerminalId: "t-1"}, nil
}

// KillTerminalCommand kills a terminal command
func (c *Client) KillTerminalCommand(ctx context.Context, p acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	c.logger.Debug("kill terminal request", zap.String("terminal_id", p.TerminalId))
	return acp.KillTerminalCommandResponse{}, nil
}

// TerminalOutput gets terminal output
func (c *Client) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	c.logger.Debug("terminal output request", zap.String("terminal_id", p.TerminalId))
	return acp.TerminalOutputResponse{Output: "ok", Truncated: false}, nil
}

// ReleaseTerminal releases a terminal
func (c *Client) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	c.logger.Debug("release terminal request", zap.String("terminal_id", p.TerminalId))
	return acp.ReleaseTerminalResponse{}, nil
}

// WaitForTerminalExit waits for terminal to exit
func (c *Client) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	c.logger.Debug("wait for terminal exit request", zap.String("terminal_id", p.TerminalId))
	exitCode := 0
	return acp.WaitForTerminalExitResponse{ExitCode: &exitCode}, nil
}

// Verify interface implementation
var _ acp.Client = (*Client)(nil)

