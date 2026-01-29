// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ShellHandlers provides WebSocket handlers for shell terminal operations.
// Shell output is streamed via the lifecycle manager and event bus.
// This handler provides shell.status, shell.subscribe (for buffer), and shell.input.
type ShellHandlers struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
}

// NewShellHandlers creates a new ShellHandlers instance
func NewShellHandlers(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *ShellHandlers {
	return &ShellHandlers{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "shell_handlers")),
	}
}

// RegisterHandlers registers shell handlers with the WebSocket dispatcher
func (h *ShellHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionShellStatus, h.wsShellStatus)
	d.RegisterFunc(ws.ActionShellSubscribe, h.wsShellSubscribe)
	d.RegisterFunc(ws.ActionShellInput, h.wsShellInput)
}

// ShellStatusRequest for shell.status action
type ShellStatusRequest struct {
	SessionID string `json:"session_id"`
}

// ShellInputRequest for shell.input action
type ShellInputRequest struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`
}

// wsShellStatus returns the status of a shell session for a session
func (h *ShellHandlers) wsShellStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get the agent execution for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "no agent running for this session",
		})
	}

	// Get shell status from agentctl
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "agent client not available",
		})
	}

	status, err := client.ShellStatus(ctx)
	if err != nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     err.Error(),
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"available":  true,
		"running":    status.Running,
		"pid":        status.Pid,
		"shell":      status.Shell,
		"cwd":        status.Cwd,
		"started_at": status.StartedAt,
	})
}

// ShellSubscribeRequest for shell.subscribe action
type ShellSubscribeRequest struct {
	SessionID string `json:"session_id"`
}

// wsShellSubscribe subscribes to shell output for a session.
// Shell output is streamed via the event bus (lifecycle manager handles this).
// This endpoint returns the buffered shell output for catchup.
func (h *ShellHandlers) wsShellSubscribe(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Verify the agent execution exists for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
	}

	// Get buffered output to include in response
	// This ensures client gets current shell state without duplicate broadcasts
	// Shell output streaming is handled by the lifecycle manager via event bus
	buffer := ""
	if client := execution.GetAgentCtlClient(); client != nil {
		if b, err := client.ShellBuffer(ctx); err == nil {
			buffer = b
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
		"buffer":     buffer,
	})
}

// wsShellInput sends input to a shell session
func (h *ShellHandlers) wsShellInput(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellInputRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get the agent execution for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
	}

	// Wait for the workspace stream to be ready with a timeout.
	// This handles the race condition where client sends shell.input before
	// the workspace stream is fully connected (e.g., joining a task too fast).
	var workspaceStream = execution.GetWorkspaceStream()
	if workspaceStream == nil {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			workspaceStream = execution.GetWorkspaceStream()
			if workspaceStream != nil {
				break
			}
		}
	}

	if workspaceStream == nil {
		return nil, fmt.Errorf("workspace stream not ready for session %s", req.SessionID)
	}

	if err := workspaceStream.WriteShellInput(req.Data); err != nil {
		return nil, fmt.Errorf("failed to send shell input: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}
