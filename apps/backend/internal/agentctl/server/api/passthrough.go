package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) registerPassthroughRoutes(api *gin.RouterGroup) {
	api.GET("/passthrough", s.handlePassthroughSnapshot)
	api.POST("/passthrough/start", s.handlePassthroughStart)
	api.POST("/passthrough/prepare", s.handlePassthroughPrepare)
	api.POST("/passthrough/cleanup", s.handlePassthroughCleanup)
	api.POST("/passthrough/input", s.handlePassthroughInput)
	api.GET("/passthrough/submissions/:submission_id", s.handlePassthroughInputReceipt)
	api.POST("/passthrough/resize", s.handlePassthroughResize)
	api.POST("/passthrough/notice", s.handlePassthroughNotice)
	api.GET("/passthrough/buffer", s.handlePassthroughBuffer)
	api.GET("/passthrough/idle", s.handlePassthroughIdle)
	api.GET("/passthrough/events", s.handlePassthroughEvents)
	api.GET("/passthrough/stream", s.handlePassthroughStream)
	api.DELETE("/passthrough/:process_id", s.handlePassthroughStop)
}

func (s *Server) handlePassthroughInputReceipt(c *gin.Context) {
	target, err := passthroughTarget(c)
	if err != nil {
		passthroughError(c, err)
		return
	}
	result, err := s.procMgr.Passthrough().LookupInput(c.Request.Context(), target, c.Param("submission_id"))
	if err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handlePassthroughNotice(c *gin.Context) {
	var request types.PassthroughInputRequest
	if !bindPassthrough(c, &request) {
		return
	}
	if err := s.procMgr.Passthrough().TerminalNotice(request.PassthroughTarget, []byte(request.Data)); err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "sent"})
}

func passthroughError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, process.ErrPassthroughConflict) {
		status = http.StatusConflict
	}
	if errors.Is(err, process.ErrPassthroughNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func bindPassthrough(c *gin.Context, request any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid passthrough request"})
		return false
	}
	return true
}

func passthroughTarget(c *gin.Context) (types.PassthroughTarget, error) {
	generation, err := strconv.ParseUint(c.Query("generation"), 10, 64)
	if err != nil {
		return types.PassthroughTarget{}, errors.New("process generation is required")
	}
	id := c.Query("process_id")
	if c.Param("process_id") != "" {
		id = c.Param("process_id")
	}
	return types.PassthroughTarget{ProcessID: id, Generation: generation}, nil
}

func (s *Server) handlePassthroughSnapshot(c *gin.Context) {
	if id := c.Query("operation_id"); id != "" {
		snapshot, err := s.procMgr.Passthrough().Lookup(id)
		if err != nil {
			passthroughError(c, err)
			return
		}
		c.JSON(http.StatusOK, snapshot)
		return
	}
	c.JSON(http.StatusOK, s.procMgr.Passthrough().Snapshot())
}

func (s *Server) handlePassthroughStart(c *gin.Context) {
	var request types.PassthroughStartRequest
	if !bindPassthrough(c, &request) {
		return
	}
	snapshot, err := s.procMgr.Passthrough().Start(c.Request.Context(), request)
	if err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (s *Server) handlePassthroughInput(c *gin.Context) {
	var request types.PassthroughInputRequest
	if !bindPassthrough(c, &request) {
		return
	}
	result, err := s.procMgr.Passthrough().Input(c.Request.Context(), request)
	if err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handlePassthroughResize(c *gin.Context) {
	var request types.PassthroughResizeRequest
	if !bindPassthrough(c, &request) {
		return
	}
	if err := s.procMgr.Passthrough().Resize(request); err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resized"})
}

func (s *Server) handlePassthroughStop(c *gin.Context) {
	target, err := passthroughTarget(c)
	if err == nil {
		err = s.procMgr.Passthrough().Stop(c.Request.Context(), target)
	}
	if err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

func (s *Server) handlePassthroughBuffer(c *gin.Context) {
	target, err := passthroughTarget(c)
	var buffer string
	if err == nil {
		buffer, err = s.procMgr.Passthrough().Buffer(target)
	}
	if err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, ShellBufferResponse{Data: buffer})
}

func (s *Server) handlePassthroughIdle(c *gin.Context) {
	target, err := passthroughTarget(c)
	if err == nil {
		err = s.procMgr.Passthrough().FirstIdle(c.Request.Context(), target)
	}
	if err != nil {
		passthroughError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "idle"})
}

func (s *Server) handlePassthroughEvents(c *gin.Context) {
	sequence, err := strconv.ParseUint(c.DefaultQuery("after", "0"), 10, 64)
	if err != nil {
		passthroughError(c, errors.New("invalid event cursor"))
		return
	}
	if websocket.IsWebSocketUpgrade(c.Request) {
		s.streamPassthroughEvents(c, sequence)
		return
	}
	page, err := s.procMgr.Passthrough().Events(c.Request.Context(), sequence, c.Query("wait") == queryParamTrue)
	if err != nil {
		return
	}
	c.JSON(http.StatusOK, page)
}
