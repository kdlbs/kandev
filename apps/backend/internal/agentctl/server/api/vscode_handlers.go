package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// VscodeStartRequest is the request body for starting code-server.
type VscodeStartRequest struct {
	Theme string `json:"theme"` // "dark" or "light"
}

func (s *Server) handleVscodeStart(c *gin.Context) {
	var req VscodeStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.VscodeStartResponse{
			Error: "invalid request: " + err.Error(),
		})
		return
	}

	// Start is non-blocking — it launches in a background goroutine.
	// Port is allocated automatically using an OS-assigned random port.
	s.procMgr.StartVscode(c.Request.Context(), req.Theme)
	s.logger.Info("code-server start initiated", zap.String("theme", req.Theme))

	// Return current status (will be "installing")
	info := s.procMgr.VscodeInfo()
	s.logger.Debug("vscode start response payload",
		zap.String("status", string(info.Status)),
		zap.Int("port", info.Port),
		zap.String("error", info.Error),
		zap.String("message", info.Message))
	c.JSON(http.StatusOK, types.VscodeStartResponse{
		Success: true,
		Status:  string(info.Status),
		Port:    info.Port,
	})
}

func (s *Server) handleVscodeStop(c *gin.Context) {
	if err := s.procMgr.StopVscode(c.Request.Context()); err != nil {
		s.logger.Error("failed to stop code-server", zap.Error(err))
		c.JSON(http.StatusInternalServerError, types.VscodeStopResponse{
			Error: err.Error(),
		})
		return
	}

	s.logger.Info("code-server stopped")
	c.JSON(http.StatusOK, types.VscodeStopResponse{Success: true})
}

func (s *Server) handleVscodeStatus(c *gin.Context) {
	info := s.procMgr.VscodeInfo()
	s.logger.Debug("vscode status response payload",
		zap.String("status", string(info.Status)),
		zap.Int("port", info.Port),
		zap.String("error", info.Error),
		zap.String("message", info.Message))
	c.JSON(http.StatusOK, types.VscodeStatusResponse{
		Status:  string(info.Status),
		Port:    info.Port,
		Error:   info.Error,
		Message: info.Message,
	})
}

func (s *Server) handleVscodeOpenFile(c *gin.Context) {
	var req types.VscodeOpenFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.VscodeOpenFileResponse{
			Error: "invalid request: " + err.Error(),
		})
		return
	}

	if req.Path == "" {
		c.JSON(http.StatusBadRequest, types.VscodeOpenFileResponse{
			Error: "path is required",
		})
		return
	}

	if err := s.procMgr.VscodeOpenFile(c.Request.Context(), req.Path, req.Line, req.Col); err != nil {
		s.logger.Error("failed to open file in vscode", zap.Error(err), zap.String("path", req.Path))
		c.JSON(http.StatusInternalServerError, types.VscodeOpenFileResponse{
			Error: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.VscodeOpenFileResponse{Success: true})
}
