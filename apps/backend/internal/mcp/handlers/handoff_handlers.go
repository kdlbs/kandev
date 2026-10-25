package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"go.uber.org/zap"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// SetHandoffService wires the cross-task handoff service used to back the
// list_related_tasks_kandev / *_task_document_kandev MCP tools. Optional —
// when nil, the registered handlers report a configuration error instead
// of registering and returning broken endpoints.
func (h *Handlers) SetHandoffService(svc *service.HandoffService) {
	h.handoffSvc = svc
	if h.taskSvc != nil {
		if svc == nil {
			h.taskSvc.SetWorkspacePolicyAttacher(nil)
		} else {
			h.taskSvc.SetWorkspacePolicyAttacher(svc)
		}
	}
	if svc != nil && h.taskSvc != nil {
		svc.SetTaskAccessChecker(h.taskSvc.AuthorizeTaskAccess)
	} else if svc != nil {
		svc.SetTaskAccessChecker(nil)
	}

}

type taskVisibilityPayload struct {
	TaskID           string `json:"task_id"`
	CallerTaskID     string `json:"caller_task_id"`
	CallerSessionID  string `json:"caller_session_id"`
	RelatedReadScope string `json:"related_read_scope"`
	MCPSurface       string `json:"mcp_surface"`
}

// handleGetTaskVisibility dispatches mcp.get_task_visibility. It shares the
// related-read attestation envelope so raw callers cannot forge workspace
// authority or a caller task identity.
func (h *Handlers) handleGetTaskVisibility(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.handoffSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "handoff service not configured", nil)
	}
	var req taskVisibilityPayload
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.CallerTaskID != req.CallerTaskID ||
		principal.CallerSessionID != req.CallerSessionID ||
		string(principal.Surface) != req.MCPSurface || principal.WorkspaceID == "" ||
		!visibilityScopeAuthorized(principal, service.RelatedReadScope(req.RelatedReadScope)) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target unavailable", map[string]interface{}{"reason": "target_unavailable"})
	}
	visibility, err := h.handoffSvc.GetTaskVisibilityForRequest(ctx, service.RelatedReadRequest{
		CallerTaskID: req.CallerTaskID, CallerWorkspaceID: principal.WorkspaceID,
		TargetTaskID: req.TaskID, Scope: service.RelatedReadScope(req.RelatedReadScope),
	})
	if err != nil {
		if reason, ok := service.RelatedReadDenialReasonFor(err); ok {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target unavailable", map[string]interface{}{"reason": string(reason)})
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task visibility lookup failed", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, visibility)
}

func visibilityScopeAuthorized(principal mcpscope.Principal, readScope service.RelatedReadScope) bool {
	return readScope == service.RelatedReadScopeTaskVisibility &&
		principal.Surface == mcpprofile.SurfaceKanbanTask && principal.TaskVisibilityGrant
}

// handleListRelatedTasks dispatches mcp.list_related_tasks.
func (h *Handlers) handleListRelatedTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.handoffSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "handoff service not configured", nil)
	}
	svc := h.handoffSvc
	var req struct {
		TaskID       string `json:"task_id"`
		CallerTaskID string `json:"caller_task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	caller := req.CallerTaskID
	if caller == "" {
		caller = req.TaskID
	}
	related, err := svc.ListRelatedForCaller(ctx, caller, req.TaskID)
	if err != nil {
		// Access denied is an expected 403; log only genuine
		// infrastructure errors, then route everything through
		// mapHandoffError for the same code mapping the document handlers use.
		if !errors.Is(err, service.ErrAccessDenied) {
			h.logger.Error("list related tasks", zap.Error(err))
		}
		return mapHandoffError(msg, err)
	}
	h.enrichRelatedTasksWithPRs(ctx, related)
	return ws.NewResponse(msg.ID, msg.Action, related)
}

// handleListTaskDocuments dispatches mcp.list_task_documents.
func (h *Handlers) handleListTaskDocuments(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.handoffSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "handoff service not configured", nil)
	}
	svc := h.handoffSvc
	var req struct {
		TaskID       string `json:"task_id"`
		CallerTaskID string `json:"caller_task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	caller := req.CallerTaskID
	if caller == "" {
		caller = req.TaskID
	}
	docs, err := svc.ListDocumentsForCaller(ctx, caller, req.TaskID)
	if err != nil {
		return mapHandoffError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id":   req.TaskID,
		"documents": docs,
	})
}

// handleGetTaskDocument dispatches mcp.get_task_document.
func (h *Handlers) handleGetTaskDocument(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.handoffSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "handoff service not configured", nil)
	}
	svc := h.handoffSvc
	var req struct {
		TaskID       string `json:"task_id"`
		DocumentKey  string `json:"document_key"`
		CallerTaskID string `json:"caller_task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.DocumentKey == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and document_key are required", nil)
	}
	caller := req.CallerTaskID
	if caller == "" {
		caller = req.TaskID
	}
	doc, err := svc.GetDocumentForCaller(ctx, caller, req.TaskID, req.DocumentKey)
	if err != nil {
		return mapHandoffError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, doc)
}

// handleWriteTaskDocument dispatches mcp.write_task_document.
func (h *Handlers) handleWriteTaskDocument(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.handoffSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "handoff service not configured", nil)
	}
	svc := h.handoffSvc
	var req struct {
		TaskID       string `json:"task_id"`
		DocumentKey  string `json:"document_key"`
		Title        string `json:"title"`
		Type         string `json:"type"`
		Content      string `json:"content"`
		CallerTaskID string `json:"caller_task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.DocumentKey == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and document_key are required", nil)
	}
	caller := req.CallerTaskID
	if caller == "" {
		caller = req.TaskID
	}
	doc, err := svc.WriteDocumentForCaller(ctx, caller, req.TaskID,
		req.DocumentKey, req.Type, req.Title, req.Content, "agent", "Agent")
	if err != nil {
		return mapHandoffError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, doc)
}

func mapHandoffError(msg *ws.Message, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, service.ErrAccessDenied):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	case errors.Is(err, service.ErrDocumentNotFound):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	case errors.Is(err, service.ErrDocumentTaskRequired),
		errors.Is(err, service.ErrDocumentKeyRequired):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
}
