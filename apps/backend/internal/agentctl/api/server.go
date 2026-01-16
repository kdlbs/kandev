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

// Server is the HTTP API server for a single agent instance.
type Server struct {
	cfg     *config.InstanceConfig
	procMgr *process.Manager
	logger  *logger.Logger
	router  *gin.Engine

	upgrader websocket.Upgrader
}

// NewServer creates a new API server for an agent instance.
func NewServer(cfg *config.InstanceConfig, procMgr *process.Manager, log *logger.Logger) *Server {
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
		api.POST("/agent/configure", s.handleAgentConfigure)
		api.POST("/start", s.handleStart)
		api.POST("/stop", s.handleStop)

		// ACP high-level methods (using acp-go-sdk)
		api.POST("/acp/initialize", s.handleACPInitialize)
		api.POST("/acp/session/new", s.handleACPNewSession)
		api.POST("/acp/session/load", s.handleACPLoadSession)
		api.POST("/acp/prompt", s.handleACPPrompt)
		api.GET("/acp/stream", s.handleACPStreamWS)

		// Permission request handling
		api.GET("/acp/permissions", s.handleGetPendingPermissions)
		api.GET("/acp/permissions/stream", s.handlePermissionStreamWS)
		api.POST("/acp/permissions/respond", s.handlePermissionRespond)

		// Workspace monitoring (git status, files)
		api.GET("/workspace/git-status/stream", s.handleGitStatusStreamWS)
		api.GET("/workspace/files/stream", s.handleFilesStreamWS)
		api.GET("/workspace/file-changes/stream", s.handleFileChangesStreamWS)

		// Workspace file operations (simple HTTP)
		api.GET("/workspace/tree", s.handleFileTree)
		api.GET("/workspace/file/content", s.handleFileContent)

		// Shell access
		api.GET("/shell/status", s.handleShellStatus)
		api.GET("/shell/stream", s.handleShellStreamWS)
		api.GET("/shell/buffer", s.handleShellBuffer)
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

// AgentConfigure request/response - configures the agent command before starting
type AgentConfigureRequest struct {
	Command string            `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
}

type AgentConfigureResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleAgentConfigure(c *gin.Context) {
	var req AgentConfigureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AgentConfigureResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	if req.Command == "" {
		c.JSON(http.StatusBadRequest, AgentConfigureResponse{
			Success: false,
			Error:   "command is required",
		})
		return
	}

	if err := s.procMgr.Configure(req.Command, req.Env); err != nil {
		s.logger.Error("failed to configure agent", zap.Error(err), zap.String("command", req.Command))
		c.JSON(http.StatusInternalServerError, AgentConfigureResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("agent configured", zap.String("command", req.Command))
	c.JSON(http.StatusOK, AgentConfigureResponse{
		Success: true,
		Message: "agent configured",
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

// ShellStatusResponse represents shell status
type ShellStatusResponse struct {
	Available bool   `json:"available"`
	Running   bool   `json:"running"`
	Pid       int    `json:"pid,omitempty"`
	Shell     string `json:"shell,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleShellStatus(c *gin.Context) {
	shell := s.procMgr.Shell()
	if shell == nil {
		c.JSON(http.StatusOK, ShellStatusResponse{
			Available: false,
			Error:     "shell not available",
		})
		return
	}

	status := shell.Status()
	c.JSON(http.StatusOK, ShellStatusResponse{
		Available: true,
		Running:   status.Running,
		Pid:       status.Pid,
		Shell:     status.Shell,
		Cwd:       status.Cwd,
		StartedAt: status.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ShellMessage represents a shell I/O message
type ShellMessage struct {
	Type string `json:"type"` // "input", "output", "ping", "pong", "exit"
	Data string `json:"data,omitempty"`
	Code int    `json:"code,omitempty"`
}

// ShellBufferResponse is the response for GET /api/v1/shell/buffer
type ShellBufferResponse struct {
	Data string `json:"data"`
}

func (s *Server) handleShellBuffer(c *gin.Context) {
	shell := s.procMgr.Shell()
	if shell == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "shell not available"})
		return
	}

	buffered := shell.GetBufferedOutput()
	c.JSON(http.StatusOK, ShellBufferResponse{
		Data: string(buffered),
	})
}

func (s *Server) handleShellStreamWS(c *gin.Context) {
	shell := s.procMgr.Shell()
	if shell == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "shell not available"})
		return
	}

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close shell websocket", zap.Error(err))
		}
	}()

	s.logger.Info("Shell stream WebSocket connected")

	// Create output channel and subscribe to shell output
	outputCh := make(chan []byte, 256)
	shell.Subscribe(outputCh)
	defer shell.Unsubscribe(outputCh)

	// Note: Buffered output is sent via GET /api/v1/shell/buffer when clients subscribe
	// This avoids duplicate output when clients reconnect

	// Done channel to signal goroutine shutdown
	done := make(chan struct{})
	defer close(done)

	// Read input from WebSocket and write to shell
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				var msg ShellMessage
				if err := conn.ReadJSON(&msg); err != nil {
					s.logger.Debug("shell WebSocket read error", zap.Error(err))
					return
				}

				switch msg.Type {
				case "input":
					if _, err := shell.Write([]byte(msg.Data)); err != nil {
						s.logger.Debug("shell write error", zap.Error(err))
					}
				case "ping":
					// Respond with pong
					if err := conn.WriteJSON(ShellMessage{Type: "pong"}); err != nil {
						s.logger.Debug("shell pong write error", zap.Error(err))
						return
					}
				}
			}
		}
	}()

	// Write output from shell to WebSocket
	for data := range outputCh {
		msg := ShellMessage{Type: "output", Data: string(data)}
		if err := conn.WriteJSON(msg); err != nil {
			s.logger.Debug("shell WebSocket write error", zap.Error(err))
			return
		}
	}
}
