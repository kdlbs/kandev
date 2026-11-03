package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/opencode"
)

func (s *Server) handleOpenCodeCredentialStatus(c *gin.Context) {
	state, err := s.procMgr.OpenCodeCredentialStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, struct {
		Version int    `json:"version"`
		State   string `json:"state"`
	}{Version: 1, State: state})
}

func (s *Server) handleOpenCodeCredentials(c *gin.Context) {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	var entries []json.RawMessage
	if err != nil || json.Unmarshal(data, &entries) != nil || len(entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid OpenCode credential payload"})
		return
	}
	if err := s.procMgr.ImportOpenCodeCredentials(c.Request.Context(), data); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, opencode.ErrCredentialRuntime) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{errKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, struct {
		Success bool `json:"success"`
	}{Success: true})
}
