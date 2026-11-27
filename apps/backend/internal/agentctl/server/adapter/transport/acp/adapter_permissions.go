package acp

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// handlePermissionRequest handles permission requests from the agent.
// Since both acpclient and adapter now use the shared types package,
// no conversion is needed - we just forward to the handler.
func (a *Adapter) handlePermissionRequest(ctx context.Context, req *PermissionRequest) (response *PermissionResponse, err error) {
	a.mu.RLock()
	handler := a.permissionHandler
	fallbackSessionID := a.sessionID
	a.mu.RUnlock()
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fallbackSessionID
	}
	turn := a.currentPromptTurn()
	var alreadyTracked bool
	var observedName *string
	var identityAmbiguous bool
	if a.usesAntigravityCLI() {
		// The identity waiter uses the same session and call key that the
		// notification publishes under activeToolCalls' lock.
		alreadyTracked, observedName, identityAmbiguous = a.waitForAntigravityPermissionToolCall(
			ctx, sessionID, req.ToolCallID, syntheticToolCallRaceWindow,
		)
	} else {
		alreadyTracked = a.waitForActiveToolCall(ctx, req.ToolCallID, syntheticToolCallRaceWindow)
	}
	if a.usesAntigravityCLI() {
		switch {
		case req.ToolName == nil:
			req.ToolName = observedName
		case observedName != nil && *observedName != *req.ToolName:
			identityAmbiguous = true
		}
		if identityAmbiguous {
			emptyName := ""
			req.ToolName = &emptyName
		} else {
			req.ToolName = a.dialect.normalizePermissionToolName(req.ToolName, req.ToolMeta, req.Title, req.ActionType)
		}
	} else {
		req.ToolName = a.dialect.normalizePermissionToolName(req.ToolName, req.ToolMeta, req.Title, req.ActionType)
	}

	// Prefer session ID from the request; fall back to adapter-level session ID
	if finish := a.beginContinuationPermission(turn, sessionID, req.ToolCallID, req.Options); finish != nil {
		defer func() { finish(response, err) }()
	}
	var capacityPermissionTracked bool
	if sessionID == fallbackSessionID {
		capacityPermissionTracked = true
	}

	// Only emit a synthetic tool_call event if no ToolCall notification preceded this.
	// waitForActiveToolCall bounds the race window between a SessionUpdate.ToolCall
	// notification and a same-id request_permission dispatched on separate goroutines.
	if capacityPermissionTracked {
		a.capacityPermissionStarted(turn, req.ToolCallID)
		defer a.capacityPermissionFinished(turn)
	}

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
		// A missing handler is a Kandev wiring failure, not a permission
		// decision. Selecting req.Options[0] would approve or refuse depending
		// on the provider's option order; cancel loudly instead.
		a.logger.Warn("no permission handler installed, cancelling request",
			zap.String("tool_call_id", req.ToolCallID),
			zap.Int("option_count", len(req.Options)))
		return &PermissionResponse{Cancelled: true}, nil
	}

	// Forward directly to handler - types are already compatible
	return handler(ctx, req)
}
