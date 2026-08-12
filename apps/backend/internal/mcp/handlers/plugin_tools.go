package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/plugins"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type pluginToolsListRequest struct {
	Surface string `json:"surface"`
}

type pluginToolInvocationRequest struct {
	PluginID     string         `json:"plugin_id"`
	LocalName    string         `json:"local_name"`
	InvocationID string         `json:"invocation_id"`
	TaskID       string         `json:"task_id"`
	SessionID    string         `json:"session_id"`
	WorkspaceID  string         `json:"workspace_id"`
	Surface      string         `json:"surface"`
	Arguments    map[string]any `json:"arguments"`
}

func (h *Handlers) handleListPluginTools(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	var req pluginToolsListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}
	snapshot, err := h.pluginSvc.AgentToolCatalog()
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	if req.Surface != "" {
		filtered := snapshot
		filtered.Tools = filtered.Tools[:0]
		for _, tool := range snapshot.Tools {
			for _, surface := range tool.Surfaces {
				if surface == req.Surface {
					filtered.Tools = append(filtered.Tools, tool)
					break
				}
			}
		}
		snapshot = filtered
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) handleInvokePluginTool(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req pluginToolInvocationRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}
	if req.PluginID == "" || req.LocalName == "" || req.Surface == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "plugin_id, local_name, and surface are required", nil)
	}
	if req.TaskID == "" || req.SessionID == "" || h.sessionRepo == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "task_id and session_id are required", nil)
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil || session.TaskID != req.TaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session is not bound to task", nil)
	}
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, req.Arguments, plugins.AgentToolInvocationContext{
		InvocationID: req.InvocationID, TaskID: req.TaskID, SessionID: req.SessionID,
		WorkspaceID: req.WorkspaceID, Surface: req.Surface,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("plugin tool invocation failed: %v", err), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"text": result.Text, "structured_content": result.StructuredContent, "is_error": result.IsError,
	})
}
