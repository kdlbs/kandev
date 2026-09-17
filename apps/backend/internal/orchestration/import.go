package orchestration

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	"net/http"
)

// importAgent explicitly preserves a legacy assistant's identity and conversation.
// The user supplies a complete new configuration, just as for a new orchestrator.
func (h *Handler) importAgent(c *gin.Context) {
	ctx := c.Request.Context()
	if err := h.Repo.AuthorizePersona(ctx, c.Param("id")); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	a, err := h.Agents.GetAgentInstance(ctx, c.Param("id"))
	if err != nil || a.WorkspaceID != c.Param("wsId") || a.Role != models.AgentRoleAssistant {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: "workspace assistant not found"})
		return
	}
	role, err := h.Registry.OrchestratorRoleID(ctx, a.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if role != "" || a.Status == models.AgentStatusWorking {
		c.JSON(http.StatusConflict, gin.H{errorResponseKey: "assistant is already registered or working"})
		return
	}
	req, err := h.prepare(c, a)
	if err != nil {
		fail(c, err)
		return
	}
	if err = h.Agents.UpdateAgentInstance(ctx, a); err != nil {
		fail(c, err)
		return
	}
	if err = h.persistConfiguration(ctx, a, req); err != nil {
		fail(c, err)
		return
	}
	if err := h.Registry.ImportLegacyState(); err != nil {
		fail(c, err)
		return
	}
	h.get(c)
}
