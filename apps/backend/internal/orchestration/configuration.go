package orchestration

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/instructions"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	"net/http"
)

type configuration struct {
	Icon               string `json:"icon"`
	Name               string `json:"name"`
	RoleID             string `json:"role_id"`
	ProfileID          string `json:"profile_id"`
	ExecutorPreference string `json:"executor_preference"`
	Instructions       string `json:"instructions"`
	Context            string `json:"context"`
}

func (h *Handler) describe(ctx context.Context, a *models.AgentInstance) (any, error) {
	role, err := h.Registry.OrchestratorRoleID(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	override, err := personas.ExecutionProfileID(a.Settings)
	if err != nil {
		return nil, err
	}
	definition, err := h.Registry.GetOrchestratorRole(ctx, role)
	if err != nil {
		return nil, err
	}
	return struct {
		ID          string             `json:"id"`
		WorkspaceID string             `json:"workspace_id"`
		Status      models.AgentStatus `json:"status"`
		configuration
	}{a.ID, a.WorkspaceID, a.Status, configuration{Icon: definition.Icon, Name: definition.Name, RoleID: role, ProfileID: override, ExecutorPreference: a.ExecutorPreference, Instructions: definition.Instructions, Context: models.DelegationContext(a)}}, nil
}
func (h *Handler) prepare(c *gin.Context, a *models.AgentInstance) (*configuration, error) {
	var req configuration
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, err
	}
	if len(req.Context) > 2000 {
		return nil, fmt.Errorf("workspace context exceeds 2000 bytes")
	}
	if err := h.Repo.ValidateWorkspace(c.Request.Context(), a.WorkspaceID); err != nil {
		return nil, err
	}
	role, err := h.Registry.GetOrchestratorRole(c.Request.Context(), req.RoleID)
	if err != nil {
		return nil, fmt.Errorf("select an available role")
	}
	// Presentation and behavior are read-only projections on an assignment. Ignore
	// legacy client snapshots so workspace edits can never overwrite the global role.
	req.Name, req.Icon, req.Instructions = role.Name, role.Icon, role.Instructions
	profiles, err := h.Registry.ExecutionProfileDirectory(c.Request.Context(), a.WorkspaceID)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, profile := range profiles {
		if profile["id"] == req.ProfileID {
			valid = true
		}
	}
	if !valid {
		return nil, fmt.Errorf("select an enabled execution profile in this workspace")
	}
	if h.ValidateExecutor != nil {
		if err := h.ValidateExecutor(c.Request.Context(), req.ExecutorPreference); err != nil {
			return nil, err
		}
	}
	if err := h.Agents.ConfigurePinnedProfile(c.Request.Context(), a, req.ProfileID); err != nil {
		return nil, err
	}
	a.Name = req.Name
	a.ExecutorPreference = req.ExecutorPreference
	err = setPersonaPresentation(a, req.Icon, req.Context)
	return &req, err
}
func (h *Handler) create(c *gin.Context) {
	a := &models.AgentInstance{WorkspaceID: c.Param("wsId"), Role: models.AgentRoleAssistant, Status: models.AgentStatusIdle, MaxConcurrentSessions: 1}
	req, err := h.prepare(c, a)
	if err != nil {
		fail(c, err)
		return
	}
	if err = h.Agents.CreateAgentInstance(c.Request.Context(), a); err != nil {
		fail(c, err)
		return
	}
	if err = h.persistConfiguration(c.Request.Context(), a, req); err != nil {
		_ = h.Agents.DeleteAgentInstance(c.Request.Context(), a.ID)
		_ = h.Registry.UnregisterOrchestrator(c.Request.Context(), a.ID)
		fail(c, err)
		return
	}
	row, err := h.describe(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}
func (h *Handler) update(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	if a.Status == models.AgentStatusWorking {
		c.JSON(http.StatusConflict, gin.H{errorResponseKey: "wait for the current turn before changing configuration"})
		return
	}
	req, err := h.prepare(c, a)
	if err != nil {
		fail(c, err)
		return
	}
	if err = h.Agents.UpdateAgentInstance(c.Request.Context(), a); err != nil {
		fail(c, err)
		return
	}
	if err = h.persistConfiguration(c.Request.Context(), a, req); err != nil {
		fail(c, err)
		return
	}
	row, err := h.describe(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
func (h *Handler) persistConfiguration(ctx context.Context, a *models.AgentInstance, req *configuration) error {
	if err := h.Registry.RegisterOrchestrator(ctx, a.ID, a.WorkspaceID, req.RoleID); err != nil {
		return err
	}
	return h.Agents.UpsertInstruction(ctx, a.ID, "AGENTS.md", instructions.Default, true)
}
