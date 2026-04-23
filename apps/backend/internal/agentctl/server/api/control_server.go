// Package api provides the HTTP REST API for agentctl control server
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ControlServer provides instance management endpoints on the control port.
// It exposes the same API regardless of deployment context (Docker or host).
// Each instance runs its own HTTP server with agent-specific endpoints.
type ControlServer struct {
	cfg     *config.Config
	instMgr *instance.Manager
	logger  *logger.Logger
	router  *gin.Engine
}

// NewControlServer creates a new ControlServer for instance management.
func NewControlServer(cfg *config.Config, instMgr *instance.Manager, log *logger.Logger) *ControlServer {
	gin.SetMode(gin.ReleaseMode)

	cs := &ControlServer{
		cfg:     cfg,
		instMgr: instMgr,
		logger:  log.WithFields(zap.String("component", "control-server")),
		router:  gin.New(),
	}

	cs.router.Use(httpmw.RequestLogger(cs.logger, "agentctl-control"))
	cs.router.Use(bearerTokenAuth(cfg.AuthToken, "/health", "/auth/handshake"))

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

	// Bootstrap handshake — nonce-authenticated, returns the self-generated auth token
	m.router.POST("/auth/handshake", m.handleHandshake)

	// Instance management API - same endpoints regardless of mode
	api := m.router.Group("/api/v1")
	api.POST("/instances", m.handleCreateInstance)
	api.GET("/instances", m.handleListInstances)
	api.GET("/instances/:id", m.handleGetInstance)
	api.DELETE("/instances/:id", m.handleDeleteInstance)
}

func (m *ControlServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleHandshake validates a bootstrap nonce and returns the self-generated auth token.
// This endpoint is only available when agentctl was started with AGENTCTL_BOOTSTRAP_NONCE.
// The nonce is one-shot: after a successful handshake, further attempts are rejected.
func (m *ControlServer) handleHandshake(c *gin.Context) {
	var req struct {
		Nonce string `json:"nonce" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid nonce"})
		return
	}

	token := m.cfg.ConsumeNonce(req.Nonce)
	if token == "" {
		m.logger.Warn("handshake failed: invalid or already-consumed nonce")
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid or already-consumed nonce"})
		return
	}

	m.logger.Info("bootstrap handshake completed, auth token issued")
	c.JSON(http.StatusOK, gin.H{"token": token})
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

	// Use a detached context so cleanup isn't canceled when the HTTP client disconnects.
	// The ControlClient has a 30s http.Client.Timeout; if it fires, the request context
	// is canceled, which cascades into force-killing processes mid-cleanup.
	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := m.instMgr.StopInstance(stopCtx, id)
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
