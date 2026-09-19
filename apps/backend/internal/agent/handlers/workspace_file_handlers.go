package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// WorkspaceFileHandlers handles workspace file operations
type WorkspaceFileHandlers struct {
	lifecycle ExecutionLookup
	logger    *logger.Logger
}

type workspaceContentSearchRequest struct {
	SessionID    string `json:"session_id"`
	Query        string `json:"query"`
	LimitPerRepo int    `json:"limit_per_repo"`
}

// NewWorkspaceFileHandlers creates new workspace file handlers
func NewWorkspaceFileHandlers(lm ExecutionLookup, log *logger.Logger) *WorkspaceFileHandlers {
	return &WorkspaceFileHandlers{
		lifecycle: lm,
		logger:    log.WithFields(zap.String("component", "workspace-file-handlers")),
	}
}

// RegisterHandlers registers workspace file handlers with the dispatcher
func (h *WorkspaceFileHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionWorkspaceFileTreeGet, h.wsGetFileTree)
	d.RegisterFunc(ws.ActionWorkspaceFileContentGet, h.wsGetFileContent)
	d.RegisterFunc(ws.ActionWorkspaceFileContentGetRef, h.wsGetFileContentAtRef)
	d.RegisterFunc(ws.ActionWorkspaceFileContentUpdate, h.wsUpdateFileContent)
	d.RegisterFunc(ws.ActionWorkspaceFileCreate, h.wsCreateFile)
	d.RegisterFunc(ws.ActionWorkspaceFileDelete, h.wsDeleteFile)
	d.RegisterFunc(ws.ActionWorkspaceFileRename, h.wsRenameFile)
	d.RegisterFunc(ws.ActionWorkspaceFilesSearch, h.wsSearchFiles)
	d.RegisterFunc(ws.ActionWorkspaceContentSearch, h.wsSearchContent)
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
	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+err.Error(), nil)
	}

	// Get agentctl client
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
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
		Repo      string `json:"repo,omitempty"`
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
	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+err.Error(), nil)
	}

	// Get agentctl client
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Request file content from agentctl
	response, err := client.RequestFileContent(ctx, req.Path, req.Repo)
	if err != nil {
		if isMissingFileContentError(err) {
			h.logger.Debug("file not found (expected for deleted or stale files)",
				zap.String("session_id", req.SessionID),
				zap.String("path", req.Path))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
		}
		h.logger.Error("failed to get file content", zap.Error(err), zap.String("session_id", req.SessionID), zap.String("path", req.Path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to get file content: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

func isMissingFileContentError(err error) bool {
	// The client derives this sentinel from the agentctl file-content 404 status.
	return errors.Is(err, agentctl.ErrFileNotFound)
}

// wsGetFileContentAtRef handles workspace.file.get_at_ref action
func (h *WorkspaceFileHandlers) wsGetFileContentAtRef(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
		Ref       string `json:"ref"`
		Repo      string `json:"repo,omitempty"`
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
	if req.Ref == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ref is required", nil)
	}

	// Get agent execution for this session
	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+err.Error(), nil)
	}

	// Get agentctl client
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Request file content at ref from agentctl
	response, err := client.RequestFileContentAtRef(ctx, req.Path, req.Ref, req.Repo)
	if err != nil {
		// "File not found at ref" is expected for new files - log as debug, not error
		errMsg := err.Error()
		if strings.Contains(errMsg, "file not found at ref") {
			h.logger.Debug("file not found at ref (expected for new files)", zap.String("session_id", req.SessionID), zap.String("path", req.Path), zap.String("ref", req.Ref))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errMsg, nil)
		}
		h.logger.Error("failed to get file content at ref", zap.Error(err), zap.String("session_id", req.SessionID), zap.String("path", req.Path), zap.String("ref", req.Ref))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to get file content at ref: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// wsUpdateFileContent handles workspace.file.update action
func (h *WorkspaceFileHandlers) wsUpdateFileContent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID      string  `json:"session_id"`
		Path           string  `json:"path"`
		Diff           string  `json:"diff"`
		OriginalHash   string  `json:"original_hash"`
		DesiredContent *string `json:"desired_content"`
		Repo           string  `json:"repo,omitempty"`
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
	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+err.Error(), nil)
	}

	// Get agentctl client
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Apply file diff via agentctl
	response, err := client.ApplyFileDiff(ctx, req.Path, req.Diff, req.OriginalHash, req.Repo, req.DesiredContent)
	if err != nil {
		h.logger.Error("failed to apply file diff", zap.Error(err), zap.String("session_id", req.SessionID), zap.String("path", req.Path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to apply file diff: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// resolveSessionFileClient parses session_id + path from the message payload
// and returns the agentctl client, or an error response.
func (h *WorkspaceFileHandlers) resolveSessionFileClient(
	ctx context.Context, msg *ws.Message,
) (sessionID, path, repo string, client *agentctl.Client, release func(), errResp *ws.Message) {
	noRelease := func() {}
	var req struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
		Repo      string `json:"repo,omitempty"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return "", "", "", nil, noRelease, errResp
	}
	if req.SessionID == "" {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return "", "", "", nil, noRelease, errResp
	}
	if req.Path == "" {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "path is required", nil)
		return "", "", "", nil, noRelease, errResp
	}

	execution, resolveErr := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if resolveErr != nil {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+resolveErr.Error(), nil)
		return "", "", "", nil, noRelease, errResp
	}
	c, releaseClient := execution.AcquireAgentCtlClient()
	if c == nil {
		errResp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
		return "", "", "", nil, releaseClient, errResp
	}
	return req.SessionID, req.Path, req.Repo, c, releaseClient, nil
}

// wsCreateFile handles workspace.file.create action
func (h *WorkspaceFileHandlers) wsCreateFile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	sessionID, path, repo, client, releaseClient, errResp := h.resolveSessionFileClient(ctx, msg)
	defer releaseClient()
	if errResp != nil {
		return errResp, nil
	}

	response, err := client.CreateFile(ctx, path, repo)
	if err != nil {
		h.logger.Error("failed to create file", zap.Error(err), zap.String("session_id", sessionID), zap.String("path", path))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to create file: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}

// wsDeleteFile handles workspace.file.delete action
func (h *WorkspaceFileHandlers) wsDeleteFile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	sessionID, path, repo, client, releaseClient, errResp := h.resolveSessionFileClient(ctx, msg)
	defer releaseClient()
	if errResp != nil {
		return errResp, nil
	}

	response, err := client.DeleteFile(ctx, path, repo)
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
	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+err.Error(), nil)
	}

	// Get agentctl client
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
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

// wsSearchContent handles workspace.content.search.
func (h *WorkspaceFileHandlers) wsSearchContent(
	ctx context.Context,
	msg *ws.Message,
) (*ws.Message, error) {
	req, err := decodeWorkspaceContentSearchRequest(msg)
	if err != nil {
		return ws.NewError(
			msg.ID, msg.Action, ws.ErrorCodeBadRequest,
			"Invalid payload: "+err.Error(), nil,
		)
	}
	if req.SessionID == "" {
		return ws.NewError(
			msg.ID, msg.Action, ws.ErrorCodeValidation,
			"session_id is required", nil,
		)
	}
	if utf8.RuneCountInString(req.Query) > streams.WorkspaceContentSearchMaxQueryRunes {
		return ws.NewError(
			msg.ID, msg.Action, ws.ErrorCodeValidation,
			"query exceeds 200 characters", nil,
		)
	}

	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(
			msg.ID, msg.Action, ws.ErrorCodeNotFound,
			"No agent found for session: "+err.Error(), nil,
		)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return ws.NewError(
			msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Agent client not available", nil,
		)
	}
	response, err := client.SearchWorkspaceContent(ctx, req.Query, req.LimitPerRepo)
	if err != nil {
		h.logger.Error(
			"failed to search workspace content",
			zap.Error(err),
			zap.String("session_id", req.SessionID),
		)
		return ws.NewError(
			msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("Failed to search workspace content: %v", err), nil,
		)
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

func decodeWorkspaceContentSearchRequest(
	msg *ws.Message,
) (workspaceContentSearchRequest, error) {
	var request workspaceContentSearchRequest
	err := msg.ParsePayload(&request)
	return request, err
}

// wsRenameFile handles workspace.file.rename action
func (h *WorkspaceFileHandlers) wsRenameFile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
		OldPath   string `json:"old_path"`
		NewPath   string `json:"new_path"`
		Repo      string `json:"repo,omitempty"`
	}

	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.OldPath == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "old_path is required", nil)
	}
	if req.NewPath == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "new_path is required", nil)
	}

	// Get agent execution for this session
	execution, err := h.lifecycle.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "No agent found for session: "+err.Error(), nil)
	}

	// Get agentctl client
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent client not available", nil)
	}

	// Rename file via agentctl
	response, err := client.RenameFile(ctx, req.OldPath, req.NewPath, req.Repo)
	if err != nil {
		h.logger.Error("failed to rename file", zap.Error(err),
			zap.String("session_id", req.SessionID),
			zap.String("old_path", req.OldPath),
			zap.String("new_path", req.NewPath))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("Failed to rename file: %v", err), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, response)
}
