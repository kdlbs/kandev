package acp

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// handlePermissionRequest handles permission requests from the agent.
// Since both acpclient and adapter now use the shared types package,
// no conversion is needed - we just forward to the handler.
func (a *Adapter) handlePermissionRequest(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
	a.mu.RLock()
	handler := a.permissionHandler
	fallbackSessionID := a.sessionID
	a.mu.RUnlock()

	// Prefer session ID from the request; fall back to adapter-level session ID
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fallbackSessionID
	}

	// Only emit a synthetic tool_call event if no ToolCall notification preceded this.
	// waitForActiveToolCall bounds the race window between a SessionUpdate.ToolCall
	// notification and a same-id request_permission dispatched on separate goroutines.
	alreadyTracked := a.waitForActiveToolCall(ctx, req.ToolCallID, syntheticToolCallRaceWindow)

	if !alreadyTracked {
		toolCallEvent := AgentEvent{
			Type:       streams.EventTypeToolCall,
			SessionID:  sessionID,
			ToolCallID: req.ToolCallID,
			ToolName:   req.ActionType,
			ToolTitle:  req.Title,
			ToolStatus: "pending_permission",
		}
		a.sendUpdate(toolCallEvent)
		a.logger.Debug("emitted synthetic tool_call for permission (no prior ToolCall)",
			zap.String("tool_call_id", req.ToolCallID))
	}

	if handler == nil {
		// Auto-approve if no handler
		if len(req.Options) > 0 {
			return &PermissionResponse{OptionID: req.Options[0].OptionID}, nil
		}
		return &PermissionResponse{Cancelled: true}, nil
	}

	// Forward directly to handler - types are already compatible
	return handler(ctx, req)
}
