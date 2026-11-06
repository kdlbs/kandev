package handlers

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Reports use the service's trusted caller boundary, never sender payload fields.
// This path records status only and does not resolve, recover, or prompt a runtime.
func (h *Handlers) handleWorkspaceCollaborationReport(ctx context.Context, msg *ws.Message, taskID, sessionID, content string) (*ws.Message, error) {
	if sessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required for a report", nil)
	}
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is unavailable", nil)
	}
	report, err := h.taskSvc.CreateWorkspaceCollaborationReport(ctx, taskID, sessionID, content)
	if err != nil {
		code := ws.ErrorCodeInternalError
		switch {
		case service.IsForbidden(err):
			code = ws.ErrorCodeForbidden
		case errors.Is(err, service.ErrWorkspaceCollaborationDisabled), errors.Is(err, repoerrors.ErrTaskNotFound), errors.Is(err, models.ErrTaskSessionNotFound):
			code = ws.ErrorCodeNotFound
		case errors.Is(err, service.ErrWorkspaceCollaborationReportInvalid):
			code = ws.ErrorCodeValidation
		}
		return ws.NewError(msg.ID, msg.Action, code, "workspace collaboration report failed", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"status": "reported", "task_id": taskID, "session_id": sessionID, "message_id": report.ID})
}
