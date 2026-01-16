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
	lifecycle *lifecycle.Manager
	logger    *logger.Logger
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
}

// wsGetFileTree handles workspace.tree.get action
func (h *WorkspaceFileHandlers) wsGetFileTree(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		Path   string `json:"path"`
		Depth  int    `json:"depth"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	// Get agent execution for this task
	execution, found := h.lifecycle.GetExecutionByTaskID(req.TaskID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for task", nil)
	}

	// Get agentctl client
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Request file tree from agentctl
	response, err := client.RequestFileTree(ctx, req.Path, req.Depth)
	if err != nil {
		h.logger.Error("failed to get file tree", zap.Error(err), zap.String("task_id", req.TaskID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to get file tree: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// wsGetFileContent handles workspace.file.get action
func (h *WorkspaceFileHandlers) wsGetFileContent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		Path   string `json:"path"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if req.Path == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "path is required", nil)
	}

	// Get agent execution for this task
	execution, found := h.lifecycle.GetExecutionByTaskID(req.TaskID)
	if !found {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for task", nil)
	}

	// Get agentctl client
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Request file content from agentctl
	response, err := client.RequestFileContent(ctx, req.Path)
	if err != nil {
		h.logger.Error("failed to get file content", zap.Error(err), zap.String("task_id", req.TaskID), zap.String("path", req.Path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to get file content: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}
