package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// WorkspaceFileHandlers handles workspace file operations
type WorkspaceFileHandlers struct {
	lifecycle       *lifecycle.Manager
	logger          *logger.Logger
}

// NewWorkspaceFileHandlers creates new workspace file handlers
func NewWorkspaceFileHandlers(lm *lifecycle.Manager, log *logger.Logger) *WorkspaceFileHandlers {
	return &WorkspaceFileHandlers{
		lifecycle: lm,
		logger:    log.WithFields(zap.String("component", "workspace-file-handlers")),
	}
}

// RegisterHandlers registers workspace file handlers with the dispatcher
func (h *WorkspaceFileHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionWorkspaceFileTreeGet, h.wsGetFileTree)
	d.RegisterFunc(ws.ActionWorkspaceFileContentGet, h.wsGetFileContent)
	d.RegisterFunc(ws.ActionWorkspaceFilesSearch, h.wsSearchFiles)
}

// wsGetFileTree handles workspace.tree.get action
func (h *WorkspaceFileHandlers) wsGetFileTree(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
		Depth     int    `json:"depth"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	// Get agent execution for this session
	execution, found := h.lifecycle.GetExecutionBySessionID(req.SessionID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session", nil)
	}

	// Get agentctl client
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Request file tree from agentctl
	response, err := client.RequestFileTree(ctx, req.Path, req.Depth)
	if err != nil {
		h.logger.Error("failed to get file tree", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to get file tree: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// wsGetFileContent handles workspace.file.get action
func (h *WorkspaceFileHandlers) wsGetFileContent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Path == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "path is required", nil)
	}

	// Get agent execution for this session
	execution, found := h.lifecycle.GetExecutionBySessionID(req.SessionID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session", nil)
	}

	// Get agentctl client
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Request file content from agentctl
	response, err := client.RequestFileContent(ctx, req.Path)
	if err != nil {
		h.logger.Error("failed to get file content", zap.Error(err), zap.String("session_id", req.SessionID), zap.String("path", req.Path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to get file content: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// wsSearchFiles handles workspace.files.search action
func (h *WorkspaceFileHandlers) wsSearchFiles(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	// Get agent execution for this session
	execution, found := h.lifecycle.GetExecutionBySessionID(req.SessionID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session", nil)
	}

	// Get agentctl client
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Default limit
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Search files via agentctl
	response, err := client.SearchFiles(ctx, req.Query, limit)
	if err != nil {
		h.logger.Error("failed to search files", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to search files: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}
