// Package handlers provides WebSocket message handlers for the agent module.
package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/agent/controller"
	"github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Handlers contains WebSocket handlers for the agent API
type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

// NewHandlers creates a new WebSocket handlers instance
func NewHandlers(ctrl *controller.Controller, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "agent-handlers")),
	}
}

// RegisterHandlers registers all agent handlers with the dispatcher
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionAgentList, h.wsListAgents)
	d.RegisterFunc(ws.ActionAgentLaunch, h.wsLaunchAgent)
	d.RegisterFunc(ws.ActionAgentStatus, h.wsGetAgentStatus)
	d.RegisterFunc(ws.ActionAgentLogs, h.wsGetAgentLogs)
	d.RegisterFunc(ws.ActionAgentStop, h.wsStopAgent)
	d.RegisterFunc(ws.ActionAgentTypes, h.wsListAgentTypes)
}

// WS handlers

func (h *Handlers) wsListAgents(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListAgents(ctx, dto.ListAgentsRequest{})
	if err != nil {
		h.logger.Error("failed to list agents", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsLaunchAgentRequest struct {
	TaskID         string            `json:"task_id"`
	AgentProfileID string            `json:"agent_profile_id"`
	WorkspacePath  string            `json:"workspace_path"`
	Env            map[string]string `json:"env,omitempty"`
}

func (h *Handlers) wsLaunchAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsLaunchAgentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.AgentProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_profile_id is required", nil)
	}
	if req.WorkspacePath == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_path is required", nil)
	}

	resp, err := h.controller.LaunchAgent(ctx, dto.LaunchAgentRequest{
		TaskID:         req.TaskID,
		AgentProfileID: req.AgentProfileID,
		WorkspacePath:  req.WorkspacePath,
		Env:            req.Env,
	})
	if err != nil {
		h.logger.Error("failed to launch agent",
			zap.String("task_id", req.TaskID),
			zap.String("agent_profile_id", req.AgentProfileID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to launch agent: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsAgentIDRequest struct {
	AgentID string `json:"agent_id"`
}

func (h *Handlers) wsGetAgentStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAgentIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	resp, err := h.controller.GetAgentStatus(ctx, dto.GetAgentStatusRequest{AgentID: req.AgentID})
	if err != nil {
		if err == controller.ErrAgentNotFound {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Agent not found", nil)
		}
		h.logger.Error("failed to get agent status", zap.String("agent_id", req.AgentID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsGetAgentLogs(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAgentIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	resp, err := h.controller.GetAgentLogs(ctx, dto.GetAgentLogsRequest{AgentID: req.AgentID})
	if err != nil {
		if err == controller.ErrAgentNotFound {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Agent not found", nil)
		}
		h.logger.Error("failed to get agent logs", zap.String("agent_id", req.AgentID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsStopAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAgentIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	resp, err := h.controller.StopAgent(ctx, dto.StopAgentRequest{AgentID: req.AgentID})
	if err != nil {
		h.logger.Error("failed to stop agent", zap.String("agent_id", req.AgentID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop agent: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsListAgentTypes(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListAgentTypes(ctx, dto.ListAgentTypesRequest{})
	if err != nil {
		h.logger.Error("failed to list agent types", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

