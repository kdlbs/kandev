package handlers

import (
	"context"
	"fmt"
	"net/url"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ProxyInvalidator can invalidate a cached VS Code reverse proxy.
type ProxyInvalidator interface {
	InvalidateProxy(sessionID string)
}

// VscodeHandlers provides WebSocket handlers for VS Code server operations.
type VscodeHandlers struct {
	lifecycleMgr     *lifecycle.Manager
	proxyInvalidator ProxyInvalidator
	logger           *logger.Logger
}

// NewVscodeHandlers creates a new VscodeHandlers instance.
func NewVscodeHandlers(lifecycleMgr *lifecycle.Manager, proxyInvalidator ProxyInvalidator, log *logger.Logger) *VscodeHandlers {
	return &VscodeHandlers{
		lifecycleMgr:     lifecycleMgr,
		proxyInvalidator: proxyInvalidator,
		logger:           log.WithFields(zap.String("component", "vscode_handlers")),
	}
}

// RegisterHandlers registers VS Code handlers with the WebSocket dispatcher.
func (h *VscodeHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionVscodeStart, h.wsVscodeStart)
	d.RegisterFunc(ws.ActionVscodeStop, h.wsVscodeStop)
	d.RegisterFunc(ws.ActionVscodeStatus, h.wsVscodeStatus)
	d.RegisterFunc(ws.ActionVscodeOpenFile, h.wsVscodeOpenFile)
}

// VscodeStartRequest for vscode.start action.
type VscodeStartRequest struct {
	SessionID string `json:"session_id"`
	Theme     string `json:"theme"`
}

// VscodeStopRequest for vscode.stop action.
type VscodeStopRequest struct {
	SessionID string `json:"session_id"`
}

// VscodeStatusRequest for vscode.status action.
type VscodeStatusRequest struct {
	SessionID string `json:"session_id"`
}

// VscodeOpenFileRequest for vscode.openFile action.
type VscodeOpenFileRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Col       int    `json:"col,omitempty"`
}

func (h *VscodeHandlers) wsVscodeStart(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req VscodeStartRequest
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

	// Start is non-blocking — returns immediately with initial status.
	// Port is allocated by agentctl using OS-assigned random port.
	resp, err := client.StartVscode(ctx, req.Theme)
	if err != nil {
		h.logger.Error("failed to start vscode", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("failed to start VS Code: %v", err), nil)
	}

	h.logger.Info("vscode start initiated",
		zap.String("session_id", req.SessionID),
		zap.Int("port", resp.Port),
		zap.String("status", resp.Status))
	h.logger.Debug("vscode start response payload",
		zap.String("session_id", req.SessionID),
		zap.String("theme", req.Theme),
		zap.Bool("success", resp.Success),
		zap.String("status", resp.Status),
		zap.Int("port", resp.Port),
		zap.String("error", resp.Error))

	return ws.NewResponse(msg.ID, msg.Action, types.VscodeStatusResponse{
		Status: resp.Status,
		Port:   resp.Port,
		URL:    buildVscodeProxyURL(req.SessionID, execution.WorkspacePath),
	})
}

func (h *VscodeHandlers) wsVscodeStop(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req VscodeStopRequest
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

	if err := client.StopVscode(ctx); err != nil {
		h.logger.Error("failed to stop vscode", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("failed to stop VS Code: %v", err), nil)
	}

	// Invalidate the cached reverse proxy so stale entries don't persist.
	if h.proxyInvalidator != nil {
		h.proxyInvalidator.InvalidateProxy(req.SessionID)
	}

	h.logger.Info("vscode stopped", zap.String("session_id", req.SessionID))

	return ws.NewResponse(msg.ID, msg.Action, types.VscodeStatusResponse{
		Status: "stopped",
	})
}

func (h *VscodeHandlers) wsVscodeStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req VscodeStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return ws.NewResponse(msg.ID, msg.Action, types.VscodeStatusResponse{Status: "stopped"})
	}

	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewResponse(msg.ID, msg.Action, types.VscodeStatusResponse{Status: "stopped"})
	}

	status, err := client.VscodeStatus(ctx)
	if err != nil {
		h.logger.Debug("vscode status request failed",
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return ws.NewResponse(msg.ID, msg.Action, types.VscodeStatusResponse{Status: "stopped"})
	}
	h.logger.Debug("vscode status response payload",
		zap.String("session_id", req.SessionID),
		zap.String("status", status.Status),
		zap.Int("port", status.Port),
		zap.String("url", status.URL),
		zap.String("error", status.Error),
		zap.String("message", status.Message))

	resp := types.VscodeStatusResponse{
		Status:  status.Status,
		Port:    status.Port,
		URL:     status.URL,
		Error:   status.Error,
		Message: status.Message,
	}
	if status.Status == "running" {
		resp.URL = buildVscodeProxyURL(req.SessionID, execution.WorkspacePath)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func buildVscodeProxyURL(sessionID, workspacePath string) string {
	base := fmt.Sprintf("/vscode/%s/", sessionID)
	if workspacePath == "" {
		return base
	}
	return base + "?folder=" + url.QueryEscape(workspacePath)
}

func (h *VscodeHandlers) wsVscodeOpenFile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req VscodeOpenFileRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Path == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "path is required", nil)
	}

	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found or no active execution", nil)
	}

	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agentctl client not available", nil)
	}

	if err := client.VscodeOpenFile(ctx, req.Path, req.Line, req.Col); err != nil {
		h.logger.Error("failed to open file in vscode",
			zap.Error(err),
			zap.String("session_id", req.SessionID),
			zap.String("path", req.Path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("failed to open file in VS Code: %v", err), nil)
	}

	h.logger.Info("opened file in vscode",
		zap.String("session_id", req.SessionID),
		zap.String("path", req.Path),
		zap.Int("line", req.Line),
		zap.Int("col", req.Col))

	return ws.NewResponse(msg.ID, msg.Action, types.VscodeOpenFileResponse{Success: true})
}
