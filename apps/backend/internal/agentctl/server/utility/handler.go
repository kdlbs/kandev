package utility

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for inference operations.
type Handler struct {
	executor *ACPInferenceExecutor
	logger   *zap.Logger
}

// NewHandler creates a new inference handler.
func NewHandler(_ string, logger *zap.Logger) *Handler {
	return &Handler{
		executor: NewACPInferenceExecutor(logger),
		logger:   logger,
	}
}

// RegisterRoutes registers the inference routes on the given router group.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	aux := api.Group("/inference")
	aux.POST("/prompt", h.handlePrompt)
}

// handlePrompt handles POST /api/v1/inference/prompt
func (h *Handler) handlePrompt(c *gin.Context) {
	var req PromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	h.logger.Info("executing inference prompt",
		zap.String("agent_id", req.AgentID),
		zap.String("model", req.Model),
	)

	resp, err := h.executor.Execute(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("inference prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   "inference execution failed",
		})
		return
	}

	if !resp.Success {
		h.logger.Warn("inference prompt unsuccessful", zap.String("error", resp.Error))
	} else {
		h.logger.Info("inference prompt completed", zap.Int("duration_ms", resp.DurationMs))
	}

	c.JSON(http.StatusOK, resp)
}
