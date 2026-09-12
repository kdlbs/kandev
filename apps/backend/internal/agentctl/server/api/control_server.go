// Package api provides the HTTP REST API for agentctl control server
package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ownershipperiod"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

// ControlServer provides instance management endpoints on the control port.
// It exposes the same API regardless of deployment context (Docker or host).
// Each instance runs its own HTTP server with agent-specific endpoints.
// adoptionOnlyPaths lists control-server routes where a superseded
// credential remains valid on the adoption-only terms of
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.7/-002.9, until confirmed. Every other
// authenticated route accepts only the current highest-numbered rotation's
// credential.
var adoptionOnlyPaths = map[string]bool{
	"/api/v1/ownership/rotate":   true,
	"/api/v1/ownership/shutdown": true,
}

type ControlServer struct {
	cfg               *config.Config
	instMgr           *instance.Manager
	logger            *logger.Logger
	router            *gin.Engine
	ownership         *ownershipState
	credentials       *credentialState
	shutdownRequested chan struct{}
	shutdownOnce      sync.Once
	unownedPeriod     time.Duration
	reaperStop        chan struct{}
	reaperStopOnce    sync.Once
	reaperWG          sync.WaitGroup

	// decideUnownedShutdown is the reaper's shutdown decision, defaulted to
	// ownership.TryBeginShutdownIfUnownedFor. Tests override it to pin the
	// exact call the reaper makes rather than only observing its side
	// effects, so reverting to a stale two-call sequence (check then latch,
	// instead of the atomic check-and-latch) fails a test even though it
	// would still pass every other reaper assertion.
	decideUnownedShutdown func(time.Duration) bool
}

// NewControlServer creates a new ControlServer for instance management.
func NewControlServer(cfg *config.Config, instMgr *instance.Manager, log *logger.Logger) *ControlServer {
	gin.SetMode(gin.ReleaseMode)

	csLogger := log.WithFields(zap.String("component", "control-server"))
	unownedPeriod, adjustments := ownershipperiod.Resolve(cfg.UnownedPeriod, cfg.IdleTimeout)
	for _, reason := range adjustments {
		csLogger.Warn(reason,
			zap.Duration("configured_unowned_period", cfg.UnownedPeriod),
			zap.Duration("idle_timeout", cfg.IdleTimeout),
			zap.Duration("effective_unowned_period", unownedPeriod))
	}

	cs := &ControlServer{
		cfg:               cfg,
		instMgr:           instMgr,
		logger:            csLogger,
		router:            gin.New(),
		ownership:         newOwnershipState(),
		credentials:       newCredentialState(cfg.AuthToken),
		shutdownRequested: make(chan struct{}),
		unownedPeriod:     unownedPeriod,
		reaperStop:        make(chan struct{}),
	}

	cs.decideUnownedShutdown = cs.ownership.TryBeginShutdownIfUnownedFor

	cs.router.Use(httpmw.RequestLogger(cs.logger, "agentctl-control"))
	cs.router.Use(controlCredentialAuth(cs.credentials, adoptionOnlyPaths, "/health", "/auth/handshake", "/identity", "/ownership/prove"))

	cs.setupRoutes()
	return cs
}

// Router returns the HTTP handler for the control server.
func (m *ControlServer) Router() http.Handler {
	return m.router
}

// CredentialSource exposes this control server's single rotating credential
// so a per-instance Server can authenticate its own requests and streams
// against it instead of a static per-instance token (design 01 "Single
// driver", AC-EXECUTORS-CONTROL-OWNERSHIP-002.6). Wire it via
// Server.SetCredentialSource before an instance starts accepting requests.
func (m *ControlServer) CredentialSource() InstanceCredentialSource {
	return m.credentials
}

// ShutdownRequested returns a channel that closes exactly once the
// ownership-shutdown operation (AC-EXECUTORS-CONTROL-OWNERSHIP-002.9) has
// been invoked. The run loop (cmd/agentctl/main.go) selects on this
// alongside its OS-signal and parent-death triggers and runs the same
// stop-every-instance-and-exit sequence.
func (m *ControlServer) ShutdownRequested() <-chan struct{} {
	return m.shutdownRequested
}

// requestShutdown signals ShutdownRequested exactly once. Safe to call more
// than once (a repeated ownership-shutdown call is a no-op beyond the first).
func (m *ControlServer) requestShutdown() {
	m.shutdownOnce.Do(func() { close(m.shutdownRequested) })
}

func (m *ControlServer) setupRoutes() {
	// Health check
	m.router.GET("/health", m.handleHealth)

	// Bootstrap handshake — nonce-authenticated, returns the self-generated auth token
	m.router.POST("/auth/handshake", m.handleHandshake)

	// Identity/capability — unauthenticated, decides adoption compatibility
	m.router.GET("/identity", m.handleIdentity)

	// Ownership proof — unauthenticated, establishes to an adopting backend
	// that this process holds the credential before that backend sends it
	m.router.POST("/ownership/prove", m.handleOwnershipProve)

	// Instance management API - same endpoints regardless of mode
	api := m.router.Group("/api/v1")
	api.POST("/instances", m.handleCreateInstance)
	api.GET("/instances", m.handleListInstances)
	api.GET("/instances/:id", m.handleGetInstance)
	api.DELETE("/instances/:id", m.handleDeleteInstance)
	api.GET("/instances/:id/turn-outcome", m.handleGetTurnOutcome)
	api.POST("/instances/:id/turn-outcome/ack", m.handleAckTurnOutcome)
	api.GET("/debug/subprocess-admission", m.handleSubprocessAdmission)
	api.GET("/ownership/details", m.handleOwnershipDetails)
	api.POST("/ownership/claim", m.handleOwnershipClaim)
	api.POST("/ownership/rotate", m.handleCredentialRotate)
	api.POST("/ownership/confirm", m.handleCredentialConfirm)
	api.POST("/ownership/shutdown", m.handleOwnershipShutdown)
}

func (m *ControlServer) handleSubprocessAdmission(c *gin.Context) {
	c.JSON(http.StatusOK, subproc.AdmissionSnapshot())
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
		// A rejected handshake has three distinct causes that otherwise look
		// identical from the outside: bootstrap nonce mode was never
		// configured, the received nonce doesn't match what agentctl started
		// with, or the configured nonce was already burned by an earlier
		// handshake. Logging both fingerprints lets a field 403 be attributed
		// to one of the three instead of guessed at.
		m.logger.Warn("handshake failed: invalid or already-consumed nonce",
			zap.Bool("bootstrap_nonce_configured", m.cfg.BootstrapNonceConfigured()),
			zap.String("configured_nonce_fingerprint", m.cfg.BootstrapNonceFingerprint()),
			zap.String("received_nonce_fingerprint", commonconfig.NonceFingerprint(req.Nonce)))
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid or already-consumed nonce"})
		return
	}

	m.ownership.Renew()
	m.logger.Info("bootstrap handshake completed, auth token issued",
		zap.String("nonce_fingerprint", commonconfig.NonceFingerprint(req.Nonce)))
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
	c.JSON(http.StatusOK, gin.H{"instances": instances})
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
