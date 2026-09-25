package handlers

import (
	"context"
	"encoding/json"
	"errors"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	agentprojects "github.com/kandev/kandev/internal/projects"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// AgentProjectMCPService exposes only the project operations needed by the
// coordinator catalog. The caller identity always comes from the MCP principal.
type AgentProjectMCPService interface {
	GetCoordinatorProject(context.Context, string) (*agentprojects.ProjectView, error)
	ListWorkers(context.Context, string) ([]agentprojects.WorkerView, error)
	CreateWorker(context.Context, string, agentprojects.CreateWorkerRequest) (*agentprojects.WorkerView, error)
	GetWorker(context.Context, string, string) (*agentprojects.WorkerView, error)
	GetCurrentProjectTask(context.Context, string) (*agentprojects.WorkerView, error)
}

func (h *Handlers) registerAgentProjectHandlers(d *guardedMCPDispatcher) {
	d.RegisterFunc(ws.ActionMCPGetAgentProject, h.handleGetAgentProject)
	d.RegisterFunc(ws.ActionMCPListAgentProjectWorkers, h.handleListAgentProjectWorkers)
	d.RegisterFunc(ws.ActionMCPCreateAgentProjectWorker, h.handleCreateAgentProjectWorker)
	d.RegisterFunc(ws.ActionMCPGetAgentProjectTask, h.handleGetAgentProjectTask)
	d.RegisterFunc(ws.ActionMCPMessageAgentProjectWorker, h.handleMessageAgentProjectWorker)
	d.RegisterFunc(ws.ActionMCPStopAgentProjectWorker, h.handleStopAgentProjectWorker)
}

func (h *Handlers) handleGetAgentProject(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, response := h.requireProjectCoordinator(ctx, msg)
	if response != nil {
		return response, nil
	}
	project, err := h.agentProjectSvc.GetCoordinatorProject(ctx, principal.ProjectMainTaskID)
	if err != nil {
		return projectError(msg, err)
	}
	return projectResponse(msg, project)
}

func (h *Handlers) handleListAgentProjectWorkers(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, response := h.requireProjectCoordinator(ctx, msg)
	if response != nil {
		return response, nil
	}
	workers, err := h.agentProjectSvc.ListWorkers(ctx, principal.ProjectMainTaskID)
	if err != nil {
		return projectError(msg, err)
	}
	return projectResponse(msg, map[string]interface{}{"workers": workers})
}

func (h *Handlers) handleCreateAgentProjectWorker(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, response := h.requireProjectCoordinator(ctx, msg)
	if response != nil {
		return response, nil
	}
	var req agentprojects.CreateWorkerRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	worker, err := h.agentProjectSvc.CreateWorker(ctx, principal.ProjectMainTaskID, req)
	if err != nil {
		return projectError(msg, err)
	}
	return projectResponse(msg, worker)
}

func (h *Handlers) handleGetAgentProjectTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, response := h.requireProjectWorker(ctx, msg)
	if response != nil {
		return response, nil
	}
	worker, err := h.agentProjectSvc.GetCurrentProjectTask(ctx, principal.CallerTaskID)
	if err != nil {
		return projectError(msg, err)
	}
	return projectResponse(msg, worker)
}

func (h *Handlers) handleMessageAgentProjectWorker(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, response := h.requireProjectCoordinator(ctx, msg)
	if response != nil {
		return response, nil
	}
	var req struct {
		WorkerTaskID string `json:"worker_task_id"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkerTaskID == "" || req.Message == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "worker_task_id and message are required", nil)
	}
	if _, err := h.agentProjectSvc.GetWorker(ctx, principal.ProjectMainTaskID, req.WorkerTaskID); err != nil {
		return projectError(msg, err)
	}
	payload, _ := json.Marshal(map[string]string{
		"task_id": req.WorkerTaskID, "prompt": req.Message,
		"sender_task_id": principal.CallerTaskID, "sender_session_id": principal.CallerSessionID,
	})
	forwarded := *msg
	forwarded.Payload = payload
	return h.handleMessageTask(ctx, &forwarded)
}

func (h *Handlers) handleStopAgentProjectWorker(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, response := h.requireProjectCoordinator(ctx, msg)
	if response != nil {
		return response, nil
	}
	var req struct {
		WorkerTaskID string `json:"worker_task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkerTaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "worker_task_id is required", nil)
	}
	if _, err := h.agentProjectSvc.GetWorker(ctx, principal.ProjectMainTaskID, req.WorkerTaskID); err != nil {
		return projectError(msg, err)
	}
	if h.taskStopper == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "worker stop service is unavailable", nil)
	}
	result, err := h.taskStopper.StopTaskForCoordinator(ctx, req.WorkerTaskID)
	if err != nil {
		return projectError(msg, err)
	}
	return projectResponse(msg, result)
}

func (h *Handlers) requireProjectCoordinator(ctx context.Context, msg *ws.Message) (mcpscope.Principal, *ws.Message) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsProjectCoordinator() || h.agentProjectSvc == nil {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "project coordinator identity is required", nil)
		return mcpscope.Principal{}, response
	}
	return principal, nil
}

func (h *Handlers) requireProjectWorker(ctx context.Context, msg *ws.Message) (mcpscope.Principal, *ws.Message) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsProjectWorker() || h.agentProjectSvc == nil {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "project worker identity is required", nil)
		return mcpscope.Principal{}, response
	}
	return principal, nil
}

func projectResponse(msg *ws.Message, value interface{}) (*ws.Message, error) {
	return ws.NewResponse(msg.ID, msg.Action, value)
}

func projectError(msg *ws.Message, err error) (*ws.Message, error) {
	code := ws.ErrorCodeInternalError
	switch {
	case errors.Is(err, agentprojects.ErrWorkerCapacity):
		code = ws.ErrorCodeConflict
	case errors.Is(err, agentprojects.ErrInvalidProject), errors.Is(err, agentprojects.ErrCoordinatorRequired):
		code = ws.ErrorCodeNotFound
	case errors.Is(err, agentprojects.ErrInvalidWorkerRequest):
		code = ws.ErrorCodeValidation
	case errors.Is(err, agentprojects.ErrDisabled):
		code = ws.ErrorCodeForbidden
	}
	return ws.NewError(msg.ID, msg.Action, code, err.Error(), nil)
}
