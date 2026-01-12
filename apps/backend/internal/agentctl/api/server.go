// Package api provides the HTTP REST API for agentctl
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/config"
	"github.com/kandev/kandev/internal/agentctl/process"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Server is the HTTP API server
type Server struct {
	cfg     *config.Config
	procMgr *process.Manager
	logger  *logger.Logger
	router  *gin.Engine

	upgrader websocket.Upgrader
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, procMgr *process.Manager, log *logger.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		cfg:     cfg,
		procMgr: procMgr,
		logger:  log.WithFields(zap.String("component", "api-server")),
		router:  gin.New(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for container-local communication
			},
		},
	}

	s.setupRoutes()
	return s
}

// Router returns the HTTP router
func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.handleHealth)

	// Agent control
	api := s.router.Group("/api/v1")
	{
		// Status and info
		api.GET("/status", s.handleStatus)
		api.GET("/info", s.handleInfo)

		// Process control
		api.POST("/start", s.handleStart)
		api.POST("/stop", s.handleStop)

		// ACP high-level methods (using acp-go-sdk)
		api.POST("/acp/initialize", s.handleACPInitialize)
		api.POST("/acp/session/new", s.handleACPNewSession)
		api.POST("/acp/prompt", s.handleACPPrompt)
		api.GET("/acp/stream", s.handleACPStreamWS)

		// Permission request handling
		api.GET("/acp/permissions", s.handleGetPendingPermissions)
		api.GET("/acp/permissions/stream", s.handlePermissionStreamWS)
		api.POST("/acp/permissions/respond", s.handlePermissionRespond)

		// Output streaming
		api.GET("/output", s.handleGetOutput)
		api.GET("/output/stream", s.handleOutputStreamWS)

		// Workspace monitoring (git status, files, diff)
		api.GET("/workspace/git-status/stream", s.handleGitStatusStreamWS)
		api.GET("/workspace/files/stream", s.handleFilesStreamWS)
		api.GET("/workspace/diff/stream", s.handleDiffStreamWS)
	}
}

// Health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Status response
type StatusResponse struct {
	AgentStatus string                 `json:"agent_status"`
	ProcessInfo map[string]interface{} `json:"process_info"`
	Uptime      string                 `json:"uptime,omitempty"`
}

func (s *Server) handleStatus(c *gin.Context) {
	c.JSON(http.StatusOK, StatusResponse{
		AgentStatus: string(s.procMgr.Status()),
		ProcessInfo: s.procMgr.GetProcessInfo(),
	})
}

// Info response
type InfoResponse struct {
	Version      string `json:"version"`
	AgentCommand string `json:"agent_command"`
	WorkDir      string `json:"work_dir"`
	Port         int    `json:"port"`
}

func (s *Server) handleInfo(c *gin.Context) {
	c.JSON(http.StatusOK, InfoResponse{
		Version:      "0.1.0",
		AgentCommand: s.cfg.AgentCommand,
		WorkDir:      s.cfg.WorkDir,
		Port:         s.cfg.Port,
	})
}

// Start request/response
type StartRequest struct {
	// Future: could add options like custom command, env overrides
}

type StartResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleStart(c *gin.Context) {
	if err := s.procMgr.Start(c.Request.Context()); err != nil {
		s.logger.Error("failed to start agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StartResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, StartResponse{
		Success: true,
		Message: "agent started",
	})
}

// Stop request/response
type StopRequest struct {
	Force bool `json:"force"`
}

type StopResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleStop(c *gin.Context) {
	if err := s.procMgr.Stop(c.Request.Context()); err != nil {
		s.logger.Error("failed to stop agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StopResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, StopResponse{
		Success: true,
		Message: "agent stopped",
	})
}
