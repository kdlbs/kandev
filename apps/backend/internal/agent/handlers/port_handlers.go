package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// TunnelController manages port tunnels.
// Implemented by websocket.TunnelManager.
type TunnelController interface {
	StartTunnel(sessionID string, port int, tunnelPort int) (int, error)
	StopTunnel(sessionID string, port int) error
	// ListTunnels returns []TunnelInfo-like structs with port/tunnel_port fields.
	ListTunnels(sessionID string) any
}

// PortHandlers provides WebSocket handlers for port listing and tunnel operations.
type PortHandlers struct {
	lifecycleMgr *lifecycle.Manager
	tunnelCtrl   TunnelController
	logger       *logger.Logger
}

// NewPortHandlers creates a new PortHandlers instance.
func NewPortHandlers(lifecycleMgr *lifecycle.Manager, tunnelCtrl TunnelController, log *logger.Logger) *PortHandlers {
	return &PortHandlers{
		lifecycleMgr: lifecycleMgr,
		tunnelCtrl:   tunnelCtrl,
		logger:       log.WithFields(zap.String("component", "port_handlers")),
	}
}

// RegisterHandlers registers port handlers with the WebSocket dispatcher.
func (h *PortHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionPortList, h.wsPortList)

	if h.tunnelCtrl != nil {
		d.RegisterFunc(ws.ActionPortTunnelStart, h.wsTunnelStart)
		d.RegisterFunc(ws.ActionPortTunnelStop, h.wsTunnelStop)
		d.RegisterFunc(ws.ActionPortTunnelList, h.wsTunnelList)
	}
}

// portListRequest for port.list action.
type portListRequest struct {
	SessionID string `json:"session_id"`
}

func (h *PortHandlers) wsPortList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req portListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found or no active execution", nil)
	}

	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agentctl client not available", nil)
	}

	ports, err := client.ListPorts(ctx)
	if err != nil {
		h.logger.Error("failed to list ports", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("failed to list ports: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"ports": ports,
	})
}

type tunnelStartRequest struct {
	SessionID  string `json:"session_id"`
	Port       int    `json:"port"`
	TunnelPort int    `json:"tunnel_port"`
}

func (h *PortHandlers) wsTunnelStart(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	var req tunnelStartRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Port < 1 || req.Port > 65535 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "port must be 1-65535", nil)
	}
	if req.TunnelPort != 0 && (req.TunnelPort < 1 || req.TunnelPort > 65535) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "tunnel_port must be 0 (random) or 1-65535", nil)
	}

	tunnelPort, err := h.tunnelCtrl.StartTunnel(req.SessionID, req.Port, req.TunnelPort)
	if err != nil {
		h.logger.Error("failed to start tunnel",
			zap.Error(err),
			zap.String("session_id", req.SessionID),
			zap.Int("port", req.Port))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("failed to start tunnel: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"tunnel_port": tunnelPort,
	})
}

type tunnelStopRequest struct {
	SessionID string `json:"session_id"`
	Port      int    `json:"port"`
}

func (h *PortHandlers) wsTunnelStop(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	var req tunnelStopRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Port < 1 || req.Port > 65535 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "port must be 1-65535", nil)
	}

	if err := h.tunnelCtrl.StopTunnel(req.SessionID, req.Port); err != nil {
		h.logger.Error("failed to stop tunnel",
			zap.Error(err),
			zap.String("session_id", req.SessionID),
			zap.Int("port", req.Port))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("failed to stop tunnel: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{})
}

type tunnelListRequest struct {
	SessionID string `json:"session_id"`
}

func (h *PortHandlers) wsTunnelList(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	var req tunnelListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	tunnels := h.tunnelCtrl.ListTunnels(req.SessionID)

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"tunnels": tunnels,
	})
}
