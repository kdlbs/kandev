// Package api provides the HTTP REST API for agentctl control server
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/config"
	"github.com/kandev/kandev/internal/agentctl/instance"
	"github.com/kandev/kandev/internal/agentctl/process"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ControlServer provides instance CRUD endpoints on the control port (9999).
// In single-instance mode, it also embeds the agent API routes directly.
type ControlServer struct {
	cfg     *config.MultiConfig
	instMgr *instance.Manager
	logger  *logger.Logger
	router  *gin.Engine

	// Single-instance mode: embedded process manager and config
	singleMode   bool
	singleCfg    *config.Config
	singleProcMgr *process.Manager
	upgrader     websocket.Upgrader
}

// NewControlServer creates a new ControlServer for multi-instance management.
func NewControlServer(cfg *config.MultiConfig, instMgr *instance.Manager, log *logger.Logger) *ControlServer {
	gin.SetMode(gin.ReleaseMode)

	cs := &ControlServer{
		cfg:     cfg,
		instMgr: instMgr,
		logger:  log.WithFields(zap.String("component", "control-server")),
		router:  gin.New(),
	}

	cs.setupRoutes()
	return cs
}

// NewSingleInstanceServer creates a ControlServer for single-instance (Docker) mode.
// It embeds the agent API routes directly on the control server.
func NewSingleInstanceServer(cfg *config.MultiConfig, singleCfg *config.Config, procMgr *process.Manager, log *logger.Logger) *ControlServer {
	gin.SetMode(gin.ReleaseMode)

	cs := &ControlServer{
		cfg:           cfg,
		logger:        log.WithFields(zap.String("component", "control-server")),
		router:        gin.New(),
		singleMode:    true,
		singleCfg:     singleCfg,
		singleProcMgr: procMgr,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for container-local communication
			},
		},
	}

	cs.setupRoutes()
	return cs
}

// Router returns the HTTP handler for the control server.
func (m *ControlServer) Router() http.Handler {
	return m.router
}

func (m *ControlServer) setupRoutes() {
	// Health check
	m.router.GET("/health", m.handleHealth)

	api := m.router.Group("/api/v1")

	if m.singleMode {
		// Single-instance mode: mount agent API routes directly
		m.setupSingleInstanceRoutes(api)
	} else {
		// Multi-instance mode: instance management API
		api.POST("/instances", m.handleCreateInstance)
		api.GET("/instances", m.handleListInstances)
		api.GET("/instances/:id", m.handleGetInstance)
		api.DELETE("/instances/:id", m.handleDeleteInstance)
	}
}

// setupSingleInstanceRoutes adds the agent API routes for single-instance mode.
// These are the same routes that api.Server provides.
func (m *ControlServer) setupSingleInstanceRoutes(api *gin.RouterGroup) {
	// Status and info
	api.GET("/status", m.handleStatus)
	api.GET("/info", m.handleInfo)

	// Process control
	api.POST("/start", m.handleStart)
	api.POST("/stop", m.handleStop)

	// ACP high-level methods
	api.POST("/acp/initialize", m.handleACPInitialize)
	api.POST("/acp/session/new", m.handleACPNewSession)
	api.POST("/acp/prompt", m.handleACPPrompt)
	api.GET("/acp/stream", m.handleACPStreamWS)

	// Permission request handling
	api.GET("/acp/permissions", m.handleGetPendingPermissions)
	api.GET("/acp/permissions/stream", m.handlePermissionStreamWS)
	api.POST("/acp/permissions/respond", m.handlePermissionRespond)

	// Workspace monitoring (git status, files)
	api.GET("/workspace/git-status/stream", m.handleGitStatusStreamWS)
	api.GET("/workspace/files/stream", m.handleFilesStreamWS)
	api.GET("/workspace/file-changes/stream", m.handleFileChangesStreamWS)

	// Workspace file operations (simple HTTP)
	api.GET("/workspace/tree", m.handleFileTree)
	api.GET("/workspace/file/content", m.handleFileContent)
}

func (m *ControlServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"mode":      m.cfg.Mode,
	})
}

func (m *ControlServer) handleCreateInstance(c *gin.Context) {
	var req instance.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		m.logger.Warn("invalid create instance request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	// Validate required fields
	if req.WorkspacePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workspace_path is required",
		})
		return
	}

	resp, err := m.instMgr.CreateInstance(c.Request.Context(), &req)
	if err != nil {
		m.logger.Error("failed to create instance", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create instance: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (m *ControlServer) handleListInstances(c *gin.Context) {
	instances := m.instMgr.ListInstances()
	c.JSON(http.StatusOK, instances)
}

func (m *ControlServer) handleGetInstance(c *gin.Context) {
	id := c.Param("id")

	inst, found := m.instMgr.GetInstance(id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "instance not found",
		})
		return
	}

	c.JSON(http.StatusOK, inst.Info())
}

func (m *ControlServer) handleDeleteInstance(c *gin.Context) {
	id := c.Param("id")

	// First check if instance exists
	if _, found := m.instMgr.GetInstance(id); !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "instance not found",
		})
		return
	}

	err := m.instMgr.StopInstance(c.Request.Context(), id)
	if err != nil {
		m.logger.Error("failed to stop instance", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to stop instance: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "instance stopped successfully",
	})
}

// =============================================================================
// Single-instance mode handlers
// These mirror the handlers in server.go but use singleProcMgr
// =============================================================================

func (m *ControlServer) handleStatus(c *gin.Context) {
	c.JSON(http.StatusOK, StatusResponse{
		AgentStatus: string(m.singleProcMgr.Status()),
		ProcessInfo: m.singleProcMgr.GetProcessInfo(),
	})
}

func (m *ControlServer) handleInfo(c *gin.Context) {
	c.JSON(http.StatusOK, InfoResponse{
		Version:      "0.1.0",
		AgentCommand: m.singleCfg.AgentCommand,
		WorkDir:      m.singleCfg.WorkDir,
		Port:         m.singleCfg.Port,
	})
}

func (m *ControlServer) handleStart(c *gin.Context) {
	if err := m.singleProcMgr.Start(c.Request.Context()); err != nil {
		m.logger.Error("failed to start agent", zap.Error(err))
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

func (m *ControlServer) handleStop(c *gin.Context) {
	if err := m.singleProcMgr.Stop(c.Request.Context()); err != nil {
		m.logger.Error("failed to stop agent", zap.Error(err))
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


// ACP handlers for single-instance mode

func (m *ControlServer) handleACPInitialize(c *gin.Context) {
	var req InitializeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, InitializeResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	adapter := m.singleProcMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, InitializeResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	err := adapter.Initialize(ctx)
	if err != nil {
		m.logger.Error("initialize failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, InitializeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	var agentInfoResp *AgentInfoResponse
	if info := adapter.GetAgentInfo(); info != nil {
		agentInfoResp = &AgentInfoResponse{
			Name:    info.Name,
			Version: info.Version,
		}
	}

	c.JSON(http.StatusOK, InitializeResponse{
		Success:   true,
		AgentInfo: agentInfoResp,
	})
}

func (m *ControlServer) handleACPNewSession(c *gin.Context) {
	var req NewSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, NewSessionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	adapter := m.singleProcMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, NewSessionResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	sessionID, err := adapter.NewSession(ctx)
	if err != nil {
		m.logger.Error("new session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, NewSessionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, NewSessionResponse{
		Success:   true,
		SessionID: sessionID,
	})
}

func (m *ControlServer) handleACPPrompt(c *gin.Context) {
	var req PromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	adapter := m.singleProcMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, PromptResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	sessionID := m.singleProcMgr.GetSessionID()
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "no active session - call new_session first",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	err := adapter.Prompt(ctx, req.Text)
	if err != nil {
		m.logger.Error("prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	m.logger.Info("prompt completed")
	c.JSON(http.StatusOK, PromptResponse{
		Success: true,
	})
}


func (m *ControlServer) handleACPStreamWS(c *gin.Context) {
	conn, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	m.logger.Info("ACP stream WebSocket connected")

	updatesCh := m.singleProcMgr.GetUpdates()

	for {
		select {
		case notification, ok := <-updatesCh:
			if !ok {
				return
			}

			data, err := json.Marshal(notification)
			if err != nil {
				m.logger.Error("failed to marshal notification", zap.Error(err))
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

func (m *ControlServer) handleGetPendingPermissions(c *gin.Context) {
	pending := m.singleProcMgr.GetPendingPermissions()

	result := make([]PendingPermissionResponse, 0, len(pending))
	for _, p := range pending {
		options := make([]PermissionOptionJSON, len(p.Request.Options))
		for i, opt := range p.Request.Options {
			options[i] = PermissionOptionJSON{
				OptionID: opt.OptionID,
				Name:     opt.Name,
				Kind:     opt.Kind,
			}
		}
		result = append(result, PendingPermissionResponse{
			PendingID:  p.ID,
			SessionID:  p.Request.SessionID,
			ToolCallID: p.Request.ToolCallID,
			Title:      p.Request.Title,
			Options:    options,
			CreatedAt:  p.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, result)
}

func (m *ControlServer) handlePermissionStreamWS(c *gin.Context) {
	conn, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	m.logger.Info("Permission stream WebSocket connected")

	permissionCh := m.singleProcMgr.GetPermissionRequests()

	for {
		select {
		case notification, ok := <-permissionCh:
			if !ok {
				return
			}

			data, err := json.Marshal(notification)
			if err != nil {
				m.logger.Error("failed to marshal permission notification", zap.Error(err))
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				m.logger.Debug("WebSocket write error", zap.Error(err))
				return
			}
		}
	}
}

func (m *ControlServer) handlePermissionRespond(c *gin.Context) {
	var req PermissionRespondRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PermissionRespondResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	m.logger.Info("received permission response",
		zap.String("pending_id", req.PendingID),
		zap.String("option_id", req.OptionID),
		zap.Bool("cancelled", req.Cancelled))

	if err := m.singleProcMgr.RespondToPermission(req.PendingID, req.OptionID, req.Cancelled); err != nil {
		m.logger.Error("failed to respond to permission", zap.Error(err))
		c.JSON(http.StatusNotFound, PermissionRespondResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, PermissionRespondResponse{
		Success: true,
	})
}


// Workspace handlers for single-instance mode

func (m *ControlServer) handleGitStatusStreamWS(c *gin.Context) {
	conn, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	sub := m.singleProcMgr.GetWorkspaceTracker().SubscribeGitStatus()
	defer m.singleProcMgr.GetWorkspaceTracker().UnsubscribeGitStatus(sub)

	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	for {
		select {
		case update, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(update)
			if err != nil {
				m.logger.Error("failed to marshal git status update", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

func (m *ControlServer) handleFilesStreamWS(c *gin.Context) {
	conn, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	sub := m.singleProcMgr.GetWorkspaceTracker().SubscribeFiles()
	defer m.singleProcMgr.GetWorkspaceTracker().UnsubscribeFiles(sub)

	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	for {
		select {
		case update, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(update)
			if err != nil {
				m.logger.Error("failed to marshal file update", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

func (m *ControlServer) handleFileChangesStreamWS(c *gin.Context) {
	conn, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	sub := m.singleProcMgr.GetWorkspaceTracker().SubscribeFileChanges()
	defer m.singleProcMgr.GetWorkspaceTracker().UnsubscribeFileChanges(sub)

	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	for {
		select {
		case notification, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(notification)
			if err != nil {
				m.logger.Error("failed to marshal file change notification", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

func (m *ControlServer) handleFileTree(c *gin.Context) {
	path := c.Query("path")
	depth := 1
	if d := c.Query("depth"); d != "" {
		if _, err := json.Number(d).Int64(); err == nil {
			depth = int(mustParseInt(d))
		}
	}

	tree, err := m.singleProcMgr.GetWorkspaceTracker().GetFileTree(path, depth)
	if err != nil {
		c.JSON(400, process.FileTreeResponse{Error: err.Error()})
		return
	}

	c.JSON(200, process.FileTreeResponse{Root: tree})
}

func (m *ControlServer) handleFileContent(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, process.FileContentResponse{Error: "path is required"})
		return
	}

	content, size, err := m.singleProcMgr.GetWorkspaceTracker().GetFileContent(path)
	if err != nil {
		c.JSON(400, process.FileContentResponse{Path: path, Error: err.Error(), Size: size})
		return
	}

	c.JSON(200, process.FileContentResponse{Path: path, Content: content, Size: size})
}