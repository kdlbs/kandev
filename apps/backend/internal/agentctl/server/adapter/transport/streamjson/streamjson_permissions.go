package streamjson

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// handleControlRequest processes control requests (permission requests) from the agent.
func (a *Adapter) handleControlRequest(requestID string, req *claudecode.ControlRequest) {
	a.logger.Info("received control request",
		zap.String("request_id", requestID),
		zap.String("subtype", req.Subtype),
		zap.String("tool_name", req.ToolName))

	// Trace incoming control request (bypasses handleMessage path)
	a.traceIncomingControl("control_request."+req.Subtype, map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request":    req,
	})

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
	// Build action details, including blocked paths if present
	actionDetails := req.Input
	if req.BlockedPaths != "" {
		if actionDetails == nil {
			actionDetails = make(map[string]any)
		}
		actionDetails["blocked_paths"] = req.BlockedPaths
	}

	permReq := &PermissionRequest{
		SessionID:     sessionID,
		ToolCallID:    req.ToolUseID,
		Title:         title,
		Options:       options,
		ActionType:    actionType,
		ActionDetails: actionDetails,
		PendingID:     requestID, // Use Claude Code's requestID so response lookup works
	}

	// If no handler, auto-allow
	if handler == nil {
		a.logger.Debug("auto-allowing tool (no handler)",
			zap.String("tool", req.ToolName))
		a.sendPermissionResponse(requestID, claudecode.BehaviorAllow)
		return
	}

	// Call permission handler (blocking) with timeout to prevent indefinite hangs
	timeout := a.cfg.GetPermissionTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := handler(ctx, permReq)
	if err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			a.logger.Warn("permission request timed out, auto-denying with interrupt",
				zap.String("request_id", requestID),
				zap.String("tool", req.ToolName),
				zap.Duration("timeout", timeout))

			// Emit cancellation event so frontend closes the dialog
			a.sendUpdate(AgentEvent{
				Type:      streams.EventTypePermissionCancelled,
				PendingID: requestID,
			})

			// Deny with interrupt to stop the current operation
			interruptFlag := true
			a.sendControlResult(requestID, &claudecode.PermissionResult{
				Behavior:  claudecode.BehaviorDeny,
				Interrupt: &interruptFlag,
				Message:   "Permission request timed out",
			}, "permission_response.timeout")
			return
		}

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

// sendControlResult sends a success control response with the given result payload and traces it.
// result can be a *PermissionResult, map[string]any (for hooks), or any other JSON-serializable value.
func (a *Adapter) sendControlResult(requestID string, result any, traceEvent string) {
	resp := &claudecode.ControlResponseMessage{
		Type:      claudecode.MessageTypeControlResponse,
		RequestID: requestID,
		Response: &claudecode.ControlResponse{
			Subtype: "success",
			Result:  result,
		},
	}
	if err := a.client.SendControlResponse(resp); err != nil {
		a.logger.Warn("failed to send control response",
			zap.Error(err), zap.String("event", traceEvent))
	}
	a.traceOutgoingControl(traceEvent, resp)
}

// sendPermissionResponse sends a permission response to the agent.
func (a *Adapter) sendPermissionResponse(requestID string, behavior string) {
	a.sendControlResult(requestID, &claudecode.PermissionResult{
		Behavior: behavior,
	}, "permission_response")
}

// handleControlCancel processes control_cancel_request messages.
// This cancels a pending permission request, closing the dialog in the UI.
func (a *Adapter) handleControlCancel(requestID string) {
	a.logger.Info("received control_cancel_request",
		zap.String("request_id", requestID))

	a.traceIncomingControl("control_cancel_request", map[string]any{
		"type":       "control_cancel_request",
		"request_id": requestID,
	})

	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypePermissionCancelled,
		PendingID: requestID,
	})
}

// handleHookCallback processes hook callback requests, dispatching based on callback ID.
// Hook responses use different payload formats than can_use_tool responses:
// - PreToolUse hooks: hookSpecificOutput with permissionDecision
// - Stop hooks: decision + optional reason
func (a *Adapter) handleHookCallback(requestID string, req *claudecode.ControlRequest) {
	a.logger.Info("received hook callback",
		zap.String("request_id", requestID),
		zap.String("callback_id", req.CallbackID),
		zap.String("hook_name", req.HookName))

	switch req.CallbackID {
	case "tool_approval":
		// Respond with "ask" to trigger a regular can_use_tool permission request.
		// Claude Code will then send a separate can_use_tool control_request.
		a.sendPreToolUseHookResponse(requestID, "ask", "Requires user approval")

	case "auto_approve":
		// Auto-approve via hookSpecificOutput
		a.sendPreToolUseHookResponse(requestID, "allow", "Auto-approved by SDK")

	case "stop_git_check":
		// Check for uncommitted changes before stopping
		a.handleStopGitCheck(requestID, req)

	default:
		a.logger.Warn("unknown hook callback ID, auto-approving",
			zap.String("callback_id", req.CallbackID))
		a.sendPreToolUseHookResponse(requestID, "allow", "Auto-approved (unknown callback)")
	}
}

// sendPreToolUseHookResponse sends a PreToolUse hook callback response.
// permissionDecision can be "allow", "deny", or "ask" (triggers can_use_tool flow).
func (a *Adapter) sendPreToolUseHookResponse(requestID, permissionDecision, reason string) {
	a.sendControlResult(requestID, map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       permissionDecision,
			"permissionDecisionReason": reason,
		},
	}, "hook_response.pre_tool_use")
}

// handleStopGitCheck processes the stop_git_check hook callback.
// Checks if the stop hook is active and approves/blocks accordingly.
func (a *Adapter) handleStopGitCheck(requestID string, req *claudecode.ControlRequest) {
	// If stop_hook_active is set, just approve (hook is being deactivated)
	if active, ok := req.HookInput["stop_hook_active"].(bool); ok && active {
		a.sendStopHookResponse(requestID, "approve", "")
		return
	}

	// For now, approve the stop — the workspace tracker integration
	// can be added later to check for uncommitted changes
	a.sendStopHookResponse(requestID, "approve", "")
}

// sendStopHookResponse sends a Stop hook callback response with decision format.
func (a *Adapter) sendStopHookResponse(requestID, decision, reason string) {
	result := map[string]any{
		"decision": decision,
	}
	if reason != "" {
		result["reason"] = reason
	}
	a.sendControlResult(requestID, result, "hook_response.stop")
}
