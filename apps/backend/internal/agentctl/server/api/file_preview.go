package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types"
)

const (
	filePreviewComplete    = "complete"
	filePreviewUnavailable = "unavailable"
)

type filePreviewRequest struct {
	Path  string `json:"path"`
	Repo  string `json:"repo"`
	Token string `json:"token"`
}

func bindFilePreviewRequest(c *gin.Context) (filePreviewRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req filePreviewRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Path) > 4096 || len(req.Repo) > 4096 || len(req.Token) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_preview_request"})
		return req, false
	}
	return req, true
}

func (s *Server) handlePrepareFilePreview(c *gin.Context) {
	req, ok := bindFilePreviewRequest(c)
	if !ok {
		return
	}
	meta, err := s.procMgr.PrepareFilePreview(c.Request.Context(), req.Path, req.Repo)
	if errors.Is(err, types.ErrFilePreviewLimit) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "preview_too_large"})
		return
	}
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"code": "file_preview_unavailable"})
		return
	}
	if c.Request.Context().Err() != nil {
		s.procMgr.CancelFilePreview(meta.Token)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(http.StatusOK)
	if err := json.NewEncoder(c.Writer).Encode(meta); err != nil || c.Request.Context().Err() != nil {
		s.procMgr.CancelFilePreview(meta.Token)
	}
}

func (s *Server) handleReadFilePreview(c *gin.Context) {
	req, ok := bindFilePreviewRequest(c)
	if !ok {
		return
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(30 * time.Second))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "attachment")
	c.Header("Trailer", "X-Kandev-Preview-Result")
	c.Status(http.StatusOK)
	c.Writer.WriteHeaderNow()
	result := filePreviewComplete
	if err := s.procMgr.CopyFilePreview(c.Request.Context(), req.Token, c.Writer); err != nil {
		result = filePreviewUnavailable
	}
	c.Header("X-Kandev-Preview-Result", result)
}

func (s *Server) handleCancelFilePreview(c *gin.Context) {
	req, ok := bindFilePreviewRequest(c)
	if !ok {
		return
	}
	s.procMgr.CancelFilePreview(req.Token)
	c.Status(http.StatusNoContent)
}
