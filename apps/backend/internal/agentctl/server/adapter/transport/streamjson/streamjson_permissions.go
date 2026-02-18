package streamjson

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// handleControlRequest processes control requests (permission requests) from the agent.
func (a *Adapter) handleControlRequest(requestID string, req *claudecode.ControlRequest) {
	a.logger.Info("received control request",
		zap.String("request_id", requestID),
		zap.String("subtype", req.Subtype),
		zap.String("tool_name", req.ToolName))

	switch req.Subtype {
	case claudecode.SubtypeCanUseTool:
		a.handleToolPermission(requestID, req)
	case claudecode.SubtypeHookCallback:
		a.handleHookCallback(requestID, req)
	default:
		a.logger.Warn("unhandled control request subtype",
			zap.String("subtype", req.Subtype))
		// Send error response
		if err := a.client.SendControlResponse(&claudecode.ControlResponseMessage{
			Type:      claudecode.MessageTypeControlResponse,
			RequestID: requestID,
			Response: &claudecode.ControlResponse{
				Subtype: "error",
				Error:   fmt.Sprintf("unhandled subtype: %s", req.Subtype),
			},
		}); err != nil {
			a.logger.Warn("failed to send error response", zap.Error(err))
		}
	}
}

// handleToolPermission processes can_use_tool permission requests.
func (a *Adapter) handleToolPermission(requestID string, req *claudecode.ControlRequest) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
	a.mu.RUnlock()

	// Determine action type based on tool name
	actionType := types.ActionTypeOther
	switch req.ToolName {
	case claudecode.ToolBash:
		actionType = types.ActionTypeCommand
	case claudecode.ToolWrite, claudecode.ToolEdit, claudecode.ToolNotebookEdit:
		actionType = types.ActionTypeFileWrite
	case claudecode.ToolRead, claudecode.ToolGlob, claudecode.ToolGrep:
		actionType = types.ActionTypeFileRead
	case claudecode.ToolWebFetch, claudecode.ToolWebSearch:
		actionType = types.ActionTypeNetwork
	}

	// Build title from tool name and key input
	title := req.ToolName
	if cmd, ok := req.Input["command"].(string); ok && req.ToolName == claudecode.ToolBash {
		title = cmd
	} else if path, ok := req.Input["file_path"].(string); ok {
		title = fmt.Sprintf("%s: %s", req.ToolName, path)
	}

	// Build permission options
	options := []PermissionOption{
		{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		{OptionID: "allowAlways", Name: "Allow Always", Kind: "allow_always"},
		{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
	}

	// Build permission request with Claude Code's requestID.
	// The handler (process manager's handlePermissionRequest) will:
	// 1. Send the permission_request notification to the frontend
	// 2. Block waiting for user response
	// 3. Return the response
	// We pass PendingID so the handler uses Claude Code's requestID
	// instead of generating a new one - this ensures the frontend and backend
	// use the same ID for response lookup.
	permReq := &PermissionRequest{
		SessionID:     sessionID,
		ToolCallID:    req.ToolUseID,
		Title:         title,
		Options:       options,
		ActionType:    actionType,
		ActionDetails: req.Input,
		PendingID:     requestID, // Use Claude Code's requestID so response lookup works
	}

	// If no handler, auto-allow
	if handler == nil {
		a.logger.Debug("auto-allowing tool (no handler)",
			zap.String("tool", req.ToolName))
		a.sendPermissionResponse(requestID, claudecode.BehaviorAllow)
		return
	}

	// Call permission handler (blocking) - it will send the notification
	ctx := context.Background()
	resp, err := handler(ctx, permReq)
	if err != nil {
		a.logger.Error("permission handler error", zap.Error(err))
		a.sendPermissionResponse(requestID, claudecode.BehaviorDeny)
		return
	}

	// Map response to behavior
	behavior := claudecode.BehaviorAllow
	if resp.Cancelled {
		behavior = claudecode.BehaviorDeny
	} else {
		switch resp.OptionID {
		case "allow", "allowAlways", "approve", "approveAlways":
			behavior = claudecode.BehaviorAllow
		case "deny", "reject", "decline":
			behavior = claudecode.BehaviorDeny
		}
	}

	a.sendPermissionResponse(requestID, behavior)
}

// sendPermissionResponse sends a permission response to the agent.
func (a *Adapter) sendPermissionResponse(requestID string, behavior string) {
	resp := &claudecode.ControlResponseMessage{
		Type:      claudecode.MessageTypeControlResponse,
		RequestID: requestID,
		Response: &claudecode.ControlResponse{
			Subtype: "success",
			Result: &claudecode.PermissionResult{
				Behavior: behavior,
			},
		},
	}

	if err := a.client.SendControlResponse(resp); err != nil {
		a.logger.Warn("failed to send permission response", zap.Error(err))
	}
}

// handleHookCallback processes hook callback requests.
func (a *Adapter) handleHookCallback(requestID string, req *claudecode.ControlRequest) {
	a.logger.Info("received hook callback",
		zap.String("request_id", requestID),
		zap.String("hook_name", req.HookName))

	// For now, acknowledge hook callbacks with success
	if err := a.client.SendControlResponse(&claudecode.ControlResponseMessage{
		Type:      claudecode.MessageTypeControlResponse,
		RequestID: requestID,
		Response: &claudecode.ControlResponse{
			Subtype: "success",
		},
	}); err != nil {
		a.logger.Warn("failed to send hook callback response", zap.Error(err))
	}
}

