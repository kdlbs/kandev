package utility

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

const inferenceErrorKey = "error"

// Handler provides HTTP handlers for inference operations.
type Handler struct {
	acpExecutor          *ACPInferenceExecutor
	codexExecutor        *CodexAppServerInferenceExecutor
	logger               *zap.Logger
	authenticatedControl func(*gin.Context) bool
}

// NewHandler creates a new inference handler.
func NewHandler(_ string, logger *zap.Logger) *Handler {
	return &Handler{
		acpExecutor:   NewACPInferenceExecutor(logger),
		codexExecutor: NewCodexAppServerInferenceExecutor(logger),
		logger:        logger,
	}
}

// RegisterRoutes registers the inference routes on the given router group.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	aux := api.Group("/inference")
	aux.POST("/prompt", h.handlePrompt)
	aux.POST("/probe", h.handleProbe)
}

// handleProbe handles POST /api/v1/inference/probe
func (h *Handler) handleProbe(c *gin.Context) {
	var req ProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProbeResponse{
			Success: false,
			Error:   "invalid inference request",
		})
		return
	}

	if !h.admitWrapper(c, req.InferenceConfig) {
		return
	}

	resp, err := h.probeExecutor(req).Probe(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("inference probe failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ProbeResponse{
			Success: false,
			Error:   "probe execution failed",
		})
		return
	}

	if !resp.Success {
		h.logger.Warn("ACP probe unsuccessful", zap.String(inferenceErrorKey, resp.Error))
	} else {
		h.logger.Info("ACP probe completed",
			zap.Int("duration_ms", resp.DurationMs),
			zap.Int("models", len(resp.Models)),
			zap.Int("modes", len(resp.Modes)))
	}

	c.JSON(http.StatusOK, resp)
}

// handlePrompt handles POST /api/v1/inference/prompt
func (h *Handler) handlePrompt(c *gin.Context) {
	var req PromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "invalid inference request",
		})
		return
	}

	if !h.admitWrapper(c, req.InferenceConfig) {
		return
	}

	h.logger.Info("executing inference prompt",
		zap.String("agent_id", req.AgentID),
		zap.String("model", req.Model),
	)

	resp, err := h.promptExecutor(req).Execute(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("inference prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   "inference execution failed",
		})
		return
	}

	if !resp.Success {
		h.logger.Warn("inference prompt unsuccessful", zap.String(inferenceErrorKey, resp.Error))
	} else {
		h.logger.Info("inference prompt completed", zap.Int("duration_ms", resp.DurationMs))
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) probeExecutor(req ProbeRequest) inferenceExecutor {
	if req.InferenceConfig != nil && req.InferenceConfig.Protocol == agent.ProtocolCodexAppServer {
		return h.codexExecutor
	}
	return h.acpExecutor
}

func (h *Handler) promptExecutor(req PromptRequest) inferenceExecutor {
	if req.InferenceConfig != nil && req.InferenceConfig.Protocol == agent.ProtocolCodexAppServer {
		return h.codexExecutor
	}
	return h.acpExecutor
}

type inferenceExecutor interface {
	Execute(context.Context, *PromptRequest) (*PromptResponse, error)
	Probe(context.Context, *ProbeRequest) (*ProbeResponse, error)
}

// SetControlAuthorization supplies the result of the server authentication middleware.
func (h *Handler) SetControlAuthorization(authenticated func(*gin.Context) bool) {
	h.authenticatedControl = authenticated
}

func (h *Handler) admitWrapper(c *gin.Context, cfg *InferenceConfigDTO) bool {
	if cfg == nil || cfg.LaunchWrapper.IsZero() {
		return true
	}
	if h.authenticatedControl == nil || !h.authenticatedControl(c) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, inferenceErrorKey: "authenticated control is required for launch wrappers"})
		return false
	}
	if cfg.LaunchWrapper.Validate() != nil || len(cfg.CommandPrefix) != 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, inferenceErrorKey: "invalid launch wrapper configuration"})
		return false
	}
	return true
}
