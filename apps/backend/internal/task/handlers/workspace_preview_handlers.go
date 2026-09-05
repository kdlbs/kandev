package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const maxWorkspacePreviewRequestBytes = 32 * 1024 * 1024

type httpWorkspacePreviewRequest struct {
	Repo    string `json:"repo,omitempty"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

type workspacePreviewClient interface {
	PublishWorkspacePreview(
		context.Context, agentctltypes.WorkspacePreviewRequest,
	) (agentctltypes.WorkspacePreviewResponse, error)
}

func (h *ProcessHandlers) httpPublishWorkspacePreview(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	if h.denySessionAccess(c, sessionID) {
		return
	}
	if c.Request.ContentLength > maxWorkspacePreviewRequestBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "workspace preview request is too large"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWorkspacePreviewRequestBytes)
	var req httpWorkspacePreviewRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "workspace preview request is too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace preview request"})
		return
	}
	if len([]byte(req.Content)) > agentctltypes.MaxWorkspacePreviewContentBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "workspace preview content exceeds 5 MiB"})
		return
	}

	session, err := h.service.GetTaskSession(c.Request.Context(), sessionID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	if !h.ensureAgentctlReady(c, session, sessionID) {
		return
	}

	client, release, ok := h.workspacePreviewClient(c, sessionID)
	if !ok {
		return
	}
	defer release()
	response, err := client.PublishWorkspacePreview(c.Request.Context(), agentctltypes.WorkspacePreviewRequest{
		Repo:    req.Repo,
		Path:    req.Path,
		Content: req.Content,
	})
	if err != nil {
		h.logger.Warn("failed to publish workspace preview", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusBadGateway, gin.H{"error": "workspace preview publish failed"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// workspacePreviewClient resolves the session's agentctl client, responding
// with 503 when the session has no live execution to publish into.
func (h *ProcessHandlers) workspacePreviewClient(c *gin.Context, sessionID string) (workspacePreviewClient, func(), bool) {
	execution, err := h.lifecycleMgr.GetOrEnsureExecution(c.Request.Context(), sessionID)
	if err != nil || execution == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agentctl not ready"})
		return nil, func() {}, false
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agentctl not ready"})
		return nil, func() {}, false
	}
	return client, release, true
}
