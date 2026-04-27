package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func (h *Handlers) listAgents(c *gin.Context) {
	filter := service.AgentListFilter{
		Role:      c.Query("role"),
		Status:    c.Query("status"),
		ReportsTo: c.Query("reports_to"),
	}
	var (
		agents []*models.AgentInstance
		err    error
	)
	if filter.Role != "" || filter.Status != "" || filter.ReportsTo != "" {
		agents, err = h.ctrl.Svc.ListAgentInstancesFiltered(
			c.Request.Context(), c.Param("wsId"), filter)
	} else {
		agents, err = h.ctrl.Svc.ListAgentsFromConfig(
			c.Request.Context(), c.Param("wsId"))
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.AgentListResponse{Agents: agents})
}

func (h *Handlers) createAgent(c *gin.Context) {
	if err := checkAgentPermission(c, service.PermCanCreateAgents); err != nil {
		return
	}
	var req dto.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := checkNoEscalation(c, req.Permissions); err != nil {
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
	agent, err := h.ctrl.Svc.GetAgentFromConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.AgentResponse{Agent: agent})
}

func (h *Handlers) updateAgent(c *gin.Context) {
	ctx := c.Request.Context()
	agent, err := h.ctrl.Svc.GetAgentFromConfig(ctx, c.Param("id"))
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
	if caller := agentCallerFromCtx(c); caller != nil {
		if !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can delete agents"})
			return
		}
	}
	if err := h.ctrl.Svc.DeleteAgentInstance(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) updateAgentStatus(c *gin.Context) {
	if caller := agentCallerFromCtx(c); caller != nil {
		targetID := c.Param("id")
		if targetID != caller.ID && !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can change other agents' status"})
			return
		}
	}
	var req dto.UpdateAgentStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent, err := h.ctrl.Svc.UpdateAgentStatus(
		c.Request.Context(), c.Param("id"),
		models.AgentStatus(req.Status), req.PauseReason)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.AgentResponse{Agent: agent})
}

// checkAgentPermission returns nil and does nothing for UI requests.
// For agent callers, it checks the specified permission and returns 403 if denied.
func checkAgentPermission(c *gin.Context, permKey string) error {
	caller := agentCallerFromCtx(c)
	if caller == nil {
		return nil
	}
	perms := service.ResolvePermissions(caller.Role, caller.Permissions)
	if !service.HasPermission(perms, permKey) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: missing " + permKey})
		return service.ErrForbidden
	}
	return nil
}

// checkNoEscalation verifies an agent caller is not granting permissions it
// does not itself hold. No-op for UI requests or empty permission strings.
func checkNoEscalation(c *gin.Context, permsJSON string) error {
	caller := agentCallerFromCtx(c)
	if caller == nil || permsJSON == "" || permsJSON == "{}" {
		return nil
	}
	callerPerms := service.ResolvePermissions(caller.Role, caller.Permissions)
	if err := service.ValidateNoEscalation(callerPerms, permsJSON); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return err
	}
	return nil
}

// isAdminRole returns true for roles that have administrative privileges.
func isAdminRole(role models.AgentRole) bool {
	return role == models.AgentRoleCEO
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
