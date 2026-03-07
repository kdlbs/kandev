package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// PortHandlers provides WebSocket handlers for port listing operations.
type PortHandlers struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
}

// NewPortHandlers creates a new PortHandlers instance.
func NewPortHandlers(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *PortHandlers {
	return &PortHandlers{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "port_handlers")),
	}
}

// RegisterHandlers registers port handlers with the WebSocket dispatcher.
func (h *PortHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionPortList, h.wsPortList)
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

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"ports": ports,
	})
}
