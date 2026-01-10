// Package wshandlers provides WebSocket message handlers for the agent manager.
package wshandlers

import (
	"context"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Handlers contains WebSocket handlers for the agent API
type Handlers struct {
	lifecycle *lifecycle.Manager
	registry  *registry.Registry
	logger    *logger.Logger
}

// NewHandlers creates a new WebSocket handlers instance
func NewHandlers(lm *lifecycle.Manager, reg *registry.Registry, log *logger.Logger) *Handlers {
	return &Handlers{
		lifecycle: lm,
		registry:  reg,
		logger:    log.WithFields(zap.String("component", "agent-ws-handlers")),
	}
}

// RegisterHandlers registers all agent handlers with the dispatcher
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionAgentList, h.ListAgents)
	d.RegisterFunc(ws.ActionAgentLaunch, h.LaunchAgent)
	d.RegisterFunc(ws.ActionAgentStatus, h.GetAgentStatus)
	d.RegisterFunc(ws.ActionAgentLogs, h.GetAgentLogs)
	d.RegisterFunc(ws.ActionAgentStop, h.StopAgent)
	d.RegisterFunc(ws.ActionAgentTypes, h.ListAgentTypes)
}

// AgentResponse represents an agent instance
type AgentResponse struct {
	ID          string            `json:"id"`
	TaskID      string            `json:"task_id"`
	AgentType   string            `json:"agent_type"`
	ContainerID string            `json:"container_id,omitempty"`
	Status      string            `json:"status"`
	Progress    int               `json:"progress"`
	StartedAt   string            `json:"started_at"`
	FinishedAt  string            `json:"finished_at,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ListAgentsResponse is the response for agent.list
type ListAgentsResponse struct {
	Agents []AgentResponse `json:"agents"`
	Total  int             `json:"total"`
}

// ListAgents handles agent.list action
func (h *Handlers) ListAgents(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.lifecycle == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent manager not available", nil)
	}

	agents := h.lifecycle.ListInstances()
	resp := ListAgentsResponse{
		Agents: make([]AgentResponse, 0, len(agents)),
		Total:  len(agents),
	}

	for _, a := range agents {
		agent := AgentResponse{
			ID:          a.ID,
			TaskID:      a.TaskID,
			AgentType:   a.AgentType,
			ContainerID: a.ContainerID,
			Status:      string(a.Status),
			Progress:    a.Progress,
			StartedAt:   a.StartedAt.Format("2006-01-02T15:04:05Z"),
		}
		if a.FinishedAt != nil && !a.FinishedAt.IsZero() {
			agent.FinishedAt = a.FinishedAt.Format("2006-01-02T15:04:05Z")
		}
		if a.ExitCode != nil {
			agent.ExitCode = a.ExitCode
		}
		if a.ErrorMessage != "" {
			agent.Error = a.ErrorMessage
		}
		resp.Agents = append(resp.Agents, agent)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// LaunchAgentRequest is the payload for agent.launch
type LaunchAgentRequest struct {
	TaskID        string            `json:"task_id"`
	AgentType     string            `json:"agent_type"`
	WorkspacePath string            `json:"workspace_path"`
	Env           map[string]string `json:"env,omitempty"`
}

// LaunchAgent handles agent.launch action
func (h *Handlers) LaunchAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req LaunchAgentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.AgentType == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_type is required", nil)
	}
	if req.WorkspacePath == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_path is required", nil)
	}

	if h.lifecycle == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent manager not available", nil)
	}

	launchReq := &lifecycle.LaunchRequest{
		TaskID:        req.TaskID,
		AgentType:     req.AgentType,
		WorkspacePath: req.WorkspacePath,
		Env:           req.Env,
	}

	instance, err := h.lifecycle.Launch(ctx, launchReq)
	if err != nil {
		h.logger.Error("failed to launch agent",
			zap.String("task_id", req.TaskID),
			zap.String("agent_type", req.AgentType),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to launch agent: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":  true,
		"agent_id": instance.ID,
		"task_id":  req.TaskID,
	})
}

// AgentIDRequest is a common request with just agent_id
type AgentIDRequest struct {
	AgentID string `json:"agent_id"`
}

// GetAgentStatus handles agent.status action
func (h *Handlers) GetAgentStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req AgentIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	if h.lifecycle == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent manager not available", nil)
	}

	agent, found := h.lifecycle.GetInstance(req.AgentID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Agent not found", nil)
	}

	resp := AgentResponse{
		ID:          agent.ID,
		TaskID:      agent.TaskID,
		AgentType:   agent.AgentType,
		ContainerID: agent.ContainerID,
		Status:      string(agent.Status),
		Progress:    agent.Progress,
		StartedAt:   agent.StartedAt.Format("2006-01-02T15:04:05Z"),
	}
	if agent.FinishedAt != nil && !agent.FinishedAt.IsZero() {
		resp.FinishedAt = agent.FinishedAt.Format("2006-01-02T15:04:05Z")
	}
	if agent.ExitCode != nil {
		resp.ExitCode = agent.ExitCode
	}
	if agent.ErrorMessage != "" {
		resp.Error = agent.ErrorMessage
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// GetAgentLogs handles agent.logs action
func (h *Handlers) GetAgentLogs(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req AgentIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	if h.lifecycle == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent manager not available", nil)
	}

	_, found := h.lifecycle.GetInstance(req.AgentID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Agent not found", nil)
	}

	// Note: Log retrieval requires Docker client access which is not available in wshandlers
	// This handler returns a stub response; full log retrieval should use the HTTP API
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"agent_id": req.AgentID,
		"logs":     []string{},
		"message":  "Use HTTP API for full log retrieval",
	})
}

// StopAgent handles agent.stop action
func (h *Handlers) StopAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req AgentIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	if h.lifecycle == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent manager not available", nil)
	}

	if err := h.lifecycle.StopAgent(ctx, req.AgentID, false); err != nil {
		h.logger.Error("failed to stop agent", zap.String("agent_id", req.AgentID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop agent: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

// AgentTypeResponse represents an agent type
type AgentTypeResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Image        string   `json:"image"`
	Capabilities []string `json:"capabilities,omitempty"`
	Enabled      bool     `json:"enabled"`
}

// ListAgentTypesResponse is the response for agent.types
type ListAgentTypesResponse struct {
	Types []AgentTypeResponse `json:"types"`
	Total int                 `json:"total"`
}

// ListAgentTypes handles agent.types action
func (h *Handlers) ListAgentTypes(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.registry == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Registry not available", nil)
	}

	types := h.registry.List()
	resp := ListAgentTypesResponse{
		Types: make([]AgentTypeResponse, 0, len(types)),
		Total: len(types),
	}

	for _, t := range types {
		resp.Types = append(resp.Types, AgentTypeResponse{
			ID:           t.ID,
			Name:         t.Name,
			Description:  t.Description,
			Image:        t.Image,
			Capabilities: t.Capabilities,
			Enabled:      t.Enabled,
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

