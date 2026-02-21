// Package api provides the HTTP REST API for agentctl
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/mcp"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Server is the HTTP API server for a single agent instance.
type Server struct {
	cfg              *config.InstanceConfig
	procMgr          *process.Manager
	mcpServer        *mcp.Server
	mcpBackendClient *mcp.ChannelBackendClient
	logger           *logger.Logger
	router           *gin.Engine

	upgrader websocket.Upgrader
}

// NewServer creates a new API server for an agent instance.
// If mcpServer is provided, MCP routes will be registered.
// If mcpBackendClient is provided, the agent stream becomes bidirectional for MCP.
func NewServer(cfg *config.InstanceConfig, procMgr *process.Manager, mcpServer *mcp.Server, mcpBackendClient *mcp.ChannelBackendClient, log *logger.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		cfg:              cfg,
		procMgr:          procMgr,
		mcpServer:        mcpServer,
		mcpBackendClient: mcpBackendClient,
		logger:           log.WithFields(zap.String("component", "api-server")),
		router:           gin.New(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for container-local communication
			},
		},
	}

	s.router.Use(httpmw.RequestLogger(s.logger, "agentctl-instance"))

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

		// Agent stream: bidirectional WebSocket for agent events, MCP, and agent operations
		// (initialize, session/new, session/load, prompt, cancel, stderr, permissions/respond)
		api.GET("/agent/stream", s.handleAgentStreamWS)

		// Unified workspace stream (git status, files, shell)
		api.GET("/workspace/stream", s.handleWorkspaceStreamWS)

		// Workspace file operations (simple HTTP)
		api.GET("/workspace/tree", s.handleFileTree)
		api.GET("/workspace/file/content", s.handleFileContent)
		api.POST("/workspace/file/content", s.handleFileUpdate)
		api.POST("/workspace/file/create", s.handleFileCreate)
		api.DELETE("/workspace/file", s.handleFileDelete)
		api.GET("/workspace/search", s.handleFileSearch)

		// Shell access (HTTP endpoints only - streaming is via /workspace/stream)
		api.GET("/shell/status", s.handleShellStatus)
		api.GET("/shell/buffer", s.handleShellBuffer)
		api.POST("/shell/start", s.handleShellStart)

		// Process runner
		api.POST("/processes/start", s.handleStartProcess)
		api.POST("/processes/stop", s.handleStopProcess)
		api.GET("/processes", s.handleListProcesses)
		api.GET("/processes/:id", s.handleGetProcess)

		// VS Code server management
		api.POST("/vscode/start", s.handleVscodeStart)
		api.POST("/vscode/stop", s.handleVscodeStop)
		api.GET("/vscode/status", s.handleVscodeStatus)
		api.POST("/vscode/open-file", s.handleVscodeOpenFile)

		// Git operations
		api.POST("/git/pull", s.handleGitPull)
		api.POST("/git/push", s.handleGitPush)
		api.POST("/git/rebase", s.handleGitRebase)
		api.POST("/git/merge", s.handleGitMerge)
		api.POST("/git/abort", s.handleGitAbort)
		api.POST("/git/commit", s.handleGitCommit)
		api.POST("/git/stage", s.handleGitStage)
		api.POST("/git/unstage", s.handleGitUnstage)
		api.POST("/git/discard", s.handleGitDiscard)
		api.POST("/git/create-pr", s.handleGitCreatePR)
		api.GET("/git/commit/:sha", s.handleGitShowCommit)
	}

	// MCP routes (if MCP server is configured)
	if s.mcpServer != nil {
		s.mcpServer.RegisterRoutes(s.router)
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
	Command string `json:"command,omitempty"`
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
		Command: s.procMgr.GetFinalCommand(),
	})
}

// AgentConfigure request/response - configures the agent command before starting
type AgentConfigureRequest struct {
	Command         string            `json:"command"`
	ContinueCommand string            `json:"continue_command,omitempty"` // For one-shot agents (Amp): command for follow-up prompts
	Env             map[string]string `json:"env,omitempty"`
	ApprovalPolicy  string            `json:"approval_policy,omitempty"` // For Codex: "untrusted", "on-failure", "on-request", "never"
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

	if err := s.procMgr.Configure(req.Command, req.Env, req.ApprovalPolicy, req.ContinueCommand); err != nil {
		s.logger.Error("failed to configure agent", zap.Error(err), zap.String("command", req.Command))
		c.JSON(http.StatusInternalServerError, AgentConfigureResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("agent configured", zap.String("command", req.Command), zap.String("approval_policy", req.ApprovalPolicy))
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

// handleShellStart starts the shell session without starting the agent process.
// This is used in passthrough mode where the agent runs directly via InteractiveRunner.
func (s *Server) handleShellStart(c *gin.Context) {
	if err := s.procMgr.StartShell(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "shell started"})
}
