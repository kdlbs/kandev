package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

func (h *Handlers) listAgents(c *gin.Context) {
	agents, err := h.ctrl.Svc.ListAgentInstances(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.AgentListResponse{Agents: agents})
}

func (h *Handlers) createAgent(c *gin.Context) {
	var req dto.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent := &models.AgentInstance{
		WorkspaceID:           c.Param("wsId"),
		Name:                  req.Name,
		AgentProfileID:        req.AgentProfileID,
		Role:                  models.AgentRole(req.Role),
		Icon:                  req.Icon,
		Status:                models.AgentStatusIdle,
		ReportsTo:             req.ReportsTo,
		Permissions:           req.Permissions,
		BudgetMonthlyCents:    req.BudgetMonthlyCents,
		MaxConcurrentSessions: req.MaxConcurrentSessions,
		DesiredSkills:         req.DesiredSkills,
		ExecutorPreference:    req.ExecutorPreference,
	}
	if err := h.ctrl.Svc.CreateAgentInstance(c.Request.Context(), agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.AgentResponse{Agent: agent})
}

func (h *Handlers) getAgent(c *gin.Context) {
	agent, err := h.ctrl.Svc.GetAgentInstance(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.AgentResponse{Agent: agent})
}

func (h *Handlers) updateAgent(c *gin.Context) {
	ctx := c.Request.Context()
	agent, err := h.ctrl.Svc.GetAgentInstance(ctx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var req dto.UpdateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	applyAgentUpdates(agent, &req)
	if err := h.ctrl.Svc.UpdateAgentInstance(ctx, agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.AgentResponse{Agent: agent})
}

func (h *Handlers) deleteAgent(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteAgentInstance(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func applyAgentUpdates(agent *models.AgentInstance, req *dto.UpdateAgentRequest) {
	if req.Name != nil {
		agent.Name = *req.Name
	}
	if req.AgentProfileID != nil {
		agent.AgentProfileID = *req.AgentProfileID
	}
	if req.Role != nil {
		agent.Role = models.AgentRole(*req.Role)
	}
	if req.Icon != nil {
		agent.Icon = *req.Icon
	}
	if req.Status != nil {
		agent.Status = models.AgentStatus(*req.Status)
	}
	if req.ReportsTo != nil {
		agent.ReportsTo = *req.ReportsTo
	}
	if req.Permissions != nil {
		agent.Permissions = *req.Permissions
	}
	if req.BudgetMonthlyCents != nil {
		agent.BudgetMonthlyCents = *req.BudgetMonthlyCents
	}
	if req.MaxConcurrentSessions != nil {
		agent.MaxConcurrentSessions = *req.MaxConcurrentSessions
	}
	if req.DesiredSkills != nil {
		agent.DesiredSkills = *req.DesiredSkills
	}
	if req.ExecutorPreference != nil {
		agent.ExecutorPreference = *req.ExecutorPreference
	}
	if req.PauseReason != nil {
		agent.PauseReason = *req.PauseReason
	}
}
