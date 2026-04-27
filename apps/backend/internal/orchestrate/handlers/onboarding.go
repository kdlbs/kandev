package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func registerOnboardingRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/onboarding-state", h.getOnboardingState)
	api.POST("/onboarding/complete", h.completeOnboarding)
}

func (h *Handlers) getOnboardingState(c *gin.Context) {
	state, err := h.ctrl.Svc.GetOnboardingState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.OnboardingStateResponse{
		Completed:   state.Completed,
		WorkspaceID: state.WorkspaceID,
		CEOAgentID:  state.CEOAgentID,
	})
}

func (h *Handlers) completeOnboarding(c *gin.Context) {
	var req dto.OnboardingCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.WorkspaceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceName is required"})
		return
	}
	if req.AgentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agentName is required"})
		return
	}

	result, err := h.ctrl.Svc.CompleteOnboarding(c.Request.Context(), service.OnboardingCompleteRequest{
		WorkspaceName:      req.WorkspaceName,
		TaskPrefix:         req.TaskPrefix,
		AgentName:          req.AgentName,
		AgentProfileID:     req.AgentProfileID,
		ExecutorPreference: req.ExecutorPreference,
		TaskTitle:          req.TaskTitle,
		TaskDescription:    req.TaskDescription,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.OnboardingCompleteResponse{
		WorkspaceID: result.WorkspaceID,
		AgentID:     result.AgentID,
		ProjectID:   result.ProjectID,
		TaskID:      result.TaskID,
	})
}
