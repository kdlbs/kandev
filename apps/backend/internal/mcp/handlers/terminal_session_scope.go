package handlers

import (
	"context"
	"encoding/json"
	"strings"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type scopedTerminalSessionRequest struct {
	TaskID          string `json:"task_id"`
	CallerSessionID string `json:"caller_session_id"`
	TargetSessionID string `json:"target_session_id"`
}

func parseScopedTerminalSessionRequest(
	ctx context.Context, payload []byte,
) (scopedTerminalSessionRequest, string, error) {
	var req scopedTerminalSessionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, "", err
	}
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.CallerSessionID = strings.TrimSpace(req.CallerSessionID)
	req.TargetSessionID = strings.TrimSpace(req.TargetSessionID)
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.WorkspaceID == "" || req.TaskID == "" || req.CallerSessionID == "" || req.TargetSessionID == "" ||
		principal.CallerTaskID != req.TaskID || principal.CallerSessionID != req.CallerSessionID ||
		req.TargetSessionID == req.CallerSessionID {
		return req, "", nil
	}
	return req, principal.WorkspaceID, nil
}

func terminalSessionScopeError(msg *ws.Message, parseErr error, workspaceID, action string) (*ws.Message, error) {
	if parseErr != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+parseErr.Error(), nil)
	}
	if workspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, action+" requires the calling task's current primary session", nil)
	}
	return nil, nil
}
