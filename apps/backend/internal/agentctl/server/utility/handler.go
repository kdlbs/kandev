package utility

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

const inferenceErrorKey = "error"

const (
	probeDiagnosticOperation = "agent_probe"
	probeDiagnosticUnknown   = "unknown"
)

// Handler provides HTTP handlers for inference operations.
type Handler struct {
	attempts             *attemptOwner
	closureStop          func(context.Context) (*ManagedProcessStopReceipt, error)
	beginOperation       func(context.Context) (context.Context, func(), error)
	acpExecutor          inferenceExecutor
	codexExecutor        inferenceExecutor
	logger               *zap.Logger
	authenticatedControl func(*gin.Context) bool
	instanceID           string
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
	aux.GET("/attempts", h.handleAttempts)
	aux.POST("/closure", h.handleInferenceClosure)
	aux.POST("/attempts", h.handlePrepareAttempt)
	aux.GET("/attempts/:id", h.handleAttempt)
	aux.POST("/attempts/:id/cancel", h.handleCancelPreparedAttempt)
	aux.POST("/attempts/:id/admit", h.handleAdmitCapacityAttempt)
	aux.POST("/attempts/:id/release", h.handleReleaseRetainedAttempt)
	aux.POST("/preparations/:id/cancel", h.handleCancelPreparation)
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
		h.logger.Error("inference probe failed",
			append(h.probeExecutionFailureFields(err), zap.Error(err))...)
		c.JSON(http.StatusInternalServerError, ProbeResponse{
			Success: false,
			Error:   "probe execution failed",
		})
		return
	}

	if !resp.Success {
		h.logger.Warn("ACP probe unsuccessful",
			append(h.probeFailureSummaryFields(resp), zap.String(inferenceErrorKey, resp.Error))...)
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

	resp, err := h.executePrompt(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("inference prompt failed", zap.Error(err))
		if resp != nil && resp.Attempt != nil {
			c.JSON(promptErrorStatus(err), resp)
			return
		}
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

// diagnosticFields returns bounded context for a probe failure record. The
// operation and class are fixed labels; the instance identity is included only
// when the trusted configuration supplied a canonical UUID.
func (h *Handler) diagnosticFields(errorClass string) []zap.Field {
	fields := []zap.Field{
		zap.String("operation", probeDiagnosticOperation),
		zap.String("error_class", errorClass),
	}
	if h.instanceID != "" {
		fields = append(fields, zap.String("instance_id", h.instanceID))
	}
	return fields
}

// probeFailureSummaryFields returns bounded context for a failed probe
// response. Only the four declared probe failure codes keep their own class;
// every other cause stays an explicit unknown.
func (h *Handler) probeFailureSummaryFields(resp *ProbeResponse) []zap.Field {
	return h.diagnosticFields(probeFailureCodeClass(resp.FailureCode))
}

func probeFailureCodeClass(code ProbeFailureCode) string {
	switch code {
	case ProbeFailureManagedRuntimeNPMResolution,
		ProbeFailureManagedRuntimeNPMPolicy,
		ProbeFailureUnsupportedContext,
		ProbeFailureAuthenticationRequired:
		return string(code)
	default:
		return probeDiagnosticUnknown
	}
}

// probeExecutionFailureFields returns bounded context for a probe that failed
// before it produced a response.
func (h *Handler) probeExecutionFailureFields(err error) []zap.Field {
	class := probeDiagnosticUnknown
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		class = "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		class = "context_canceled"
	}
	return h.diagnosticFields(class)
}

func isCanonicalUUID(value string) bool {
	if len(value) != 36 || strings.ToLower(value) != value {
		return false
	}
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
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

// SetInstanceID supplies the trusted instance identity for failure
// diagnostics. Invalid or missing identity stays omitted. Call once before
// serving requests.
func (h *Handler) SetInstanceID(instanceID string) {
	if isCanonicalUUID(instanceID) {
		h.instanceID = instanceID
	}
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
