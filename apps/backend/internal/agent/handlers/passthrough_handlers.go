// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// PassthroughHandlers provides WebSocket handlers for agent CLI passthrough operations.
// These handlers route stdin to the agent process in passthrough mode.
type PassthroughHandlers struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
}

// NewPassthroughHandlers creates a new PassthroughHandlers instance
func NewPassthroughHandlers(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *PassthroughHandlers {
	return &PassthroughHandlers{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "passthrough_handlers")),
	}
}

// RegisterHandlers registers passthrough handlers with the WebSocket dispatcher
func (h *PassthroughHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionAgentStdin, h.wsAgentStdin)
	d.RegisterFunc(ws.ActionAgentResize, h.wsAgentResize)
}

// AgentStdinRequest for agent.stdin action
type AgentStdinRequest struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`
}

// wsAgentStdin sends input to the agent process stdin (passthrough mode)
func (h *PassthroughHandlers) wsAgentStdin(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req AgentStdinRequest
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

	// Check if this session is running in passthrough mode
	passthroughProcessID := execution.PassthroughProcessID
	if passthroughProcessID == "" {
		return nil, fmt.Errorf("session %s is not in passthrough mode", req.SessionID)
	}

	// Write to the interactive runner's stdin
	if err := h.lifecycleMgr.WritePassthroughStdin(ctx, req.SessionID, req.Data); err != nil {
		h.logger.Error("failed to write to passthrough stdin",
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to send agent input: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}

// AgentResizeRequest for agent.resize action
type AgentResizeRequest struct {
	SessionID string `json:"session_id"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

// wsAgentResize resizes the PTY for an agent in passthrough mode
func (h *PassthroughHandlers) wsAgentResize(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req AgentResizeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	if req.Cols == 0 || req.Rows == 0 {
		return nil, fmt.Errorf("cols and rows must be non-zero")
	}

	// Get the agent execution for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
	}

	// Check if this session is running in passthrough mode
	if execution.PassthroughProcessID == "" {
		return nil, fmt.Errorf("session %s is not in passthrough mode", req.SessionID)
	}

	// Resize the PTY
	if err := h.lifecycleMgr.ResizePassthroughPTY(ctx, req.SessionID, req.Cols, req.Rows); err != nil {
		h.logger.Error("failed to resize passthrough PTY",
			zap.String("session_id", req.SessionID),
			zap.Uint16("cols", req.Cols),
			zap.Uint16("rows", req.Rows),
			zap.Error(err))
		return nil, fmt.Errorf("failed to resize terminal: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}
