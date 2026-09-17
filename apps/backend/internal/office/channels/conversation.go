package channels

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/agents"
)

const (
	errorResponseKey = "error"
)

// openConversation is a browser operation. Agent callers operate tasks through
// their runtime capabilities rather than creating another persona's inbox.
func (h *Handler) openConversation(c *gin.Context) {
	if agents.CallerFromContext(c) != nil {
		c.JSON(http.StatusForbidden, gin.H{errorResponseKey: "conversation setup requires a user"})
		return
	}
	agent, err := h.svc.agents.GetAgentInstance(c.Request.Context(), c.Param("id"))
	if err != nil || agent.WorkspaceID != c.Param("wsId") {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: "agent not found"})
		return
	}
	channel, err := h.svc.repo.EnsureAgentConversation(c.Request.Context(), agent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorResponseKey: err.Error()})
		return
	}
	if h.svc.publishConversation != nil {
		h.svc.publishConversation(c.Request.Context(), channel.TaskID)
	}
	c.JSON(http.StatusOK, gin.H{"channel": channel})
}
