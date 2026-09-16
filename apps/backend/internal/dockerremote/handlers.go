// Package dockerremote serves the remote Docker executor's connection test.
// The probe itself lives behind the agent-runtime seam; this package only
// speaks HTTP.
package dockerremote

import (
	"bufio"
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// ExecutorFetcher looks up a saved executor so a build can target that
// executor's own daemon.
type ExecutorFetcher interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

// Handler serves the remote Docker connection test and image build.
type Handler struct {
	prober    *agentruntime.RemoteDockerProber
	builder   *agentruntime.RemoteDockerBuilder
	executors ExecutorFetcher
	logger    *logger.Logger
}

// NewHandler builds the handler with the production probe and build paths.
func NewHandler(executors ExecutorFetcher, log *logger.Logger) *Handler {
	return &Handler{
		prober:    agentruntime.NewRemoteDockerProber(log),
		builder:   agentruntime.NewRemoteDockerBuilder(log),
		executors: executors,
		logger:    log,
	}
}

// RegisterRoutes mounts the remote Docker routes.
func RegisterRoutes(router *gin.Engine, executors ExecutorFetcher, log *logger.Logger) {
	h := NewHandler(executors, log)
	// Both routes are administrative. The test dials a host the caller
	// names, and a saved profile grants effective root on that host; the
	// build then runs arbitrary Dockerfile instructions with the remote
	// daemon's authority. Kubernetes executors are gated the same way.
	router.POST("/api/v1/remote-docker/test", authn.RequireAdmin(), h.httpTest)
	router.POST("/api/v1/remote-docker/executors/:id/build", authn.RequireAdmin(), h.httpBuild)
}

// buildRequest is the body of POST /api/v1/remote-docker/executors/:id/build.
type buildRequest struct {
	Dockerfile string             `json:"dockerfile" binding:"required"`
	Tag        string             `json:"tag" binding:"required"`
	BuildArgs  map[string]*string `json:"build_args,omitempty"`
}

func (h *Handler) httpBuild(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor id required"})
		return
	}
	var req buildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	executor, err := h.executors.GetExecutor(c.Request.Context(), id)
	if err != nil || executor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "executor not found"})
		return
	}
	if executor.Type != models.ExecutorTypeRemoteDocker {
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor is not a remote Docker executor"})
		return
	}

	stream, err := h.builder.Build(c.Request.Context(), executor.Config, req.Dockerfile, req.Tag, req.BuildArgs)
	if err != nil {
		h.logger.Error("remote docker build failed to start",
			zap.String("executor_id", id), zap.String("tag", req.Tag), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			h.logger.Warn("failed to close remote build stream", zap.Error(closeErr))
		}
	}()

	c.Header("Content-Type", "application/x-ndjson")
	c.Status(http.StatusOK)
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		if _, writeErr := c.Writer.Write(append(scanner.Bytes(), '\n')); writeErr != nil {
			return
		}
		c.Writer.Flush()
	}
}

func (h *Handler) httpTest(c *gin.Context) {
	var req agentruntime.RemoteDockerTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.prober.Run(c.Request.Context(), req))
}
