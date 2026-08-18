package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
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
	invocation, contextErrorResponse, err := h.resolvePluginToolInvocationContext(ctx, msg, req)
	if err != nil || contextErrorResponse != nil {
		return contextErrorResponse, err
	}
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, req.Arguments, invocation)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("plugin tool invocation failed: %v", err), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"text": result.Text, "structured_content": result.StructuredContent, "is_error": result.IsError,
	})
}

func (h *Handlers) resolvePluginToolInvocationContext(ctx context.Context, msg *ws.Message, req pluginToolInvocationRequest) (plugins.AgentToolInvocationContext, *ws.Message, error) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok || h.sessionRepo == nil || h.taskSvc == nil {
		return pluginToolContextError(msg, ws.ErrorCodeInternalError, "plugin tool execution context is unavailable")
	}
	taskID, sessionID := execution.TaskID, execution.SessionID
	if pluginToolRequestContextMismatch(req, execution) {
		return pluginToolContextError(msg, ws.ErrorCodeBadRequest, "request context does not match the running execution")
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return pluginToolContextError(msg, ws.ErrorCodeBadRequest, "running session is not bound to task")
	}
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return pluginToolContextError(msg, ws.ErrorCodeInternalError, "failed to resolve the execution workspace")
	}
	if req.WorkspaceID != "" && req.WorkspaceID != task.WorkspaceID {
		return pluginToolContextError(msg, ws.ErrorCodeBadRequest, "request context does not match the running execution")
	}
	return plugins.AgentToolInvocationContext{
		InvocationID: req.InvocationID, TaskID: taskID, SessionID: sessionID,
		WorkspaceID: task.WorkspaceID, Surface: req.Surface,
	}, nil, nil
}

func pluginToolRequestContextMismatch(req pluginToolInvocationRequest, execution streams.MCPExecutionContext) bool {
	return (req.TaskID != "" && req.TaskID != execution.TaskID) ||
		(req.SessionID != "" && req.SessionID != execution.SessionID)
}

func pluginToolContextError(msg *ws.Message, code, message string) (plugins.AgentToolInvocationContext, *ws.Message, error) {
	response, err := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return plugins.AgentToolInvocationContext{}, response, err
}
