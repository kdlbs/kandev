package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type guardedTTYExecPayload struct {
	TaskID    string   `json:"task_id"`
	SessionID string   `json:"session_id"`
	Argv      []string `json:"argv"`
}

func (h *Handlers) handleGuardedTTYExec(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload guardedTTYExecPayload
	decoder := json.NewDecoder(bytes.NewReader(msg.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid guarded TTY request", nil)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid guarded TTY request", nil)
	}
	execution, hasExecution := streams.MCPExecutionContextFromContext(ctx)
	principal, hasPrincipal := mcpscope.PrincipalFromContext(ctx)
	if !hasExecution || !hasPrincipal || principal.Surface != mcpprofile.SurfaceKanbanTask ||
		principal.WorkspaceID == "" ||
		payload.TaskID != execution.TaskID || payload.SessionID != execution.SessionID ||
		principal.CallerTaskID != execution.TaskID || principal.CallerSessionID != execution.SessionID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Guarded TTY execution is not bound to this task session", nil)
	}
	if !streams.ValidateGuardedTTYArgv(payload.Argv) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "argv exceeds guarded TTY limits", nil)
	}
	if h.guardedTTYService == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Guarded TTY execution is not available", nil)
	}
	receipt, err := h.guardedTTYService.ExecuteGuardedTTY(ctx, streams.GuardedTTYExecRequest{
		Execution: execution,
		Argv:      append([]string(nil), payload.Argv...),
	})
	if err != nil || receipt == nil {
		h.logger.Error("guarded TTY execution failed", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Guarded TTY execution failed", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, receipt)
}
