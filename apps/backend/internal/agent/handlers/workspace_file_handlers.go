package handlers

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
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
	d.RegisterFunc(ws.ActionWorkspaceFileContentUpdate, h.wsUpdateFileContent)
	d.RegisterFunc(ws.ActionWorkspaceFileCreate, h.wsCreateFile)
	d.RegisterFunc(ws.ActionWorkspaceFileDelete, h.wsDeleteFile)
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

// wsUpdateFileContent handles workspace.file.update action
func (h *WorkspaceFileHandlers) wsUpdateFileContent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID    string `json:"session_id"`
		Path         string `json:"path"`
		Diff         string `json:"diff"`
		OriginalHash string `json:"original_hash"`
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
	if req.Diff == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "diff is required", nil)
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

	// Apply file diff via agentctl
	response, err := client.ApplyFileDiff(ctx, req.Path, req.Diff, req.OriginalHash)
	if err != nil {
		h.logger.Error("failed to apply file diff", zap.Error(err), zap.String("session_id", req.SessionID), zap.String("path", req.Path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to apply file diff: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// resolveSessionFileClient parses session_id + path from the message payload
// and returns the agentctl client, or an error response.
func (h *WorkspaceFileHandlers) resolveSessionFileClient(
	msg *ws.Message,
) (sessionID, path string, client *agentctl.Client, errResp *ws.Message) {
	var req struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return "", "", nil, errResp
	}
	if req.SessionID == "" {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return "", "", nil, errResp
	}
	if req.Path == "" {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "path is required", nil)
		return "", "", nil, errResp
	}

	execution, found := h.lifecycle.GetExecutionBySessionID(req.SessionID)
	if !found {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session", nil)
		return "", "", nil, errResp
	}
	c := execution.GetAgentCtlClient()
	if c == nil {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
		return "", "", nil, errResp
	}
	return req.SessionID, req.Path, c, nil
}

// wsCreateFile handles workspace.file.create action
func (h *WorkspaceFileHandlers) wsCreateFile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	sessionID, path, client, errResp := h.resolveSessionFileClient(msg)
	if errResp != nil {
		return errResp, nil
	}

	response, err := client.CreateFile(ctx, path)
	if err != nil {
		h.logger.Error("failed to create file", zap.Error(err), zap.String("session_id", sessionID), zap.String("path", path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to create file: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// wsDeleteFile handles workspace.file.delete action
func (h *WorkspaceFileHandlers) wsDeleteFile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	sessionID, path, client, errResp := h.resolveSessionFileClient(msg)
	if errResp != nil {
		return errResp, nil
	}

	response, err := client.DeleteFile(ctx, path)
	if err != nil {
		h.logger.Error("failed to delete file", zap.Error(err), zap.String("session_id", sessionID), zap.String("path", path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to delete file: %v", err), nil)
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
