package streamjson

import (
	"context"
	"fmt"
	"time"

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
// This is the fallback path when hooks are not registered or the hook responds "ask".
// Note: plan content and session-mode-clear for ExitPlanMode are handled in
// processToolUseBlock (which fires first when the tool_use block is streamed).
func (a *Adapter) handleToolPermission(requestID string, req *claudecode.ControlRequest) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
	a.mu.RUnlock()

	isExitPlanMode := req.ToolName == toolExitPlanMode

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
		PendingID:     requestID,
	}

	// If no handler, auto-allow
	if handler == nil {
		a.logger.Debug("auto-allowing tool (no handler)",
			zap.String("tool", req.ToolName))
		a.sendPermissionResponse(requestID, claudecode.BehaviorAllow)
		if isExitPlanMode {
			a.scheduleExitPlanModeCompletion()
		}
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

	// ExitPlanMode: Claude Code ends the turn after approval without a result message.
	// Schedule delayed completion to unblock Prompt().
	if isExitPlanMode && behavior == claudecode.BehaviorAllow {
		a.scheduleExitPlanModeCompletion()
	}
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
func (a *Adapter) handleHookCallback(requestID string, req *claudecode.ControlRequest) {
	hookEventName, _ := req.Input["hook_event_name"].(string)
	a.logger.Info("received hook callback",
		zap.String("request_id", requestID),
		zap.String("callback_id", req.CallbackID),
		zap.String("hook_event_name", hookEventName))

	switch req.CallbackID {
	case "tool_approval":
		// Respond with "ask" to trigger a regular can_use_tool permission request.
		// Claude Code will then send a separate can_use_tool control_request.
		a.sendPreToolUseHookResponse(requestID, "ask", "Requires user approval")

	case "auto_approve":
		// Auto-approve via hookSpecificOutput
		a.sendPreToolUseHookResponse(requestID, "allow", "Auto-approved by SDK")

		// ExitPlanMode: Claude Code ends the turn after approval without a result message.
		// Capture plan content from hook input and schedule delayed completion.
		// Note: Claude Code sends hook data in the "input" field (not "hook_input").
		if toolName, ok := req.Input["tool_name"].(string); ok && toolName == toolExitPlanMode {
			a.handleExitPlanModeHookApproval()
		}

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

// handleExitPlanModeHookApproval schedules completion for ExitPlanMode.
// Called when ExitPlanMode is auto-approved via hook callback.
// Note: plan content and session-mode-clear are handled in processToolUseBlock
// (which fires first when the tool_use block is streamed).
func (a *Adapter) handleExitPlanModeHookApproval() {
	a.scheduleExitPlanModeCompletion()
}

// exitPlanModeCompletionDelay is the time to wait after ExitPlanMode approval before
// emitting a synthetic completion. This allows trailing stream events to arrive first.
const exitPlanModeCompletionDelay = 2 * time.Second

// scheduleExitPlanModeCompletion schedules a delayed completion for ExitPlanMode.
// Claude Code does not send a result message after ExitPlanMode approval — the turn
// simply ends. This goroutine waits briefly for trailing stream_events, then emits
// a complete event and signals Prompt() to unblock.
// Uses exitPlanPending atomic flag to prevent duplicate completion if a result
// message arrives before the delay expires.
func (a *Adapter) scheduleExitPlanModeCompletion() {
	a.exitPlanPending.Store(true)

	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	go func(expectedSessionID, expectedOperationID string) {
		select {
		case <-time.After(exitPlanModeCompletionDelay):
			// Timer expired, proceed with completion
		case <-a.ctx.Done():
			a.exitPlanPending.Store(false)
			return
		}

		// Guard against stale goroutine completing the wrong turn
		a.mu.RLock()
		currentSessionID := a.sessionID
		currentOperationID := a.operationID
		a.mu.RUnlock()
		if currentSessionID != expectedSessionID || currentOperationID != expectedOperationID {
			a.logger.Debug("ExitPlanMode completion skipped (stale operation)",
				zap.String("expected_session_id", expectedSessionID),
				zap.String("current_session_id", currentSessionID))
			return
		}

		if !a.exitPlanPending.CompareAndSwap(true, false) {
			a.logger.Debug("ExitPlanMode completion cancelled (result message arrived)")
			return
		}

		a.logger.Info("ExitPlanMode: emitting delayed completion",
			zap.String("session_id", sessionID),
			zap.String("operation_id", operationID))

		// Drain plan content
		planContent := a.drainExitPlanContent()

		completeData := map[string]any{"is_error": false}
		if planContent != "" {
			completeData["plan_content"] = planContent
		}

		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypeComplete,
			SessionID:   sessionID,
			OperationID: operationID,
			Data:        completeData,
		})
		a.signalResultCompletion(true, "")
	}(sessionID, operationID)
}
