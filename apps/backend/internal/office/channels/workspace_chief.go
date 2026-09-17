package channels

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/agents"
	"net/http"
)

type WorkflowEnsurer interface {
	EnsureOfficeWorkflow(context.Context, string) (string, error)
}

func (s *ChannelService) SetWorkflowEnsurer(ensurer WorkflowEnsurer) { s.workflowEnsurer = ensurer }

func (h *Handler) getWorkspaceChief(c *gin.Context) {
	id, err := h.svc.repo.GetWorkspaceChief(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorResponseKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent_id": id})
}

func (h *Handler) setWorkspaceChief(c *gin.Context) {
	if agents.CallerFromContext(c) != nil {
		c.JSON(http.StatusForbidden, gin.H{errorResponseKey: "chief selection requires a user"})
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
		return
	}
	if err := h.svc.repo.SetWorkspaceChief(c.Request.Context(), c.Param("wsId"), req.AgentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent_id": req.AgentID})
}

// Enabling orchestration does not create a workspace or a delivery workflow.
func (h *Handler) enableWorkspaceAgents(c *gin.Context) {
	if agents.CallerFromContext(c) != nil {
		c.JSON(http.StatusForbidden, gin.H{errorResponseKey: "agent setup requires a user"})
		return
	}
	if err := h.svc.repo.ValidateWorkspace(c.Request.Context(), c.Param("wsId")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: "workspace not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}
