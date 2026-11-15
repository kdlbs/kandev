package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type httpValidateRepositoryRemoteURLRequest struct {
	RemoteURL string `json:"remote_url"`
}

func (h *RepositoryHandlers) httpValidateRepositoryRemoteURL(c *gin.Context) {
	var body httpValidateRepositoryRemoteURLRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidRequestBody})
		return
	}
	if body.RemoteURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "remote_url is required"})
		return
	}
	preview, err := h.service.ValidateRepositoryRemoteURLUpdate(c.Request.Context(), c.Param("id"), body.RemoteURL)
	if err != nil {
		h.writeRepositoryMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, preview)
}

type wsValidateRepositoryRemoteURLRequest struct {
	ID        string `json:"id"`
	RemoteURL string `json:"remote_url"`
}

func (h *RepositoryHandlers) wsValidateRepositoryRemoteURL(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsValidateRepositoryRemoteURLRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.RemoteURL == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "remote_url is required", nil)
	}
	preview, err := h.service.ValidateRepositoryRemoteURLUpdate(ctx, req.ID, req.RemoteURL)
	if err != nil {
		return h.repositoryMutationWSError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, preview)
}

// writeRepositoryMutationError maps the shared repository mutation contract to
// the registered HTTP surface. Selection failures keep their stable typed
// status and error_code; read-only and version conflicts are bounded 409s.
func (h *RepositoryHandlers) writeRepositoryMutationError(c *gin.Context, err error) {
	if isClientDisconnect(err) {
		abortClientDisconnect(c)
		return
	}
	if status, ok := repositorySelectionHTTPStatus(err); ok {
		c.JSON(status, taskErrorBody(err))
		return
	}
	switch {
	case errors.Is(err, service.ErrRepositoryReadOnly):
		c.JSON(http.StatusConflict, gin.H{"error": workspaceReadOnlyMsg})
	case errors.Is(err, repoerrors.ErrTaskVersionConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "repository changed while validating the clone URL"})
	case errors.Is(err, service.ErrInvalidRepositorySettings):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository settings", errorDetailKeyErrorCode: "repository_selection_invalid"})
	default:
		handleNotFound(c, h.logger, err, "repository not found")
	}
}

func (h *RepositoryHandlers) repositoryMutationWSError(msg *ws.Message, err error) (*ws.Message, error) {
	if code, ok := repositorySelectionWSCode(err); ok {
		return ws.NewError(msg.ID, msg.Action, code, "Repository could not be verified", nil)
	}
	switch {
	case errors.Is(err, service.ErrRepositoryReadOnly):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, workspaceReadOnlyMsg, nil)
	case errors.Is(err, repoerrors.ErrTaskVersionConflict):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "Repository changed while validating the clone URL", nil)
	case errors.Is(err, service.ErrInvalidRepositorySettings):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid repository settings", nil)
	case errors.Is(err, repoerrors.ErrRepositoryNotFound):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository not found", nil)
	default:
		h.logger.Error("failed to mutate repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update repository", nil)
	}
}
