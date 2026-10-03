package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

// httpRegisterRemoteRepositoryRequest carries a remote repository locator plus
// the optional provider hints a picker already knows. The backend verifies the
// locator before anything is persisted, so none of the hints are trusted as-is.
type httpRegisterRemoteRepositoryRequest struct {
	RemoteURL      string `json:"remote_url"`
	Provider       string `json:"provider"`
	ProviderHost   string `json:"provider_host"`
	ProviderScope  string `json:"provider_scope"`
	ProviderRepoID string `json:"provider_repo_id"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`
	DefaultBranch  string `json:"default_branch"`
}

// httpRegisterRemoteRepository registers a provider-hosted repository in the
// workspace. It answers 201 when the call created the repository and 200 when
// the locator resolved to a repository the workspace already had.
func (h *RepositoryHandlers) httpRegisterRemoteRepository(c *gin.Context) {
	var body httpRegisterRemoteRepositoryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidRequestBody})
		return
	}
	if body.RemoteURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "remote_url is required"})
		return
	}
	workspaceID := c.Param("id")
	if h.rejectReadOnlyWorkspaceHTTP(c, workspaceID) {
		return
	}
	repository, created, err := h.service.RegisterRemoteRepository(c.Request.Context(), &service.RegisterRemoteRepositoryRequest{
		WorkspaceID:    workspaceID,
		RemoteURL:      body.RemoteURL,
		Provider:       body.Provider,
		ProviderHost:   body.ProviderHost,
		ProviderScope:  body.ProviderScope,
		ProviderRepoID: body.ProviderRepoID,
		ProviderOwner:  body.ProviderOwner,
		ProviderName:   body.ProviderName,
		DefaultBranch:  body.DefaultBranch,
	})
	if err != nil {
		h.writeRegisterRemoteRepositoryError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, dto.FromRepository(repository))
}

func (h *RepositoryHandlers) writeRegisterRemoteRepositoryError(c *gin.Context, err error) {
	if isClientDisconnect(err) {
		abortClientDisconnect(c)
		return
	}
	if status, ok := repositorySelectionHTTPStatus(err); ok {
		c.JSON(status, taskErrorBody(err))
		return
	}
	if errors.Is(err, service.ErrInvalidRepositorySettings) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if service.IsForbidden(err) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	h.logger.Error("failed to register remote repository", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register remote repository"})
}
