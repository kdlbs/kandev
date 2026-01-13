// Package api provides the HTTP REST API for agentctl control server
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/config"
	"github.com/kandev/kandev/internal/agentctl/instance"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ControlServer provides instance CRUD endpoints on the control port (9999).
type ControlServer struct {
	cfg     *config.MultiConfig
	instMgr *instance.Manager
	logger  *logger.Logger
	router  *gin.Engine
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

// Router returns the HTTP handler for the control server.
func (m *ControlServer) Router() http.Handler {
	return m.router
}

func (m *ControlServer) setupRoutes() {
	// Health check
	m.router.GET("/health", m.handleHealth)

	// Instance management API
	api := m.router.Group("/api/v1")
	{
		api.POST("/instances", m.handleCreateInstance)
		api.GET("/instances", m.handleListInstances)
		api.GET("/instances/:id", m.handleGetInstance)
		api.DELETE("/instances/:id", m.handleDeleteInstance)
	}
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

