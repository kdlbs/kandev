package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/utility/controller"
	"github.com/kandev/kandev/internal/utility/dto"
	"github.com/kandev/kandev/internal/utility/service"
)

// InferenceExecutor executes inference prompts via agentctl.
type InferenceExecutor interface {
	// ExecuteInferencePrompt executes an inference prompt via an active session's agentctl.
	ExecuteInferencePrompt(ctx context.Context, sessionID, agentID, model, prompt string) (*agentctlutil.PromptResponse, error)
	// ListInferenceAgentsWithContext returns installed agents that support inference.
	ListInferenceAgentsWithContext(ctx context.Context) []lifecycle.InferenceAgentInfo
}

// HostUtilityExecutor runs sessionless utility prompts via the long-lived
// per-agent-type host agentctl instances and exposes the cached per-agent
// capabilities (models, modes) populated by the boot-time ACP probe.
type HostUtilityExecutor interface {
	ExecutePrompt(ctx context.Context, agentType, model, mode, prompt string) (*hostutility.PromptResult, error)
	Get(agentType string) (hostutility.AgentCapabilities, bool)
}

// UserSettingsProvider provides user settings for default utility agent/model.
type UserSettingsProvider interface {
	// GetDefaultUtilitySettings returns the user's default utility agent/model settings.
	GetDefaultUtilitySettings(ctx context.Context) (agentID, model string, err error)
}

// Handlers provides HTTP handlers for utility agents.
type Handlers struct {
	controller   *controller.Controller
	executor     InferenceExecutor
	hostExecutor HostUtilityExecutor
	userSettings UserSettingsProvider
	logger       *logger.Logger
}

// NewHandlers creates new utility agent handlers.
func NewHandlers(ctrl *controller.Controller, executor InferenceExecutor, hostExecutor HostUtilityExecutor, userSettings UserSettingsProvider, log *logger.Logger) *Handlers {
	return &Handlers{
		controller:   ctrl,
		executor:     executor,
		hostExecutor: hostExecutor,
		userSettings: userSettings,
		logger:       log.WithFields(zap.String("component", "utility-handlers")),
	}
}

// RegisterRoutes registers the utility agent routes.
func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, executor InferenceExecutor, hostExecutor HostUtilityExecutor, userSettings UserSettingsProvider, log *logger.Logger) {
	handlers := NewHandlers(ctrl, executor, hostExecutor, userSettings, log)
	api := router.Group("/api/v1/utility")
	api.GET("/agents", handlers.httpListAgents)
	api.GET("/agents/:id", handlers.httpGetAgent)
	api.POST("/agents", handlers.httpCreateAgent)
	api.PATCH("/agents/:id", handlers.httpUpdateAgent)
	api.DELETE("/agents/:id", handlers.httpDeleteAgent)
	api.GET("/template-variables", handlers.httpGetTemplateVariables)
	api.POST("/execute", handlers.httpExecutePrompt)
	api.GET("/agents/:id/calls", handlers.httpListCalls)
	api.GET("/inference-agents", handlers.httpListInferenceAgents)
}

func (h *Handlers) httpListAgents(c *gin.Context) {
	resp, err := h.controller.ListAgents(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list utility agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list utility agents"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpGetAgent(c *gin.Context) {
	resp, err := h.controller.GetAgent(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrAgentNotFound) {
			status = http.StatusNotFound
		}
		h.logger.Error("failed to get utility agent", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to get utility agent"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpCreateAgent(c *gin.Context) {
	var req dto.CreateUtilityAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.CreateAgent(c.Request.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidAgent) {
			status = http.StatusBadRequest
		}
		h.logger.Error("failed to create utility agent", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to create utility agent"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *Handlers) httpUpdateAgent(c *gin.Context) {
	var req dto.UpdateUtilityAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateAgent(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, service.ErrAgentNotFound):
			status = http.StatusNotFound
		case errors.Is(err, service.ErrInvalidAgent):
			status = http.StatusBadRequest
		case errors.Is(err, service.ErrBuiltinAgent):
			status = http.StatusForbidden
		}
		h.logger.Error("failed to update utility agent", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to update utility agent"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpDeleteAgent(c *gin.Context) {
	if err := h.controller.DeleteAgent(c.Request.Context(), c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, service.ErrAgentNotFound):
			status = http.StatusNotFound
		case errors.Is(err, service.ErrBuiltinAgent):
			status = http.StatusForbidden
		}
		h.logger.Error("failed to delete utility agent", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to delete utility agent"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handlers) httpGetTemplateVariables(c *gin.Context) {
	resp := h.controller.GetTemplateVariables(c.Request.Context())
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpExecutePrompt(c *gin.Context) {
	var req dto.ExecutePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ExecutePromptResponse{Error: "invalid payload"})
		return
	}

	ctx := c.Request.Context()

	// Get default utility settings from user settings
	var defaults *service.DefaultUtilitySettings
	if h.userSettings != nil {
		agentID, model, err := h.userSettings.GetDefaultUtilitySettings(ctx)
		if err == nil && (agentID != "" || model != "") {
			defaults = &service.DefaultUtilitySettings{AgentID: agentID, Model: model}
		}
	}

	sessionless := req.SessionID == ""

	// Prepare the prompt request (resolve template, get agent/model info)
	prepared, err := h.controller.PreparePromptRequest(ctx, req, defaults, sessionless)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrAgentNotFound) {
			status = http.StatusNotFound
		}
		h.logger.Error("failed to prepare prompt", zap.Error(err))
		c.JSON(status, dto.ExecutePromptResponse{Error: "failed to prepare prompt"})
		return
	}

	// Validate inference agent is resolved before persisting a call record —
	// missing agent_id is a client error, not an execution failure.
	if prepared.AgentCLI == "" {
		h.logger.Warn("execute prompt: missing agent_id")
		c.JSON(http.StatusBadRequest, dto.ExecutePromptResponse{
			Error: lifecycle.ErrInferenceAgentIDRequired.Error(),
		})
		return
	}

	// Create call record for tracking
	callID, err := h.controller.CreateCall(ctx, req.UtilityAgentID, req.SessionID, prepared.ResolvedPrompt, prepared.Model)
	if err != nil {
		h.logger.Error("failed to create call record", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ExecutePromptResponse{Error: "failed to create call record"})
		return
	}

	if sessionless {
		h.executeSessionless(c, ctx, prepared, callID)
		return
	}

	// Execute via agentctl using an existing task session's agentctl
	resp, err := h.executor.ExecuteInferencePrompt(ctx, req.SessionID, prepared.AgentCLI, prepared.Model, prepared.ResolvedPrompt)
	if err != nil {
		h.logger.Error("failed to execute prompt", zap.Error(err), zap.String("call_id", callID))
		_ = h.controller.FailCall(ctx, callID, err.Error(), 0)
		c.JSON(http.StatusInternalServerError, dto.ExecutePromptResponse{
			CallID: callID,
			Error:  "failed to execute prompt: " + err.Error(),
		})
		return
	}

	if !resp.Success {
		_ = h.controller.FailCall(ctx, callID, resp.Error, resp.DurationMs)
		c.JSON(http.StatusOK, dto.ExecutePromptResponse{
			CallID:     callID,
			Error:      resp.Error,
			DurationMs: resp.DurationMs,
		})
		return
	}

	// Mark call as completed
	if err := h.controller.CompleteCall(ctx, callID, resp.Response, resp.PromptTokens, resp.ResponseTokens, resp.DurationMs); err != nil {
		h.logger.Warn("failed to update call record", zap.Error(err), zap.String("call_id", callID))
	}

	c.JSON(http.StatusOK, dto.ExecutePromptResponse{
		Success:        true,
		CallID:         callID,
		Response:       resp.Response,
		Model:          resp.Model,
		PromptTokens:   resp.PromptTokens,
		ResponseTokens: resp.ResponseTokens,
		DurationMs:     resp.DurationMs,
	})
}

// executeSessionless runs the prepared prompt through the host utility manager
// (no task session). Used for flows like "enhance prompt" in the new-task modal.
func (h *Handlers) executeSessionless(c *gin.Context, ctx context.Context, prepared *service.PromptRequest, callID string) {
	if h.hostExecutor == nil {
		_ = h.controller.FailCall(ctx, callID, "host utility not configured", 0)
		c.JSON(http.StatusServiceUnavailable, dto.ExecutePromptResponse{
			CallID: callID,
			Error:  "host utility not configured; session_id is required",
		})
		return
	}
	result, err := h.hostExecutor.ExecutePrompt(ctx, prepared.AgentCLI, prepared.Model, "", prepared.ResolvedPrompt)
	if err != nil {
		h.logger.Error("failed to execute sessionless prompt", zap.Error(err), zap.String("call_id", callID))
		_ = h.controller.FailCall(ctx, callID, err.Error(), 0)
		c.JSON(http.StatusInternalServerError, dto.ExecutePromptResponse{
			CallID: callID,
			Error:  "failed to execute prompt: " + err.Error(),
		})
		return
	}
	if err := h.controller.CompleteCall(ctx, callID, result.Response, result.PromptTokens, result.ResponseTokens, result.DurationMs); err != nil {
		h.logger.Warn("failed to update call record", zap.Error(err), zap.String("call_id", callID))
	}
	c.JSON(http.StatusOK, dto.ExecutePromptResponse{
		Success:        true,
		CallID:         callID,
		Response:       result.Response,
		Model:          result.Model,
		PromptTokens:   result.PromptTokens,
		ResponseTokens: result.ResponseTokens,
		DurationMs:     result.DurationMs,
	})
}

func (h *Handlers) httpListCalls(c *gin.Context) {
	utilityID := c.Param("id")
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	resp, err := h.controller.ListCalls(c.Request.Context(), utilityID, limit)
	if err != nil {
		h.logger.Error("failed to list calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list calls"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpListInferenceAgents(c *gin.Context) {
	inferenceAgents := h.executor.ListInferenceAgentsWithContext(c.Request.Context())

	// Build the response from the host utility capability cache (boot-time
	// ACP probe). Only include agents whose probe reached StatusOK — an
	// agent that isn't authenticated, not installed, or still probing
	// can't actually run a utility prompt, so showing it in the picker
	// just leads the user into a dead end.
	//
	// hostExecutor is an optional dependency (see executeSessionless);
	// without it we can't check health or surface models, so the list is
	// empty by design rather than showing unusable options.
	result := make([]dto.InferenceAgentDTO, 0, len(inferenceAgents))
	if h.hostExecutor == nil {
		c.JSON(http.StatusOK, dto.InferenceAgentsResponse{Agents: result})
		return
	}
	for _, ia := range inferenceAgents {
		// The cache is keyed by ag.ID() (see bootstrapAgent), not
		// ag.Name() — built-in ACP agents return distinct strings for the
		// two (e.g. "claude-acp" vs "Claude ACP Agent").
		caps, ok := h.hostExecutor.Get(ia.ID)
		if !ok || caps.Status != hostutility.StatusOK {
			continue
		}
		models := make([]dto.InferenceModelDTO, 0, len(caps.Models))
		for _, m := range caps.Models {
			models = append(models, dto.InferenceModelDTO{
				ID:          m.ID,
				Name:        m.Name,
				Description: m.Description,
				IsDefault:   m.ID == caps.CurrentModelID,
				Meta:        m.Meta,
			})
		}
		result = append(result, dto.InferenceAgentDTO{
			ID:          ia.ID,
			Name:        ia.Name,
			DisplayName: ia.DisplayName,
			Models:      models,
		})
	}
	c.JSON(http.StatusOK, dto.InferenceAgentsResponse{Agents: result})
}
