package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"go.uber.org/zap"
)

const maxWorkspacePreviewRequestBytes = 32 * 1024 * 1024

type workspacePreviewPublishRequest struct {
	Repo    string `json:"repo,omitempty"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

type workspacePreviewPublishResponse struct {
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Version uint64 `json:"version"`
}

func (s *Server) handleWorkspacePreviewPublish(c *gin.Context) {
	if c.Request.ContentLength > maxWorkspacePreviewRequestBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "workspace preview request is too large"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWorkspacePreviewRequestBytes)
	var req workspacePreviewPublishRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "workspace preview request is too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace preview request"})
		return
	}

	response, err := s.procMgr.PublishWorkspacePreview(process.WorkspacePreviewRequest{
		Repo:    req.Repo,
		Path:    req.Path,
		Content: req.Content,
	})
	if err != nil {
		switch {
		case errors.Is(err, process.ErrWorkspacePreviewContentTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		case errors.Is(err, process.ErrWorkspacePreviewPathInvalid),
			errors.Is(err, process.ErrWorkspacePreviewTypeUnsupported):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, process.ErrWorkspacePreviewUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			s.logger.Warn("workspace preview publish failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "workspace preview server unavailable"})
		}
		return
	}

	c.JSON(http.StatusOK, workspacePreviewPublishResponse{
		Port:    response.Port,
		Path:    response.Path,
		Version: response.Version,
	})
}
